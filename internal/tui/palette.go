package tui

import (
	"strconv"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nunoarmada/kxconfig/internal/safetext"
)

// command is one entry of the command palette.
type command struct {
	name, desc string
	run        func(m *Model) tea.Cmd
}

// commands lists the palette entries in display order.
func commands() []command {
	return []command{
		{"import", "Import a kubeconfig file", (*Model).startImport},
		{"rename", "Rename selected project", (*Model).startRename},
		{"delete", "Delete selected project", (*Model).startDelete},
		{"backups", "Backups of selected project", (*Model).startBackups},
		{"shell", "Open kx shell", (*Model).startShell},
		{"edit", "Open in $EDITOR", (*Model).startEdit},
		{"doctor", "Check permissions and links", (*Model).startDoctor},
		{"reload", "Reload from disk", (*Model).reloadKey},
		{"quit", "Quit kx", func(*Model) tea.Cmd { return tea.Quit }},
	}
}

const paletteRows = 8

// paletteDialog is the ":" prompt: type to narrow the commands, enter runs.
type paletteDialog struct {
	in             textinput.Model
	cursor, offset int
	upDown         string // "↑↓", or words in ASCII mode
}

func newPaletteDialog(m *Model) *paletteDialog {
	in := textinput.New()
	in.Prompt = ": "
	st := in.Styles()
	st.Cursor.Blink = false
	st.Cursor.Color = m.sty.pal.accent
	in.SetStyles(st)
	in.Focus()
	upDown := "↑↓"
	if m.gl.ascii {
		upDown = "up/down"
	}
	return &paletteDialog{in: in, upDown: upDown}
}

// matches returns the commands that fuzzy-match q, names that start with q
// first, otherwise in display order.
func matches(q string) []command {
	var pre, rest []command
	for _, c := range commands() {
		switch {
		case !fuzzyMatch(q, c.name):
		case strings.HasPrefix(c.name, strings.ToLower(q)):
			pre = append(pre, c)
		default:
			rest = append(rest, c)
		}
	}
	return append(pre, rest...)
}

func (d *paletteDialog) update(m *Model, msg tea.Msg) (dialog, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			return nil, nil
		case "up", "ctrl+p":
			d.cursor = max(0, d.cursor-1)
			return d, nil
		case "down", "ctrl+n":
			d.cursor = min(len(matches(d.in.Value()))-1, d.cursor+1)
			d.cursor = max(0, d.cursor)
			return d, nil
		case "enter":
			ms := matches(d.in.Value())
			if len(ms) == 0 {
				return d, nil
			}
			// run may push a dialog of its own; closing this slot keeps it.
			return nil, ms[min(d.cursor, len(ms)-1)].run(m)
		}
	}
	var cmd tea.Cmd
	d.in, cmd = d.in.Update(msg)
	d.cursor, d.offset = 0, 0
	return d, cmd
}

// matchPositions returns the rune indexes of name that the query matched.
func matchPositions(q, name string) map[int]bool {
	pos := map[int]bool{}
	p := []rune(strings.ToLower(q))
	i := 0
	for j, r := range []rune(name) {
		if i < len(p) && unicode.ToLower(r) == p[i] {
			pos[j] = true
			i++
		}
	}
	return pos
}

func (d *paletteDialog) view(m *Model, maxW, _ int) string {
	w := min(maxW, 64)
	if m.w < 50 {
		w = maxW
	}
	iw := dialogInner(w)
	d.in.SetWidth(max(1, iw-4))
	q := d.in.Value()
	lines := []string{d.in.View()}
	ms := matches(q)
	if len(ms) == 0 {
		lines = append(lines, m.sty.dim.Render(m.truncRow(`No command matches "`+safetext.Clean(q)+`"`, iw)))
	}
	sel := min(d.cursor, len(ms)-1)
	if sel < d.offset {
		d.offset = sel
	}
	if sel >= d.offset+paletteRows {
		d.offset = sel - paletteRows + 1
	}
	d.offset = max(0, min(d.offset, max(0, len(ms)-paletteRows)))
	for i := d.offset; i < len(ms) && i < d.offset+paletteRows; i++ {
		c := ms[i]
		mode, cur := rowPlain, " "
		if i == sel {
			mode, cur = rowSelected, m.gl.cursor
		}
		cells := []cell{{cur + " ", m.sty.plain}}
		pos := matchPositions(q, c.name)
		for j, r := range []rune(c.name) {
			st := m.sty.plain
			if pos[j] {
				st = m.sty.accent
			}
			cells = append(cells, cell{string(r), st})
		}
		cells = append(cells, cell{padRight("", 9-len([]rune(c.name))) + c.desc, m.sty.dim})
		lines = append(lines, m.row(iw, mode, cells...))
	}
	if hidden := len(ms) - paletteRows; hidden > 0 {
		lines = append(lines, m.sty.dim.Render("  +"+strconv.Itoa(hidden)+" more"))
	}
	return m.dialogBox("Commands", lines, w, false, 1)
}

func (d *paletteDialog) hints() []hint {
	return []hint{{"", "type to search"}, {d.upDown, "move"}, {"enter", "run"}, {"esc", "close"}}
}
