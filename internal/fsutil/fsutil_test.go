package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func mustMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

func TestWriteFileAtomicCreates0600(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg")
	if err := WriteFileAtomic(p, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "hello" {
		t.Fatalf("content = %q", got)
	}
	if m := mustMode(t, p); m != 0o600 {
		t.Fatalf("mode = %o", m)
	}
}

func TestWriteFileAtomicFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.WriteFile(b, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(b, a); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(a, []byte("new")); err != nil {
		t.Fatal(err)
	}
	li, err := os.Lstat(a)
	if err != nil || li.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("a is no longer a symlink: %v %v", li, err)
	}
	got, _ := os.ReadFile(b)
	if string(got) != "new" {
		t.Fatalf("b content = %q", got)
	}
	if m := mustMode(t, b); m != 0o600 {
		t.Fatalf("b mode = %o", m)
	}
}

func TestWriteFileAtomicTightensExistingMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(p, []byte("y")); err != nil {
		t.Fatal(err)
	}
	if m := mustMode(t, p); m != 0o600 {
		t.Fatalf("mode = %o", m)
	}
}

func TestCopyFile0600(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "src"), filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CopyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "data" {
		t.Fatalf("content = %q", got)
	}
	if m := mustMode(t, dst); m != 0o600 {
		t.Fatalf("mode = %o", m)
	}
}

func TestEnsureDirTightens(t *testing.T) {
	d := filepath.Join(t.TempDir(), "kx")
	if err := os.Mkdir(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(d); err != nil {
		t.Fatal(err)
	}
	if m := mustMode(t, d); m != 0o700 {
		t.Fatalf("mode = %o", m)
	}
	n := filepath.Join(d, "x", "y")
	if err := EnsureDir(n); err != nil {
		t.Fatal(err)
	}
	if m := mustMode(t, n); m != 0o700 {
		t.Fatalf("new dir mode = %o", m)
	}
}

func TestTooOpen(t *testing.T) {
	for mode, want := range map[os.FileMode]bool{0o600: false, 0o640: true, 0o604: true} {
		if got := TooOpen(mode); got != want {
			t.Errorf("TooOpen(%o) = %v, want %v", mode, got, want)
		}
	}
}
