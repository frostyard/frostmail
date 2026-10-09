package itip_test

// CONTRACT TEST for task card T-0087 (docs/tasks). Do not edit.

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/contentline"
	"github.com/frostyard/frostmail/internal/itip"
)

const user = "test1@mailtest.test"

// overrideKey is series.ics's moved occurrence: 09:15 in Berlin on Oct 14.
const overrideKey = "2026-10-14T07:15:00.000Z"

var now = time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)

func read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// vcalendar parses src's only top-level component, a VCALENDAR.
func vcalendar(t *testing.T, src []byte) *contentline.Component {
	t.Helper()
	cs, err := contentline.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, src)
	}
	if len(cs) != 1 || cs[0].Name != "VCALENDAR" {
		t.Fatalf("top-level components = %d, want one VCALENDAR", len(cs))
	}
	return cs[0]
}

func value(c *contentline.Component, name string) string {
	if p := c.Prop(name); p != nil {
		return p.Value
	}
	return ""
}

// attendee is c's ATTENDEE line for email, or nil.
func attendee(c *contentline.Component, email string) *contentline.Prop {
	for _, p := range c.PropsNamed("ATTENDEE") {
		if strings.EqualFold(strings.TrimPrefix(strings.ToLower(p.Value), "mailto:"), email) {
			return &p
		}
	}
	return nil
}

// sameProp reports whether two properties have the same parameters and
// value.
func sameProp(a, b *contentline.Prop) bool {
	return a != nil && b != nil && a.Value == b.Value && reflect.DeepEqual(a.Params, b.Params)
}

// checkLines fails unless every line of out ends in eol and is at most 75
// octets long without it.
func checkLines(t *testing.T, out []byte, eol string) {
	t.Helper()
	if !bytes.HasSuffix(out, []byte(eol)) {
		t.Errorf("the object does not end in %q", eol)
	}
	for line := range strings.SplitSeq(strings.TrimSuffix(string(out), eol), eol) {
		if strings.ContainsAny(line, "\r\n") {
			t.Errorf("a line ends other than in %q: %q", eol, line)
		}
		if len(line) > 75 {
			t.Errorf("a line is %d octets: %q", len(line), line)
		}
	}
}

func TestFromMessage(t *testing.T) {
	for _, tc := range []struct{ eml, ics string }{
		{"invite.eml", "google-request.ics"},      // text/calendar wins over an attachment
		{"attachment.eml", "outlook-request.ics"}, // application/ics
		{"octet.eml", "series.ics"},               // a part named .ics
	} {
		got, err := itip.FromMessage(read(t, tc.eml))
		if err != nil || !bytes.Equal(got, read(t, tc.ics)) {
			t.Errorf("%s = %q, %v; want %s", tc.eml, got, err, tc.ics)
		}
	}
	if _, err := itip.FromMessage(read(t, "plain.eml")); !errors.Is(err, itip.ErrNoInvitation) {
		t.Errorf("plain.eml: %v", err)
	}
}

