package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nunoarmada/kxconfig/internal/safetext"
	"github.com/nunoarmada/kxconfig/internal/store"
)

// dialog is an overlay that owns the keyboard while it is open. The model
// keeps a stack of them; Task 9 dialogs (rename, delete, import, backups,
// palette, doctor) implement this interface and are pushed with Model.push.
type dialog interface {
	// update handles a message. It returns the dialog that should occupy this
	// slot: itself to stay open, another dialog to replace it, nil to close.
	update(m *Model, msg tea.Msg) (dialog, tea.Cmd)
	// view draws the dialog already boxed, at most maxW x maxH.
	view(m *Model, maxW, maxH int) string
	// hints is the hint bar while the dialog is open. It must not be empty:
	// the last entry is pinned and survives when the bar is too narrow.
	hints() []hint
}

// dialogBox draws a rounded box of outer width w with title in the top
// border. body lines must already fit the inner width (see dialogInner).
// destructive dialogs use the error colour.
func (m *Model) dialogBox(title string, body []string, w int, destructive bool, vpad int) string {
	padX := 2
	if w < 50 {
		padX, vpad = 1, 0
	}
	bst := m.sty.focusBorder
	if destructive {
		bst = m.sty.err
	}
	b := m.gl.border
	iw := w - 2 - 2*padX

	t := " " + ansi.Truncate(title, max(w-6, 0), m.gl.ell) + " "
	top := bst.Render(b.TopLeft) + bst.Bold(true).Render(t) +
		bst.Render(strings.Repeat(b.Top, max(0, w-2-lipgloss.Width(t)))+b.TopRight)

	side := func(content string) string {
		return bst.Render(b.Left) + strings.Repeat(" ", padX) + m.fit(content, iw) +
			strings.Repeat(" ", padX) + bst.Render(b.Right)
	}
	out := []string{top}
	for i := 0; i < vpad; i++ {
		out = append(out, side(""))
	}
	for _, l := range body {
		out = append(out, side(l))
	}
	for i := 0; i < vpad; i++ {
		out = append(out, side(""))
	}
	out = append(out, bst.Render(b.BottomLeft+strings.Repeat(b.Bottom, w-2)+b.BottomRight))
	return strings.Join(out, "\n")
}

// dialogInner is the text width inside a dialog box of outer width w.
func dialogInner(w int) int {
	if w < 50 {
		return max(1, w-4)
	}
	return max(1, w-6)
}

// wrap breaks s into lines of at most w columns.
func wrap(s string, w int) []string {
	return strings.Split(ansi.Wrap(s, w, ""), "\n")
}

// inputDialog is a single-line text prompt with live validation: the label,
// the input, and one row for the error. Enter is disabled while invalid.
type inputDialog struct {
	title, label string
	in           textinput.Model
	validate     func(m *Model, v string) string // "" when valid, else the message to show
	submit       func(m *Model, value string) tea.Cmd
	err          string
	allowEmpty   bool   // enter accepts an empty value (it clears something)
	help         string // optional dim row under the input, always shown
	// note, when set, adds a row under the input: dim text, or a warning.
	note func(m *Model, v string) (text string, warn bool)
}

func newInputDialog(m *Model, title, label, initial string,
	validate func(m *Model, v string) string, submit func(m *Model, value string) tea.Cmd) *inputDialog {
	in := textinput.New()
	st := in.Styles()
	st.Cursor.Blink = false
	st.Cursor.Color = m.sty.pal.accent
	in.SetStyles(st)
	in.SetValue(initial)
	in.CursorEnd()
	in.Focus()
	return &inputDialog{title: title, label: label, in: in, validate: validate, submit: submit}
}

func (d *inputDialog) update(m *Model, msg tea.Msg) (dialog, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			return nil, nil
		case "enter":
			v := d.in.Value()
			if (v == "" && !d.allowEmpty) || (v != "" && d.validate(m, v) != "") {
				return d, nil
			}
			return nil, d.submit(m, v)
		}
	}
	var cmd tea.Cmd
	d.in, cmd = d.in.Update(msg)
	d.err = ""
	if v := d.in.Value(); v != "" {
		d.err = d.validate(m, v)
	}
	return d, cmd
}

