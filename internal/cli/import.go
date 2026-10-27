package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/v2/internal/safetext"
	"github.com/nunoarmada/kxconfig/v2/internal/store"
)

func (a *app) newImport() *cobra.Command {
	var project, onConflict string
	var replace, useAfter, yes bool
	cmd := &cobra.Command{
		Use:   "import FILE",
		Short: "Merge (or replace with) a kubeconfig file into a project",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.importCmd(args[0], project, onConflict, replace, useAfter, yes)
		},
	}
	a.projectFlag(cmd, &project, "target project `NAME` (created if missing)")
	cmd.Flags().BoolVar(&replace, "replace", false, "replace the project with the file instead of merging")
	cmd.Flags().StringVar(&onConflict, "on-conflict", "fail", "what to do with same-name entries that differ: fail, rename, overwrite or skip")
	cmd.Flags().BoolVar(&yes, "yes", false, "do not ask for confirmation when replacing, or when overwriting re-points existing contexts")
	cmd.Flags().BoolVar(&useAfter, "use", false, "activate the project afterwards")
	_ = cmd.RegisterFlagCompletionFunc("on-conflict", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"fail", "rename", "overwrite", "skip"}, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func (a *app) importCmd(file, project, onConflict string, replace, useAfter, yes bool) error {
	policy, err := kubecfg.ParsePolicy(onConflict)
	if err != nil {
		return err
	}
	if useAfter && a.Env.InShell {
		return errors.New("refused inside a kx shell: --use would change the global project; exit the shell first")
	}
	if project == "" {
		if project, err = a.askProjectName(file); err != nil {
			return err
		}
	}
	mode := store.ImportMerge
	if replace {
		mode = store.ImportReplace
		// Replacing an existing project discards its current content.
		if _, serr := os.Stat(a.Store.ProjectPath(project)); serr == nil && !yes {
			if err := a.confirm(fmt.Sprintf("Replace project %s with %s (a backup is kept)?", project, file)); err != nil {
				return err
			}
		}
	}
	opts := kubecfg.MergeOptions{Default: policy}
	if mode == store.ImportMerge && policy == kubecfg.PolicyOverwrite {
		if err := a.confirmRebound(file, project, opts, yes); err != nil {
			return err
		}
	}
	res, err := a.Store.Import(file, project, mode, opts)
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintf(a.Err, "warning: %s\n", safetext.Clean(w))
	}
	if errors.Is(err, kubecfg.ErrConflicts) {
		a.printConflicts(res.Report.Conflicts)
		return fmt.Errorf("%s in project %s; nothing changed (choose with --on-conflict rename|overwrite|skip)", plural(len(res.Report.Conflicts), "conflict"), safetext.Clean(project))
	}
	if err != nil {
		return err
	}
	a.printImport(res)
	if useAfter {
		return a.useCmd(project, "", false)
	}
	return nil
}

// confirmRebound previews the merge and, when overwriting would re-point
// existing contexts at the incoming cluster/user, lists them and asks for
// confirmation (--yes without a terminal). Nothing is written before that.
func (a *app) confirmRebound(file, project string, opts kubecfg.MergeOptions, yes bool) error {
	rep, err := a.Store.PreviewMerge(file, project, opts)
	if err != nil || len(rep.Rebound) == 0 {
		return nil // Import reports real errors itself
	}
	var names []string
	for _, r := range rep.Rebound {
		names = append(names, safetext.Clean(r.Name))
	}
	_, _ = fmt.Fprintf(a.Err, "warning: these existing contexts now use the incoming cluster/user: %s\n", strings.Join(names, ", "))
	if yes {
		return nil
	}
	return a.confirm(fmt.Sprintf("Overwrite and re-point %s in project %s (a backup is kept)?", plural(len(names), "context"), project))
}

// askProjectName prompts for a project name on a terminal; the default is the
// file's base name without extension.
func (a *app) askProjectName(file string) (string, error) {
	if !a.IsTTY {
		return "", errors.New("import needs -p NAME when stdin is not a terminal")
	}
	base := filepath.Base(file)
	def := strings.TrimSuffix(base, filepath.Ext(base))
	_, _ = fmt.Fprintf(a.Err, "Project name [%s]: ", safetext.Clean(def))
	ans, err := a.readLine()
	if err != nil {
		return "", err
	}
	if ans == "" {
		return def, nil
	}
	return ans, nil
}

func (a *app) printConflicts(cs []kubecfg.Conflict) {
	tw := tabwriter.NewWriter(a.Err, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KIND\tNAME\tEXISTING\tINCOMING")
	for _, c := range cs {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", safetext.Clean(string(c.Ref.Kind)), safetext.Clean(c.Ref.Name), safetext.Clean(c.Existing), safetext.Clean(c.Incoming))
	}
	_ = tw.Flush()
}

func (a *app) printImport(res store.ImportResult) {
	r := res.Report
	state := "unchanged"
	switch {
	case res.Created:
		state = "created"
	case res.Changed:
		state = "updated"
	}
	_, _ = fmt.Fprintf(a.Out, "Project %s %s: added %d, updated %d, renamed %d, skipped %d, dropped %d\n",
		safetext.Clean(res.Project), state, len(r.Added), len(r.Overwritten), len(r.Renamed), len(r.Skipped), len(r.Dropped))
	for _, d := range r.Dropped {
		_, _ = fmt.Fprintf(a.Out, "  dropped %s %s: references a skipped or missing cluster/user\n", safetext.Clean(string(d.Kind)), safetext.Clean(d.Name))
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", word)
	}
	return fmt.Sprintf("%d %ss", n, word)
}
