package mimex

// CONTRACT TEST for task card T-0008 (docs/tasks). Do not edit.

import (
	"testing"
	"unicode/utf8"
)

func TestPreview(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"Hello there.\nHow are you?", 100, "Hello there. How are you?"},
		{"Line one\r\nLine two", 100, "Line one Line two"},
		{"Thursday works.\n\nOn Tue, Oct 6, 2026 at 11:15 AM Bob <bob@x.test> wrote:\n> Are you free?\n> Lunch?", 100, "Thursday works."},
		{"Danke!\n\nAm 06.10.2026 um 11:15 schrieb Bob:\n\n> Hallo", 100, "Danke!"},
		{"Merci\n\nLe mar. 6 oct. 2026, Bob a écrit :\n> Salut", 100, "Merci"},
		{"Gracias\nEl mar, 6 oct 2026, Bob escribió:\n> Hola", 100, "Gracias"},
		{"Bis dann\n\nAm Di., 6. Okt. 2026 um 11:15 Uhr schrieb Bob <bob@x.test>:\n> Hallo", 100, "Bis dann"},
		{"Ok\nBob WROTE:\n> shouting", 100, "Ok"},
		{"Reply text\n> quoted\n   > indented quote\nmore reply", 100, "Reply text more reply"},
		{"A line ending wrote:\nnot followed by a quote", 100, "A line ending wrote: not followed by a quote"},
		{"Body\n-- \nBob Builder\nCEO", 100, "Body"},
		{"Body\n--\nsig", 100, "Body"},
		{"--not a signature", 100, "--not a signature"},
		{"Body\n-----Original Message-----\nFrom: x", 100, "Body"},
		{"Body\n   -----Original Message-----   \nFrom: x", 100, "Body"},
		{"Body\n________________________________\nFrom: x", 100, "Body"},
		{"Body\n_________\nnine underscores are text", 100, "Body _________ nine underscores are text"},
		{"> only quoted\n> lines", 100, ""},
		{"   \n\n  \t ", 100, ""},
		{"spaced\t\tout   words", 100, "spaced out words"},
		{"word word word word", 12, "word word…"},
		{"supercalifragilisticexpialidocious", 10, "supercali…"},
		{"日本語のテキストです", 5, "日本語の…"},
		{"exactly eleven", 14, "exactly eleven"},
		{"short", 0, ""},
		{"short", -3, ""},
	}
	for _, tc := range cases {
		got := Preview(tc.in, tc.max)
		if got != tc.want {
			t.Errorf("Preview(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
		}
		if tc.max > 0 && utf8.RuneCountInString(got) > tc.max {
			t.Errorf("Preview(%q, %d) has %d runes", tc.in, tc.max, utf8.RuneCountInString(got))
		}
	}
}
