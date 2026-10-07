package mimex

// CONTRACT TEST for task card T-0006 (docs/tasks). Do not edit.

import (
	"slices"
	"testing"
)

func TestParseMessageIDs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"<a@b.test>", []string{"a@b.test"}},
		{"<a@b.test> <c@d.test>", []string{"a@b.test", "c@d.test"}},
		{"<a@b.test>\r\n\t<c@d.test>", []string{"a@b.test", "c@d.test"}},
		{"<a@b.test>,<c@d.test>", []string{"a@b.test", "c@d.test"}},
		{"<a@b.test><c@d.test>", []string{"a@b.test", "c@d.test"}},
		{"<a@b.test> <a@b.test> <c@d.test> <a@b.test>", []string{"a@b.test", "c@d.test"}},
		{"< a@b.test >", []string{"a@b.test"}},
		{"<foo\r\n bar@baz.test>", []string{"foobar@baz.test"}},
		{"<a@b.test> (comment <x@y.test>) <c@d.test>", []string{"a@b.test", "c@d.test"}},
		{"(outer (nested) still comment) <a@b.test>", []string{"a@b.test"}},
		{"<a@b.test> (unterminated <x@y.test>", []string{"a@b.test"}},
		{`Message from Bob <bob-1@host.test> of "Mon, 5 Oct 2026"`, []string{"bob-1@host.test"}},
		{"<a@b.test> <c@d.te", []string{"a@b.test", "c@d.te"}},
		{"<>", nil},
		{"<a@b.test> <> <c@d.test>", []string{"a@b.test", "c@d.test"}},
		{"a@b.test", []string{"a@b.test"}},
		{"a@b.test c@d.test,e@f.test", []string{"a@b.test", "c@d.test", "e@f.test"}},
		{"not an id", nil},
		{"", nil},
		{"   \r\n ", nil},
	}
	for _, tc := range cases {
		got := ParseMessageIDs(tc.in)
		if !slices.Equal(got, tc.want) {
			t.Errorf("ParseMessageIDs(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
