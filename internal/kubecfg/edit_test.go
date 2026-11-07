package kubecfg

import (
	"reflect"
	"testing"

	"k8s.io/client-go/tools/clientcmd/api"
)

// editCfg: a->(ca,shared), b->(cb,shared), c->(cc,uc); current is a.
func editCfg() *api.Config {
	c := mk("a")
	addCluster(c, "ca", "https://a")
	addCluster(c, "cb", "https://b")
	addCluster(c, "cc", "https://c")
	addUser(c, "shared", "t")
	addUser(c, "uc", "t")
	addCtx(c, "a", "ca", "shared", "")
	addCtx(c, "b", "cb", "shared", "")
	addCtx(c, "c", "cc", "uc", "")
	return c
}

func TestDeleteContextRemovesOrphans(t *testing.T) {
	cfg := editCfg()
	rep, err := DeleteContext(cfg, "a", false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rep.RemovedClusters, []string{"ca"}) || len(rep.RemovedUsers) != 0 {
		t.Fatalf("report = %+v", rep)
	}
	if cfg.Contexts["a"] != nil || cfg.Clusters["ca"] != nil {
		t.Fatal("context a / cluster ca not removed")
	}
	if cfg.AuthInfos["shared"] == nil {
		t.Fatal("shared user must be kept")
	}
}

func TestDeleteContextKeepOrphans(t *testing.T) {
	cfg := editCfg()
	rep, err := DeleteContext(cfg, "c", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.RemovedClusters)+len(rep.RemovedUsers) != 0 {
		t.Fatalf("report = %+v", rep)
	}
	if cfg.Clusters["cc"] == nil || cfg.AuthInfos["uc"] == nil {
		t.Fatal("orphans must be kept")
	}
	if cfg.Contexts["c"] != nil {
		t.Fatal("context not removed")
	}
}

func TestDeleteCurrentPicksFirstRemaining(t *testing.T) {
	cfg := editCfg()
	rep, err := DeleteContext(cfg, "a", false)
	if err != nil {
		t.Fatal(err)
	}
	if rep.NewCurrent != "b" || cfg.CurrentContext != "b" {
		t.Fatalf("NewCurrent=%q current=%q", rep.NewCurrent, cfg.CurrentContext)
	}
	only := mk("x")
	addCtx(only, "x", "", "", "")
	if _, err := DeleteContext(only, "x", false); err != nil || only.CurrentContext != "" {
		t.Fatalf("err=%v current=%q", err, only.CurrentContext)
	}
}

func TestDeleteNonCurrentKeepsCurrent(t *testing.T) {
	cfg := editCfg()
	rep, err := DeleteContext(cfg, "b", false)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentContext != "a" || rep.NewCurrent != "a" {
		t.Fatalf("NewCurrent=%q current=%q", rep.NewCurrent, cfg.CurrentContext)
	}
}

func TestDeleteMissingErrors(t *testing.T) {
	cfg := editCfg()
	if _, err := DeleteContext(cfg, "nope", false); err == nil {
		t.Fatal("expected error")
	}
	if len(cfg.Contexts) != 3 {
		t.Fatal("config mutated")
	}
}

func TestOrphansDoesNotMutate(t *testing.T) {
	cfg := editCfg()
	cl, us := Orphans(cfg, "c")
	if !reflect.DeepEqual(cl, []string{"cc"}) || !reflect.DeepEqual(us, []string{"uc"}) {
		t.Fatalf("clusters=%v users=%v", cl, us)
	}
	if len(cfg.Contexts) != 3 || len(cfg.Clusters) != 3 {
		t.Fatal("Orphans mutated config")
	}
}

func TestSetCurrentContext(t *testing.T) {
	cfg := editCfg()
	if err := SetCurrentContext(cfg, "b"); err != nil || cfg.CurrentContext != "b" {
		t.Fatalf("err=%v current=%q", err, cfg.CurrentContext)
	}
	if err := SetCurrentContext(cfg, "nope"); err == nil || cfg.CurrentContext != "b" {
		t.Fatalf("err=%v current=%q", err, cfg.CurrentContext)
	}
}

func TestSetNamespace(t *testing.T) {
	cfg := editCfg()
	if err := SetNamespace(cfg, "prod"); err != nil {
		t.Fatal(err)
	}
	if cfg.Contexts["a"].Namespace != "prod" {
		t.Fatal("namespace not set")
	}
}

