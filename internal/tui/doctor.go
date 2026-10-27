package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/nunoarmada/kxconfig/internal/safetext"
	"github.com/nunoarmada/kxconfig/internal/store"
)

// startDoctor checks the kx home and shows the findings.
func (m *Model) startDoctor() tea.Cmd {
	issues, err := m.store.Doctor(false)
	if err != nil {
		return m.failed("run the check", err)
	}
	m.push(&doctorDialog{issues: issues})
	return nil
}

// doctorDialog lists what Doctor found. f repairs the fixable ones.
type doctorDialog struct {
	issues []store.Issue
	offset int
}

func (d *doctorDialog) fixable() int {
	n := 0
	for _, i := range d.issues {
		if i.Fixable && !i.Fixed {
			n++
		}
	}
	return n
}

func (d *doctorDialog) update(m *Model, msg tea.Msg) (dialog, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch k.String() {
	case "esc", "q":
		return nil, nil
	case "j", "down":
		d.offset++
	case "k", "up":
		d.offset = max(0, d.offset-1)
	case "f":
		if d.fixable() == 0 {
			return d, nil
		}
		issues, err := m.store.Doctor(true)
		if err != nil {
			return d, m.failed("fix problems", err)
		}
		d.issues = issues
		d.offset = 0
		return d, m.refresh()
	}
	return d, nil
}

func (d *doctorDialog) view(m *Model, maxW, maxH int) string {
	w := min(maxW, 72)
	if m.w < 50 {
		w = maxW
	}
	iw := dialogInner(w)
	if len(d.issues) == 0 {
		return m.dialogBox("Doctor", []string{m.sty.succ.Render(m.gl.ok + " No problems found.")}, w, false, 1)
	}
	var all []string
	for _, i := range d.issues {
		suffix := m.sty.dim.Render("  fixable")
		if i.Fixed {
			suffix = "  " + m.sty.succ.Render(m.gl.ok+" fixed")
		} else if !i.Fixable {
			suffix = ""
		}
		msg := wrap(m.gl.warn+" "+safetext.Clean(i.Kind)+"  "+safetext.Clean(i.Message), iw-9)
		for n, l := range msg {
			if n == 0 {
				l = m.sty.warn.Render(m.gl.warn) + l[len(m.gl.warn):]
			}
			if n == len(msg)-1 {
				l += suffix
			}
			all = append(all, l)
		}
		all = append(all, m.sty.dim.Render("  "+safetext.Clean(m.tilde(i.Path))))
	}
	rows := max(1, maxH-2-2)
	if m.w < 50 {
		rows = max(1, maxH-2)
	}
	if len(all) > rows {
		d.offset = max(0, min(d.offset, len(all)-rows))
		all = all[d.offset : d.offset+rows]
	} else {
		d.offset = 0
	}
	return m.dialogBox("Doctor", all, w, false, 1)
}

func (d *doctorDialog) hints() []hint {
	if d.fixable() == 0 {
		return []hint{{"esc", "close"}}
	}
	return []hint{{"f", "fix fixable"}, {"esc", "close"}}
}
