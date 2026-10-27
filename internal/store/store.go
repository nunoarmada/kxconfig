// Package store manages kx projects on disk: the config-<name> master files,
// the ~/.kube/config symlink that activates one of them, and kx/state.json.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nunoarmada/kxconfig/v2/internal/backup"
	"github.com/nunoarmada/kxconfig/v2/internal/fsutil"
	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
)

const projectPrefix = "config-"

var (
	// ErrForeignConfig means <root>/config is a regular file kx did not create
	// and whose content is not a known project.
	ErrForeignConfig = errors.New("config is a regular file that is not a kx project")
	// ErrForeignSymlink means <root>/config is a symlink that does not point at
	// a project file; kx never touches it.
	ErrForeignSymlink = errors.New("config is a symlink that does not point to a kx project")
	// ErrNotFound means the named project does not exist.
	ErrNotFound = errors.New("project not found")
	// ErrEmptyProject means a project file has zero length (for example
	// truncated by another tool); kx refuses to merge into or write over it.
	ErrEmptyProject = errors.New("project file is empty; refusing to write")
	// ErrInvalidName means a project name is not acceptable.
	ErrInvalidName = errors.New("invalid project name")
)

var (
	nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	// Suffixes that discovery ignores (client-go lock files, backups, editor files).
	ignoredSuffixes = []string{".lock", ".bak", ".orig", ".swp", "~"}
)

// Store is a kx home directory (normally ~/.kube).
type Store struct {
	Root    string
	Backups *backup.Manager

	// create takes a backup of data; nil means Backups.CreateData. Tests
	// inject failures.
	create func(project, op string, data []byte) (backup.Backup, error)
}

// New returns a Store rooted at root.
func New(root string) *Store {
	return &Store{
		Root:    root,
		Backups: &backup.Manager{Dir: filepath.Join(root, "kx", "backups")},
	}
}

// DefaultRoot returns $KX_HOME as an absolute path, or ~/.kube when it is
// unset. It is absolute so symlink targets and shell copies never depend on
// the working directory.
func DefaultRoot() (string, error) {
	if h := os.Getenv("KX_HOME"); h != "" {
		abs, err := filepath.Abs(h)
		if err != nil {
			return "", fmt.Errorf("resolve KX_HOME: %w", err)
		}
		return abs, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return filepath.Join(home, ".kube"), nil
}

// ValidateName reports whether name can be used as a project name.
func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%w %q: use letters, digits, '.', '_' or '-' (max 64), starting with a letter or digit", ErrInvalidName, name)
	}
	for _, suf := range ignoredSuffixes {
		if strings.HasSuffix(name, suf) {
			return fmt.Errorf("%w %q: must not end in %q", ErrInvalidName, name, suf)
		}
	}
	return nil
}

// ProjectPath returns <root>/config-<name>. The name must be validated first.
func (s *Store) ProjectPath(name string) string {
	return filepath.Join(s.Root, projectPrefix+name)
}

// ConfigPath returns <root>/config, the file kubectl reads.
func (s *Store) ConfigPath() string {
	return filepath.Join(s.Root, "config")
}

func (s *Store) statePath() string {
	return filepath.Join(s.Root, "kx", "state.json")
}

// Project describes one config-<name> file. Err is non-nil when the file does
// not parse as a kubeconfig.
type Project struct {
	Name, Path string
	Mode       fs.FileMode
	Err        error
	Active     bool
}

// Projects lists project files sorted by name. Files that match the naming
// rules but do not parse are included with Err set.
func (s *Store) Projects() ([]Project, error) {
	des, err := os.ReadDir(s.Root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	active, _, err := s.Active()
	if err != nil {
		return nil, err
	}
	var out []Project
	for _, de := range des {
		name, ok := strings.CutPrefix(de.Name(), projectPrefix)
		if !ok || ValidateName(name) != nil || !de.Type().IsRegular() {
			continue
		}
		info, err := de.Info()
		if err != nil {
			return nil, err
		}
		p := Project{
			Name:   name,
			Path:   filepath.Join(s.Root, de.Name()),
			Mode:   info.Mode().Perm(),
			Active: name == active,
		}
		_, p.Err = kubecfg.Load(p.Path)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type state struct {
	Previous string `json:"previous"`
}

// Previous returns the project that was active before the current one, or ""
// when none is recorded.
func (s *Store) Previous() (string, error) {
	data, err := os.ReadFile(s.statePath())
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var st state
	if err := json.Unmarshal(data, &st); err != nil {
		return "", fmt.Errorf("parse %s: %w", s.statePath(), err)
	}
	return st.Previous, nil
}

func (s *Store) setPrevious(name string) error {
	data, err := json.Marshal(state{Previous: name})
	if err != nil {
		return err
	}
	if err := fsutil.EnsureDir(filepath.Dir(s.statePath())); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(s.statePath(), append(data, '\n'))
}
