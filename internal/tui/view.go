package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nunoarmada/kxconfig/internal/store"
)

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// render draws the whole frame as exactly m.h lines of m.w columns (or the
// one-line notice when the terminal is too small).
func (m Model) render() string {
	lay := m.layout()
	if lay.mode == layoutTiny {
		msg := fmt.Sprintf("Terminal too small (%dx%d) %s need %d%s%d", m.w, m.h, m.gl.dash, minW, m.gl.times, minH)
		return ansi.Truncate(msg, max(m.w, 0), "")
	}
	lines := []string{m.header(lay)}
	if lay.banner {
		lines = append(lines, m.bannerRow(lay))
	}
	body := m.body(lay)
	if n := len(m.dialogs); n > 0 {
		body = m.overlay(body, m.dialogs[n-1], lay)
	}
	lines = append(lines, body, m.statusRow(), m.hintBar(m.w))
	return m.block(strings.Join(lines, "\n"), m.w, m.h)
}

// body draws the area between the banner and the status line.
func (m Model) body(lay layoutInfo) string {
	switch {
	case m.loadErr != nil:
		return m.centered(lay, []string{
			m.sty.err.Render(m.gl.fail+" ") + m.sty.title.Render("Could not read projects"),
			"",
			firstLine(m.loadErr.Error()),
			"",
			m.sty.accent.Render("ctrl+r") + m.sty.dim.Render(" try again   ") + m.sty.accent.Render("q") + m.sty.dim.Render(" quit"),
		})
	case len(m.projects) == 0:
		return m.emptyState(lay)
	}
	switch lay.mode {
	case layoutWide:
		return joinCols(lay.bodyH, lay.gutter,
			m.projectsPane(lay.proj, m.focus == paneProjects, false),
			m.contextsPane(lay.ctx, m.focus == paneContexts, false),
			m.detailsPane(lay.det, m.focus == paneDetails, false))
	case layoutMedium:
		right := m.contextsPane(lay.ctx, m.focus == paneContexts, false)
		if !lay.hideDetails {
			right += "\n" + m.detailsPane(lay.det, m.focus == paneDetails, false)
		}
		return joinCols(lay.bodyH, lay.gutter,
			m.projectsPane(lay.proj, m.focus == paneProjects, false), right)
	}
	switch m.focus {
	case paneContexts:
		return m.contextsPane(lay.ctx, true, true)
	case paneDetails:
		return m.detailsPane(lay.det, true, true)
	}
	return m.projectsPane(lay.proj, true, true)
}

// joinCols places multi-line blocks side by side separated by gutter columns.
func joinCols(h, gutter int, cols ...string) string {
	split := make([][]string, len(cols))
	for i, c := range cols {
		split[i] = strings.Split(c, "\n")
	}
	sep := strings.Repeat(" ", gutter)
	out := make([]string, h)
	for y := range out {
		parts := make([]string, len(cols))
		for i := range cols {
			if y < len(split[i]) {
				parts[i] = split[i][y]
			}
		}
		out[y] = strings.Join(parts, sep)
	}
	return strings.Join(out, "\n")
}

// centered places a block of lines in the middle of the body.
func (m Model) centered(lay layoutInfo, lines []string) string {
	bw := 0
	for _, l := range lines {
		bw = max(bw, lipgloss.Width(l))
	}
	bw = min(bw, m.w-2)
	x := max(0, (m.w-bw)/2)
	y := max(0, (lay.bodyH-len(lines))/2)
	out := make([]string, 0, lay.bodyH)
	for i := 0; i < y; i++ {
		out = append(out, "")
	}
	for _, l := range lines {
		out = append(out, strings.Repeat(" ", x)+ansi.Truncate(l, bw, m.gl.ell))
	}
	return m.block(strings.Join(out, "\n"), m.w, lay.bodyH)
}

