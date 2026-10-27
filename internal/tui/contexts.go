package tui

import (
	"fmt"

	"github.com/nunoarmada/kxconfig/v2/internal/kubecfg"
	"github.com/nunoarmada/kxconfig/v2/internal/safetext"
)

// contextsPane draws the contexts of the selected project.
func (m Model) contextsPane(r rect, focused, single bool) string {
	iw := r.w - 4
	vis := m.visibleContexts()
	count := fmt.Sprint(len(m.cur.ctxs))
	if m.clist.filter != "" {
		count = fmt.Sprintf("%d/%d", len(vis), len(m.cur.ctxs))
	}
	meta := count
	if m.cur.name != "" {
		meta = safetext.Clean(m.cur.name) + " " + count
	}
	title := m.paneTitle("CONTEXTS", meta, m.paneNote(paneContexts, single), iw, focused)

	if len(m.cur.ctxs) == 0 {
		note := "No contexts."
		if m.cur.name != "" {
			note = "No contexts in " + safetext.Clean(m.cur.name) + "."
		}
		return m.box(r.w, r.h, focused, title, []string{m.sty.dim.Render(note)})
	}

	drop := r.w < 50
	cols := m.contextCols(iw-3, drop)
	rows := m.layout().contextRows(m.showsFilter(paneContexts))
	lines := []string{m.contextHeader(cols, drop)}
	if len(vis) == 0 {
		lines = append(lines, m.sty.dim.Render("No contexts match."))
	}
	for i := m.clist.offset; i < len(vis) && i < m.clist.offset+rows; i++ {
		lines = append(lines, m.contextRow(m.cur.ctxs[vis[i]], cols, drop, i == m.clist.cursor, focused, iw))
	}
	return m.box(r.w, r.h, focused, title, m.withFilter(lines, rows+1, paneContexts, iw, focused))
}

// contextRow is: cursor, current marker, name, cluster and namespace columns.
func (m Model) contextRow(c kubecfg.ContextInfo, cols ctxCols, drop, selected, focused bool, iw int) string {
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
	if c.Current {
		marker = cell{s: m.gl.current, st: m.sty.accent}
		nameSt = m.sty.bold
	}
	ns := cell{s: dash(safetext.Clean(c.Namespace))}
	if c.Namespace == "" {
		ns.st = m.sty.dim
	}
	cells := []cell{cursor, marker, {s: " "}, {s: m.fit(safetext.Clean(c.Name), cols.name), st: nameSt}}
	if !drop {
		cells = append(cells, cell{s: "  "}, cell{s: m.fit(dash(safetext.Clean(c.Cluster)), cols.cluster)})
	}
	cells = append(cells, cell{s: "  "}, ns)
	return m.row(iw, mode, cells...)
}
