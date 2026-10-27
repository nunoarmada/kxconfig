package store

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/nunoarmada/kxconfig/internal/fsutil"
	"github.com/nunoarmada/kxconfig/internal/kubecfg"
)

// initialProject is the backup namespace for adopted ~/.kube/config files. It
// is deliberately not a valid project name.
const initialProject = "_initial"

// LinkState describes what <root>/config currently is.
type LinkState int

const (
	LinkNone           LinkState = iota // nothing at <root>/config
	LinkOK                              // symlink to an existing project file
	LinkBroken                          // symlink to a project file that is missing
	LinkForeignFile                     // a regular file kx did not create
	LinkForeignSymlink                  // a symlink that does not point at a project
)

// Active reports the active project, derived from the symlink target. The
// name is returned for LinkOK and LinkBroken only.
func (s *Store) Active() (string, LinkState, error) {
	fi, err := os.Lstat(s.ConfigPath())
	if errors.Is(err, fs.ErrNotExist) {
		return "", LinkNone, nil
	}
	if err != nil {
		return "", LinkNone, err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return "", LinkForeignFile, nil
	}
	target, err := os.Readlink(s.ConfigPath())
	if err != nil {
		return "", LinkNone, err
	}
	name, ok := s.projectFromTarget(target)
	if !ok {
		return "", LinkForeignSymlink, nil
	}
	pfi, err := os.Lstat(s.ProjectPath(name))
	if errors.Is(err, fs.ErrNotExist) {
		return name, LinkBroken, nil
	}
	if err != nil {
		return "", LinkNone, err
	}
	if !pfi.Mode().IsRegular() {
		return "", LinkForeignSymlink, nil
	}
	return name, LinkOK, nil
}

// projectFromTarget accepts a bare "config-<name>" or an absolute path to that
// file directly inside the root.
func (s *Store) projectFromTarget(target string) (string, bool) {
	if filepath.IsAbs(target) {
		if filepath.Clean(filepath.Dir(target)) != filepath.Clean(s.Root) {
			return "", false
		}
		target = filepath.Base(target)
	} else if target != filepath.Base(target) {
		return "", false
	}
	name, ok := strings.CutPrefix(target, projectPrefix)
	if !ok || ValidateName(name) != nil {
		return "", false
	}
	return name, true
}

// Use makes name the active project by atomically replacing the
// <root>/config symlink. A pre-existing ~/.kube/config is only replaced when a
// backup was taken and its content is already held by a project.
func (s *Store) Use(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	path := s.ProjectPath(name)
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !fi.Mode().IsRegular()) {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	if err != nil {
		return err
	}
	if _, err := kubecfg.Load(path); err != nil {
		return fmt.Errorf("project %s is not a valid kubeconfig: %w", name, err)
	}

	current, st, err := s.Active()
	if err != nil {
		return err
	}
	switch st {
	case LinkForeignSymlink:
		return ErrForeignSymlink
	case LinkForeignFile:
		same, err := s.matchesAnyProject(path)
		if err != nil {
			return err
		}
		if !same {
			return ErrForeignConfig
		}
		if _, err := s.Backups.Create(initialProject, "adopt", s.ConfigPath()); err != nil {
			return fmt.Errorf("back up %s: %w", s.ConfigPath(), err)
		}
	}

	if err := s.swapLink(projectPrefix + name); err != nil {
		return err
	}
	if current != "" && current != name {
		if err := s.setPrevious(current); err != nil {
			return fmt.Errorf("activated %s but could not record previous project: %w", name, err)
		}
	}
	return nil
}

// matchesAnyProject reports whether <root>/config is byte-identical to the
// preferred project file or to any other valid project file.
func (s *Store) matchesAnyProject(preferred string) (bool, error) {
	cfg, err := os.ReadFile(s.ConfigPath())
	if err != nil {
		return false, err
	}
	if same, err := sameContent(preferred, cfg); err != nil || same {
		return same, err
	}
	ps, err := s.Projects()
	if err != nil {
		return false, err
	}
	for _, p := range ps {
		if p.Err != nil || p.Path == preferred {
			continue
		}
		if same, err := sameContent(p.Path, cfg); err != nil || same {
			return same, err
		}
	}
	return false, nil
}

func sameContent(path string, want []byte) (bool, error) {
	got, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	return bytes.Equal(got, want), nil
}

// swapLink atomically points <root>/config at target: the new link is created
// under a temporary name and renamed over the old entry.
func (s *Store) swapLink(target string) error {
	tmp := filepath.Join(s.Root, ".kx-link-"+rand.Text())
	if err := os.Symlink(target, tmp); err != nil {
		return fmt.Errorf("create symlink: %w", err)
	}
	if err := os.Rename(tmp, s.ConfigPath()); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("activate %s: %w", target, err)
	}
	return nil
}

// AdoptForeign backs up a foreign <root>/config to the "_initial" backup
// project, optionally keeps it as the new project importAs, and removes it so
// Use can link in its place. The file is never removed without a backup.
func (s *Store) AdoptForeign(importAs string) error {
	if importAs != "" {
		if err := ValidateName(importAs); err != nil {
			return err
		}
	}
	if _, st, err := s.Active(); err != nil {
		return err
	} else if st != LinkForeignFile {
		return errors.New("config is not a foreign regular file; nothing to adopt")
	}
	if importAs != "" {
		if _, err := os.Lstat(s.ProjectPath(importAs)); err == nil {
			return fmt.Errorf("project %q already exists", importAs)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if _, err := s.Backups.Create(initialProject, "adopt", s.ConfigPath()); err != nil {
		return fmt.Errorf("back up %s: %w", s.ConfigPath(), err)
	}
	if importAs != "" {
		if err := fsutil.CopyFile(s.ConfigPath(), s.ProjectPath(importAs)); err != nil {
			return fmt.Errorf("import as %s: %w", importAs, err)
		}
	}
	return os.Remove(s.ConfigPath())
}
