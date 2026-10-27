package store

import (
	"os"
	"path/filepath"
	"testing"
)

func writeShellCopy(t *testing.T, s *Store, pid int, project string) string {
	t.Helper()
	p := s.ShellCopyPath(pid, project)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, p, validCfg, 0o600)
	return p
}

func TestShellCopyPath(t *testing.T) {
	s := newStore(t)
	want := filepath.Join(s.Root, "kx", "shells", "42-prod.yaml")
	if got := s.ShellCopyPath(42, "prod"); got != want {
		t.Fatalf("ShellCopyPath = %q, want %q", got, want)
	}
}

func TestLiveShells(t *testing.T) {
	s := newStore(t)
	own := os.Getpid()
	writeShellCopy(t, s, own, "prod")
	writeShellCopy(t, s, own, "prod-2") // different project: must not match "prod"
	writeShellCopy(t, s, 999999, "prod")
	pids, err := s.LiveShells("prod")
	if err != nil || len(pids) != 1 || pids[0] != own {
		t.Fatalf("LiveShells = %v, %v", pids, err)
	}
	if pids, err := (&Store{Root: t.TempDir()}).LiveShells("prod"); err != nil || len(pids) != 0 {
		t.Fatalf("no shells dir: %v, %v", pids, err)
	}
}

func TestPruneStaleShells(t *testing.T) {
	s := newStore(t)
	stale := writeShellCopy(t, s, 999999, "prod")
	own := writeShellCopy(t, s, os.Getpid(), "prod")
	n, err := s.PruneStaleShells()
	if err != nil || n != 1 {
		t.Fatalf("PruneStaleShells = %d, %v", n, err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale copy still present")
	}
	if _, err := os.Stat(own); err != nil {
		t.Fatal("own copy removed")
	}
}
