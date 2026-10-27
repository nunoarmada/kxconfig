package kubecfg

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/tools/clientcmd/api"
)

// mk builds a config from compact specs. Each name gets cluster, user and
// context entries as requested.
func mk(current string) *api.Config {
	c := api.NewConfig()
	c.CurrentContext = current
	return c
}

func addCluster(c *api.Config, name, server string) {
	c.Clusters[name] = &api.Cluster{Server: server}
}

func addUser(c *api.Config, name, token string) {
	c.AuthInfos[name] = &api.AuthInfo{Token: token}
}

func addCtx(c *api.Config, name, cluster, user, ns string) {
	c.Contexts[name] = &api.Context{Cluster: cluster, AuthInfo: user, Namespace: ns}
}

func ref(k Kind, n string) EntityRef { return EntityRef{Kind: k, Name: n} }

func TestMergeAddsNew(t *testing.T) {
	master := mk("m")
	addCluster(master, "m", "https://m")
	addUser(master, "m", "tm")
	addCtx(master, "m", "m", "m", "")
	incoming := mk("n")
	addCluster(incoming, "n", "https://n")
	addUser(incoming, "n", "tn")
	addCtx(incoming, "n", "n", "n", "ns")

	got, rep, err := Merge(master, incoming, MergeOptions{})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got.CurrentContext != "m" {
		t.Errorf("CurrentContext = %q, want master's m", got.CurrentContext)
	}
	if len(got.Clusters) != 2 || len(got.AuthInfos) != 2 || len(got.Contexts) != 2 {
		t.Errorf("unexpected sizes: %d %d %d", len(got.Clusters), len(got.AuthInfos), len(got.Contexts))
	}
	wantAdded := []EntityRef{ref(KindCluster, "n"), ref(KindUser, "n"), ref(KindContext, "n")}
	if !reflect.DeepEqual(rep.Added, wantAdded) {
		t.Errorf("Added = %v, want %v", rep.Added, wantAdded)
	}
	if len(master.Clusters) != 1 || len(incoming.Clusters) != 1 {
		t.Error("inputs were mutated")
	}
}

func TestMergeIdenticalIsNoop(t *testing.T) {
	path := filepath.Join("testdata", "rel", "config")
	master, err := LoadFlattened(path)
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := LoadFlattened(path)
	if err != nil {
		t.Fatal(err)
	}
	got, rep, err := Merge(master, incoming, MergeOptions{})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(rep.Added)+len(rep.Overwritten)+len(rep.Skipped)+len(rep.Renamed)+len(rep.Conflicts) != 0 {
		t.Errorf("expected only Unchanged, got %+v", rep)
	}
	if len(rep.Unchanged) != 3 {
		t.Errorf("Unchanged = %v, want 3 entries", rep.Unchanged)
	}
	if !apiequality.Semantic.DeepEqual(got, master) {
		t.Error("result differs from master")
	}
}

