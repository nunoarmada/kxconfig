package tui

import (
	"path/filepath"

	"github.com/nunoarmada/kxconfig/v2/internal/store"
)

// Minimum terminal size; below it only a notice is drawn.
const (
	minW, minH = 40, 10
	wideW      = 120 // three panes from this width
	mediumW    = 80  // two columns from this width
	fullH      = 24  // multi-pane layouts need this many rows
)

type layoutMode int

const (
	layoutTiny   layoutMode = iota // too small: notice only
	layoutNarrow                   // one focused pane
	layoutMedium                   // projects | contexts over details
	layoutWide                     // projects | contexts | details
)

type rect struct{ w, h int }

// layoutInfo is the geometry of the frame for the current size and state.
type layoutInfo struct {
	mode           layoutMode
	w, h           int
	banner         bool
	bodyH          int
	proj, ctx, det rect
	hideDetails    bool // medium layout without room for the details pane
	gutter         int
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

// layout computes pane sizes. Frame rows: header, banner (0-1), body, status
// line, hint bar.
func (m Model) layout() layoutInfo {
	l := layoutInfo{w: m.w, h: m.h, gutter: 1}
	if m.w < minW || m.h < minH {
		l.mode = layoutTiny
		return l
	}
	l.banner = m.banner() != ""
	l.bodyH = m.h - 3
	if l.banner {
		l.bodyH--
	}
	full := rect{m.w, l.bodyH}
	switch {
	case m.w >= wideW && m.h >= fullH:
		l.mode = layoutWide
		pw := clamp(m.w*24/100, 24, 36)
		dw := clamp(m.w*32/100, 34, 56)
		l.proj = rect{pw, l.bodyH}
		l.det = rect{dw, l.bodyH}
		l.ctx = rect{m.w - pw - dw - 2*l.gutter, l.bodyH}
	case m.w >= mediumW && m.h >= fullH:
		l.mode = layoutMedium
		pw := clamp(m.w*32/100, 22, 34)
		rw := m.w - pw - l.gutter
		l.proj = rect{pw, l.bodyH}
		if l.bodyH < 14 {
			l.hideDetails = true
			l.ctx = rect{rw, l.bodyH}
			break
		}
		ch := max(6, l.bodyH*60/100)
		l.ctx = rect{rw, ch}
		l.det = rect{rw, l.bodyH - ch}
	default:
		l.mode = layoutNarrow
		l.proj, l.ctx, l.det = full, full, full
	}
	return l
}

func (l layoutInfo) hasDetails() bool {
	return l.mode == layoutNarrow || l.mode == layoutWide || (l.mode == layoutMedium && !l.hideDetails)
}

// projectRows is how many project rows fit: pane height minus border, title
// and the optional filter row.
func (l layoutInfo) projectRows(filterRow bool) int {
	return max(0, l.proj.h-2-1-b2i(filterRow))
}

// contextRows is how many context rows fit: minus border, title, column
// header and the optional filter row.
func (l layoutInfo) contextRows(filterRow bool) int {
	return max(0, l.ctx.h-2-1-1-b2i(filterRow))
}

func (l layoutInfo) detailRows() int { return max(0, l.det.h-3) }

func (l layoutInfo) detailsInner() int { return max(0, l.det.w-4) }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// banner returns the plain banner text for the current link state and
// whether it is a warning (otherwise info). The text is "" when there is none.
func (m Model) bannerText() (text string, warn bool) {
	if m.loadErr != nil {
		return "", false
	}
	cfg := m.tilde(m.store.ConfigPath())
	switch {
	case m.env.InShell:
		return m.gl.info + " Inside kx shell for " + m.env.Project +
			": context/namespace changes stay in this shell. Exit it to switch projects.", false
	case m.link == store.LinkBroken:
		return m.gl.warn + " " + cfg + " points to a missing project (" + filepath.Base(m.linkTarget) +
			"). Press enter on a project to fix it.", true
	case m.link == store.LinkForeignSymlink:
		return m.gl.warn + " " + cfg + " is a link kx doesn't manage (" + m.gl.arrow + " " + m.linkTarget +
			"). kx will not change it.", true
	case m.link == store.LinkForeignFile && len(m.projects) == 0:
		return "", false // the empty state already offers "a adopt"
	case m.link == store.LinkForeignFile:
		return m.gl.info + " " + cfg + " is not managed by kx yet. Using a project backs it up first.", false
	}
	return "", false
}

func (m Model) banner() string {
	t, _ := m.bannerText()
	return t
}
