// Package safetext makes text from kubeconfig files safe to print on a
// terminal. Imported files are untrusted: a context name may carry escape
// sequences that rewrite the title, clear the screen or fake output.
package safetext

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Clean removes escape sequences and control characters from s. Tabs and line
// breaks become a space; printable Unicode is kept. Unicode format characters
// (bidi overrides such as U+202E, U+200E marks, zero-width spaces) are dropped
// because they can reorder or hide text; U+200D (zero width joiner) stays so
// emoji sequences keep working. U+2028 and U+2029 are line breaks and become
// a space.
func Clean(s string) string {
	if s == "" {
		return s
	}
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r' || r == '\u2028' || r == '\u2029':
			return ' '
		case unicode.IsControl(r):
			return -1
		case r == '\u200d':
			return r
		case unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, s)
}