func (d *inputDialog) view(m *Model, maxW, _ int) string {
	w := min(maxW, 56)
	if m.w < 50 {
		w = maxW
	}
	iw := dialogInner(w)
	d.in.SetWidth(max(1, iw-4))
	errRow := ""
	if d.err != "" {
		errRow = m.sty.err.Render(ansi.Truncate(m.gl.fail+" "+d.err, iw, m.gl.ell))
	}
	body := []string{d.label, d.in.View(), errRow}
	if d.help != "" {
		body = append(body, m.sty.dim.Render(ansi.Truncate(d.help, iw, m.gl.ell)))
	}
	if d.note != nil {
		if text, warn := d.note(m, d.in.Value()); text != "" {
			st := m.sty.dim
			if warn {
				st = m.sty.warn
				text = m.gl.warn + " " + text
			}
			body = append(body, st.Render(ansi.Truncate(text, iw, m.gl.ell)))
		}
	}
	return m.dialogBox(d.title, body, w, false, 1)
}

func (d *inputDialog) hints() []hint {
	return []hint{{"enter", "confirm"}, {"esc", "cancel"}}
}

// validName checks a new project name: syntax, then that it is free. It
// returns the message to show, or "" when the name is fine.
func (m *Model) validName(name string) string {
	if msg := nameSyntax(name); msg != "" {
		return msg
	}
	for _, p := range m.projects {
		if p.Name == name {
			return "Project " + name + " already exists"
		}
	}
	return ""
}

// nameSyntax checks that name is an acceptable project name, ignoring whether
// it is taken. It returns the message to show, or "".
func nameSyntax(name string) string {
	if err := store.ValidateName(name); err != nil {
		if strings.Contains(err.Error(), "must not end in") {
			return "Names must not end in .lock, .bak, .orig, .swp or ~"
		}
		return "Use letters, digits, . _ - (max 64)"
	}
	return ""
}

// freeName returns base, or base-2, base-3... when taken.
func (m *Model) freeName(base string) string {
	name := base
	for i := 2; m.validName(name) != "" && i < 100; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return name
}

// newAdoptNameDialog asks for the project name to keep a foreign
// ~/.kube/config under. With target set, that project is activated after.
func newAdoptNameDialog(m *Model, target string) *inputDialog {
	return newInputDialog(m, "Import current config", "Name for the imported project", m.freeName("default"),
		(*Model).validName,
		func(m *Model, name string) tea.Cmd {
			if target == "" {
				target = name
			}
			return m.adoptAndUse(name, target)
		})
}

// adoptDialog is shown when enter is pressed on a project while
// ~/.kube/config is a regular file that kx does not manage.
type adoptDialog struct{ target string }

func newAdoptDialog(target string) *adoptDialog { return &adoptDialog{target: target} }

func (d *adoptDialog) update(m *Model, msg tea.Msg) (dialog, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch k.String() {
	case "esc":
		return nil, nil
	case "i":
		return newAdoptNameDialog(m, d.target), nil
	case "r":
		return nil, m.adoptAndUse("", d.target)
	}
	return d, nil
}

func (d *adoptDialog) view(m *Model, maxW, _ int) string {
	w := min(maxW, 64)
	if m.w < 50 {
		w = maxW
	}
	iw := dialogInner(w)
	cfg := safetext.Clean(m.tilde(m.store.ConfigPath()))
	backups := safetext.Clean(m.tilde(m.store.Backups.Dir))
	body := wrap(cfg+" is not managed by kx. It will be backed up to "+backups+"/_initial first.", iw)
	body = append(body, "")
	for _, it := range [][2]string{
		{"i", "Keep it as a new project (you name it), then use " + safetext.Clean(d.target)},
		{"r", "Replace it with a link to " + safetext.Clean(d.target)},
		{"esc", "Cancel"},
	} {
		for n, l := range wordWrap(it[1], max(8, iw-5)) {
			if n == 0 {
				body = append(body, m.sty.accent.Render(it[0])+strings.Repeat(" ", 5-lipgloss.Width(it[0]))+l)
			} else {
				body = append(body, "     "+l) // hang under the text
			}
		}
	}
	return m.dialogBox("Use "+safetext.Clean(d.target)+"?", body, w, false, 1)
}

func (d *adoptDialog) hints() []hint {
	return []hint{{"i", "import as project"}, {"r", "replace (backed up)"}, {"esc", "cancel"}}
}
