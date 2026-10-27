package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nunoarmada/kxconfig/v2/internal/fsutil"
	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/v2/internal/safetext"
	"github.com/nunoarmada/kxconfig/v2/internal/shell"
	"github.com/nunoarmada/kxconfig/v2/internal/store"
)

type lsRow struct {
	Name     string `json:"name"`
	Active   bool   `json:"active"`
	Contexts int    `json:"contexts"`
	Status   string `json:"status"`
}

// status is invalid > perms <octal> > ok.
func status(p store.Project) string {
	switch {
	case p.Err != nil:
		return "invalid"
	case fsutil.TooOpen(p.Mode.Perm()):
		return fmt.Sprintf("perms %04o", p.Mode.Perm())
	}
	return "ok"
}

func (a *app) newLs() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List projects",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if output != "" && output != "json" {
				return fmt.Errorf("unknown output format %q (want json)", output)
			}
			ps, err := a.Store.Projects()
			if err != nil {
				return err
			}
			rows := make([]lsRow, 0, len(ps))
			for _, p := range ps {
				r := lsRow{Name: p.Name, Active: p.Active, Status: status(p)}
				if p.Err == nil {
					if cfg, err := kubecfg.Load(p.Path); err == nil {
						r.Contexts = len(cfg.Contexts)
					}
				}
				rows = append(rows, r)
			}
			if output == "json" {
				enc := json.NewEncoder(a.Out)
				enc.SetIndent("", "  ")
				return enc.Encode(rows)
			}
			tw := tabwriter.NewWriter(a.Out, 0, 4, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ACTIVE\tNAME\tCONTEXTS\tSTATUS")
			for _, r := range rows {
				mark, ctxs := "", "-"
				if r.Active {
					mark = "*"
				}
				if r.Status != "invalid" {
					ctxs = fmt.Sprint(r.Contexts)
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", mark, safetext.Clean(r.Name), ctxs, safetext.Clean(r.Status))
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVarP(&output, "output", "o", "", "output format: json")
	_ = cmd.RegisterFlagCompletionFunc("output", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func (a *app) newCurrent() *cobra.Command {
	return &cobra.Command{
		Use:   "current",
		Short: "Print the active project (and this shell's project inside kx shell)",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			name, _, err := a.Store.Active()
			if err != nil {
				return err
			}
			if name == "" && !a.Env.InShell {
				return errors.New("no active project")
			}
			if name != "" {
				_, _ = fmt.Fprintln(a.Out, safetext.Clean(name))
			}
			if a.Env.InShell {
				_, _ = fmt.Fprintf(a.Out, "shell: %s\n", safetext.Clean(a.Env.Project))
			}
			return nil
		},
	}
}

func (a *app) newShell() *cobra.Command {
	return &cobra.Command{
		Use:   "shell PROJECT",
		Short: "Open a subshell isolated to one project",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			_, _ = fmt.Fprintf(a.Err, "kx: entering shell for %s (exit to leave). Tip: show $KX_PROJECT in your prompt.\n", safetext.Clean(args[0]))
			return shell.Spawn(a.Store, args[0], a.Env)
		},
		ValidArgsFunction: a.projectArgs,
	}
}

func (a *app) newDoctor() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check permissions, the config link and project files",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			issues, err := a.Store.Doctor(fix)
			if err != nil {
				return err
			}
			if len(issues) == 0 {
				_, _ = fmt.Fprintln(a.Out, "No problems found.")
				return nil
			}
			remaining := 0
			for _, i := range issues {
				state := ""
				switch {
				case i.Fixed:
					state = " [fixed]"
				case i.Fixable && !fix:
					state = " [fixable with --fix]"
				}
				if !i.Fixed {
					remaining++
				}
				_, _ = fmt.Fprintf(a.Out, "%s: %s: %s%s\n", safetext.Clean(string(i.Kind)), safetext.Clean(i.Path), safetext.Clean(i.Message), state)
			}
			if remaining > 0 {
				return fmt.Errorf("%d problem(s) remain", remaining)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "repair what can be repaired safely (permissions, stale shell copies)")
	return cmd
}

func (a *app) newVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the kx version",
		Args:  cobra.NoArgs,
		Run:   func(*cobra.Command, []string) { _, _ = fmt.Fprintln(a.Out, a.Version) },
	}
}
