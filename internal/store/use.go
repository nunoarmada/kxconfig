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

	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/nunoarmada/kxconfig/v2/internal/backup"
	"github.com/nunoarmada/kxconfig/v2/internal/fsutil"
	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
)

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
		// Use has no result to carry a warning; a failed prune only leaves
		// old backups behind and the new one exists, so ignoring it is safe.
		if _, _, err := s.backup(backup.InitialProject, "adopt", s.ConfigPath()); err != nil {
			return err
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

// matchesAnyProject reports whether <root>/config holds the same kubeconfig
// as the preferred project file or any other valid project file. Files are
// compared semantically (formatting, comments and the spelling of file
// references do not matter), because importing re-serialises a config.
func (s *Store) matchesAnyProject(preferred string) (bool, error) {
	data, err := os.ReadFile(s.ConfigPath())
	if err != nil {
		return false, err
	}
	cfg, err := kubecfg.LoadBytes(data, s.ConfigPath())
	if err != nil {
		cfg = nil // unparsable: only a byte-identical project can match
	}
	if same, err := sameContent(preferred, data, cfg); err != nil || same {
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
		if same, err := sameContent(p.Path, data, cfg); err != nil || same {
			return same, err
		}
	}
	return false, nil
}

// sameContent reports whether the project at path is byte-identical to want
// or, when want parsed as cfg, holds the same kubeconfig.
func sameContent(path string, want []byte, cfg *api.Config) (bool, error) {
	got, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	if bytes.Equal(got, want) {
		return true, nil
	}
	if cfg == nil {
		return false, nil
	}
	proj, err := kubecfg.LoadBytes(got, path)
	if err != nil {
		return false, nil
	}
	return sameConfig(cfg, proj)
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

// AdoptForeign backs up a foreign <root>/config to the backup.InitialProject backup
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
	// Same as in Use: the prune warning is safe to ignore.
	if _, _, err := s.backup(backup.InitialProject, "adopt", s.ConfigPath()); err != nil {
		return err
	}
	if importAs != "" {
		if err := fsutil.CopyFile(s.ConfigPath(), s.ProjectPath(importAs)); err != nil {
			return fmt.Errorf("import as %s: %w", importAs, err)
		}
	}
	return os.Remove(s.ConfigPath())
}
