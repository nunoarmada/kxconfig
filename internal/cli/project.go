package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nunoarmada/kxconfig/v2/internal/backup"
	"github.com/nunoarmada/kxconfig/v2/internal/safetext"
	"github.com/nunoarmada/kxconfig/v2/internal/store"
)

func (a *app) newRename() *cobra.Command {
	return &cobra.Command{
		Use:   "rename OLD NEW",
		Short: "Rename a project",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			a.warnLiveShells(args[0])
			if err := a.Store.Rename(args[0], args[1]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(a.Out, "Renamed project %s to %s\n", safetext.Clean(args[0]), safetext.Clean(args[1]))
			return nil
		},
		ValidArgsFunction: a.projectArgs,
	}
}

func (a *app) newDelete() *cobra.Command {
	var force, yes bool
	cmd := &cobra.Command{
		Use:   "delete PROJECT",
		Short: "Delete a project (a backup is kept)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			if err := a.requireProject(name); err != nil {
				return err
			}
			active, st, aerr := a.Store.Active()
			wasActive := aerr == nil && st == store.LinkOK && active == name
			if wasActive && !force {
				return fmt.Errorf("project %s is active; use --force to delete it and deactivate", safetext.Clean(name))
			}
			if !yes {
				if err := a.confirm(fmt.Sprintf("Delete project %s?", name)); err != nil {
					return err
				}
			}
			a.warnLiveShells(name)
			if err := a.Store.Delete(name, force); err != nil {
				if errors.Is(err, store.ErrActive) {
					return fmt.Errorf("project %s is active; use --force to delete it and deactivate", safetext.Clean(name))
				}
				return err
			}
			_, _ = fmt.Fprintf(a.Out, "Deleted project %s (backup kept; see 'kx backups %s')\n", safetext.Clean(name), safetext.Clean(name))
			if wasActive {
				_, _ = fmt.Fprintf(a.Out, "The config link was removed, so no project is active; run 'kx use <project>'.\n")
			}
			return nil
		},
		ValidArgsFunction: a.projectArgs,
	}
	cmd.Flags().BoolVar(&force, "force", false, "delete even if the project is active")
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	return cmd
}

func (a *app) newBackups() *cobra.Command {
	var initial bool
	cmd := &cobra.Command{
		Use:   "backups PROJECT | --initial",
		Short: "List a project's backups, newest first (--initial: of the original ~/.kube/config)",
		Args: func(cmd *cobra.Command, args []string) error {
			if initial {
				return cobra.NoArgs(cmd, args)
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(_ *cobra.Command, args []string) error {
			name := backup.InitialProject
			if !initial {
				name = args[0]
				if err := store.ValidateName(name); err != nil {
					return err
				}
			}
			bs, err := a.Store.Backups.List(name)
			if err != nil {
				return err
			}
			if len(bs) == 0 {
				_, _ = fmt.Fprintf(a.Out, "No backups for project %s\n", safetext.Clean(name))
				return nil
			}
			tw := tabwriter.NewWriter(a.Out, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ID\tTIME\tOP")
			for _, b := range bs {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", safetext.Clean(b.ID), b.Time.Local().Format(time.DateTime), safetext.Clean(b.Op))
			}
			return tw.Flush()
		},
		ValidArgsFunction: a.projectArgs,
	}
	cmd.Flags().BoolVar(&initial, "initial", false, "list the backups of the original ~/.kube/config files kx adopted")
	return cmd
}

// requireProject fails with ErrNotFound unless the project's file exists.
func (a *app) requireProject(name string) error {
	if err := store.ValidateName(name); err != nil {
		return err
	}
	if _, err := os.Stat(a.Store.ProjectPath(name)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s", store.ErrNotFound, safetext.Clean(name))
		}
		return err
	}
	return nil
}

func (a *app) newRestore() *cobra.Command {
	var yes, initial bool
	var as string
	cmd := &cobra.Command{
		Use:   "restore PROJECT [ID] | --initial [ID] --as NAME",
		Short: "Restore a project from a backup (default: the latest)",
		Long: `Restore a project from one of its backups; the current file is backed up first.

With --initial, the original ~/.kube/config that kx backed up when it first
replaced it (see 'kx backups --initial') is written to a NEW project given by
--as. It never overwrites an existing project or ~/.kube/config.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if initial {
				return cobra.MaximumNArgs(1)(cmd, args)
			}
			return cobra.RangeArgs(1, 2)(cmd, args)
		},
		RunE: func(_ *cobra.Command, args []string) error {
			if initial {
				return a.restoreInitial(args, as)
			}
			name := args[0]
			if err := store.ValidateName(name); err != nil {
				return err
			}
			var id string
			if len(args) == 2 {
				id = args[1]
			} else {
				bs, err := a.Store.Backups.List(name)
				if err != nil {
					return err
				}
				if len(bs) == 0 {
					return fmt.Errorf("no backups for project %s", safetext.Clean(name))
				}
				id = bs[0].ID
			}
			if !yes {
				if err := a.confirm(fmt.Sprintf("Restore project %s from backup %s (the current file is backed up first)?", name, id)); err != nil {
					return err
				}
			}
			a.warnLiveShells(name)
			if err := a.Store.Restore(name, id); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(a.Out, "Restored project %s from backup %s\n", safetext.Clean(name), safetext.Clean(id))
			return nil
		},
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return a.completeProjects(toComplete), cobra.ShellCompDirectiveNoFileComp
			case 1:
				if store.ValidateName(args[0]) != nil {
					break
				}
				bs, _ := a.Store.Backups.List(args[0])
				ids := make([]string, 0, len(bs))
				for _, b := range bs {
					ids = append(ids, b.ID)
				}
				return ids, cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation")
	cmd.Flags().BoolVar(&initial, "initial", false, "restore a backup of the original ~/.kube/config as a new project (needs --as)")
	cmd.Flags().StringVar(&as, "as", "", "with --initial: the new project `NAME`")
	return cmd
}

// restoreInitial writes an "_initial" backup (the newest unless args has an
// ID) to the new project as.
func (a *app) restoreInitial(args []string, as string) error {
	if as == "" {
		return errors.New("restore --initial needs --as NAME: the new project to write the original config to")
	}
	id := ""
	if len(args) == 1 {
		id = args[0]
	}
	if err := a.Store.RestoreInitial(id, as); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.Out, "Restored the original config as project %s (activate it with 'kx use %s')\n", safetext.Clean(as), safetext.Clean(as))
	return nil
}
