package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func errorsIs(err, target error) bool { return errors.Is(err, target) }

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertState(t *testing.T, s *Store, wantName string, wantState LinkState) {
	t.Helper()
	name, st, err := s.Active()
	if err != nil || name != wantName || st != wantState {
		t.Fatalf("Active() = %q, %v, %v; want %q, %v", name, st, err, wantName, wantState)
	}
}

func TestUseCreatesSymlinkAndPrevious(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	addProject(t, s, "prod", validCfg)
	assertState(t, s, "", LinkNone)
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Previous(); p != "" {
		t.Fatalf("Previous() after first use = %q", p)
	}
	if err := s.Use("prod"); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(s.ConfigPath())
	if err != nil || target != "config-prod" {
		t.Fatalf("link target = %q, %v", target, err)
	}
	assertState(t, s, "prod", LinkOK)
	if p, err := s.Previous(); err != nil || p != "dev" {
		t.Fatalf("Previous() = %q, %v", p, err)
	}
	fi, err := os.Stat(filepath.Join(s.Root, "kx", "state.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("state.json: %v, %v", fi, err)
	}
	// re-using the active project must not clobber previous
	if err := s.Use("prod"); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Previous(); p != "dev" {
		t.Fatalf("Previous() after re-use = %q, want dev", p)
	}
	// no temp links left behind
	des, _ := os.ReadDir(s.Root)
	for _, de := range des {
		if de.Name() != "config" && de.Name() != "config-dev" && de.Name() != "config-prod" && de.Name() != "kx" {
			t.Errorf("stray file %q", de.Name())
		}
	}
}

func TestUseIdenticalForeignAdoptsSilently(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	writeFile(t, s.ConfigPath(), validCfg, 0o600)
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	assertState(t, s, "dev", LinkOK)
	bs, err := s.Backups.List("_initial")
	if err != nil || len(bs) != 1 || bs[0].Op != "adopt" {
		t.Fatalf("backups = %v, %v", bs, err)
	}
	if got := readFile(t, bs[0].Path); got != validCfg {
		t.Fatalf("backup content = %q", got)
	}
}

func TestUseForeignFileRefuses(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	foreign := validCfg + "# mine\n"
	writeFile(t, s.ConfigPath(), foreign, 0o600)

	err := s.Use("dev")
	if !errors.Is(err, ErrForeignConfig) {
		t.Fatalf("Use = %v, want ErrForeignConfig", err)
	}
	if got := readFile(t, s.ConfigPath()); got != foreign {
		t.Fatalf("foreign file modified: %q", got)
	}
	if fi, _ := os.Lstat(s.ConfigPath()); fi.Mode()&os.ModeSymlink != 0 {
		t.Fatal("foreign file was replaced by a symlink")
	}
	assertState(t, s, "", LinkForeignFile)

	if err := s.AdoptForeign(""); err != nil {
		t.Fatal(err)
	}
	bs, _ := s.Backups.List("_initial")
	if len(bs) != 1 || readFile(t, bs[0].Path) != foreign {
		t.Fatalf("backup missing or wrong: %v", bs)
	}
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	assertState(t, s, "dev", LinkOK)
}

func TestAdoptForeignAsProject(t *testing.T) {
	s := newStore(t)
	foreign := validCfg + "# mine\n"
	writeFile(t, s.ConfigPath(), foreign, 0o644)
	if err := s.AdoptForeign("old"); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, s.ProjectPath("old")); got != foreign {
		t.Fatalf("config-old = %q", got)
	}
	if fi, _ := os.Stat(s.ProjectPath("old")); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v", fi.Mode())
	}
	if _, err := os.Lstat(s.ConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("config still present: %v", err)
	}
	bs, _ := s.Backups.List("_initial")
	if len(bs) != 1 || readFile(t, bs[0].Path) != foreign {
		t.Fatalf("backup = %v", bs)
	}
}

