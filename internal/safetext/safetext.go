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
// breaks become a space; printable Unicode is kept.
func Clean(s string) string {
	if s == "" {
		return s
	}
	s = ansi.Strip(s)
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
}
