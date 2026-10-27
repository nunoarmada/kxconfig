package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nunoarmada/kxconfig/internal/store"
)

// hint is one entry of the key hint bar. An empty key renders the label only.
type hint struct{ key, label string }

var helpHint = hint{"?", "help"}

// moveKeys only move cursors or focus; they do not dismiss an error status.
var moveKeys = map[string]bool{
	"j": true, "k": true, "up": true, "down": true, "g": true, "G": true,
	"home": true, "end": true, "pgup": true, "pgdown": true, "ctrl+u": true, "ctrl+d": true,
	"tab": true, "shift+tab": true, "h": true, "l": true, "left": true, "right": true,
	"?": true, "/": true,
}

// handleKey routes a key press: overlay first, then the filter input, then
// the focused pane. ctrl+c always quits.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if k == "ctrl+c" {
		return tea.Quit
	}
	if len(m.dialogs) > 0 {
		return m.updateDialog(msg)
	}
	if m.filtering {
		return m.updateFilter(msg)
	}
	if m.status.kind == statusError && !moveKeys[k] {
		m.status = status{seq: m.status.seq}
	}

	switch k {
	case "q":
		return tea.Quit
	case "esc":
		if m.activeFilter() != "" {
			m.applyFilter("")
			return nil
		}
		return tea.Quit
	case "?":
		m.push(newHelpDialog())
	case "tab":
		m.cycleFocus(1)
	case "shift+tab":
		m.cycleFocus(-1)
	case "h", "left":
		m.stepFocus(-1)
	case "l", "right":
		m.stepFocus(1)
	case "j", "down", "k", "up", "g", "G", "home", "end", "pgdown", "pgup", "ctrl+d", "ctrl+u":
		m.moveCursor(k)
	case "/":
		return m.startFilter()
	case ":":
		return m.later("Command palette")
	case "ctrl+r":
		if err := m.reload(); err != nil {
			m.loadErr = err
			return m.failed("read projects", err)
		}
		return m.say(statusInfo, "Reloaded from disk")
	case "enter":
		switch m.focus {
		case paneProjects:
			return m.activate()
		case paneContexts:
			return m.setContext()
		}
	case "i":
		return m.later("Import")
	case "a":
		if len(m.projects) == 0 && m.link == store.LinkForeignFile {
			m.push(newAdoptNameDialog(m, ""))
		}
	default:
		return m.paneKey(k)
	}
	return nil
}

// paneKey handles the keys that belong to one pane.
func (m *Model) paneKey(k string) tea.Cmd {
	switch m.focus {
	case paneProjects:
		switch k {
		case "r":
			return m.later("Rename")
		case "D":
			return m.later("Delete")
		case "d":
			return m.say(statusInfo, "Use D to delete a project")
		case "b":
			return m.later("Backups")
		case "e":
			return m.later("Edit")
		case "s":
			if m.env.InShell {
				return m.say(statusWarn, "Already inside a kx shell")
			}
			return m.later("Shell")
		}
	case paneContexts:
		switch k {
		case "n":
			return m.later("Set namespace")
		case "d":
			return m.later("Delete context")
		}
	}
	return nil
}

// later reports an action whose dialog is not built yet.
func (m *Model) later(what string) tea.Cmd {
	return m.say(statusInfo, what+" is not available yet")
}

// availablePanes lists the panes that can take focus at the current size.
func (m Model) availablePanes() []pane {
	if len(m.projects) == 0 {
		return []pane{paneProjects}
	}
	if m.layout().hasDetails() {
		return []pane{paneProjects, paneContexts, paneDetails}
	}
	return []pane{paneProjects, paneContexts}
}

func (m *Model) setFocus(p pane) {
	m.focus = p
}

// showsFilter reports whether pane p has a filter row: being typed, or kept.
func (m Model) showsFilter(p pane) bool {
	switch p {
	case paneProjects:
		return m.plist.filter != "" || (m.filtering && m.focus == p)
	case paneContexts:
		return m.clist.filter != "" || (m.filtering && m.focus == p)
	}
	return false
}

// cycleFocus moves focus with wrap-around (tab, shift+tab).
func (m *Model) cycleFocus(dir int) {
	av := m.availablePanes()
	for i, p := range av {
		if p == m.focus {
			m.setFocus(av[(i+dir+len(av))%len(av)])
			return
		}
	}
	m.setFocus(av[0])
}

// stepFocus moves focus without wrapping (h, l, left, right).
func (m *Model) stepFocus(dir int) {
	av := m.availablePanes()
	for i, p := range av {
		if p == m.focus {
			m.setFocus(av[max(0, min(i+dir, len(av)-1))])
			return
		}
	}
}

// activeFilter is the filter of the focused list pane.
func (m Model) activeFilter() string {
	switch m.focus {
	case paneProjects:
		return m.plist.filter
	case paneContexts:
		return m.clist.filter
	}
	return ""
}

