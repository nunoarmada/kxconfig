package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		return a.replaceForeign()
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
		def := a.freeName("old")
		_, _ = fmt.Fprintf(a.Err, "Project name [%s]: ", def)
		name, err := a.readLine()
		if err != nil {
			return err
		}
		if name == "" {
			name = def
		}
		return a.Store.AdoptForeign(name)
	case "r", "replace":
		return a.replaceForeign()
	}
	return errors.New("aborted")
}

// replaceForeign backs up and removes the foreign config, and says where the
// backup went.
func (a *app) replaceForeign() error {
	if err := a.Store.AdoptForeign(""); err != nil {
		return err
	}
	where := filepath.Join(a.Store.Backups.Dir, "_initial")
	if bs, err := a.Store.Backups.List("_initial"); err == nil && len(bs) > 0 {
		where = bs[0].Path
	}
	_, _ = fmt.Fprintf(a.Out, "Replaced %s; the previous file is backed up at %s\n", a.Store.ConfigPath(), where)
	return nil
}

// freeName returns base, or base-2, base-3... for the first unused project name.
func (a *app) freeName(base string) string {
	name := base
	for i := 2; ; i++ {
		if _, err := os.Lstat(a.Store.ProjectPath(name)); err != nil {
			return name
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
}
