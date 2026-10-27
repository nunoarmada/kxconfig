package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func findIssue(issues []Issue, kind string) *Issue {
	for i := range issues {
		if issues[i].Kind == kind {
			return &issues[i]
		}
	}
	return nil
}

func TestDoctorFixesPerms(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	if err := os.Chmod(s.ProjectPath("dev"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(s.Root, "kx", "backups"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(s.Root, "kx"), 0o755); err != nil {
		t.Fatal(err)
	}

	issues, err := s.Doctor(false)
	if err != nil {
		t.Fatal(err)
	}
	pi := findIssue(issues, "perms")
	if pi == nil || !pi.Fixable {
		t.Fatalf("issues = %+v", issues)
	}
	if fi, _ := os.Stat(s.ProjectPath("dev")); fi.Mode().Perm() != 0o644 {
		t.Fatal("Doctor(false) must not change anything")
	}

	issues, err = s.Doctor(true)
	if err != nil {
		t.Fatal(err)
	}
	if i := findIssue(issues, "perms"); i == nil || !i.Fixed {
		t.Fatalf("issues after fix = %+v", issues)
	}
	if fi, _ := os.Stat(s.ProjectPath("dev")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("project mode = %v, want 0600", fi.Mode().Perm())
	}
	for _, d := range []string{"kx", "kx/backups"} {
		if fi, _ := os.Stat(filepath.Join(s.Root, d)); fi.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode = %v, want 0700", d, fi.Mode().Perm())
		}
	}
	if issues, _ := s.Doctor(false); len(issues) != 0 {
		t.Fatalf("still issues after fix: %+v", issues)
	}
}

func TestDoctorBrokenLink(t *testing.T) {
	s := newStore(t)
	if err := os.Symlink("config-gone", s.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	issues, err := s.Doctor(true)
	if err != nil {
		t.Fatal(err)
	}
	i := findIssue(issues, "broken-link")
	if i == nil || i.Fixable || i.Path != s.ConfigPath() {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestDoctorInvalidProjectAndForeignConfig(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "bad", "clusters: [unterminated\n :::")
	writeFile(t, s.ConfigPath(), validCfg, 0o600)
	issues, err := s.Doctor(true)
	if err != nil {
		t.Fatal(err)
	}
	if i := findIssue(issues, "invalid-project"); i == nil || i.Fixable {
		t.Fatalf("issues = %+v", issues)
	}
	if i := findIssue(issues, "foreign-config"); i == nil || i.Fixable {
		t.Fatalf("issues = %+v", issues)
	}
	if _, err := os.Stat(s.ProjectPath("bad")); err != nil {
		t.Fatal("invalid project must never be modified or removed")
	}
}

func TestDoctorStaleShells(t *testing.T) {
	s := newStore(t)
	stale := writeShellCopy(t, s, 999999, "prod")
	issues, err := s.Doctor(false)
	if err != nil {
		t.Fatal(err)
	}
	if i := findIssue(issues, "stale-shell"); i == nil || !i.Fixable || i.Path != stale {
		t.Fatalf("issues = %+v", issues)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatal("Doctor(false) removed the copy")
	}
	if _, err := s.Doctor(true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale copy not removed by Doctor(true)")
	}
}

func TestDoctorKeepsFixError(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	if err := os.Chmod(s.ProjectPath("dev"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := chmod
	chmod = func(string, os.FileMode) error { return errors.New("operation not permitted") }
	t.Cleanup(func() { chmod = orig })
	issues, err := s.Doctor(true)
	if err != nil {
		t.Fatal(err)
	}
	i := findIssue(issues, "perms")
	if i == nil || i.Fixed || !strings.Contains(i.Message, "operation not permitted") {
		t.Fatalf("issue = %+v", i)
	}
}
