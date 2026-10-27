// Package fsutil provides small filesystem helpers that keep credential
// files private (0600) and kx directories private (0700).
package fsutil

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// LockWait is how long WithLock keeps retrying a lock held by someone else.
var LockWait = time.Second

// WriteFileAtomic writes data to path via a temp file in the same directory
// (chmod 0600, fsync, rename). If path is a symlink, the resolved target is
// written so the link itself survives.
func WriteFileAtomic(path string, data []byte) error {
	target, err := resolveTarget(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".kx-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// resolveTarget returns the symlink-resolved path if path is a symlink
// (even a dangling one), otherwise path unchanged.
func resolveTarget(path string) (string, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return path, nil
	}
	link, err := os.Readlink(path)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(link) {
		link = filepath.Join(filepath.Dir(path), link)
	}
	// Follow chained links; a dangling final link is still a valid write target.
	if resolved, err := filepath.EvalSymlinks(link); err == nil {
		return resolved, nil
	}
	return link, nil
}

// CopyFile copies src to dst atomically; dst ends up with mode 0600.
func CopyFile(src, dst string) (err error) {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", src, cerr)
		}
	}()
	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	return WriteFileAtomic(dst, data)
}

// EnsureDir creates dir (and parents) with mode 0700 and tightens it to 0700
// if it already existed with wider permissions.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if fi.Mode().Perm() != 0o700 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

// TooOpen reports whether mode grants any permission to group or others.
func TooOpen(mode fs.FileMode) bool {
	return mode.Perm()&0o077 != 0
}

// WithLock runs fn while holding the client-go compatible lock file
// path+".lock" (created with O_EXCL, removed afterwards), so kx and kubectl
// do not write the same kubeconfig at the same time. A held lock is retried
// for LockWait and then reported as an error; it is never removed by force.
func WithLock(path string, fn func() error) error {
	lock := path + ".lock"
	deadline := time.Now().Add(LockWait)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_ = f.Close()
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("lock %s: %w", path, err)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s is locked by another process (kubectl?); retry, or remove %s if it is stale", path, lock)
		}
		time.Sleep(25 * time.Millisecond)
	}
	defer func() { _ = os.Remove(lock) }()
	return fn()
}

// WithLocks is WithLock for several files: the locks are taken in sorted path
// order (so two kx processes cannot deadlock) and all are released when fn
// returns or one of them cannot be taken.
func WithLocks(paths []string, fn func() error) error {
	sorted := slices.Sorted(slices.Values(paths))
	sorted = slices.Compact(sorted)
	run := fn
	for i := len(sorted) - 1; i >= 0; i-- {
		inner, p := run, sorted[i]
		run = func() error { return WithLock(p, inner) }
	}
	return run()
}
