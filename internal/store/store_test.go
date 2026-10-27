package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validCfg = "apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\n"

func newStore(t *testing.T) *Store {
	t.Helper()
	return New(t.TempDir())
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func addProject(t *testing.T, s *Store, name, content string) {
	t.Helper()
	writeFile(t, s.ProjectPath(name), content, 0o600)
}

func TestValidateName(t *testing.T) {
	ok := []string{"dev", "prod.eu", "a_b-1", "A", strings.Repeat("a", 64)}
	bad := []string{"", "../x", "a/b", "-x", "_initial", ".x", strings.Repeat("a", 65), "x.lock", "x.bak", "x~", "x.orig", "x.swp"}
	for _, n := range ok {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidateName(n); !errors.Is(err, ErrInvalidName) {
			t.Errorf("ValidateName(%q) = %v, want ErrInvalidName", n, err)
		}
	}
}

func TestNewSetsBackupDir(t *testing.T) {
	s := newStore(t)
	if want := filepath.Join(s.Root, "kx", "backups"); s.Backups.Dir != want {
		t.Fatalf("Backups.Dir = %q, want %q", s.Backups.Dir, want)
	}
}

func TestDefaultRoot(t *testing.T) {
	t.Setenv("KX_HOME", "/tmp/kxhome")
	got, err := DefaultRoot()
	if err != nil || got != "/tmp/kxhome" {
		t.Fatalf("DefaultRoot() = %q, %v", got, err)
	}
	t.Setenv("KX_HOME", "")
	t.Setenv("HOME", "/tmp/fakehome")
	got, err = DefaultRoot()
	if err != nil || got != "/tmp/fakehome/.kube" {
		t.Fatalf("DefaultRoot() = %q, %v", got, err)
	}
}

func TestProjectsDiscovery(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	writeFile(t, s.ProjectPath("dev")+".bak", validCfg, 0o600)
	writeFile(t, s.ProjectPath("dev")+".lock", "", 0o600)
	writeFile(t, s.ProjectPath("x")+"~", validCfg, 0o600)
	addProject(t, s, "bad", "this: [is not\n kubeconfig")
	if err := os.Mkdir(s.ProjectPath("adir"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(s.Root, "config-"), validCfg, 0o600)

	ps, err := s.Projects()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range ps {
		names = append(names, p.Name)
	}
	if got := strings.Join(names, ","); got != "bad,dev" {
		t.Fatalf("names = %s, want bad,dev", got)
	}
	if ps[0].Err == nil {
		t.Error("bad should have Err")
	}
	if ps[1].Err != nil {
		t.Errorf("dev Err = %v", ps[1].Err)
	}
	if ps[1].Mode != 0o600 || ps[1].Path != s.ProjectPath("dev") {
		t.Errorf("dev mode/path = %v %q", ps[1].Mode, ps[1].Path)
	}
}

func TestProjectsMissingRoot(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "nope"))
	ps, err := s.Projects()
	if err != nil || len(ps) != 0 {
		t.Fatalf("Projects() = %v, %v", ps, err)
	}
}

func TestProjectsMarksActive(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	addProject(t, s, "prod", validCfg)
	if err := s.Use("prod"); err != nil {
		t.Fatal(err)
	}
	ps, _ := s.Projects()
	if ps[0].Active || !ps[1].Active {
		t.Fatalf("Active flags = %v %v, want false true", ps[0].Active, ps[1].Active)
	}
}

func TestPreviousEmpty(t *testing.T) {
	s := newStore(t)
	if p, err := s.Previous(); err != nil || p != "" {
		t.Fatalf("Previous() = %q, %v", p, err)
	}
}
