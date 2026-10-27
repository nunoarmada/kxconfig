package shell

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/v2/internal/store"
)

const cfg = "apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\n"

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s := store.New(t.TempDir())
	if err := os.WriteFile(s.ProjectPath("prod"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	return s
}

func count(env []string, prefix string) int {
	n := 0
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			n++
		}
	}
	return n
}

func TestCommandEnvAndCopy(t *testing.T) {
	s := newStore(t)
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("KUBECONFIG", "/stale/path")
	t.Setenv("KX_PROJECT", "stale")
	cmd, cleanup, err := Command(s, "prod", store.Env{})
	if err != nil {
		t.Fatal(err)
	}
	copyPath := s.ShellCopyPath(os.Getpid(), "prod")
	for _, want := range []string{"KUBECONFIG=" + copyPath, "KX_PROJECT=prod", "KX_SHELL=1"} {
		if !slices.Contains(cmd.Env, want) {
			t.Errorf("env missing %q", want)
		}
	}
	for _, k := range []string{"KUBECONFIG=", "KX_PROJECT=", "KX_SHELL="} {
		if n := count(cmd.Env, k); n != 1 {
			t.Errorf("%s appears %d times in env", k, n)
		}
	}
	if cmd.Path != "/bin/sh" {
		t.Errorf("cmd.Path = %q", cmd.Path)
	}
	fi, err := os.Stat(copyPath)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("copy: %v, %v", fi, err)
	}
	if di, _ := os.Stat(filepath.Dir(copyPath)); di.Mode().Perm() != 0o700 {
		t.Errorf("shells dir mode = %v", di.Mode().Perm())
	}
	if got, err := kubecfg.Load(copyPath); err != nil || len(got.Contexts) != 0 {
		t.Errorf("copy does not parse as the (empty) project: %+v, %v", got, err)
	}

	// Run a non-interactive variant to prove the child sees the copy.
	cmd.Args = []string{"sh", "-c", "echo $KUBECONFIG:$KX_PROJECT:$KX_SHELL"}
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(out)), copyPath+":prod:1"; got != want {
		t.Errorf("child saw %q, want %q", got, want)
	}

	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(copyPath); !os.IsNotExist(err) {
		t.Fatal("copy not removed by cleanup")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("second cleanup: %v", err)
	}
}

func TestCommandShellFallback(t *testing.T) {
	s := newStore(t)
	t.Setenv("SHELL", "")
	cmd, cleanup, err := Command(s, "prod", store.Env{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cleanup() }()
	if cmd.Path != "/bin/sh" {
		t.Fatalf("cmd.Path = %q, want /bin/sh", cmd.Path)
	}
}

func TestCommandRefusesNested(t *testing.T) {
	s := newStore(t)
	_, _, err := Command(s, "prod", store.Env{InShell: true, Project: "prod"})
	if err == nil || !strings.Contains(err.Error(), "already inside a kx shell") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(s.ShellCopyPath(os.Getpid(), "prod")); !os.IsNotExist(statErr) {
		t.Fatal("copy created despite refusal")
	}
}

func TestCommandErrors(t *testing.T) {
	s := newStore(t)
	if _, _, err := Command(s, "missing", store.Env{}); err == nil {
		t.Error("expected error for missing project")
	}
	if _, _, err := Command(s, "../x", store.Env{}); err == nil {
		t.Error("expected error for invalid name")
	}
}

func TestCommandPrunesStale(t *testing.T) {
	s := newStore(t)
	stale := s.ShellCopyPath(999999, "prod")
	if err := os.MkdirAll(filepath.Dir(stale), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := Command(s, "prod", store.Env{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cleanup() }()
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale copy not pruned")
	}
}

func TestCommandResolvesRelativeFileRefs(t *testing.T) {
	s := newStore(t)
	t.Setenv("SHELL", "/bin/sh")
	if err := os.WriteFile(filepath.Join(s.Root, "ca.crt"), []byte("CA"), 0o600); err != nil {
		t.Fatal(err)
	}
	withRef := "apiVersion: v1\nkind: Config\nclusters:\n- name: c\n  cluster:\n    server: https://x\n    certificate-authority: ca.crt\ncontexts: []\nusers: []\n"
	if err := os.WriteFile(s.ProjectPath("prod"), []byte(withRef), 0o600); err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := Command(s, "prod", store.Env{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cleanup() }()
	got, err := kubecfg.Load(s.ShellCopyPath(os.Getpid(), "prod"))
	if err != nil {
		t.Fatal(err)
	}
	if ca := got.Clusters["c"].CertificateAuthority; ca != filepath.Join(s.Root, "ca.crt") {
		t.Fatalf("certificate-authority = %q, want absolute path to master dir file", ca)
	}
	fi, _ := os.Stat(s.ShellCopyPath(os.Getpid(), "prod"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("copy mode = %v", fi.Mode().Perm())
	}
}

func TestSpawnIgnoresShellExitStatus(t *testing.T) {
	s := newStore(t)
	t.Setenv("SHELL", "/bin/false")
	if err := Spawn(s, "prod", store.Env{}); err != nil {
		t.Fatalf("Spawn = %v, want nil for non-zero shell exit", err)
	}
	if _, err := os.Stat(s.ShellCopyPath(os.Getpid(), "prod")); !os.IsNotExist(err) {
		t.Fatal("copy not cleaned up")
	}
}
