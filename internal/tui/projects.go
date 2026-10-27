package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nunoarmada/kxconfig/internal/store"
)

// projectsPane draws the projects list. single marks the one-pane layout,
// which adds the "n/3 tab" pane indicator to the title.
func (m Model) projectsPane(r rect, focused, single bool) string {
	iw := r.w - 4
	vis := m.visibleProjects()
	count := fmt.Sprint(len(m.projects))
	if m.plist.filter != "" {
		count = fmt.Sprintf("%d/%d", len(vis), len(m.projects))
	}
	title := m.paneTitle("PROJECTS", count, m.paneNote(paneProjects, single), iw, focused)

	rows := m.layout().projectRows(m.showsFilter(paneProjects))
	var lines []string
	if len(vis) == 0 {
		lines = append(lines, m.sty.dim.Render("No projects match."))
	}
	for i := m.plist.offset; i < len(vis) && i < m.plist.offset+rows; i++ {
		lines = append(lines, m.projectRow(m.projects[vis[i]], i == m.plist.cursor, focused, iw))
	}
	return m.box(r.w, r.h, focused, title, m.withFilter(lines, rows, paneProjects, iw, focused))
}

// paneNote is the right side of a pane title in the single-pane layout.
func (m Model) paneNote(p pane, single bool) string {
	if !single {
		return ""
	}
	return fmt.Sprintf("%d/%d tab %s", int(p)+1, int(paneCount), m.gl.cursor)
}

// withFilter pads lines to rows and appends the filter row when there is one.
func (m Model) withFilter(lines []string, rows int, p pane, iw int, focused bool) []string {
	if !m.showsFilter(p) {
		return lines
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	prefix := m.sty.accent.Render("/") + " "
	if m.filtering && focused {
		m.finput.SetWidth(max(1, iw-4))
		return append(lines, prefix+m.finput.View())
	}
	return append(lines, prefix+m.activeFilterOf(p))
}

func (m Model) activeFilterOf(p pane) string {
	if p == paneProjects {
		return m.plist.filter
	}
	return m.clist.filter
}

// projectRow is: cursor, active marker, name, then a right-aligned badge.
func (m Model) projectRow(p store.Project, selected, focused bool, iw int) string {
	cursor := cell{s: " "}
	mode := rowPlain
	switch {
	case selected && focused:
		cursor.s, mode = m.gl.cursor, rowSelected
	case selected:
		cursor = cell{s: m.gl.cursor, st: m.sty.dim}
	}
	marker := cell{s: " "}
	nameSt := m.sty.plain
	if p.Active {
		marker = cell{s: m.gl.active, st: m.sty.active}
		nameSt = m.sty.bold
	}

	avail := max(0, iw-3)
	nameW := lipgloss.Width(p.Name)
	badge, bst := m.projectBadge(p, nameW, avail)
	name := cell{s: p.Name, st: nameSt}
	cells := []cell{cursor, marker, {s: " "}, name}
	if badge != "" {
		bw := lipgloss.Width(badge)
		name.s = m.fit(p.Name, avail-1-bw)
		cells = []cell{cursor, marker, {s: " "}, name, {s: " "}, {s: badge, st: bst}}
	}
	return m.row(iw, mode, cells...)
}

// projectBadge picks the first badge that fits next to the name, in the order
// invalid > loose permissions > live shells.
func (m Model) projectBadge(p store.Project, nameW, avail int) (string, lipgloss.Style) {
	type badge struct {
		text string
		st   lipgloss.Style
	}
	var cands []badge
	if p.Err != nil {
		cands = append(cands, badge{m.gl.warn + " invalid", m.sty.warn})
	}
	if p.Mode&0o077 != 0 {
		cands = append(cands, badge{fmt.Sprintf("%s %04o", m.gl.warn, uint32(p.Mode.Perm())), m.sty.warn})
	}
	if n := m.shells[p.Name]; n > 0 {
		cands = append(cands, badge{fmt.Sprintf("$%d", n), m.sty.dim})
	}
	for _, c := range cands {
		if min(nameW, 6)+1+lipgloss.Width(c.text) <= avail {
			return c.text, c.st
		}
	}
	return "", m.sty.plain
}

// ctxCols are the column widths of the contexts table.
type ctxCols struct{ name, cluster, ns int }

// contextCols fits the table into avail columns. Below 50 columns of pane
// width the cluster column is dropped.
func (m Model) contextCols(avail int, dropCluster bool) ctxCols {
	c := ctxCols{name: lipgloss.Width("NAME"), cluster: lipgloss.Width("CLUSTER"), ns: lipgloss.Width("NAMESPACE")}
	for _, i := range m.visibleContexts() {
		x := m.cur.ctxs[i]
		c.name = max(c.name, lipgloss.Width(x.Name))
		c.cluster = max(c.cluster, lipgloss.Width(x.Cluster))
		c.ns = max(c.ns, lipgloss.Width(dash(x.Namespace)))
	}
	gaps := 4
	if dropCluster {
		c.cluster, gaps = 0, 2
	}
	// Shrink the widest column until the table fits, keeping a floor of 4.
	for c.name+c.cluster+c.ns+gaps > avail {
		w := []*int{&c.name, &c.cluster, &c.ns}
		widest := w[0]
		for _, p := range w[1:] {
			if *p > *widest {
				widest = p
			}
		}
		if *widest <= 4 {
			break
		}
		*widest--
	}
	return c
}

func (m Model) contextHeader(cols ctxCols, drop bool) string {
	h := "   " + m.fit("NAME", cols.name)
	if !drop {
		h += "  " + m.fit("CLUSTER", cols.cluster)
	}
	h += "  " + "NAMESPACE"
	return m.sty.dim.Render(strings.TrimRight(h, " "))
}