func TestParse(t *testing.T) {
	opts := calendar.Options{Local: time.UTC, UserEmails: []string{user}}
	m, err := itip.Parse(read(t, "google-request.ics"), opts)
	if err != nil {
		t.Fatal(err)
	}
	main := m.Main()
	if m.Method != itip.MethodRequest || len(m.Events) != 1 || main.UID != "7kq3h8b2f9a1c0e4d5@google.com" ||
		main.Sequence != 0 || main.PartStat != "needsaction" || !main.Start.Equal(time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("google = %s %+v", m.Method, main)
	}
	if !bytes.Equal(m.Source, read(t, "google-request.ics")) {
		t.Error("Source is not the object as given")
	}
	m, _ = itip.Parse(read(t, "outlook-request.ics"), opts)
	if main := m.Main(); m.Method != itip.MethodRequest || main.Sequence != 1 || main.PartStat != "needsaction" ||
		!main.Start.Equal(time.Date(2026, 10, 20, 18, 0, 0, 0, time.UTC)) {
		t.Errorf("outlook = %s %+v", m.Method, main)
	}
	m, _ = itip.Parse(read(t, "series.ics"), opts)
	if len(m.Events) != 2 || m.Main().RecurrenceID != "" || m.Main().Summary != "Standup" ||
		m.Events[1].RecurrenceID != overrideKey || m.Main().PartStat != "tentative" {
		t.Errorf("series = %+v", m.Events)
	}
	for name, method := range map[string]string{"cancel.ics": itip.MethodCancel, "reply.ics": itip.MethodReply} {
		if m, err := itip.Parse(read(t, name), opts); err != nil || m.Method != method {
			t.Errorf("%s = %+v, %v", name, m, err)
		}
	}
	event := "BEGIN:VEVENT\r\nUID:x\r\nDTSTART:20261015T140000Z\r\nEND:VEVENT\r\n"
	for method, want := range map[string]string{"": itip.MethodPublish, "METHOD:PUBLISH\r\n": itip.MethodPublish,
		"METHOD:COUNTER\r\n": itip.MethodOther, "METHOD:REFRESH\r\n": itip.MethodOther} {
		src := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\n" + method + event + "END:VCALENDAR\r\n"
		if m, err := itip.Parse([]byte(src), opts); err != nil || m.Method != want {
			t.Errorf("%q: %+v, %v", method, m, err)
		}
	}
	if _, err := itip.Parse([]byte("BEGIN:VCALENDAR\r\nMETHOD:REQUEST\r\nEND:VCALENDAR\r\n"), opts); !errors.Is(err, itip.ErrNoEvent) {
		t.Errorf("no event: %v", err)
	}
	if _, err := itip.Parse([]byte("not a calendar"), opts); err == nil {
		t.Error("garbage parsed")
	}
}

func TestReply(t *testing.T) {
	src := read(t, "google-request.ics")
	out, err := itip.Reply(src, "", user, "accepted", "", now)
	if err != nil {
		t.Fatal(err)
	}
	checkLines(t, out, "\r\n")
	cal, orig := vcalendar(t, out), vcalendar(t, src)
	if value(cal, "METHOD") != "REPLY" || value(cal, "VERSION") != "2.0" || value(cal, "PRODID") == "" {
		t.Errorf("calendar properties: %s", out)
	}
	events := cal.ChildrenNamed("VEVENT")
	if len(events) != 1 {
		t.Fatalf("VEVENTs = %d", len(events))
	}
	ev, was := events[0], orig.ChildrenNamed("VEVENT")[0]
	for _, name := range []string{"UID", "SEQUENCE", "DTSTART", "DTEND", "ORGANIZER", "SUMMARY"} {
		if !sameProp(ev.Prop(name), was.Prop(name)) {
			t.Errorf("%s = %+v, want %+v", name, ev.Prop(name), was.Prop(name))
		}
	}
	if value(ev, "DTSTAMP") != "20261009T120000Z" {
		t.Errorf("DTSTAMP = %q", value(ev, "DTSTAMP"))
	}
	atts := ev.PropsNamed("ATTENDEE")
	if len(atts) != 1 {
		t.Fatalf("ATTENDEEs = %+v", atts)
	}
	a := atts[0]
	if a.Value != "mailto:"+user || a.Param("PARTSTAT") != "ACCEPTED" || a.Param("RSVP") != "" || a.Param("CN") != "Test One" {
		t.Errorf("ATTENDEE = %+v", a)
	}
	for _, name := range []string{"RECURRENCE-ID", "COMMENT", "DESCRIPTION", "X-GOOGLE-CONFERENCE"} {
		if ev.Prop(name) != nil {
			t.Errorf("%s is in the reply", name)
		}
	}
	if len(ev.ChildrenNamed("VALARM")) != 0 {
		t.Error("an alarm is in the reply")
	}
	zones := cal.ChildrenNamed("VTIMEZONE")
	if len(zones) != 1 || value(zones[0], "TZID") != "America/New_York" ||
		len(zones[0].ChildrenNamed("STANDARD")) != 1 || len(zones[0].ChildrenNamed("DAYLIGHT")) != 1 {
		t.Errorf("VTIMEZONEs = %+v", zones)
	}
	events2, err := calendar.Parse(out, calendar.Options{Local: time.UTC, UserEmails: []string{user}})
	if err != nil || len(events2) != 1 || events2[0].PartStat != "accepted" ||
		!events2[0].Start.Equal(time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("the reply reads as %+v, %v", events2, err)
	}
}

