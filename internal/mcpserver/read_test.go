package mcpserver

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCut(t *testing.T) {
	for _, c := range []struct {
		s    string
		n    int
		want string
		cut  bool
	}{
		{"short", 10, "short", false},
		{"exactly", 7, "exactly", false},
		{"abcdef", 3, "abc", true},
		{"aé", 2, "a", true},   // é is two bytes: never split
		{"éé", 3, "é", true},   // the second é would end at 4
		{"€x", 2, "", true},    // € is three bytes
		{"a€", 4, "a€", false}, // whole
	} {
		got, cut := cut(c.s, c.n)
		if got != c.want || cut != c.cut || !utf8.ValidString(got) {
			t.Errorf("cut(%q, %d) = %q, %v; want %q, %v", c.s, c.n, got, cut, c.want, c.cut)
		}
	}
	text, cutOff := cut(strings.Repeat("é", maxText), maxText)
	if !cutOff || len(text) != maxText || !utf8.ValidString(text) {
		t.Errorf("a text of twice 32 KiB = %d bytes, %v; want 32 KiB of whole characters", len(text), cutOff)
	}
}