// emptyState is shown when there are no projects at all.
func (m Model) emptyState(lay layoutInfo) string {
	key := func(k string) string { return "  " + m.sty.accent.Render(k) + "   " }
	lines := []string{
		m.sty.title.Render("No projects yet"),
		"",
		"kx keeps one kubeconfig per project in " + m.tilde(m.store.ProjectPath("<name>")) + ".",
		"",
		key("i") + "import a kubeconfig file",
	}
	if m.link == store.LinkForeignFile {
		lines = append(lines, key("a")+"adopt the current "+m.tilde(m.store.ConfigPath())+" as a project")
	}
	lines = append(lines, key("q")+"quit")
	return m.centered(lay, lines)
}

// header is the top row: wordmark and active project, or a breadcrumb in the
// single-pane layout, with "? help" on the right.
func (m Model) header(lay layoutInfo) string {
	left := " " + m.sty.accent.Render("kx") + "  "
	switch {
	case m.env.InShell:
		global := m.sty.dim.Render("none")
		if m.active != "" {
			global = m.active
		}
		left += m.sty.warn.Bold(true).Render(m.gl.shellMark+" shell: "+m.env.Project) +
			m.sty.dim.Render("  "+m.gl.dot+"  global: ") + global
	case lay.mode == layoutNarrow:
		left = " " + m.sty.accent.Render("kx") + " " + m.sty.dim.Render(m.gl.crumb) + " "
		if m.focus == paneProjects || m.cur.name == "" {
			left += m.sty.title.Render(paneProjects.String())
		} else {
			left += m.cur.name + " " + m.sty.dim.Render(m.gl.crumb) + " " + m.sty.title.Render(m.focus.String())
		}
	case m.active != "":
		left += m.sty.dim.Render("active: ") + m.sty.active.Render(m.gl.active) + " " + m.sty.active.Bold(true).Render(m.active)
	default:
		left += m.sty.dim.Render("active: none")
	}
	right := m.sty.accent.Render("?") + " " + m.sty.dim.Render("help") + " "
	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return ansi.Truncate(left, m.w, m.gl.ell)
	}
	return left + strings.Repeat(" ", gap) + right
}

// bannerRow draws the persistent one-row banner.
func (m Model) bannerRow(lay layoutInfo) string {
	text, warn := m.bannerText()
	if m.w < 80 {
		if i := strings.Index(text, ". "); i >= 0 {
			text = text[:i+1]
		}
	}
	st := m.sty.dim
	if warn {
		st = m.sty.warn
	}
	return st.Render(" " + ansi.Truncate(text, max(m.w-2, 0), m.gl.ell))
}

// statusRow is the reserved row above the hint bar.
func (m Model) statusRow() string {
	var glyph string
	var st lipgloss.Style
	switch m.status.kind {
	case statusSuccess:
		glyph, st = m.gl.ok, m.sty.succ
	case statusError:
		glyph, st = m.gl.fail, m.sty.err
	case statusWarn:
		glyph, st = m.gl.warn, m.sty.warn
	case statusInfo:
		glyph, st = m.gl.info, m.sty.dim
	default:
		return ""
	}
	return st.Render(" " + ansi.Truncate(glyph+" "+firstLine(m.status.text), max(m.w-2, 0), m.gl.ell))
}

// overlay draws d centred over the body.
func (m Model) overlay(body string, d dialog, lay layoutInfo) string {
	maxW := min(72, m.w-4)
	if m.w < 50 {
		maxW = m.w
	}
	maxH := max(3, lay.bodyH-2)
	box := d.view(&m, maxW, maxH)
	ow, oh := lipgloss.Size(box)
	x, y := max(0, (m.w-ow)/2), max(0, (lay.bodyH-oh)/2)
	base := m.block(body, m.w, lay.bodyH)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(base),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
}

// tilde abbreviates the home directory as ~.
func (m Model) tilde(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if p == home {
			return "~"
		}
		if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
			return "~/" + rest
		}
	}
	return p
}
