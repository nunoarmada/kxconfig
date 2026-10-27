// Package cli implements the kx command line on top of the store, kubecfg,
// backup and shell packages.
package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nunoarmada/kxconfig/internal/safetext"
	"github.com/nunoarmada/kxconfig/internal/store"
)

// Deps carries everything the commands need, so tests can run them in-process.
type Deps struct {
	Store   *store.Store
	Env     store.Env
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
	IsTTY   bool // stdin is a terminal: prompts are allowed
	RunTUI  func() error
	Version string
}

// app holds the dependencies plus the buffered input shared by all prompts.
type app struct {
	Deps
	in *bufio.Reader
}

// NewRootCmd builds the kx command tree.
func NewRootCmd(d Deps) *cobra.Command {
	a := &app{Deps: d, in: bufio.NewReader(d.In)}

	root := &cobra.Command{
		Use:   "kx",
		Short: "Manage kubeconfig projects",
		Long: `kx keeps one kubeconfig per project and switches between them.
Without arguments it opens the interactive UI (needs a terminal).`,
		Args:          cobra.MaximumNArgs(1),
		SilenceErrors: true,
		// Usage is only useful for flag and argument mistakes; runtime errors
		// switch it off in PersistentPreRun, which runs after parsing.
		PersistentPreRun: func(cmd *cobra.Command, _ []string) { cmd.SilenceUsage = true },
		RunE: func(cmd *cobra.Command, args []string) error {
			switch {
			case len(args) == 1 && args[0] == "-":
				return a.useCmd("-", "", false)
			case len(args) == 1:
				return fmt.Errorf("unknown command %q for kx; see 'kx --help'", args[0])
			case !a.IsTTY:
				return errors.New("the interactive UI needs a terminal; see 'kx --help'")
			}
			return a.RunTUI()
		},
	}
	root.SetOut(d.Out)
	root.SetErr(d.Err)
	root.SetIn(d.In)

	root.AddCommand(
		a.newUse(), a.newImport(), a.newLs(), a.newCurrent(),
		a.newCtx(), a.newNs(), a.newRmCtx(),
		a.newRename(), a.newDelete(), a.newBackups(), a.newRestore(),
		a.newShell(), a.newDoctor(), a.newVersion(),
	)
	return root
}

// Run executes kx with args and returns the process exit code: 0 on success,
// 1 on any error, which is printed once as "kx: <message>" on d.Err.
func Run(d Deps, args []string) int {
	cmd := NewRootCmd(d)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		_, _ = fmt.Fprintf(d.Err, "kx: %s\n", safetext.Clean(err.Error()))
		return 1
	}
	return 0
}

// confirm asks a yes/no question on a terminal. Without one it fails with a
// message pointing at --yes.
func (a *app) confirm(question string) error {
	if !a.IsTTY {
		return errors.New("refusing to continue without confirmation; stdin is not a terminal (pass --yes)")
	}
	_, _ = fmt.Fprintf(a.Err, "%s [y/N] ", question)
	line, err := a.readLine()
	if err != nil {
		return err
	}
	if l := strings.ToLower(line); l != "y" && l != "yes" {
		return errors.New("aborted")
	}
	return nil
}

// readLine reads one trimmed line; EOF without text is an error.
func (a *app) readLine() (string, error) {
	line, err := a.in.ReadString('\n')
	if err != nil && line == "" {
		if errors.Is(err, io.EOF) {
			return "", errors.New("aborted: no input")
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// warnLiveShells tells the user when kx shells are running for project.
func (a *app) warnLiveShells(project string) {
	pids, err := a.Store.LiveShells(project)
	if err != nil {
		_, _ = fmt.Fprintf(a.Err, "warning: could not check running kx shells: %v\n", err)
		return
	}
	if len(pids) > 0 {
		_, _ = fmt.Fprintf(a.Err, "warning: %d kx shell(s) are running for %s (pids %v); they keep their own copy and will not see this change\n", len(pids), safetext.Clean(project), pids)
	}
}

// completeProjects completes project names.
func (a *app) completeProjects(toComplete string) []string {
	ps, err := a.Store.Projects()
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range ps {
		if strings.HasPrefix(p.Name, toComplete) {
			out = append(out, p.Name)
		}
	}
	return out
}

// projectArgs completes a project name for the first positional argument.
func (a *app) projectArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return a.completeProjects(toComplete), cobra.ShellCompDirectiveNoFileComp
}

// projectFlag registers -p/--project with project-name completion.
func (a *app) projectFlag(cmd *cobra.Command, target *string, usage string) {
	cmd.Flags().StringVarP(target, "project", "p", "", usage)
	_ = cmd.RegisterFlagCompletionFunc("project", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return a.completeProjects(toComplete), cobra.ShellCompDirectiveNoFileComp
	})
}
