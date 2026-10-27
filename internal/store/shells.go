package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const shellExt = ".yaml"

// ShellsDir returns <root>/kx/shells.
func (s *Store) ShellsDir() string {
	return filepath.Join(s.Root, "kx", "shells")
}

// ShellCopyPath returns where the kx shell with the given pid keeps its copy
// of project: <root>/kx/shells/<pid>-<project>.yaml.
func (s *Store) ShellCopyPath(pid int, project string) string {
	return filepath.Join(s.ShellsDir(), strconv.Itoa(pid)+"-"+project+shellExt)
}

type shellCopy struct {
	pid     int
	project string
	path    string
}

func parseShellName(name string) (pid int, project string, ok bool) {
	base, ok := strings.CutSuffix(name, shellExt)
	if !ok {
		return 0, "", false
	}
	p, project, ok := strings.Cut(base, "-")
	if !ok || project == "" {
		return 0, "", false
	}
	pid, err := strconv.Atoi(p)
	if err != nil || pid <= 0 {
		return 0, "", false
	}
	return pid, project, true
}

func (s *Store) shellCopies() ([]shellCopy, error) {
	des, err := os.ReadDir(s.ShellsDir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []shellCopy
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		pid, project, ok := parseShellName(de.Name())
		if !ok {
			continue
		}
		out = append(out, shellCopy{pid, project, filepath.Join(s.ShellsDir(), de.Name())})
	}
	return out, nil
}

// pidAlive reports whether a process with this pid exists. EPERM means it
// exists but belongs to someone else.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// LiveShells returns the pids of running kx shells for project.
func (s *Store) LiveShells(project string) ([]int, error) {
	copies, err := s.shellCopies()
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, c := range copies {
		if c.project == project && pidAlive(c.pid) {
			pids = append(pids, c.pid)
		}
	}
	return pids, nil
}

// PruneStaleShells removes shell copies whose process is gone and returns how
// many it removed.
func (s *Store) PruneStaleShells() (int, error) {
	copies, err := s.shellCopies()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, c := range copies {
		if pidAlive(c.pid) {
			continue
		}
		if err := os.Remove(c.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return n, err
		}
		n++
	}
	return n, nil
}
