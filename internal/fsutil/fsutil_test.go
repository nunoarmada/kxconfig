package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestWriteFileAtomicFollowsRelativeSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(dir, "sub", "b")
	if err := os.WriteFile(b, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(dir, "a")
	if err := os.Symlink(filepath.Join("sub", "b"), a); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(a, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if li, err := os.Lstat(a); err != nil || li.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("a is no longer a symlink: %v %v", li, err)
	}
	if got, _ := os.ReadFile(b); string(got) != "new" {
		t.Fatalf("b content = %q", got)
	}
}

func TestWithLockCreatesAndRemovesLockFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg")
	ran := false
	err := WithLock(p, func() error {
		ran = true
		if _, err := os.Stat(p + ".lock"); err != nil {
			t.Errorf("lock file missing while held: %v", err)
		}
		return nil
	})
	if err != nil || !ran {
		t.Fatalf("WithLock = %v, ran %v", err, ran)
	}
	if _, err := os.Stat(p + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("lock file left behind: %v", err)
	}
}

func TestWithLockHeldElsewhereFails(t *testing.T) {
	old := LockWait
	LockWait = 60 * time.Millisecond
	defer func() { LockWait = old }()
	p := filepath.Join(t.TempDir(), "cfg")
	if err := os.WriteFile(p+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ran := false
	err := WithLock(p, func() error { ran = true; return nil })
	if err == nil || !strings.Contains(err.Error(), ".lock") {
		t.Fatalf("err = %v, want a lock error naming the lock file", err)
	}
	if ran {
		t.Fatal("fn ran without the lock")
	}
	if _, err := os.Stat(p + ".lock"); err != nil {
		t.Fatalf("someone else's lock file was removed: %v", err)
	}
}

func TestWithLockRemovesLockOnError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cfg")
	want := errors.New("boom")
	if err := WithLock(p, func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(p + ".lock"); !os.IsNotExist(err) {
		t.Fatalf("lock file left behind: %v", err)
	}
}

func TestWithLocksTakesAllAndReleases(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	err := WithLocks([]string{b, a}, func() error {
		for _, p := range []string{a, b} {
			if _, err := os.Stat(p + ".lock"); err != nil {
				t.Errorf("lock for %s missing: %v", p, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{a, b} {
		if _, err := os.Stat(p + ".lock"); !os.IsNotExist(err) {
			t.Errorf("lock for %s left behind", p)
		}
	}
}

func TestWithLocksReleasesOnFailure(t *testing.T) {
	old := LockWait
	LockWait = 40 * time.Millisecond
	defer func() { LockWait = old }()
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := os.WriteFile(b+".lock", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	ran := false
	if err := WithLocks([]string{a, b}, func() error { ran = true; return nil }); err == nil || ran {
		t.Fatalf("err = %v, ran = %v; want a lock error", err, ran)
	}
	if _, err := os.Stat(a + ".lock"); !os.IsNotExist(err) {
		t.Fatal("the first lock was not released after the second failed")
	}
	if _, err := os.Stat(b + ".lock"); err != nil {
		t.Fatal("someone else's lock was removed")
	}
}
