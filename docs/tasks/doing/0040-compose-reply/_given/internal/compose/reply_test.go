package compose

// CONTRACT TEST for task card T-0040 (docs/tasks). Do not edit.

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

var (
	ann   = Address{Name: "Ann Smith", Addr: "ann@x.test"}
	bob   = Address{Name: "Bob", Addr: "bob@x.test"}
	carol = Address{Addr: "carol@x.test"}
	me    = Address{Name: "Test One", Addr: "Test1@MailTest.test"}
	list  = Address{Name: "Team", Addr: "team@x.test"}
)

func TestReplyRecipients(t *testing.T) {
	self := []string{"test1@mailtest.test", "alias@mailtest.test"}
	cases := []struct {
		name   string
		src    Source
		all    bool
		to, cc []Address
	}{
		{"reply goes to the sender", Source{From: ann, To: []Address{me, bob}, Cc: []Address{carol}}, false,
			[]Address{ann}, nil},
		{"Reply-To wins over From", Source{From: ann, ReplyTo: []Address{list}, To: []Address{me}}, false,
			[]Address{list}, nil},
		{"reply all copies the other recipients, not me", Source{From: ann, To: []Address{me, bob}, Cc: []Address{carol, {Addr: "ALIAS@mailtest.test"}}}, true,
			[]Address{ann}, []Address{bob, carol}},
		{"reply all skips duplicates of To and of each other", Source{From: ann, To: []Address{ann, bob}, Cc: []Address{{Name: "B", Addr: "BOB@x.test"}, carol}}, true,
			[]Address{ann}, []Address{bob, carol}},
		{"replying to my own message goes to its recipients", Source{From: me, To: []Address{bob, carol}}, false,
			[]Address{bob, carol}, nil},
		{"reply all to my own message", Source{From: me, To: []Address{bob}, Cc: []Address{carol, me}}, true,
			[]Address{bob}, []Address{carol}},
	}
	for _, tc := range cases {
		to, cc := ReplyRecipients(tc.src, self, tc.all)
		if !reflect.DeepEqual(to, tc.to) || !reflect.DeepEqual(cc, tc.cc) {
			t.Errorf("%s:\n to %v cc %v\nwant to %v cc %v", tc.name, to, cc, tc.to, tc.cc)
		}
	}
}

func TestSubjects(t *testing.T) {
	reply := map[string]string{
		"Plan":          "Re: Plan",
		"  Plan  ":      "Re: Plan",
		"Re: Plan":      "Re: Plan",
		"RE: Plan":      "RE: Plan",
		"re:Plan":       "re:Plan",
		"AW: Plan":      "AW: Plan",
		"Sv: Plan":      "Sv: Plan",
		"":              "Re: ",
		"Fwd: Plan":     "Re: Fwd: Plan",
		"Reply to Plan": "Re: Reply to Plan",
		"Release notes": "Re: Release notes",
	}
	for in, want := range reply {
		if got := ReplySubject(in); got != want {
			t.Errorf("ReplySubject(%q) = %q, want %q", in, got, want)
		}
	}
	forward := map[string]string{
		"Plan":       "Fwd: Plan",
		"Fwd: Plan":  "Fwd: Plan",
		"FW: Plan":   "FW: Plan",
		"fwd:Plan":   "fwd:Plan",
		"Re: Plan":   "Fwd: Re: Plan",
		"":           "Fwd: ",
		"Forward Me": "Fwd: Forward Me",
	}
	for in, want := range forward {
		if got := ForwardSubject(in); got != want {
			t.Errorf("ForwardSubject(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReplyReferences(t *testing.T) {
	cases := []struct {
		src  Source
		want []string
	}{
		{Source{MessageID: "a@x"}, []string{"a@x"}},
		{Source{MessageID: "c@x", References: []string{"a@x", "b@x"}}, []string{"a@x", "b@x", "c@x"}},
		{Source{MessageID: "b@x", References: []string{"a@x", "b@x"}}, []string{"a@x", "b@x"}},
		{Source{References: []string{"a@x", "a@x", "b@x"}}, []string{"a@x", "b@x"}},
		{Source{}, nil},
	}
	for _, tc := range cases {
		if got := ReplyReferences(tc.src); !slices.Equal(got, tc.want) {
			t.Errorf("ReplyReferences(%+v) = %v, want %v", tc.src, got, tc.want)
		}
	}
	var long []string
	for i := range 30 {
		long = append(long, string(rune('a'+i%26))+strings.Repeat("x", i/26)+"@x")
	}
	got := ReplyReferences(Source{MessageID: "new@x", References: long})
	if len(got) != 20 || got[0] != long[0] || got[19] != "new@x" || got[18] != long[29] {
		t.Fatalf("long References = %d entries %v; want the first, then the latest 19 ending with the source", len(got), got)
	}
}

func TestAttribution(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no time zone database")
	}
	date := time.Date(2026, 10, 5, 13, 41, 0, 0, time.UTC)
	if got, want := Attribution(date, "Ann Smith", ny), "On Mon, Oct 5, 2026 at 9:41 AM, Ann Smith wrote:"; got != want {
		t.Fatalf("Attribution = %q, want %q", got, want)
	}
	if got, want := Attribution(date, "", time.UTC), "On Mon, Oct 5, 2026 at 1:41 PM, someone wrote:"; got != want {
		t.Fatalf("Attribution without a name = %q, want %q", got, want)
	}
}

func TestQuoteHTML(t *testing.T) {
	date := time.Date(2026, 10, 5, 13, 41, 0, 0, time.UTC)
	html := QuoteHTML(Source{From: Address{Name: "Ann <A&B>", Addr: "ann@x.test"}, Date: date, HTML: "<p>Hi <b>team</b></p>"}, time.UTC)
	want := `<p><br></p><p>On Mon, Oct 5, 2026 at 1:41 PM, Ann &lt;A&amp;B&gt; wrote:</p><blockquote type="cite"><p>Hi <b>team</b></p></blockquote>`
	if html != want {
		t.Fatalf("QuoteHTML (HTML source) =\n%s\nwant\n%s", html, want)
	}
	text := QuoteHTML(Source{From: Address{Addr: "bob@x.test"}, Date: date, Text: "a < b\n\nsee & you"}, time.UTC)
	want = `<p><br></p><p>On Mon, Oct 5, 2026 at 1:41 PM, bob@x.test wrote:</p><blockquote type="cite"><p>a &lt; b<br><br>see &amp; you</p></blockquote>`
	if text != want {
		t.Fatalf("QuoteHTML (text source) =\n%s\nwant\n%s", text, want)
	}
}

func TestForwardHTML(t *testing.T) {
	date := time.Date(2026, 10, 5, 13, 41, 0, 0, time.UTC)
	src := Source{
		From: ann, To: []Address{bob, carol}, Subject: "Q3 <plan>", Date: date, Text: "Numbers attached.",
	}
	want := `<p><br></p><p>Begin forwarded message:</p><blockquote type="cite">` +
		`<p><b>From:</b> Ann Smith &lt;ann@x.test&gt;<br><b>Subject:</b> Q3 &lt;plan&gt;<br>` +
		`<b>Date:</b> October 5, 2026 at 1:41 PM<br><b>To:</b> Bob &lt;bob@x.test&gt;, carol@x.test</p>` +
		`<p>Numbers attached.</p></blockquote>`
	if got := ForwardHTML(src, time.UTC); got != want {
		t.Fatalf("ForwardHTML =\n%s\nwant\n%s", got, want)
	}
}
