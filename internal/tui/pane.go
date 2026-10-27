package tui

import (
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nunoarmada/kxconfig/v2/internal/safetext"
)

// cell is one styled piece of a list row.
type cell struct {
	s  string
	st lipgloss.Style
}

type rowMode int

const (
	rowPlain    rowMode = iota
	rowSelected         // cursor row of the focused list
)

// row joins cells into one line of exactly width columns. In rowSelected mode
// every cell, and the padding up to width, is drawn as the selection.
func (m Model) row(width int, mode rowMode, cells ...cell) string {
	var b strings.Builder
	used := 0
	for _, c := range cells {
		if used >= width {
			break
		}
		text := ansi.Truncate(c.s, width-used, m.gl.ell)
		st := c.st
		if mode == rowSelected {
			st = m.sty.selected(st)
		}
		b.WriteString(st.Render(text))
		used += lipgloss.Width(text)
	}
	if used < width {
		pad := strings.Repeat(" ", width-used)
		if mode == rowSelected {
			pad = m.sty.selected(m.sty.plain).Render(pad)
		}
		b.WriteString(pad)
	}
	return b.String()
}

// fit truncates s to w columns with an ellipsis and pads it with spaces.
func (m Model) fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, m.gl.ell)
	if pad := w - lipgloss.Width(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

// blank returns h lines of w spaces.
func blank(w, h int) string {
	if h <= 0 {
		return ""
	}
	line := strings.Repeat(" ", max(w, 0))
	return strings.TrimSuffix(strings.Repeat(line+"\n", h), "\n")
}

// block forces s to exactly w columns by h lines: lines are truncated and
// padded, missing lines are blank, surplus lines are dropped.
func (m Model) block(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	for i, l := range lines {
		lines[i] = m.fit(l, w)
	}
	return strings.Join(lines, "\n")
}

// box draws a rounded pane of outer size w x h. The first inner row is title
// (already styled), followed by lines. Rows are truncated, never wrapped.
func (m Model) box(w, h int, focused bool, title string, lines []string) string {
	if w < 5 || h < 3 {
		return blank(w, h)
	}
	bst := m.sty.border
	if focused {
		bst = m.sty.focusBorder
	}
	b := m.gl.border
	iw := w - 4
	var out []string
	out = append(out, bst.Render(b.TopLeft+strings.Repeat(b.Top, w-2)+b.TopRight))
	left, right := bst.Render(b.Left+" "), bst.Render(" "+b.Right)
	rows := append([]string{title}, lines...)
	for i := 0; i < h-2; i++ {
		content := ""
		if i < len(rows) {
			content = rows[i]
		}
		out = append(out, left+m.fit(content, iw)+right)
	}
	out = append(out, bst.Render(b.BottomLeft+strings.Repeat(b.Bottom, w-2)+b.BottomRight))
	return strings.Join(out, "\n")
}

// paneTitle renders "NAME scope count" with an optional right-aligned note.
func (m Model) paneTitle(name, meta, right string, iw int, focused bool) string {
	ts := m.sty.title
	if focused {
		ts = m.sty.titleFocus
	}
	left := ts.Render(name)
	if meta != "" {
		left += " " + m.sty.dim.Render(meta)
	}
	if right != "" {
		gap := iw - lipgloss.Width(left) - lipgloss.Width(right)
		if gap >= 2 {
			left += strings.Repeat(" ", gap) + m.sty.dim.Render(right)
		}
	}
	return left
}

// listState is the cursor, scroll offset and filter of one list pane. The
// cursor indexes the visible (filtered) rows.
type listState struct {
	cursor, offset int
	filter         string
}

// move sets the cursor to c clamped to n rows.
func (l *listState) move(c, n int) {
	l.cursor = max(0, min(c, n-1))
}

// scroll keeps the cursor inside a window of rows rows.
func (l *listState) scroll(n, rows int) {
	if rows <= 0 || n <= rows {
		l.offset = 0
		return
	}
	l.move(l.cursor, n)
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+rows {
		l.offset = l.cursor - rows + 1
	}
	l.offset = max(0, min(l.offset, n-rows))
}

// fuzzyMatch reports whether pat is a case-insensitive subsequence of s.
func fuzzyMatch(pat, s string) bool {
	if pat == "" {
		return true
	}
	p := []rune(strings.ToLower(pat))
	i := 0
	for _, r := range strings.ToLower(s) {
		if unicode.ToLower(r) == p[i] {
			i++
			if i == len(p) {
				return true
			}
		}
	}
	return false
}

// firstLine returns the first line of s.
func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return safetext.Clean(strings.TrimSpace(s))
}

// dash returns "-" for an empty value.
func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
