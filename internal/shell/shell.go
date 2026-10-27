// Package shell implements "kx shell": a subshell whose KUBECONFIG points at
// a private copy of a project, so context and namespace changes stay local.
package shell

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/nunoarmada/kxconfig/v2/internal/fsutil"
	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/v2/internal/store"
)

// Command prepares a subshell for project: it refuses when already inside a
// kx shell, prunes stale copies, copies the project's master to
// ShellCopyPath(os.Getpid(), project) and returns the command to run plus a
// cleanup that removes the copy (safe to call more than once).
func Command(s *store.Store, project string, env store.Env) (*exec.Cmd, func() error, error) {
	if env.InShell {
		return nil, nil, errors.New("already inside a kx shell (exit it first)")
	}
	if err := store.ValidateName(project); err != nil {
		return nil, nil, err
	}
	master := s.ProjectPath(project)
	if _, err := os.Stat(master); errors.Is(err, fs.ErrNotExist) {
		return nil, nil, fmt.Errorf("%w: %s", store.ErrNotFound, project)
	} else if err != nil {
		return nil, nil, err
	}
	if _, err := s.PruneStaleShells(); err != nil {
		return nil, nil, fmt.Errorf("prune stale shell copies: %w", err)
	}
	copyPath := s.ShellCopyPath(os.Getpid(), project)
	if err := fsutil.EnsureDir(s.ShellsDir()); err != nil {
		return nil, nil, err
	}
	// Relative file references (certs, keys) resolve against the master's
	// directory; make them absolute so they keep working from kx/shells/.
	cfg, err := kubecfg.Load(master)
	if err != nil {
		return nil, nil, fmt.Errorf("project %s is not a valid kubeconfig: %w", project, err)
	}
	if err := clientcmd.ResolveLocalPaths(cfg); err != nil {
		return nil, nil, fmt.Errorf("resolve paths in %s: %w", master, err)
	}
	if err := kubecfg.Save(cfg, copyPath); err != nil {
		return nil, nil, fmt.Errorf("copy %s: %w", master, err)
	}

	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	cmd := exec.Command(sh)
	cmd.Env = append(withoutKeys(os.Environ(), "KUBECONFIG", "KX_PROJECT", "KX_SHELL"),
		"KUBECONFIG="+copyPath, "KX_PROJECT="+project, "KX_SHELL=1")
	cleanup := func() error {
		if err := os.Remove(copyPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	return cmd, cleanup, nil
}

func withoutKeys(env []string, keys ...string) []string {
	out := make([]string, 0, len(env))
outer:
	for _, e := range env {
		for _, k := range keys {
			if strings.HasPrefix(e, k+"=") {
				continue outer
			}
		}
		out = append(out, e)
	}
	return out
}

// Spawn runs the subshell attached to the terminal and removes the copy when
// it exits. A non-zero exit status of the shell is not an error.
func Spawn(s *store.Store, project string, env store.Env) error {
	cmd, cleanup, err := Command(s, project, env)
	if err != nil {
		return err
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	runErr := cmd.Run()
	// The user's last command may have failed; that is not a kx failure.
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		runErr = nil
	}
	return errors.Join(runErr, cleanup())
}
