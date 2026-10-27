package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nunoarmada/kxconfig/v2/internal/backup"
	"github.com/nunoarmada/kxconfig/v2/internal/safetext"
)

// relTime is a short "3h ago" style age.
func relTime(now, t time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d/time.Hour))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d/(24*time.Hour)))
	}
	return fmt.Sprintf("%dmo ago", int(d/(30*24*time.Hour)))
}

// startBackups opens the backups list of the selected project.
func (m *Model) startBackups() tea.Cmd {
	p, ok := m.selectedProject()
	if !ok {
		return m.noProject()
	}
	bs, err := m.store.Backups.List(p.Name)
	if err != nil {
		return m.failed("list backups of "+p.Name, err)
	}
	m.push(&backupsDialog{project: p.Name, list: bs})
	return nil
}

// backupsDialog lists a project's backups, newest first. Enter asks to
// restore the one under the cursor.
type backupsDialog struct {
	project        string
	list           []backup.Backup
	cursor, offset int
}

func (d *backupsDialog) update(m *Model, msg tea.Msg) (dialog, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch k.String() {
	case "esc", "q":
		return nil, nil
	case "j", "down":
		d.cursor++
	case "k", "up":
		d.cursor--
	case "g", "home":
		d.cursor = 0
	case "G", "end":
		d.cursor = len(d.list) - 1
	case "enter":
		if len(d.list) == 0 {
			return d, nil
		}
		return d.confirmRestore(m), nil
	}
	d.cursor = max(0, min(d.cursor, len(d.list)-1))
	return d, nil
}

func (d *backupsDialog) confirmRestore(m *Model) dialog {
	b := d.list[d.cursor]
	project := d.project
	id := b.ID
	text := fmt.Sprintf("Restore %s to backup %s (%s)? The current file is backed up first.",
		safetext.Clean(project), safetext.Clean(id), relTime(time.Now(), b.Time))
	c := &confirmDialog{
		title: "Restore backup?", body: []string{text}, yes: "Restore", no: "Cancel",
		back: d,
		run: func(m *Model) tea.Cmd {
			if err := m.store.Restore(project, id); err != nil {
				return m.failed("restore "+project, err)
			}
			if cmd := m.refresh(); cmd != nil {
				return cmd
			}
			return m.say(statusSuccess, "Restored "+project+" from backup "+id)
		},
	}
	if w := m.shellWarning(project); w != "" {
		c.warn = []string{w}
	}
	return c
}

func (d *backupsDialog) view(m *Model, maxW, maxH int) string {
	w := min(maxW, 72)
	if m.w < 50 {
		w = maxW
	}
	iw := dialogInner(w)
	title := "Backups " + m.gl.dash + " " + safetext.Clean(d.project) + " (" + fmt.Sprint(len(d.list)) + ")"
	if len(d.list) == 0 {
		return m.dialogBox(title, []string{"No backups for " + safetext.Clean(d.project) + " yet."}, w, false, 1)
	}

	// The ID already ends in the operation, so there is no OP column.
	idW, whenW := 0, 8
	for _, b := range d.list {
		idW = max(idW, lipgloss.Width(safetext.Clean(b.ID)))
	}
	// The date shows at 80 columns and up, when it fits next to the rest.
	absolute := m.w >= 80 && 2+idW+2+whenW+2+len("2006-01-02 15:04") <= iw
	if absolute {
		whenW += 2 + len("2006-01-02 15:04")
	}
	head := m.sty.dim.Render(m.fit("  "+padRight("ID", idW)+"  "+padRight("WHEN", whenW), iw))

	rows := max(1, maxH-2-2-1) // borders, vertical padding, header
	if m.w < 50 {
		rows = max(1, maxH-2-1)
	}
	if d.cursor < d.offset {
		d.offset = d.cursor
	}
	if d.cursor >= d.offset+rows {
		d.offset = d.cursor - rows + 1
	}
	d.offset = max(0, min(d.offset, max(0, len(d.list)-rows)))

	now := time.Now()
	lines := []string{head}
	for i := d.offset; i < len(d.list) && i < d.offset+rows; i++ {
		b := d.list[i]
		when := padRight(relTime(now, b.Time), 8)
		if absolute {
			when += "  " + b.Time.Local().Format("2006-01-02 15:04")
		}
		cur := " "
		mode := rowPlain
		if i == d.cursor {
			cur, mode = m.gl.cursor, rowSelected
		}
		lines = append(lines, m.row(iw, mode,
			cell{cur + " ", m.sty.plain},
			cell{padRight(safetext.Clean(b.ID), idW) + "  ", m.sty.plain},
			cell{when, m.sty.plain}))
	}
	return m.dialogBox(title, lines, w, false, 1)
}

func (d *backupsDialog) hints() []hint {
	if len(d.list) == 0 {
		return []hint{{"esc", "close"}}
	}
	return []hint{{"enter", "restore"}, {"esc", "close"}}
}

// padRight pads s with spaces to w columns.
func padRight(s string, w int) string {
	if pad := w - lipgloss.Width(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}
