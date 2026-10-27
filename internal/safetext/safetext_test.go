package safetext

import "testing"

func TestClean(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"plain-name_1.x", "plain-name_1.x"},
		{"héllo ✓ 日本", "héllo ✓ 日本"},
		{"evil\x1b]0;pwned\x07\x1b[2J", "evil"},
		{"a\x1b[31mred\x1b[0mb", "aredb"},
		{"a\tb\nc\r\nd", "a b c  d"},
		{"x\x00y\x07z\x7fw\u0085v\u009bu", "xyzwvu"},
		{"\x1b", ""},
		{"trail\x1b[", "trail"},
		{"gpj.\u202eexe", "gpj.exe"},
		{"a\u200eb\u200fc\u2066d\u2069e", "abcde"},
		{"a\u200bb\ufeffc", "abc"},
		{"line\u2028sep\u2029para", "line sep para"},
		{"\U0001F468\u200d\U0001F469", "\U0001F468\u200d\U0001F469"},
	} {
		if got := Clean(c.in); got != c.want {
			t.Errorf("Clean(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
