package tui

import (
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// colorMode is how much colour the terminal can show.
type colorMode int

const (
	modeTrue colorMode = iota // TrueColor: the palette as is
	mode256                   // ANSI256: the palette, with explicit selection colours
	modeANSI                  // 16 colours: selection is reverse video
	modeMono                  // NO_COLOR / no colour at all: glyphs, bold, underline and reverse only
)

func modeFor(p colorprofile.Profile) colorMode {
	switch p {
	case colorprofile.ANSI:
		return modeANSI
	case colorprofile.ANSI256:
		return mode256
	case colorprofile.Ascii, colorprofile.NoTTY:
		return modeMono
	default:
		return modeTrue
	}
}

// palette holds the colour tokens of the design brief for one background.
type palette struct {
	accent, active, focus, border, dim, warn, err, success, selBg, selFg color.Color
}

func newPalette(isDark bool) palette {
	ld := lipgloss.LightDark(isDark)
	c := lipgloss.Color
	return palette{
		accent:  ld(c("#2A5DB0"), c("#79A8FF")),
		active:  ld(c("#1A7F45"), c("#5FD38D")),
		focus:   ld(c("#2A5DB0"), c("#79A8FF")),
		border:  ld(c("#B8BDC6"), c("#4A505A")),
		dim:     ld(c("#6B7079"), c("#8A8F98")),
		warn:    ld(c("#8A5A00"), c("#E5C07B")),
		err:     ld(c("#B42318"), c("#FF7A7A")),
		success: ld(c("#1A7F45"), c("#5FD38D")),
		selBg:   ld(c("#D6E4FA"), c("#2B3A55")),
		selFg:   ld(c("#0B1B33"), c("#E6EDF7")),
	}
}

// styles are the Lip Gloss styles derived from the palette. Body text never
// gets a foreground colour: it uses the terminal default.
type styles struct {
	mode colorMode
	pal  palette

	plain, bold                          lipgloss.Style
	accent, active, dim, warn, err, succ lipgloss.Style
	border, focusBorder                  lipgloss.Style
	title, titleFocus                    lipgloss.Style
}

func newStyles(isDark bool, p colorprofile.Profile) styles {
	s := styles{mode: modeFor(p), pal: newPalette(isDark)}
	if s.mode == mode256 {
		// Pick palette cells directly: a truecolor value downsampled to 256
		// colours can lose the contrast the selection needs.
		ld := lipgloss.LightDark(isDark)
		s.pal.selBg = ld(lipgloss.ANSIColor(189), lipgloss.ANSIColor(237))
		s.pal.selFg = ld(lipgloss.ANSIColor(17), lipgloss.ANSIColor(255))
	}
	fg := func(c color.Color) lipgloss.Style {
		if s.mode == modeMono {
			return lipgloss.NewStyle()
		}
		return lipgloss.NewStyle().Foreground(c)
	}
	s.plain = lipgloss.NewStyle()
	s.bold = lipgloss.NewStyle().Bold(true)
	s.accent = fg(s.pal.accent).Bold(true)
	s.active = fg(s.pal.active)
	s.dim = fg(s.pal.dim)
	if s.mode == modeMono {
		s.dim = lipgloss.NewStyle().Faint(true) // no colour: de-emphasise with the weight
	}
	s.warn = fg(s.pal.warn)
	s.err = fg(s.pal.err)
	s.succ = fg(s.pal.success)
	s.border = fg(s.pal.border)
	s.focusBorder = fg(s.pal.focus)
	s.title = lipgloss.NewStyle().Bold(true)
	s.titleFocus = fg(s.pal.focus).Bold(true).Underline(true)
	return s
}

// selected returns st restyled as the cursor row of a focused list: a
// background fill in colour terminals, reverse video otherwise. A cell that
// carries its own foreground keeps it.
func (s styles) selected(st lipgloss.Style) lipgloss.Style {
	if s.mode == modeANSI || s.mode == modeMono {
		return st.Reverse(true).Bold(true)
	}
	st = st.Background(s.pal.selBg).Bold(true)
	// The dim token is unreadable on the selection fill: use the selection
	// foreground (bold keeps the emphasis).
	if fg := st.GetForeground(); fg == s.pal.dim {
		st = st.Foreground(s.pal.selFg)
	} else if _, none := fg.(lipgloss.NoColor); none {
		st = st.Foreground(s.pal.selFg)
	}
	return st
}

// glyphs are the symbols used by the UI, in UTF-8 and ASCII flavours.
type glyphs struct {
	ascii                                          bool
	cursor, active, current                        string
	warn, ok, fail, info                           string
	dot, crumb, ell, times, dash, arrow, shellMark string
	arrows, upDown                                 string
	border                                         lipgloss.Border
}

func newGlyphs(ascii bool) glyphs {
	if ascii {
		return glyphs{
			ascii: true, cursor: ">", active: "*", current: "@",
			warn: "!", ok: "+", fail: "x", info: "-",
			dot: "-", crumb: ">", ell: "~", times: "x", dash: "-", arrow: "->", shellMark: "$",
			arrows: "left right", upDown: "up down",
			border: lipgloss.ASCIIBorder(),
		}
	}
	return glyphs{
		cursor: "▸", active: "●", current: "★",
		warn: "⚠", ok: "✓", fail: "✗", info: "·",
		dot: "·", crumb: "›", ell: "…", times: "×", dash: "—", arrow: "→", shellMark: "$",
		arrows: "← →", upDown: "↑ ↓",
		border: lipgloss.RoundedBorder(),
	}
}

// useASCII reports whether the UI must avoid non-ASCII glyphs: KX_ASCII=1, or
// an effective locale (LC_ALL, then LC_CTYPE, then LANG) that is not UTF-8.
func useASCII() bool {
	if os.Getenv("KX_ASCII") == "1" {
		return true
	}
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(name); v != "" {
			v = strings.ToLower(v)
			return !strings.Contains(v, "utf-8") && !strings.Contains(v, "utf8")
		}
	}
	return true
}