func TestReplyKeepsTheAttendeeAsWritten(t *testing.T) {
	out, err := itip.Reply(read(t, "outlook-request.ics"), "", user, "tentative", "Sure, if it is short", now)
	if err != nil {
		t.Fatal(err)
	}
	checkLines(t, out, "\r\n")
	cal := vcalendar(t, out)
	ev := cal.ChildrenNamed("VEVENT")[0]
	atts := ev.PropsNamed("ATTENDEE")
	if len(atts) != 1 || atts[0].Value != "MAILTO:Test1@MailTest.test" || atts[0].Param("PARTSTAT") != "TENTATIVE" ||
		atts[0].Param("ROLE") != "REQ-PARTICIPANT" || atts[0].Param("RSVP") != "" {
		t.Errorf("ATTENDEE = %+v", atts)
	}
	if c := ev.Prop("COMMENT"); c == nil || c.Value != `Sure\, if it is short` || c.Text() != "Sure, if it is short" {
		t.Errorf("COMMENT = %+v", c)
	}
	if value(ev, "SEQUENCE") != "1" || ev.Prop("DTSTART").Param("TZID") != "Eastern Standard Time" {
		t.Errorf("event: %s", out)
	}
	if zones := cal.ChildrenNamed("VTIMEZONE"); len(zones) != 1 || value(zones[0], "TZID") != "Eastern Standard Time" {
		t.Errorf("VTIMEZONEs = %+v", zones)
	}
}

func TestReplyToASeries(t *testing.T) {
	src := read(t, "series.ics")
	out, err := itip.Reply(src, "", user, "declined", "", now)
	if err != nil {
		t.Fatal(err)
	}
	checkLines(t, out, "\r\n")
	cal := vcalendar(t, out)
	events := cal.ChildrenNamed("VEVENT")
	if len(events) != 1 || events[0].Prop("RECURRENCE-ID") != nil || value(events[0], "DTSTART") != "20261012T091500" ||
		value(events[0], "SEQUENCE") != "2" || attendee(events[0], user).Param("PARTSTAT") != "DECLINED" {
		t.Errorf("series reply: %s", out)
	}
	if len(cal.ChildrenNamed("VTIMEZONE")) != 0 {
		t.Error("a VTIMEZONE the source does not have")
	}

	out, err = itip.Reply(src, overrideKey, user, "accepted", "", now)
	if err != nil {
		t.Fatal(err)
	}
	events = vcalendar(t, out).ChildrenNamed("VEVENT")
	if len(events) != 1 || value(events[0], "RECURRENCE-ID") != "20261014T091500" ||
		events[0].Prop("RECURRENCE-ID").Param("TZID") != "Europe/Berlin" || value(events[0], "DTSTART") != "20261014T110000" {
		t.Fatalf("occurrence reply: %s", out)
	}
	if a := attendee(events[0], user); a == nil || a.Param("PARTSTAT") != "ACCEPTED" || a.Param("CN") != "Test One" {
		t.Errorf("ATTENDEE = %+v", a)
	}
}

func TestReplyRefuses(t *testing.T) {
	src := read(t, "google-request.ics")
	if _, err := itip.Reply(src, "", "nobody@example.com", "accepted", "", now); !errors.Is(err, itip.ErrNotInvited) {
		t.Errorf("a stranger: %v", err)
	}
	if _, err := itip.Reply(src, "", user, "needsaction", "", now); !errors.Is(err, itip.ErrAnswer) {
		t.Errorf("no answer: %v", err)
	}
	if _, err := itip.Reply(read(t, "series.ics"), "2026-10-16T07:15:00.000Z", user, "accepted", "", now); !errors.Is(err, itip.ErrNoOccurrence) {
		t.Errorf("an occurrence without its own VEVENT: %v", err)
	}
}

