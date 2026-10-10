package itip

import (
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

// Reply writes the iTIP REPLY that tells the organizer attendee's answer
// (accepted, declined or tentative) to the event in src, an invitation or a
// stored event: its single event or master when recurrenceID is "", else
// the VEVENT with that recurrence ID (as calendar.Event.RecurrenceID
// formats it). comment, when not empty, is a note to the organizer.
func Reply(src []byte, recurrenceID, attendee, answer, comment string, now time.Time) ([]byte, error) {
	partStat, err := answerPartStat(answer)
	if err != nil {
		return nil, err
	}
	cal, err := firstCalendar(src)
	if err != nil {
		return nil, err
	}
	events, err := chosenEvents(cal, recurrenceID, false)
	if err != nil {
		return nil, err
	}
	event := events[0]
	person := answeredAttendee(event, attendee, partStat)
	if person == nil {
		return nil, ErrNotInvited
	}
	var out strings.Builder
	for _, p := range []contentline.Prop{
		{Name: "BEGIN", Value: "VCALENDAR"}, {Name: "VERSION", Value: "2.0"},
		{Name: "PRODID", Value: "-//Frostyard//Frostmail//EN"}, {Name: "METHOD", Value: "REPLY"},
	} {
		out.WriteString(p.Encode("\r\n"))
	}
	writeReplyZones(&out, src, cal, event)
	out.WriteString(contentline.Prop{Name: "BEGIN", Value: "VEVENT"}.Encode("\r\n"))
	for _, name := range []string{"UID", "RECURRENCE-ID", "SEQUENCE", "DTSTART", "DTEND", "DURATION", "SUMMARY", "ORGANIZER"} {
		p := event.Prop(name)
		if p == nil {
			continue
		}
		if clean, changed := withoutSchedule(*p); changed {
			out.WriteString(clean.Encode("\r\n"))
			continue
		}
		out.WriteString(copiedProperty(src, *p))
	}
	out.WriteString(contentline.Prop{Name: "DTSTAMP", Value: now.UTC().Format("20060102T150405Z")}.Encode("\r\n"))
	clean, _ := withoutSchedule(*person)
	out.WriteString(clean.Encode("\r\n"))
	if comment != "" {
		out.WriteString(contentline.Prop{Name: "COMMENT", Value: contentline.EscapeText(comment)}.Encode("\r\n"))
	}
	out.WriteString(contentline.Prop{Name: "END", Value: "VEVENT"}.Encode("\r\n"))
	out.WriteString(contentline.Prop{Name: "END", Value: "VCALENDAR"}.Encode("\r\n"))
	return []byte(out.String()), nil
}

// copiedProperty preserves the source spelling and quoting while refolding.
func copiedProperty(src []byte, p contentline.Prop) string {
	line := string(src[p.Start:p.End])
	unfold := strings.NewReplacer("\r\n ", "", "\r\n\t", "", "\n ", "", "\n\t", "")
	return contentline.Fold(strings.TrimRight(unfold.Replace(line), "\r\n"), "\r\n")
}

func writeReplyZones(out *strings.Builder, src []byte, cal, event *contentline.Component) {
	zones := make(map[string]bool)
	for _, name := range []string{"DTSTART", "DTEND", "RECURRENCE-ID"} {
		if p := event.Prop(name); p != nil && p.Param("TZID") != "" {
			zones[p.Param("TZID")] = true
		}
	}
	for _, zone := range cal.ChildrenNamed("VTIMEZONE") {
		id := zone.Prop("TZID")
		if id == nil || !zones[id.Value] {
			continue
		}
		text := strings.ReplaceAll(string(src[zone.Start:zone.End]), "\r\n", "\n")
		out.WriteString(strings.ReplaceAll(text, "\n", "\r\n"))
	}
}
