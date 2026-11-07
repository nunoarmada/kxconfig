package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/v2/internal/safetext"
	"github.com/nunoarmada/kxconfig/v2/internal/store"
)

// editContextDialog offers what can be edited on the context under the
// cursor: its name, or the name of the cluster it uses.
type editContextDialog struct {
	target  store.Target
	ctx     string
	cluster string // "" when the context names no existing cluster
}

// startEditContext opens the edit menu for the context under the cursor
// (e on a context).
func (m *Model) startEditContext() tea.Cmd {
	d, cmd := m.newEditContextDialog()
	if d != nil {
		m.push(d)
	}
	return cmd
}

// startRenameContext goes straight to renaming the context under the cursor
// (r on a context).
func (m *Model) startRenameContext() tea.Cmd {
	d, cmd := m.newEditContextDialog()
	if d != nil {
		m.push(d.renameContext(m))
	}
	return cmd
}

// newEditContextDialog prepares the edit menu for the context under the
// cursor. With no dialog, the command reports why (or is nil).
func (m *Model) newEditContextDialog() (*editContextDialog, tea.Cmd) {
	p, ok := m.selectedProject()
	if !ok {
		return nil, nil
	}
	c, ok := m.selectedContext()
	if !ok {
		return nil, nil
	}
	t, fail := m.contextTarget(p, "edit context")
	if fail != nil {
		return nil, fail
	}
	cfg, err := kubecfg.Load(t.Path)
	if err != nil {
		return nil, m.failed("read "+p.Name, err)
	}
	d := &editContextDialog{target: t, ctx: c.Name}
	if cfg.Clusters[c.Cluster] != nil {
		d.cluster = c.Cluster
	}
	return d, nil
}

func (d *editContextDialog) update(m *Model, msg tea.Msg) (dialog, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch k.String() {
	case "esc":
		return nil, nil
	case "r", "n":
		return d.renameContext(m), nil
	case "c":
		if d.cluster == "" {
			return d, nil
		}
		return d.renameCluster(m), nil
	}
	return d, nil
}

func (d *editContextDialog) view(m *Model, maxW, _ int) string {
	w := min(maxW, 64)
	if m.w < 50 {
		w = maxW
	}
	iw := dialogInner(w)
	items := [][2]string{{"r", "Rename context " + safetext.Clean(d.ctx)}}
	if d.cluster != "" {
		items = append(items, [2]string{"c", "Rename cluster " + safetext.Clean(d.cluster) + " (all contexts that use it follow)"})
	}
	items = append(items, [2]string{"esc", "Cancel"})
	var body []string
	for _, it := range items {
		for n, l := range wordWrap(it[1], max(8, iw-5)) {
			if n == 0 {
				body = append(body, m.sty.accent.Render(it[0])+strings.Repeat(" ", 5-lipgloss.Width(it[0]))+l)
			} else {
				body = append(body, "     "+l)
			}
		}
	}
	if d.cluster == "" {
		body = append(body, "", m.sty.dim.Render(m.truncRow("This context names no existing cluster.", iw)))
	}
	if d.target.InShell {
		body = append(body, "", m.sty.dim.Render(m.truncRow("Edits this kx shell's copy only.", iw)))
	}
	return m.dialogBox("Edit context", body, w, false, 1)
}

func (d *editContextDialog) hints() []hint {
	if d.cluster == "" {
		return []hint{{"r", "rename context"}, {"esc", "cancel"}}
	}
	return []hint{{"r", "rename context"}, {"c", "rename cluster"}, {"esc", "cancel"}}
}

// renameContext is the input dialog for a new context name.
func (d *editContextDialog) renameContext(m *Model) dialog {
	t, old := d.target, d.ctx
	taken := map[string]bool{}
	for _, c := range m.cur.ctxs {
		taken[c.Name] = true
	}
	in := newInputDialog(m, "Rename context", "New name for context "+safetext.Clean(old), old,
		func(_ *Model, v string) string { return entityNameMsg(v, old, "Context", taken) },
		func(m *Model, name string) tea.Cmd {
			if name == old {
				return nil
			}
			if err := m.store.RenameContext(t, old, name); err != nil {
				return m.failed("rename context "+old, err)
			}
			if cmd := m.refresh(); cmd != nil {
				return cmd
			}
			m.selectContext(name)
			m.settle()
			return m.say(statusSuccess, "Renamed context "+old+" "+m.gl.arrow+" "+name+m.shellSuffix(t))
		})
	in.note = d.note(m)
	return in
}

// renameCluster is the input dialog for a new cluster name.
func (d *editContextDialog) renameCluster(m *Model) dialog {
	t, old := d.target, d.cluster
	cfg, err := kubecfg.Load(t.Path)
	taken := map[string]bool{}
	users := 0
	if err == nil {
		for n := range cfg.Clusters {
			taken[n] = true
		}
		for _, c := range cfg.Contexts {
			if c != nil && c.Cluster == old {
				users++
			}
		}
	}
	in := newInputDialog(m, "Rename cluster", "New name for cluster "+safetext.Clean(old), old,
		func(_ *Model, v string) string { return entityNameMsg(v, old, "Cluster", taken) },
		func(m *Model, name string) tea.Cmd {
			if name == old {
				return nil
			}
			moved, err := m.store.RenameCluster(t, old, name)
			if err != nil {
				return m.failed("rename cluster "+old, err)
			}
			if cmd := m.refresh(); cmd != nil {
				return cmd
			}
			return m.say(statusSuccess, "Renamed cluster "+old+" "+m.gl.arrow+" "+name+
				" ("+plural(len(moved), "context")+" updated)"+m.shellSuffix(t))
		})
	warn := d.note(m)
	in.note = func(m *Model, v string) (string, bool) {
		if text, w := warn(m, v); text != "" {
			return text, w
		}
		if users > 1 {
			return plural(users, "context") + " use this cluster; all are updated.", false
		}
		return "", false
	}
	return in
}

// note warns about live kx shells that keep their own copy of the project.
func (d *editContextDialog) note(m *Model) func(*Model, string) (string, bool) {
	w := ""
	if !d.target.InShell {
		w = m.shellWarning(d.target.Project)
	}
	return func(*Model, string) (string, bool) { return w, w != "" }
}

// shellSuffix marks a status message for an edit of a kx shell's copy.
func (m *Model) shellSuffix(t store.Target) string {
	if t.InShell {
		return " in this kx shell"
	}
	return ""
}

// entityNameMsg checks a new context or cluster name: "" when it is fine.
func entityNameMsg(v, old, kind string, taken map[string]bool) string {
	if msg := kubecfg.ValidEntityName(v); msg != "" {
		return "Invalid: " + msg
	}
	if v != old && taken[v] {
		return kind + " " + safetext.Clean(v) + " already exists"
	}
	return ""
}
