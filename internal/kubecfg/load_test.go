package kubecfg

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/tools/clientcmd/api"
)

func TestLoadFlattenedResolvesRelativePaths(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("testdata", "rel", "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFlattened(filepath.Join("testdata", "rel", "config"))
	if err != nil {
		t.Fatalf("LoadFlattened: %v", err)
	}
	c := cfg.Clusters["rel"]
	if c == nil {
		t.Fatal("cluster rel missing")
	}
	if string(c.CertificateAuthorityData) != string(want) {
		t.Errorf("CertificateAuthorityData = %q, want %q", c.CertificateAuthorityData, want)
	}
	if c.CertificateAuthority != "" {
		t.Errorf("CertificateAuthority = %q, want empty", c.CertificateAuthority)
	}
}

func TestLoadMissingFileErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestSaveLoadRoundTripAndMode(t *testing.T) {
	cfg := api.NewConfig()
	cfg.Clusters["a"] = &api.Cluster{Server: "https://a"}
	cfg.AuthInfos["a"] = &api.AuthInfo{Token: "t"}
	cfg.Contexts["a"] = &api.Context{Cluster: "a", AuthInfo: "a"}
	cfg.CurrentContext = "a"
	path := filepath.Join(t.TempDir(), "cfg")
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentContext != "a" || got.Clusters["a"].Server != "https://a" || got.AuthInfos["a"].Token != "t" {
		t.Errorf("round trip mismatch: %+v", got)
	}
}

func TestSaveFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.yaml")
	link := filepath.Join(dir, "link.yaml")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := Save(api.NewConfig(), link); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("symlink was replaced: %v %v", fi, err)
	}
}
