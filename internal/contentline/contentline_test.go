package contentline

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

const card = "BEGIN:VCARD\r\n" +
	"VERSION:3.0\r\n" +
	"N:Doe;Jane;Q.\\, Jr;;\r\n" +
	"FN:Jane Doe\r\n" +
	"item1.EMAIL;type=INTERNET;type=pref:jane@example.com\r\n" +
	"item1.X-ABLabel:_$!<Other>!$_\r\n" +
	"TEL;TYPE=work,voice:+1 555 0100\r\n" +
	"NOTE:line one\\nline two\\, with comma\r\n" +
	" and a fold\r\n" +
	"X-GROUPED;X-P=\"a:b;c\";X-Q=^'q^'^n:v\r\n" +
	"END:VCARD\r\n"

func TestParseCard(t *testing.T) {
	cs, err := Parse([]byte(card))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].Name != "VCARD" || len(cs[0].Props) != 8 {
		t.Fatalf("components = %+v", cs)
	}
	c := cs[0]
	if c.Start != 0 || c.End != len(card) {
		t.Errorf("span = [%d,%d), want [0,%d)", c.Start, c.End, len(card))
	}
	n := c.Prop("N")
	if got := n.Fields(); !slices.Equal(got, []string{"Doe", "Jane", "Q., Jr", "", ""}) {
		t.Errorf("N fields = %q", got)
	}
	email := c.Prop("EMAIL")
	if email.Group != "item1" || email.Value != "jane@example.com" {
		t.Errorf("EMAIL = %+v", email)
	}
	if !email.HasParam("TYPE", "pref") || !email.HasParam("type", "internet") || email.HasParam("TYPE", "home") {
		t.Errorf("EMAIL types = %q", email.ParamValues("TYPE"))
	}
	if got := c.Prop("TEL").ParamValues("TYPE"); !slices.Equal(got, []string{"work", "voice"}) {
		t.Errorf("TEL types = %q", got)
	}
	note := c.Prop("NOTE")
	if got := note.Text(); got != "line one\nline two, with commaand a fold" {
		t.Errorf("NOTE = %q", got)
	}
	if got := card[note.Start:note.End]; got != "NOTE:line one\\nline two\\, with comma\r\n and a fold\r\n" {
		t.Errorf("NOTE span = %q", got)
	}
	x := c.Prop("X-GROUPED")
	if x.Param("X-P") != "a:b;c" || x.Param("x-q") != "\"q\"\n" || x.Value != "v" {
		t.Errorf("X-GROUPED = %+v", x)
	}
}

func TestParseNested(t *testing.T) {
	src := "BEGIN:VCALENDAR\nVERSION:2.0\nBEGIN:VEVENT\nUID:1\nBEGIN:VALARM\nACTION:DISPLAY\nEND:VALARM\nEND:VEVENT\n\nBEGIN:VTODO\nUID:2\nEND:VTODO\nEND:VCALENDAR"
	cs, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	cal := cs[0]
	if len(cal.Children) != 2 || cal.End != len(src) {
		t.Fatalf("calendar = %+v", cal)
	}
	ev := cal.ChildrenNamed("VEVENT")[0]
	if ev.Prop("UID").Value != "1" || len(ev.ChildrenNamed("VALARM")) != 1 {
		t.Errorf("event = %+v", ev)
	}
	if got := src[ev.Start:ev.End]; !strings.HasPrefix(got, "BEGIN:VEVENT\n") || !strings.HasSuffix(got, "END:VEVENT\n") {
		t.Errorf("event span = %q", got)
	}
	if ev.Prop("SUMMARY") != nil || len(cal.PropsNamed("VERSION")) != 1 {
		t.Error("lookups")
	}
}

