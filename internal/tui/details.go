package tui

import (
	"fmt"

	"github.com/nunoarmada/kxconfig/internal/safetext"
)

// detailsPane draws the facts of the selected context. It shows server, user
// name and auth kind only: never secret values.
func (m Model) detailsPane(r rect, focused, single bool) string {
	iw := r.w - 4
	title := m.paneTitle("DETAILS", "", m.paneNote(paneDetails, single), iw, focused)
	lines := m.detailLines(iw)
	rows := max(0, r.h-3)
	off := max(0, min(m.dscroll, len(lines)-rows))
	if off < len(lines) {
		lines = lines[off:]
	}
	return m.box(r.w, r.h, focused, title, lines)
}

// detailLines returns the unscrolled detail rows for the selected context.
func (m Model) detailLines(iw int) []string {
	dim := m.sty.dim
	if m.cur.name == "" {
		return []string{dim.Render("-")}
	}
	if m.cur.err != nil {
		return []string{
			m.sty.warn.Render(m.gl.warn + " Cannot parse config-" + safetext.Clean(m.cur.name) + ": " + firstLine(m.cur.err.Error())),
			"",
			"Press e to edit it.",
		}
	}
	c, ok := m.selectedContext()
	if !ok {
		return []string{dim.Render("-")}
	}
	kv := func(k, v string) string {
		return dim.Render(fmt.Sprintf("%-9s ", k)) + v
	}
	return []string{
		kv("context", safetext.Clean(c.Name)),
		kv("cluster", dash(safetext.Clean(c.Cluster))),
		kv("server", dash(safetext.Clean(c.Server))),
		kv("user", dash(safetext.Clean(c.User))),
		kv("auth", dash(safetext.Clean(c.AuthKind))),
		kv("file", safetext.Clean(m.tilde(m.cur.path))),
	}
}
