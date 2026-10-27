package store

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nunoarmada/kxconfig/internal/fsutil"
)

// Issue is one problem found by Doctor. Fixable issues are repaired when
// Doctor runs with fix=true, and then have Fixed set.
type Issue struct {
	Kind, Path, Message string
	Fixable             bool
	Fixed               bool
}

// Doctor checks the kx home for problems: project files readable by others,
// kx directories wider than 0700, a broken or foreign <root>/config, projects
// that do not parse, and stale shell copies. With fix=true the fixable ones
// (permissions, stale shell copies) are repaired. Invalid projects and the
// config link are never modified.
func (s *Store) Doctor(fix bool) ([]Issue, error) {
	var issues []Issue
	add := func(i Issue) *Issue {
		issues = append(issues, i)
		return &issues[len(issues)-1]
	}

	projects, err := s.Projects()
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.Err != nil {
			add(Issue{Kind: "invalid-project", Path: p.Path, Message: fmt.Sprintf("project %s does not parse: %v", p.Name, p.Err)})
		}
		if fsutil.TooOpen(p.Mode) {
			i := add(Issue{Kind: "perms", Path: p.Path, Fixable: true, Message: fmt.Sprintf("project %s has mode %04o, want 0600", p.Name, p.Mode)})
			if fix {
				i.Fixed = fixed(i, chmod(p.Path, 0o600))
			}
		}
	}

	name, st, err := s.Active()
	if err != nil {
		return nil, err
	}
	switch st {
	case LinkBroken:
		add(Issue{Kind: "broken-link", Path: s.ConfigPath(), Message: fmt.Sprintf("config points at missing project %s (run kx use <project>)", name)})
	case LinkForeignFile:
		add(Issue{Kind: "foreign-config", Path: s.ConfigPath(), Message: "config is a regular file that kx did not create (kx use adopts it with a backup)"})
	case LinkForeignSymlink:
		add(Issue{Kind: "foreign-symlink", Path: s.ConfigPath(), Message: "config is a symlink that does not point to a kx project; kx leaves it alone"})
	}

	kxDir := filepath.Join(s.Root, "kx")
	walkErr := filepath.WalkDir(kxDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == kxDir && os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() && !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		mode := info.Mode().Perm()
		switch {
		case d.IsDir() && mode != 0o700:
			i := add(Issue{Kind: "perms", Path: path, Fixable: true, Message: fmt.Sprintf("directory has mode %04o, want 0700", mode)})
			if fix {
				i.Fixed = fixed(i, chmod(path, 0o700))
			}
		case !d.IsDir() && fsutil.TooOpen(mode):
			i := add(Issue{Kind: "perms", Path: path, Fixable: true, Message: fmt.Sprintf("file has mode %04o, want 0600", mode)})
			if fix {
				i.Fixed = fixed(i, chmod(path, 0o600))
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	copies, err := s.shellCopies()
	if err != nil {
		return nil, err
	}
	for _, c := range copies {
		if pidAlive(c.pid) {
			continue
		}
		i := add(Issue{Kind: "stale-shell", Path: c.path, Fixable: true, Message: fmt.Sprintf("shell copy of %s from dead process %d", c.project, c.pid)})
		if fix {
			i.Fixed = fixed(i, os.Remove(c.path))
		}
	}
	return issues, nil
}

// chmod is os.Chmod; tests replace it to simulate failures.
var chmod = os.Chmod

// fixed reports whether a repair succeeded, recording the error otherwise.
func fixed(i *Issue, err error) bool {
	if err != nil {
		i.Message += " (fix failed: " + err.Error() + ")"
		return false
	}
	return true
}
