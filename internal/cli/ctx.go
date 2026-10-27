package cli

import (
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/v2/internal/safetext"
)

func (a *app) newCtx() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "ctx [NAME]",
		Short: "List contexts, or switch the current one",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			t, err := a.Store.Target(project, a.Env)
			if err != nil {
				return err
			}
			a.warnForeignKubeconfig(t.Path)
			if len(args) == 1 {
				if err := a.Store.SetContext(t, args[0]); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(a.Out, "Switched to context %s\n", safetext.Clean(args[0]))
				return nil
			}
			cfg, err := kubecfg.Load(t.Path)
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(a.Out, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "CURRENT\tNAME\tCLUSTER\tUSER\tNAMESPACE")
			for _, c := range kubecfg.Contexts(cfg) {
				mark := ""
				if c.Current {
					mark = "*"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", mark, safetext.Clean(c.Name), safetext.Clean(c.Cluster), safetext.Clean(c.User), safetext.Clean(c.Namespace))
			}
			return tw.Flush()
		},
	}
	a.projectFlag(cmd, &project, "edit project `NAME`'s file instead of the current one")
	cmd.ValidArgsFunction = func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return a.completeContexts(project, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func (a *app) newNs() *cobra.Command {
	var project string
	cmd := &cobra.Command{
		Use:   "ns NAMESPACE",
		Short: "Set the namespace of the current context (\"\" clears it)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if args[0] != "" && !kubecfg.ValidNamespace(args[0]) {
				return fmt.Errorf("invalid namespace %q: use lowercase letters, digits and '-' (max 63), starting and ending with a letter or digit (\"\" clears it)", safetext.Clean(args[0]))
			}
			t, err := a.Store.Target(project, a.Env)
			if err != nil {
				return err
			}
			a.warnForeignKubeconfig(t.Path)
			if err := a.Store.SetNamespace(t, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(a.Out, "Namespace set to %s\n", safetext.Clean(args[0]))
			return nil
		},
	}
	a.projectFlag(cmd, &project, "edit project `NAME`'s file instead of the current one")
	return cmd
}

func (a *app) newRmCtx() *cobra.Command {
	var project string
	var keepOrphans, yes bool
	cmd := &cobra.Command{
		Use:   "rm-ctx CONTEXT",
		Short: "Delete a context (and its orphaned cluster and user)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.rmCtxCmd(args[0], project, keepOrphans, yes)
		},
	}
	a.projectFlag(cmd, &project, "project `NAME` (default: the active project, or $KX_PROJECT in a kx shell)")
	cmd.Flags().BoolVar(&keepOrphans, "keep-orphans", false, "keep the cluster and user that only this context used")
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	cmd.ValidArgsFunction = func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return a.completeContexts(project, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
	return cmd
}

func (a *app) rmCtxCmd(ctx, project string, keepOrphans, yes bool) error {
	note := false
	if project == "" && a.Env.InShell {
		if a.Env.Project == "" {
			return errors.New("inside a kx shell but KX_PROJECT is not set; pass -p")
		}
		project, note = a.Env.Project, true
	}
	t, err := a.Store.Target(project, a.Env)
	if err != nil {
		return err
	}
	cfg, err := kubecfg.Load(t.Path)
	if err != nil {
		return err
	}
	if cfg.Contexts[ctx] == nil {
		return fmt.Errorf("context %q not found in project %s", safetext.Clean(ctx), safetext.Clean(t.Project))
	}
	_, _ = fmt.Fprintf(a.Out, "Will remove context %s from project %s\n", safetext.Clean(ctx), safetext.Clean(t.Project))
	if !keepOrphans {
		cl, us := kubecfg.Orphans(cfg, ctx)
		for _, c := range cl {
			_, _ = fmt.Fprintf(a.Out, "  and orphaned cluster %s\n", safetext.Clean(c))
		}
		for _, u := range us {
			_, _ = fmt.Fprintf(a.Out, "  and orphaned user %s\n", safetext.Clean(u))
		}
	}
	if !yes {
		if err := a.confirm("Remove it?"); err != nil {
			return err
		}
	}
	if _, err := a.Store.RemoveContext(t.Project, ctx, keepOrphans); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.Out, "Removed context %s\n", safetext.Clean(ctx))
	if note {
		_, _ = fmt.Fprintln(a.Err, "note: this kx shell uses a snapshot; it was not updated")
	}
	return nil
}

// completeContexts completes context names of the file ctx/ns would edit.
func (a *app) completeContexts(project, toComplete string) []string {
	t, err := a.Store.Target(project, a.Env)
	if err != nil {
		return nil
	}
	cfg, err := kubecfg.Load(t.Path)
	if err != nil {
		return nil
	}
	var out []string
	for _, c := range kubecfg.Contexts(cfg) {
		if strings.HasPrefix(c.Name, toComplete) {
			out = append(out, c.Name)
		}
	}
	return out
}