// moveCursor moves the cursor (or scroll offset) of the focused pane.
func (m *Model) moveCursor(k string) {
	lay := m.layout()
	var n, rows int
	var l *listState
	switch m.focus {
	case paneProjects:
		l, n, rows = &m.plist, len(m.visibleProjects()), lay.projectRows(m.showsFilter(paneProjects))
	case paneContexts:
		l, n, rows = &m.clist, len(m.visibleContexts()), lay.contextRows(m.showsFilter(paneContexts))
	default:
		l, n, rows = &listState{cursor: m.dscroll}, len(m.detailLines(lay.detailsInner())), lay.detailRows()
		defer func() { m.dscroll = l.cursor }()
	}
	page := max(1, rows-1)
	c := l.cursor
	switch k {
	case "j", "down":
		c++
	case "k", "up":
		c--
	case "g", "home":
		c = 0
	case "G", "end":
		c = n - 1
	case "pgdown", "ctrl+d":
		c += page
	case "pgup", "ctrl+u":
		c -= page
	}
	if m.focus == paneDetails {
		n = max(1, n-rows+1) // scroll offsets, not rows
	}
	l.move(c, n)
	if m.focus == paneProjects {
		m.syncSelection()
	}
}

// syncSelection reloads the contexts when the project cursor moved.
func (m *Model) syncSelection() {
	p, ok := m.selectedProject()
	if (ok && p.Name != m.cur.name) || (!ok && m.cur.name != "") {
		m.loadSelected()
	}
}

// startFilter opens the filter input on the focused list.
func (m *Model) startFilter() tea.Cmd {
	if m.focus == paneDetails || len(m.projects) == 0 {
		return nil
	}
	in := textinput.New()
	in.Prompt = ""
	in.SetVirtualCursor(true)
	st := in.Styles()
	st.Cursor.Blink = false
	st.Cursor.Color = m.sty.pal.accent
	in.SetStyles(st)
	in.SetValue(m.activeFilter())
	in.CursorEnd()
	m.finput = in
	m.filtering = true
	return m.finput.Focus()
}

// updateFilter edits the filter of the focused list while it is being typed.
func (m *Model) updateFilter(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		m.filtering = false
		m.finput.Blur()
		return nil
	case "esc":
		m.filtering = false
		m.finput.Blur()
		m.applyFilter("")
		return nil
	case "down", "up":
		m.moveCursor(msg.String())
		return nil
	}
	var cmd tea.Cmd
	m.finput, cmd = m.finput.Update(msg)
	m.applyFilter(m.finput.Value())
	return cmd
}

// applyFilter sets the filter of the focused list and returns its cursor to
// the first match.
func (m *Model) applyFilter(v string) {
	switch m.focus {
	case paneProjects:
		m.plist.filter = v
		m.plist.cursor, m.plist.offset = 0, 0
		m.syncSelection()
	case paneContexts:
		m.clist.filter = v
		m.clist.cursor, m.clist.offset = 0, 0
	}
}

// hints returns the hint bar entries for the current state. The last entry is
// pinned: it survives when the others have to be dropped.
func (m Model) hints() []hint {
	if n := len(m.dialogs); n > 0 {
		return m.dialogs[n-1].hints()
	}
	if m.filtering {
		return []hint{{"", "type to filter"}, {"enter", "keep"}, {"esc", "clear"}}
	}
	if len(m.projects) == 0 {
		h := []hint{{"i", "import"}}
		if m.link == store.LinkForeignFile {
			h = append(h, hint{"a", "adopt"})
		}
		return append(h, hint{"q", "quit"}, helpHint)
	}
	switch m.focus {
	case paneContexts:
		return []hint{{"enter", "set current"}, {"n", "namespace"}, {"d", "delete"}, {"/", "filter"}, {":", "commands"}, helpHint}
	case paneDetails:
		return []hint{{"tab", "next pane"}, {":", "commands"}, helpHint}
	}
	return []hint{
		{"enter", "use"}, {"/", "filter"}, {"i", "import"}, {"r", "rename"}, {"D", "delete"},
		{"b", "backups"}, {"s", "shell"}, {"e", "edit"}, {":", "commands"}, helpHint,
	}
}

// hintBar renders the hint bar on one row of width w. Items are dropped from
// the right, never the pinned last one.
func (m Model) hintBar(w int) string {
	hs := m.hints()
	render := func(h hint) (string, int) {
		if h.key == "" {
			return m.sty.dim.Render(h.label), lipgloss.Width(h.label)
		}
		return m.sty.accent.Render(h.key) + " " + m.sty.dim.Render(h.label), lipgloss.Width(h.key) + 1 + lipgloss.Width(h.label)
	}
	avail := w - 2 // one column of padding on each side
	pinned := hs[len(hs)-1]
	pinStr, pinW := render(pinned)
	var parts []string
	used := pinW
	for _, h := range hs[:len(hs)-1] {
		s, hw := render(h)
		if used+2+hw > avail {
			break
		}
		parts = append(parts, s)
		used += 2 + hw
	}
	parts = append(parts, pinStr)
	line := " " + strings.Join(parts, "  ")
	return ansi.Truncate(line, w, "")
}
