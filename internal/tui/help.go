package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type helpEntry struct{ key, desc string }

type helpGroup struct {
	name    string
	entries []helpEntry
	inline  string // one plain row instead of entries
}

// helpGroups are the groups of the help overlay, in the order of the design
// brief. Arrow glyphs depend on the glyph set.
func (m *Model) helpGroups() []helpGroup {
	return []helpGroup{
		{name: "NAVIGATION", entries: []helpEntry{
			{"tab / shift+tab", "next / prev pane"}, {m.leftRightKeys(), "pane left / right"},
			{"j k " + m.gl.upDown, "move"}, {"g G", "top / bottom"},
			{"/", "filter list"}, {"esc", "back / clear / quit"},
			{":", "command palette"}, {"ctrl+r", "reload from disk"},
			{"?", "this help"}, {"q", "quit"},
			{"ctrl+c", "quit from anywhere"},
		}},
		{name: "PROJECTS", entries: []helpEntry{
			{"enter", "use (activate globally)"}, {"i", "import kubeconfig"},
			{"r", "rename"}, {"D", "delete (backup kept)"},
			{"b", "backups / restore"}, {"s", "open kx shell"},
			{"e", "edit in $EDITOR"},
		}},
		{name: "CONTEXTS", entries: []helpEntry{
			{"enter", "set as current context"}, {"n", "set namespace"},
			{"d", "delete context (and orphans)"},
		}},
		{name: "PALETTE", inline: "import  rename  delete  backups  shell  edit  doctor  reload  quit"},
	}
}

// leftRightKeys is the key label for pane left / right; the ASCII set drops
// the arrow words, which only repeat the h l pair.
func (m *Model) leftRightKeys() string {
	if m.gl.ascii {
		return "h l"
	}
	return "h l " + m.gl.arrows
}

// helpDialog lists every key. It scrolls when the terminal is short.
type helpDialog struct {
	offset  int
	scrolls bool // set by view: the text is taller than the box
}

func newHelpDialog() *helpDialog { return &helpDialog{} }

func (d *helpDialog) update(m *Model, msg tea.Msg) (dialog, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch k.String() {
	case "esc", "?", "q":
		return nil, nil
	case "j", "down":
		d.offset++
	case "k", "up":
		d.offset = max(0, d.offset-1)
	case "g", "home":
		d.offset = 0
	case "G", "end":
		d.offset = 1 << 20
	}
	return d, nil
}

// lines renders the help text for an inner width iw. Two columns when they
// fit, one otherwise.
func (d *helpDialog) lines(m *Model, iw int) []string {
	groups := m.helpGroups()
	// One key column width for every group, so descriptions line up.
	var lAll, rAll []helpEntry
	for _, g := range groups {
		for i, e := range g.entries {
			if i%2 == 0 {
				lAll = append(lAll, e)
			} else {
				rAll = append(rAll, e)
			}
		}
	}
	lk, rk := keyWidth(lAll), keyWidth(rAll)
	lw, rw := colWidth(lAll, lk), colWidth(rAll, rk)
	twoCols := lw+2+rw <= iw
	var allEntries []helpEntry
	for _, g := range groups {
		allEntries = append(allEntries, g.entries...)
	}
	kw := keyWidth(allEntries)

	var out []string
	for gi, g := range groups {
		if gi > 0 {
			out = append(out, "")
		}
		out = append(out, m.sty.title.Render(g.name))
		if g.inline != "" {
			out = append(out, wrap(g.inline, iw)...)
			continue
		}
		if twoCols {
			for i := 0; i < len(g.entries); i += 2 {
				row := m.fit(m.entryCell(g.entries[i], lk, lw), lw)
				if i+1 < len(g.entries) {
					row += "  " + m.entryCell(g.entries[i+1], rk, rw)
				}
				out = append(out, strings.TrimRight(row, " "))
			}
			continue
		}
		for _, e := range g.entries {
			out = append(out, m.entryCell(e, kw, iw))
		}
	}
	return out
}

// colWidth is the natural width of a column of entries with keys padded to kw.
func colWidth(es []helpEntry, kw int) int {
	w := 0
	for _, e := range es {
		w = max(w, kw+2+lipgloss.Width(e.desc))
	}
	return w
}

func keyWidth(es []helpEntry) int {
	w := 0
	for _, e := range es {
		w = max(w, lipgloss.Width(e.key))
	}
	return w
}

// entryCell renders "key  description" with the key padded to kw columns.
func (m *Model) entryCell(e helpEntry, kw, w int) string {
	pad := strings.Repeat(" ", max(0, kw-lipgloss.Width(e.key)))
	return ansi.Truncate(m.sty.accent.Render(e.key)+pad+"  "+e.desc, w, m.gl.ell)
}

// wide: with one key column shared by every group, two columns need ~76 text
// columns, more than the usual 72-wide dialog leaves.
func (d *helpDialog) wide() bool { return true }

func (d *helpDialog) view(m *Model, maxW, maxH int) string {
	w := min(maxW, 84)
	if m.w >= 50 {
		w = min(m.w-4, 84)
	}
	maxH = max(3, maxH-2)
	iw := dialogInner(w)
	all := d.lines(m, iw)
	rows := max(1, maxH-2) // top and bottom border
	if len(all) > rows {
		d.offset = max(0, min(d.offset, len(all)-rows))
		all = all[d.offset : d.offset+rows]
	} else {
		d.offset = 0
	}
	title := "Keys"
	d.scrolls = len(d.lines(m, iw)) > rows
	if d.scrolls {
		title = "Keys  (j/k scroll)"
	}
	return m.dialogBox(title, all, w, false, 0)
}

func (d *helpDialog) hints() []hint {
	if d.scrolls {
		return []hint{{"j/k", "scroll"}, {"esc", "close"}}
	}
	return []hint{{"esc", "close"}}
}
