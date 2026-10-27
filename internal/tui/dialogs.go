package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nunoarmada/kxconfig/v2/internal/safetext"
)

// confirmDialog asks a yes/no question. The default is cancel: y confirms,
// n, esc and enter cancel. Destructive questions use the error border.
type confirmDialog struct {
	title       string
	body        []string // paragraphs, wrapped to the dialog width
	warn        []string // warning rows, drawn with the warning glyph
	yes, no     string   // button labels
	destructive bool
	run         func(m *Model) tea.Cmd
	back        dialog // shown again on cancel; nil closes
}

func (d *confirmDialog) update(m *Model, msg tea.Msg) (dialog, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch k.String() {
	case "y":
		return nil, d.run(m)
	case "n", "esc", "enter":
		return d.back, nil
	}
	return d, nil
}

func (d *confirmDialog) view(m *Model, maxW, _ int) string {
	w := min(maxW, 64)
	if m.w < 50 {
		w = maxW
	}
	iw := dialogInner(w)
	var body []string
	for _, p := range d.body {
		if p == "" {
			body = append(body, "")
			continue
		}
		body = append(body, wrap(p, iw)...)
	}
	for _, p := range d.warn {
		for i, l := range wrap(m.gl.warn+" "+p, iw) {
			if i > 0 {
				l = "  " + l
			}
			body = append(body, m.sty.warn.Render(l))
		}
	}
	btn := func(key, label string) string {
		return "[ " + m.sty.accent.Render(key) + " " + label + " ]"
	}
	buttons := btn("y", d.yes) + "  " + btn("n", d.no)
	pad := iw - lipgloss.Width(buttons)
	body = append(body, "", strings.Repeat(" ", max(0, pad))+buttons)
	return m.dialogBox(d.title, body, w, d.destructive, 1)
}

func (d *confirmDialog) hints() []hint {
	return []hint{{"y", "confirm"}, {"n/esc", "cancel"}}
}

// typeConfirmDialog makes the user type word before a destructive action
// runs. Enter does nothing until the text matches exactly.
type typeConfirmDialog struct {
	title  string
	body   []string
	word   string
	action string // verb on the hint bar, e.g. "delete"
	in     textinput.Model
	run    func(m *Model) tea.Cmd
}

func newTypeConfirmDialog(m *Model, title string, body []string, word, action string, run func(m *Model) tea.Cmd) *typeConfirmDialog {
	in := textinput.New()
	st := in.Styles()
	st.Cursor.Blink = false
	st.Cursor.Color = m.sty.pal.accent
	in.SetStyles(st)
	in.Focus()
	return &typeConfirmDialog{title: title, body: body, word: word, action: action, in: in, run: run}
}

func (d *typeConfirmDialog) update(m *Model, msg tea.Msg) (dialog, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			return nil, nil
		case "enter":
			if d.in.Value() == d.word {
				return nil, d.run(m)
			}
			return d, nil
		}
	}
	var cmd tea.Cmd
	d.in, cmd = d.in.Update(msg)
	return d, cmd
}

func (d *typeConfirmDialog) view(m *Model, maxW, _ int) string {
	w := min(maxW, 64)
	if m.w < 50 {
		w = maxW
	}
	iw := dialogInner(w)
	var body []string
	for _, p := range d.body {
		body = append(body, wrap(p, iw)...)
	}
	d.in.SetWidth(max(1, iw-4))
	body = append(body, "Type "+m.sty.bold.Render(safetext.Clean(d.word))+" to confirm", d.in.View())
	return m.dialogBox(d.title, body, w, true, 1)
}

func (d *typeConfirmDialog) hints() []hint {
	return []hint{{"", "type the name"}, {"enter", d.action}, {"esc", "cancel"}}
}

// shellWarning is the warning row for live kx shells that use project, or "".
func (m *Model) shellWarning(project string) string {
	pids, err := m.store.LiveShells(project)
	if err != nil || len(pids) == 0 {
		return ""
	}
	p := safetext.Clean(project)
	if len(pids) == 1 {
		return "1 kx shell is using " + p + "; it keeps its copy."
	}
	return fmt.Sprintf("%d kx shells are using %s; they keep their copy.", len(pids), p)
}

// plural returns "1 word" or "n words".
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// truncRow truncates s to w columns with the ellipsis of the glyph set.
func (m *Model) truncRow(s string, w int) string {
	return ansi.Truncate(s, max(w, 0), m.gl.ell)
}