func TestSetNamespaceNoCurrent(t *testing.T) {
	zero := mk("")
	dangling := mk("ghost")
	addCtx(dangling, "a", "", "", "")
	for name, cfg := range map[string]*api.Config{"zero contexts": zero, "dangling": dangling, "empty current": editCfgNoCurrent()} {
		err := SetNamespace(cfg, "x")
		if err == nil || err.Error() != "no current context" {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func editCfgNoCurrent() *api.Config {
	c := editCfg()
	c.CurrentContext = ""
	return c
}

func TestContextsSortedWithInfo(t *testing.T) {
	cfg := editCfg()
	cfg.Contexts["b"].Namespace = "ns"
	got := Contexts(cfg)
	if len(got) != 3 || got[0].Name != "a" || got[1].Name != "b" || got[2].Name != "c" {
		t.Fatalf("got %+v", got)
	}
	want := ContextInfo{Name: "b", Cluster: "cb", User: "shared", Namespace: "ns", Server: "https://b", AuthKind: "token"}
	if got[1] != want {
		t.Fatalf("got %+v want %+v", got[1], want)
	}
	if !got[0].Current || got[1].Current {
		t.Fatal("Current flag wrong")
	}
}

func TestContextsDanglingCurrent(t *testing.T) {
	cfg := editCfg()
	cfg.CurrentContext = "ghost"
	addCtx(cfg, "d", "missing-cluster", "missing-user", "")
	for _, ci := range Contexts(cfg) {
		if ci.Current {
			t.Fatalf("%s marked current", ci.Name)
		}
		if ci.Name == "d" && (ci.Server != "" || ci.AuthKind != "none") {
			t.Fatalf("dangling refs: %+v", ci)
		}
	}
}

func TestAuthKind(t *testing.T) {
	tests := []struct {
		name string
		in   *api.AuthInfo
		want string
	}{
		{"nil", nil, "none"},
		{"empty", &api.AuthInfo{}, "none"},
		{"token", &api.AuthInfo{Token: "t"}, "token"},
		{"token file", &api.AuthInfo{TokenFile: "/f"}, "token"},
		{"cert file", &api.AuthInfo{ClientCertificate: "/c"}, "client-cert"},
		{"cert data", &api.AuthInfo{ClientCertificateData: []byte("x")}, "client-cert"},
		{"exec", &api.AuthInfo{Exec: &api.ExecConfig{Command: "aws"}}, "exec: aws"},
		{"provider", &api.AuthInfo{AuthProvider: &api.AuthProviderConfig{Name: "gcp"}}, "auth-provider: gcp"},
		{"basic", &api.AuthInfo{Username: "u", Password: "p"}, "basic"},
		{"exec beats token", &api.AuthInfo{Exec: &api.ExecConfig{Command: "x"}, Token: "t"}, "exec: x"},
	}
	for _, tc := range tests {
		if got := AuthKind(tc.in); got != tc.want {
			t.Errorf("%s: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestContextsRedactsServer(t *testing.T) {
	cfg := editCfg()
	cfg.Clusters["ca"].Server = "https://u:p@h:6443/x?token=t#f"
	for _, ci := range Contexts(cfg) {
		if ci.Name == "a" && ci.Server != "https://h:6443/x" {
			t.Fatalf("Server = %q, want redacted", ci.Server)
		}
	}
	if got := RedactServer("https://u:p@h:6443/x?token=t#f"); got != "https://h:6443/x" {
		t.Fatalf("RedactServer = %q", got)
	}
}

func TestRenameContextKeepsCurrent(t *testing.T) {
	cfg := editCfg()
	if err := RenameContext(cfg, "a", "alpha"); err != nil {
		t.Fatal(err)
	}
	if cfg.Contexts["a"] != nil || cfg.Contexts["alpha"] == nil {
		t.Fatal("context not renamed")
	}
	if cfg.CurrentContext != "alpha" {
		t.Fatalf("current = %q, want alpha", cfg.CurrentContext)
	}
	if cfg.Contexts["alpha"].Cluster != "ca" {
		t.Fatal("context content changed")
	}
}

func TestRenameContextErrors(t *testing.T) {
	for name, tc := range map[string][2]string{
		"missing": {"zz", "y"},
		"taken":   {"a", "b"},
		"empty":   {"a", ""},
		"spaces":  {"a", " a "},
		"control": {"a", "a\x1bb"},
	} {
		cfg := editCfg()
		if err := RenameContext(cfg, tc[0], tc[1]); err == nil {
			t.Errorf("%s: no error", name)
		}
		if cfg.Contexts["a"] == nil || cfg.CurrentContext != "a" {
			t.Errorf("%s: config changed on error", name)
		}
	}
	if err := RenameContext(editCfg(), "a", "a"); err != nil {
		t.Errorf("same name: %v", err)
	}
}

func TestRenameClusterRepointsContexts(t *testing.T) {
	cfg := editCfg()
	cfg.Contexts["b"].Cluster = "ca" // a and b share ca now
	moved, err := RenameCluster(cfg, "ca", "main")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(moved, []string{"a", "b"}) {
		t.Fatalf("moved = %v", moved)
	}
	if cfg.Clusters["ca"] != nil || cfg.Clusters["main"] == nil {
		t.Fatal("cluster not renamed")
	}
	for _, n := range []string{"a", "b"} {
		if cfg.Contexts[n].Cluster != "main" {
			t.Errorf("context %s still points at %q", n, cfg.Contexts[n].Cluster)
		}
	}
	if cfg.Contexts["c"].Cluster != "cc" {
		t.Error("unrelated context changed")
	}
}

func TestRenameClusterErrors(t *testing.T) {
	for name, tc := range map[string][2]string{
		"missing": {"zz", "y"},
		"taken":   {"ca", "cb"},
		"empty":   {"ca", ""},
	} {
		cfg := editCfg()
		if _, err := RenameCluster(cfg, tc[0], tc[1]); err == nil {
			t.Errorf("%s: no error", name)
		}
		if cfg.Clusters["ca"] == nil || cfg.Contexts["a"].Cluster != "ca" {
			t.Errorf("%s: config changed on error", name)
		}
	}
}
