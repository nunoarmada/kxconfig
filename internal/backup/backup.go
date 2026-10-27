// Package backup keeps timestamped copies of kubeconfig files, per project,
// with a fixed retention.
package backup

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nunoarmada/kxconfig/v2/internal/fsutil"
)

// Keep is the number of backups retained per project.
const Keep = 20

// InitialProject is the backup namespace for adopted ~/.kube/config files.
// It is deliberately not a valid project name, and its backups are never
// pruned: they may be the only copy of a user's original config.
const InitialProject = "_initial"

const (
	idLayout = "20060102T150405.000Z"
	ext      = ".yaml"
)

// Manager stores backups under Dir/<project>/<ID>.yaml.
type Manager struct {
	Dir string           // <root>/kx/backups
	Now func() time.Time // defaults to time.Now

	remove func(string) error // defaults to os.Remove; overridden in tests
}

var opRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z][a-z0-9]*)*$`)

// Backup describes one stored backup file.
type Backup struct {
	ID, Project, Op, Path string
	Time                  time.Time
}

type entry struct {
	Backup
	seq int // collision counter: 1 for the first, 2 for "-2", ...
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) projectDir(project string) string {
	return filepath.Join(m.Dir, project)
}

// Create copies src into the project's backup dir and prunes old backups.
// Project names must already be validated by the caller; op must match
// opRe (lowercase words joined by "-", no trailing "-<digits>").
//
// If the copy succeeds but pruning old backups fails, Create returns the
// created Backup together with a non-nil error wrapping the prune failure.
func (m *Manager) Create(project, op, src string) (Backup, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return Backup{}, fmt.Errorf("create backup: %w", err)
	}
	return m.CreateData(project, op, data)
}

// CreateData is Create for content the caller already read, so the backup
// holds exactly the bytes that were parsed and are about to be replaced.
func (m *Manager) CreateData(project, op string, data []byte) (Backup, error) {
	if !opRe.MatchString(op) {
		return Backup{}, fmt.Errorf("invalid backup op %q", op)
	}
	dir := m.projectDir(project)
	if err := fsutil.EnsureDir(m.Dir); err != nil {
		return Backup{}, err
	}
	if err := fsutil.EnsureDir(dir); err != nil {
		return Backup{}, err
	}
	t := m.now().UTC().Truncate(time.Millisecond)
	base := t.Format(idLayout) + "-" + op
	id := base
	for n := 2; ; n++ {
		_, err := os.Lstat(filepath.Join(dir, id+ext))
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return Backup{}, err
		}
		id = base + "-" + strconv.Itoa(n)
	}
	path := filepath.Join(dir, id+ext)
	if err := fsutil.WriteFileAtomic(path, data); err != nil {
		return Backup{}, fmt.Errorf("create backup: %w", err)
	}
	b := Backup{ID: id, Project: project, Op: op, Path: path, Time: t}
	if err := m.prune(project); err != nil {
		return b, fmt.Errorf("prune backups: %w", err)
	}
	return b, nil
}

// List returns the project's backups, newest first. A missing directory yields
// an empty list; files that are not named <ID>.yaml are ignored.
func (m *Manager) List(project string) ([]Backup, error) {
	entries, err := m.entries(project)
	if err != nil {
		return nil, err
	}
	out := make([]Backup, len(entries))
	for i, e := range entries {
		out[i] = e.Backup
	}
	return out, nil
}

// Get returns the backup with the given ID, or an error wrapping
// fs.ErrNotExist if there is none.
func (m *Manager) Get(project, id string) (Backup, error) {
	entries, err := m.entries(project)
	if err != nil {
		return Backup{}, err
	}
	for _, e := range entries {
		if e.ID == id {
			return e.Backup, nil
		}
	}
	return Backup{}, fmt.Errorf("backup %q in project %q: %w", id, project, fs.ErrNotExist)
}

func (m *Manager) entries(project string) ([]entry, error) {
	dir := m.projectDir(project)
	des, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []entry
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		id, ok := strings.CutSuffix(de.Name(), ext)
		if !ok {
			continue
		}
		e, ok := parseID(id)
		if !ok {
			continue
		}
		e.Project = project
		e.Path = filepath.Join(dir, de.Name())
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Time.Equal(out[j].Time) {
			return out[i].Time.After(out[j].Time)
		}
		return out[i].seq > out[j].seq
	})
	return out, nil
}

// parseID parses "<timestamp>-<op>[-<n>]".
func parseID(id string) (entry, bool) {
	ts, rest, ok := strings.Cut(id, "-")
	if !ok || rest == "" {
		return entry{}, false
	}
	t, err := time.Parse(idLayout, ts)
	if err != nil {
		return entry{}, false
	}
	op, seq := rest, 1
	if i := strings.LastIndex(rest, "-"); i > 0 {
		if n, err := strconv.Atoi(rest[i+1:]); err == nil && n >= 2 {
			op, seq = rest[:i], n
		}
	}
	return entry{Backup: Backup{ID: id, Op: op, Time: t}, seq: seq}, true
}

func (m *Manager) prune(project string) error {
	if project == InitialProject {
		return nil
	}
	entries, err := m.entries(project)
	if err != nil {
		return err
	}
	rm := m.remove
	if rm == nil {
		rm = os.Remove
	}
	for _, e := range entries[min(Keep, len(entries)):] {
		if err := rm(e.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
