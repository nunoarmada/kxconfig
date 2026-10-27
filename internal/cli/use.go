package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nunoarmada/kxconfig/internal/store"
)

func (a *app) newUse() *cobra.Command {
	var adopt string
	var adoptReplace bool
	cmd := &cobra.Command{
		Use:   "use PROJECT|-",
		Short: "Activate a project globally (- goes back to the previous one)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if adopt != "" && adoptReplace {
				return errors.New("--adopt and --adopt-replace are mutually exclusive")
			}
			return a.useCmd(args[0], adopt, adoptReplace)
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return a.projectArgs(cmd, args, toComplete)
		},
	}
	cmd.Flags().StringVar(&adopt, "adopt", "", "if ~/.kube/config is not a kx project, keep it as project `NAME` and continue")
	cmd.Flags().BoolVar(&adoptReplace, "adopt-replace", false, "if ~/.kube/config is not a kx project, replace it (a backup is kept)")
	return cmd
}

func (a *app) useCmd(name, adopt string, adoptReplace bool) error {
	if a.Env.InShell {
		return errors.New("refused inside a kx shell: use would change the global project; exit the shell first")
	}
	if name == "-" {
		prev, err := a.Store.Previous()
		if err != nil {
			return err
		}
		if prev == "" {
			return errors.New("no previous project")
		}
		name = prev
	}

	err := a.Store.Use(name)
	if errors.Is(err, store.ErrForeignConfig) {
		if err := a.resolveForeign(adopt, adoptReplace); err != nil {
			return err
		}
		err = a.Store.Use(name)
	}
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.Out, "Switched to project %s\n", name)
	return nil
}

// resolveForeign deals with a ~/.kube/config that kx did not create: flags
// decide, a terminal asks, anything else fails with the flags to use.
func (a *app) resolveForeign(adopt string, adoptReplace bool) error {
	switch {
	case adopt != "":
		return a.Store.AdoptForeign(adopt)
	case adoptReplace:
		return a.Store.AdoptForeign("")
	case !a.IsTTY:
		return fmt.Errorf("%s is a regular file that is not a kx project; rerun with --adopt=<name> to keep it as a project, or --adopt-replace to replace it (a backup is kept)", a.Store.ConfigPath())
	}
	_, _ = fmt.Fprintf(a.Err, "%s is not a kx project.\n[i]mport as project / [r]eplace (backed up) / [a]bort: ", a.Store.ConfigPath())
	ans, err := a.readLine()
	if err != nil {
		return err
	}
	switch strings.ToLower(ans) {
	case "i", "import":
		_, _ = fmt.Fprint(a.Err, "Project name: ")
		name, err := a.readLine()
		if err != nil {
			return err
		}
		return a.Store.AdoptForeign(name)
	case "r", "replace":
		return a.Store.AdoptForeign("")
	}
	return errors.New("aborted")
}
