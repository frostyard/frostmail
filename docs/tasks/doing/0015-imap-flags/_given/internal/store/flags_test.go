package store

// CONTRACT TEST for task card T-0015 (docs/tasks). Do not edit.

import (
	"reflect"
	"testing"
)

func TestFlagsFromIMAP(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want Flags
	}{
		{"none", nil, Flags{}},
		{"system flags any case", []string{`\Seen`, `\ANSWERED`, `\flagged`, `\Draft`, `\Deleted`},
			Flags{Seen: true, Answered: true, Flagged: true, Draft: true, Deleted: true, Color: 1}},
		{"ignored system flags", []string{`\Recent`, `\*`, `\Unknown`}, Flags{}},
		{"forwarded", []string{`$Forwarded`}, Flags{Forwarded: true}},
		{"forwarded lowercase", []string{`$forwarded`}, Flags{Forwarded: true}},
		{"red flag has no bits", []string{`\Flagged`}, Flags{Flagged: true, Color: 1}},
		{"orange", []string{`\Flagged`, `$MailFlagBit0`}, Flags{Flagged: true, Color: 2}},
		{"yellow", []string{`\Flagged`, `$MailFlagBit1`}, Flags{Flagged: true, Color: 3}},
		{"blue", []string{`\Flagged`, `$mailflagbit2`}, Flags{Flagged: true, Color: 5}},
		{"gray", []string{`\Flagged`, `$MailFlagBit1`, `$MailFlagBit2`}, Flags{Flagged: true, Color: 7}},
		{"bits without the flag", []string{`$MailFlagBit0`}, Flags{}},
		{"keywords sorted and deduplicated", []string{`Work`, `$Junk`, `work`, ``, `Alpha`},
			Flags{Keywords: []string{`$Junk`, `Alpha`, `Work`}}},
	}
	for _, tc := range cases {
		if got := FlagsFromIMAP(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: FlagsFromIMAP(%q) = %+v, want %+v", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestIMAPFlags(t *testing.T) {
	cases := []struct {
		name string
		in   Flags
		want []string
	}{
		{"none", Flags{}, nil},
		{"order", Flags{Seen: true, Answered: true, Flagged: true, Deleted: true, Draft: true, Forwarded: true, Color: 1},
			[]string{`\Seen`, `\Answered`, `\Flagged`, `\Deleted`, `\Draft`, `$Forwarded`}},
		{"orange", Flags{Flagged: true, Color: 2}, []string{`\Flagged`, `$MailFlagBit0`}},
		{"gray", Flags{Flagged: true, Color: 7}, []string{`\Flagged`, `$MailFlagBit1`, `$MailFlagBit2`}},
		{"purple", Flags{Flagged: true, Color: 6}, []string{`\Flagged`, `$MailFlagBit0`, `$MailFlagBit2`}},
		{"color without the flag is dropped", Flags{Color: 4}, nil},
		{"keywords last", Flags{Seen: true, Keywords: []string{"$Junk", "Work"}}, []string{`\Seen`, "$Junk", "Work"}},
	}
	for _, tc := range cases {
		if got := tc.in.IMAPFlags(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %+v.IMAPFlags() = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestFlagsRoundTrip(t *testing.T) {
	for color := 1; color <= 7; color++ {
		f := Flags{Seen: true, Flagged: true, Color: color, Keywords: []string{"Work"}}
		if got := FlagsFromIMAP(f.IMAPFlags()); !reflect.DeepEqual(got, f) {
			t.Errorf("round trip of %+v = %+v", f, got)
		}
	}
}
