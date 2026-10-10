package itip_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/contentline"
	"github.com/frostyard/frostmail/internal/itip"
)

// agent is the SCHEDULE-AGENT of p, or "".
func agent(p *contentline.Prop) string {
	for _, q := range p.Params {
		if q.Name == "SCHEDULE-AGENT" && len(q.Values) == 1 {
			return q.Values[0]
		}
	}
	return ""
}

// TestScheduleByClient: a copy maild answers by mail tells the server to
// leave scheduling to the client on every event's ORGANIZER, and changes
// nothing else (ADR-0022).
func TestScheduleByClient(t *testing.T) {
	src, err := itip.ForCalendar(read(t, "series.ics"))
	if err != nil {
		t.Fatal(err)
	}
	if itip.ClientScheduled(src) {
		t.Fatal("a copy without SCHEDULE-AGENT is client-scheduled")
	}
	out, err := itip.ScheduleByClient(src)
	if err != nil {
		t.Fatal(err)
	}
	if !itip.ClientScheduled(out) {
		t.Errorf("not client-scheduled:\n%s", out)
	}
	n := 0
	for _, ev := range vcalendar(t, out).Children {
		if ev.Name != "VEVENT" {
			continue
		}
		n++
		if org := ev.Prop("ORGANIZER"); org == nil || agent(org) != "CLIENT" || org.Value != "mailto:maria@example.com" {
			t.Errorf("organizer = %+v", org)
		}
	}
	if n != 2 {
		t.Errorf("events = %d", n)
	}
	strip := func(b []byte) string {
		var keep []string
		for line := range strings.SplitSeq(string(b), "\n") {
			if !strings.HasPrefix(line, "ORGANIZER") {
				keep = append(keep, line)
			}
		}
		return strings.Join(keep, "\n")
	}
	if strip(out) != strip(src) {
		t.Errorf("more than the organizer changed:\n%s", out)
	}
	if again, err := itip.ScheduleByClient(out); err != nil || !bytes.Equal(again, out) {
		t.Errorf("again = %s, %v", again, err)
	}
	server := bytes.ReplaceAll(src, []byte("ORGANIZER;CN=Maria:"), []byte("ORGANIZER;CN=Maria;SCHEDULE-AGENT=SERVER:"))
	if itip.ClientScheduled(server) {
		t.Error("SCHEDULE-AGENT=SERVER is client-scheduled")
	}
	if over, err := itip.ScheduleByClient(server); err != nil || !bytes.Equal(over, out) {
		t.Errorf("over SERVER = %s, %v", over, err)
	}
}

// TestReplyLeavesScheduleParams: an iTIP message carries no SCHEDULE-*
// parameters, though the copy it is built from does.
func TestReplyLeavesScheduleParams(t *testing.T) {
	src, err := itip.ForCalendar(read(t, "series.ics"))
	if err != nil {
		t.Fatal(err)
	}
	src = bytes.ReplaceAll(src, []byte("ATTENDEE;CN=Test One;PARTSTAT=TENTATIVE:"),
		[]byte("ATTENDEE;CN=Test One;PARTSTAT=TENTATIVE;SCHEDULE-STATUS=2.0:"))
	copyOf, err := itip.ScheduleByClient(src)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := itip.Reply(copyOf, "", user, "accepted", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(reply), "SCHEDULE-") {
		t.Errorf("reply carries schedule parameters:\n%s", reply)
	}
	ev := vcalendar(t, reply).Children
	var event *contentline.Component
	for _, c := range ev {
		if c.Name == "VEVENT" {
			event = c
		}
	}
	if event == nil || event.Prop("ORGANIZER") == nil || event.Prop("ORGANIZER").Value != "mailto:maria@example.com" {
		t.Fatalf("reply = %s", reply)
	}
	if a := attendee(event, user); a == nil || a.Param("PARTSTAT") != "ACCEPTED" || a.Param("CN") != "Test One" {
		t.Errorf("attendee = %+v", a)
	}
}