func TestParseBare21Types(t *testing.T) {
	cs, err := Parse([]byte("BEGIN:VCARD\nVERSION:2.1\nTEL;WORK;VOICE:1\nEND:VCARD\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cs[0].Prop("TEL").ParamValues("TYPE"); !slices.Equal(got, []string{"WORK", "VOICE"}) {
		t.Errorf("types = %q", got)
	}
}

func TestParseErrors(t *testing.T) {
	for _, src := range []string{
		"BEGIN:VCARD\nFN:x\n",                   // not closed
		"BEGIN:VCARD\nEND:VEVENT\n",             // wrong END
		"FN:x\n",                                // outside a component
		"BEGIN:VCARD\nno colon here\nEND:VCARD", // not a property
		" continued\n",                          // continuation first
		"BEGIN:VCARD\nX;P=\"open:v\nEND:VCARD",  // unterminated quote
		"BEGIN:VCARD\nBAD NAME:v\nEND:VCARD",    // space in a name
	} {
		if _, err := Parse([]byte(src)); !errors.Is(err, ErrSyntax) {
			t.Errorf("Parse(%q) = %v, want ErrSyntax", src, err)
		}
	}
}

func TestFold(t *testing.T) {
	long := "DESCRIPTION:" + strings.Repeat("é", 60) // 12 + 120 octets
	got := Fold(long, "\r\n")
	for i, l := range strings.Split(strings.TrimSuffix(got, "\r\n"), "\r\n") {
		if len(l) > 75 {
			t.Errorf("line %d is %d octets", i, len(l))
		}
		if i > 0 && l[0] != ' ' {
			t.Errorf("line %d does not start with a space", i)
		}
	}
	cs, err := Parse([]byte("BEGIN:X\r\n" + got + "END:X\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if v := cs[0].Prop("DESCRIPTION").Value; v != strings.Repeat("é", 60) {
		t.Errorf("unfolded = %q", v)
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	p := Prop{
		Name:   "ATTENDEE",
		Params: []Param{{Name: "CN", Values: []string{`Doe, "JD"`}}, {Name: "PARTSTAT", Values: []string{"ACCEPTED"}}},
		Value:  "mailto:jd@example.com",
	}
	enc := p.Encode("\r\n")
	if want := "ATTENDEE;CN=\"Doe, ^'JD^'\";PARTSTAT=ACCEPTED:mailto:jd@example.com\r\n"; enc != want {
		t.Errorf("Encode = %q, want %q", enc, want)
	}
	cs, err := Parse([]byte("BEGIN:VEVENT\r\n" + enc + "END:VEVENT\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := cs[0].Prop("ATTENDEE")
	if got.Param("CN") != `Doe, "JD"` || got.Param("PARTSTAT") != "ACCEPTED" || got.Value != p.Value {
		t.Errorf("round trip = %+v", got)
	}
	text := "a\\b;c,d\ne"
	q := Prop{Name: "NOTE", Value: EscapeText(text)}
	if q.Text() != text {
		t.Errorf("Text(EscapeText(%q)) = %q", text, q.Text())
	}
	r := Prop{Name: "N", Value: JoinFields("Doe", "Jane", "a;b")}
	if got := r.Fields(); !slices.Equal(got, []string{"Doe", "Jane", "a;b"}) {
		t.Errorf("Fields(JoinFields) = %q", got)
	}
}

func TestApply(t *testing.T) {
	src := []byte(card)
	cs, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	c := cs[0]
	fn, tel := c.Prop("FN"), c.Prop("TEL")
	out, err := Apply(src,
		Edit{Start: tel.Start, End: tel.End},                           // delete
		Edit{Start: fn.Start, End: fn.End, Text: "FN:Jane Q. Doe\r\n"}, // replace
		Edit{Start: c.End - len("END:VCARD\r\n"), End: c.End - len("END:VCARD\r\n"), Text: "NICKNAME:JQ\r\n"},
	)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(card, "FN:Jane Doe\r\n", "FN:Jane Q. Doe\r\n", 1)
	want = strings.Replace(want, "TEL;TYPE=work,voice:+1 555 0100\r\n", "", 1)
	want = strings.Replace(want, "END:VCARD", "NICKNAME:JQ\r\nEND:VCARD", 1)
	if string(out) != want {
		t.Errorf("Apply =\n%s\nwant\n%s", out, want)
	}
	if _, err := Apply(src, Edit{Start: 0, End: 10}, Edit{Start: 5, End: 12}); err == nil {
		t.Error("overlapping edits applied")
	}
	if LineEnding(src) != "\r\n" || LineEnding([]byte("A:b\nC:d\n")) != "\n" {
		t.Error("LineEnding")
	}
}