func TestAdoptForeignRefusals(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "old", validCfg)
	writeFile(t, s.ConfigPath(), "x: y\n", 0o600)
	if err := s.AdoptForeign("old"); err == nil {
		t.Fatal("want error when target project exists")
	}
	if got := readFile(t, s.ProjectPath("old")); got != validCfg {
		t.Fatal("existing project overwritten")
	}
	if got := readFile(t, s.ConfigPath()); got != "x: y\n" {
		t.Fatal("foreign config touched")
	}
	if err := s.AdoptForeign("../x"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("err = %v, want ErrInvalidName", err)
	}
	// not a foreign file: nothing to adopt
	_ = os.Remove(s.ConfigPath())
	if err := s.AdoptForeign(""); err == nil {
		t.Fatal("want error when config is not a foreign file")
	}
}

func TestUseForeignSymlinkRefuses(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	outside := filepath.Join(t.TempDir(), "elsewhere")
	writeFile(t, outside, validCfg, 0o600)
	if err := os.Symlink(outside, s.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	if err := s.Use("dev"); !errors.Is(err, ErrForeignSymlink) {
		t.Fatalf("Use = %v, want ErrForeignSymlink", err)
	}
	if got, _ := os.Readlink(s.ConfigPath()); got != outside {
		t.Fatalf("symlink modified: %q", got)
	}
	assertState(t, s, "", LinkForeignSymlink)
}

func TestActiveAbsoluteLinkInsideRoot(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	if err := os.Symlink(s.ProjectPath("dev"), s.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	assertState(t, s, "dev", LinkOK)
}

func TestActiveBrokenLink(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	if err := s.Use("dev"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.ProjectPath("dev")); err != nil {
		t.Fatal(err)
	}
	assertState(t, s, "dev", LinkBroken)
	// Use fixes it
	addProject(t, s, "prod", validCfg)
	if err := s.Use("prod"); err != nil {
		t.Fatal(err)
	}
	assertState(t, s, "prod", LinkOK)
}

func TestActiveSymlinkToNonProjectIsForeign(t *testing.T) {
	s := newStore(t)
	for _, target := range []string{"config-dev.bak", "config-../x", "other", "sub/config-dev"} {
		_ = os.Remove(s.ConfigPath())
		if err := os.Symlink(target, s.ConfigPath()); err != nil {
			t.Fatal(err)
		}
		assertState(t, s, "", LinkForeignSymlink)
	}
}

func TestUseMissingProject(t *testing.T) {
	s := newStore(t)
	if err := s.Use("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Use = %v, want ErrNotFound", err)
	}
	if _, err := os.Lstat(s.ConfigPath()); !os.IsNotExist(err) {
		t.Fatal("config created")
	}
}

func TestUseInvalidNameAndUnparsable(t *testing.T) {
	s := newStore(t)
	if err := s.Use("../x"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Use = %v, want ErrInvalidName", err)
	}
	addProject(t, s, "bad", "this: [is not\n kubeconfig")
	if err := s.Use("bad"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("Use(bad) = %v, want parse error", err)
	}
	if _, err := os.Lstat(s.ConfigPath()); !os.IsNotExist(err) {
		t.Fatal("config created for invalid project")
	}
}

func TestUseIdenticalToOtherProjectAdopts(t *testing.T) {
	s := newStore(t)
	addProject(t, s, "dev", validCfg)
	addProject(t, s, "prod", validCfg+"# prod\n")
	writeFile(t, s.ConfigPath(), validCfg, 0o600) // identical to dev, switching to prod
	if err := s.Use("prod"); err != nil {
		t.Fatal(err)
	}
	assertState(t, s, "prod", LinkOK)
	bs, _ := s.Backups.List("_initial")
	if len(bs) != 1 || !bytes.Equal([]byte(readFile(t, bs[0].Path)), []byte(validCfg)) {
		t.Fatalf("backup = %v", bs)
	}
}