func TestReimportSameRelativeFileIsNoop(t *testing.T) {
	src := filepath.Join("testdata", "rel", "config")
	incoming, err := LoadFlattened(src)
	if err != nil {
		t.Fatal(err)
	}
	// Master was produced by an earlier import: flattened and saved elsewhere.
	masterPath := filepath.Join(t.TempDir(), "master.yaml")
	first, err := LoadFlattened(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(first, masterPath); err != nil {
		t.Fatal(err)
	}
	master, err := LoadFlattened(masterPath)
	if err != nil {
		t.Fatal(err)
	}
	_, rep, err := Merge(master, incoming, MergeOptions{})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(rep.Unchanged) != 3 || len(rep.Conflicts) != 0 || len(rep.Added) != 0 {
		t.Errorf("re-import should be a no-op, got %+v", rep)
	}
}

func conflictPair() (*api.Config, *api.Config) {
	master := mk("default")
	addCluster(master, "default", "https://old.example")
	addUser(master, "default", "old-token")
	addCtx(master, "default", "default", "default", "a")
	incoming := mk("default")
	addCluster(incoming, "default", "https://new.example")
	addUser(incoming, "default", "new-token")
	addCtx(incoming, "default", "default", "default", "b")
	return master, incoming
}

func TestMergeConflictFailDefault(t *testing.T) {
	master, incoming := conflictPair()
	// Only the cluster differs; user and context are identical.
	incoming.AuthInfos["default"] = master.AuthInfos["default"].DeepCopy()
	incoming.Contexts["default"] = master.Contexts["default"].DeepCopy()
	got, rep, err := Merge(master, incoming, MergeOptions{})
	if !errors.Is(err, ErrConflicts) {
		t.Fatalf("err = %v, want ErrConflicts", err)
	}
	if got != nil {
		t.Error("expected nil config on conflict")
	}
	if len(rep.Conflicts) != 1 {
		t.Fatalf("Conflicts = %v, want 1", rep.Conflicts)
	}
	c := rep.Conflicts[0]
	if c.Ref != ref(KindCluster, "default") {
		t.Errorf("Ref = %v", c.Ref)
	}
	if !strings.Contains(c.Existing, "https://old.example") || !strings.Contains(c.Incoming, "https://new.example") {
		t.Errorf("summaries = %q / %q", c.Existing, c.Incoming)
	}
}

func TestDetectConflicts(t *testing.T) {
	master, incoming := conflictPair()
	got := DetectConflicts(master, incoming)
	if len(got) != 3 {
		t.Fatalf("got %d conflicts, want 3: %v", len(got), got)
	}
	want := []EntityRef{ref(KindCluster, "default"), ref(KindUser, "default"), ref(KindContext, "default")}
	for i, w := range want {
		if got[i].Ref != w {
			t.Errorf("conflict %d ref = %v, want %v", i, got[i].Ref, w)
		}
	}
}

func TestMergeRenameRewritesRefs(t *testing.T) {
	master, incoming := conflictPair()
	got, rep, err := Merge(master, incoming, MergeOptions{Default: PolicyRename})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	for _, k := range []Kind{KindCluster, KindUser, KindContext} {
		if rep.Renamed[ref(k, "default")] != "default-2" {
			t.Errorf("Renamed[%s] = %q", k, rep.Renamed[ref(k, "default")])
		}
	}
	if len(rep.Renamed) != 3 {
		t.Errorf("Renamed has %d entries, want 3", len(rep.Renamed))
	}
	if got.Clusters["default"].Server != "https://old.example" {
		t.Error("master cluster was changed")
	}
	if got.Clusters["default-2"].Server != "https://new.example" {
		t.Error("renamed cluster missing")
	}
	ctx := got.Contexts["default-2"]
	if ctx == nil || ctx.Cluster != "default-2" || ctx.AuthInfo != "default-2" || ctx.Namespace != "b" {
		t.Errorf("renamed context wrong: %+v", ctx)
	}
	if got.Contexts["default"].Cluster != "default" {
		t.Error("master context refs changed")
	}
	if incoming.Contexts["default"].Cluster != "default" {
		t.Error("incoming mutated")
	}
}

func TestMergeRenameSkipsTakenSuffixes(t *testing.T) {
	master, incoming := conflictPair()
	addCluster(master, "default-2", "https://other")
	got, _, err := Merge(master, incoming, MergeOptions{Default: PolicyRename})
	if err != nil {
		t.Fatal(err)
	}
	if got.Clusters["default-3"] == nil || got.Clusters["default-2"].Server != "https://other" {
		t.Errorf("expected default-3 for renamed cluster, got %v", got.Clusters)
	}
}

func TestMergeRenamedClusterRewritesNonConflictingContext(t *testing.T) {
	master := mk("")
	addCluster(master, "c", "https://old")
	incoming := mk("")
	addCluster(incoming, "c", "https://new")
	addUser(incoming, "u", "t")
	addCtx(incoming, "fresh", "c", "u", "")
	got, _, err := Merge(master, incoming, MergeOptions{Default: PolicyRename})
	if err != nil {
		t.Fatal(err)
	}
	if got.Contexts["fresh"].Cluster != "c-2" {
		t.Errorf("context cluster = %q, want c-2", got.Contexts["fresh"].Cluster)
	}
}

func TestMergeOverwriteAndSkip(t *testing.T) {
	master, incoming := conflictPair()
	got, rep, err := Merge(master, incoming, MergeOptions{Default: PolicyOverwrite})
	if err != nil {
		t.Fatal(err)
	}
	if got.Clusters["default"].Server != "https://new.example" || got.AuthInfos["default"].Token != "new-token" || got.Contexts["default"].Namespace != "b" {
		t.Error("overwrite did not take incoming")
	}
	if len(rep.Overwritten) != 3 {
		t.Errorf("Overwritten = %v", rep.Overwritten)
	}

	master, incoming = conflictPair()
	got, rep, err = Merge(master, incoming, MergeOptions{Default: PolicySkip})
	if err != nil {
		t.Fatal(err)
	}
	if got.Clusters["default"].Server != "https://old.example" || got.Contexts["default"].Namespace != "a" {
		t.Error("skip did not keep master")
	}
	// The incoming context references skipped entities, so it is dropped
	// rather than skipped (see TestMergeSkippedClusterDropsReferencingContext).
	if len(rep.Skipped) != 2 || len(rep.Dropped) != 1 {
		t.Errorf("Skipped = %v, Dropped = %v", rep.Skipped, rep.Dropped)
	}
}

func TestMergePerItemOverridesDefault(t *testing.T) {
	master, incoming := conflictPair()
	opts := MergeOptions{
		Default: PolicyFail,
		PerItem: map[EntityRef]ConflictPolicy{
			ref(KindCluster, "default"): PolicyOverwrite,
			ref(KindUser, "default"):    PolicyOverwrite,
			ref(KindContext, "default"): PolicyRename,
		},
	}
	got, rep, err := Merge(master, incoming, opts)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if got.Clusters["default"].Server != "https://new.example" {
		t.Error("cluster not overwritten")
	}
	if got.AuthInfos["default"].Token != "new-token" {
		t.Error("user not overwritten")
	}
	if rep.Renamed[ref(KindContext, "default")] != "default-2" || got.Contexts["default-2"] == nil {
		t.Errorf("context not renamed: %+v", rep.Renamed)
	}
	if len(rep.Renamed) != 1 {
		t.Errorf("only the context should be renamed: %v", rep.Renamed)
	}
}

func TestMergePerItemFailReportsOnlyFailing(t *testing.T) {
	master, incoming := conflictPair()
	opts := MergeOptions{
		Default: PolicyOverwrite,
		PerItem: map[EntityRef]ConflictPolicy{ref(KindUser, "default"): PolicyFail},
	}
	got, rep, err := Merge(master, incoming, opts)
	if !errors.Is(err, ErrConflicts) || got != nil {
		t.Fatalf("got %v, %v", got, err)
	}
	if len(rep.Conflicts) != 1 || rep.Conflicts[0].Ref != ref(KindUser, "default") {
		t.Errorf("Conflicts = %v", rep.Conflicts)
	}
}

func TestSummarizeNeverLeaksSecrets(t *testing.T) {
	cfg := mk("")
	cfg.AuthInfos["u"] = &api.AuthInfo{
		Token:         "s3cret",
		ClientKeyData: []byte("KEYDATA-xyz"),
		Password:      "pw-hunter2",
	}
	cfg.AuthInfos["v"] = &api.AuthInfo{Username: "bob", Password: "pw-hunter2", ClientCertificateData: []byte("CERTDATA")}
	addCluster(cfg, "c", "https://user:pass@host:6443")
	for _, name := range []string{"u", "v"} {
		s := Summarize(ref(KindUser, name), cfg)
		if s == "" {
			t.Errorf("empty summary for %s", name)
		}
		for _, bad := range []string{"s3cret", "KEYDATA", "hunter2", "CERTDATA"} {
			if strings.Contains(s, bad) {
				t.Errorf("summary %q leaks %q", s, bad)
			}
		}
	}
	if s := Summarize(ref(KindCluster, "c"), cfg); strings.Contains(s, "pass") || !strings.Contains(s, "host:6443") {
		t.Errorf("cluster summary = %q", s)
	}
}

func TestSummarizeContext(t *testing.T) {
	cfg := mk("")
	addCtx(cfg, "x", "cl", "us", "kube-system")
	addCtx(cfg, "y", "cl", "us", "")
	if got := Summarize(ref(KindContext, "x"), cfg); got != "cl/us kube-system" {
		t.Errorf("got %q", got)
	}
	if got := Summarize(ref(KindContext, "y"), cfg); got != "cl/us" {
		t.Errorf("got %q", got)
	}
}

func TestParsePolicy(t *testing.T) {
	for _, s := range []string{"fail", "rename", "overwrite", "skip"} {
		p, err := ParsePolicy(s)
		if err != nil || string(p) != s {
			t.Errorf("ParsePolicy(%q) = %v, %v", s, p, err)
		}
	}
	if _, err := ParsePolicy("bogus"); err == nil {
		t.Error("expected error for bogus policy")
	}
}

func TestSummarizeServerRedaction(t *testing.T) {
	cases := map[string]string{
		"https://user:pw@h:6443/p?token=x#f": "https://h:6443/p",
		"https://h:6443":                     "https://h:6443",
		"mailto:secret@example.com":          "(unparseable server)",
		"host-without-scheme":                "(unparseable server)",
		"https:///p?token=x":                 "(unparseable server)",
		"":                                   "(unparseable server)",
	}
	for in, want := range cases {
		cfg := mk("")
		addCluster(cfg, "c", in)
		if got := Summarize(ref(KindCluster, "c"), cfg); got != want {
			t.Errorf("Summarize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMergeSkippedClusterDropsReferencingContext(t *testing.T) {
	master, incoming := conflictPair()
	// User and context identical-by-name; only cluster conflicts and is skipped.
	opts := MergeOptions{PerItem: map[EntityRef]ConflictPolicy{ref(KindCluster, "default"): PolicySkip}, Default: PolicyOverwrite}
	got, rep, err := Merge(master, incoming, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rep.Dropped, []EntityRef{ref(KindContext, "default")}) {
		t.Errorf("Dropped = %v", rep.Dropped)
	}
	if got.Contexts["default"].Namespace != "a" || got.Contexts["default"].Cluster != "default" {
		t.Errorf("master context changed: %+v", got.Contexts["default"])
	}
	if got.Clusters["default"].Server != "https://old.example" {
		t.Error("skipped cluster changed")
	}
}

func TestMergeSkippedUserDropsNewContext(t *testing.T) {
	master := mk("")
	addUser(master, "u", "master-token")
	incoming := mk("")
	addCluster(incoming, "c", "https://c")
	addUser(incoming, "u", "incoming-token")
	addCtx(incoming, "fresh", "c", "u", "")
	got, rep, err := Merge(master, incoming, MergeOptions{Default: PolicySkip})
	if err != nil {
		t.Fatal(err)
	}
	if got.Contexts["fresh"] != nil {
		t.Error("context bound to master's user must not be added")
	}
	if !reflect.DeepEqual(rep.Dropped, []EntityRef{ref(KindContext, "fresh")}) {
		t.Errorf("Dropped = %v", rep.Dropped)
	}
}

func TestMergeDropsContextWithUndefinedRefs(t *testing.T) {
	master := mk("")
	addUser(master, "u", "t")
	addCluster(master, "c", "https://c")
	incoming := mk("")
	addCluster(incoming, "c", "https://c")
	addCtx(incoming, "b-undef-user", "c", "u", "") // u not in incoming
	addCtx(incoming, "a-undef-cluster", "zz", "", "")
	addCtx(incoming, "ok", "c", "", "")
	got, rep, err := Merge(master, incoming, MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []EntityRef{ref(KindContext, "a-undef-cluster"), ref(KindContext, "b-undef-user")}
	if !reflect.DeepEqual(rep.Dropped, want) {
		t.Errorf("Dropped = %v, want %v", rep.Dropped, want)
	}
	if got.Contexts["ok"] == nil || got.Contexts["b-undef-user"] != nil || got.Contexts["a-undef-cluster"] != nil {
		t.Errorf("contexts = %v", got.Contexts)
	}
}

func TestMergeIntoEmptyMasterInitialisesMaps(t *testing.T) {
	incoming := mk("")
	addCluster(incoming, "c", "https://c")
	addUser(incoming, "u", "t")
	addCtx(incoming, "x", "c", "u", "")
	got, _, err := Merge(&api.Config{}, incoming, MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Clusters) != 1 || len(got.AuthInfos) != 1 || len(got.Contexts) != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestConflictSummariesDifferForSameAuthKind(t *testing.T) {
	master, incoming := conflictPair() // both users are "token" with different values
	cs := DetectConflicts(master, incoming)
	var u Conflict
	for _, c := range cs {
		if c.Ref.Kind == KindUser {
			u = c
		}
	}
	if u.Existing == u.Incoming {
		t.Errorf("summaries identical: %q", u.Existing)
	}
	for _, s := range []string{u.Existing, u.Incoming} {
		if strings.Contains(s, "old-token") || strings.Contains(s, "new-token") {
			t.Errorf("leak: %q", s)
		}
	}
}