// withoutAttendee drops email's ATTENDEE lines from every VEVENT of src.
func withoutAttendee(t *testing.T, src []byte, email string) string {
	t.Helper()
	var edits []contentline.Edit
	for _, ev := range vcalendar(t, src).ChildrenNamed("VEVENT") {
		if a := attendee(ev, email); a != nil {
			edits = append(edits, contentline.Edit{Start: a.Start, End: a.End})
		}
	}
	out, err := contentline.Apply(src, edits...)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestSetPartStat(t *testing.T) {
	src := read(t, "google-request.ics")
	out, err := itip.SetPartStat(src, "", user, "accepted")
	if err != nil {
		t.Fatal(err)
	}
	if withoutAttendee(t, out, user) != withoutAttendee(t, src, user) {
		t.Errorf("more than the attendee's line changed:\n%s", out)
	}
	checkLines(t, out, "\r\n")
	ev, was := vcalendar(t, out).ChildrenNamed("VEVENT")[0], vcalendar(t, src).ChildrenNamed("VEVENT")[0]
	a, b := attendee(ev, user), attendee(was, user)
	if a == nil || a.Param("PARTSTAT") != "ACCEPTED" || a.Param("RSVP") != "" || a.Value != b.Value {
		t.Fatalf("ATTENDEE = %+v", a)
	}
	for _, name := range []string{"CUTYPE", "ROLE", "CN", "X-NUM-GUESTS"} {
		if a.Param(name) != b.Param(name) {
			t.Errorf("%s = %q, want %q", name, a.Param(name), b.Param(name))
		}
	}
	events, _ := calendar.Parse(out, calendar.Options{Local: time.UTC, UserEmails: []string{user}})
	if events[0].PartStat != "accepted" {
		t.Errorf("reads as %q", events[0].PartStat)
	}

	out, err = itip.SetPartStat(read(t, "outlook-request.ics"), "", user, "declined")
	if err != nil {
		t.Fatal(err)
	}
	if a := attendee(vcalendar(t, out).ChildrenNamed("VEVENT")[0], user); a == nil ||
		a.Value != "MAILTO:Test1@MailTest.test" || a.Param("PARTSTAT") != "DECLINED" {
		t.Errorf("outlook: %+v", a)
	}
}

func TestSetPartStatOfASeries(t *testing.T) {
	src := read(t, "series.ics") // LF line endings, kept
	opts := calendar.Options{Local: time.UTC, UserEmails: []string{user}}
	out, err := itip.SetPartStat(src, "", user, "declined")
	if err != nil {
		t.Fatal(err)
	}
	checkLines(t, out, "\n")
	if withoutAttendee(t, out, user) != withoutAttendee(t, src, user) {
		t.Errorf("more than the attendee's lines changed:\n%s", out)
	}
	events, _ := calendar.Parse(out, opts)
	if len(events) != 2 || events[0].PartStat != "declined" || events[1].PartStat != "declined" {
		t.Errorf("the series reads as %+v", events)
	}

	out, err = itip.SetPartStat(src, overrideKey, user, "accepted")
	if err != nil {
		t.Fatal(err)
	}
	events, _ = calendar.Parse(out, opts)
	if events[0].PartStat != "tentative" || events[1].PartStat != "accepted" {
		t.Errorf("one occurrence: %+v", events)
	}
	master := vcalendar(t, out).ChildrenNamed("VEVENT")[0]
	if !sameProp(attendee(master, user), attendee(vcalendar(t, src).ChildrenNamed("VEVENT")[0], user)) {
		t.Error("the master's line changed")
	}

	if _, err := itip.SetPartStat(src, "2026-10-16T07:15:00.000Z", user, "accepted"); !errors.Is(err, itip.ErrNoOccurrence) {
		t.Errorf("unknown occurrence: %v", err)
	}
	if _, err := itip.SetPartStat(src, "", "nobody@example.com", "accepted"); !errors.Is(err, itip.ErrNotInvited) {
		t.Errorf("a stranger: %v", err)
	}
	if _, err := itip.SetPartStat(src, "", user, "maybe"); !errors.Is(err, itip.ErrAnswer) {
		t.Errorf("a bad answer: %v", err)
	}
}

func TestForCalendar(t *testing.T) {
	src := read(t, "google-request.ics")
	out, err := itip.ForCalendar(src)
	if err != nil || string(out) != strings.Replace(string(src), "METHOD:REQUEST\r\n", "", 1) {
		t.Errorf("ForCalendar = %s, %v", out, err)
	}
	plain := []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:x\r\nDTSTART:20261015T140000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	if out, err := itip.ForCalendar(plain); err != nil || !bytes.Equal(out, plain) {
		t.Errorf("without METHOD = %s, %v", out, err)
	}
}
