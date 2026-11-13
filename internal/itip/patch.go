package itip

import (
	"errors"
	"fmt"
	"strings"

	"github.com/frostyard/frostmail/internal/contentline"
)

// SetPartStat patches attendee's PARTSTAT to answer in src, changing only
// the attendee's ATTENDEE lines: in every VEVENT when recurrenceID is "",
// else in the VEVENT with that recurrence ID.
func SetPartStat(src []byte, recurrenceID, attendee, answer string) ([]byte, error) {
	partStat, err := answerPartStat(answer)
	if err != nil {
		return nil, err
	}
	cal, err := firstCalendar(src)
	if err != nil {
		return nil, err
	}
	events, err := chosenEvents(cal, recurrenceID, true)
	if err != nil {
		return nil, err
	}
	var edits []contentline.Edit
	for _, event := range events {
		if p := answeredAttendee(event, attendee, partStat); p != nil {
			edits = append(edits, contentline.Edit{Start: p.Start, End: p.End, Text: p.Encode(contentline.LineEnding(src))})
		}
	}
	if len(edits) == 0 {
		return nil, ErrNotInvited
	}
	out, err := contentline.Apply(src, edits...)
	if err != nil {
		return nil, fmt.Errorf("patch invitation attendee: %w", err)
	}
	return out, nil
}

// ForCalendar returns an invitation as a calendar object to store: src
// without its METHOD line, which calendar collections do not take.
func ForCalendar(src []byte) ([]byte, error) {
	cal, err := firstCalendar(src)
	if err != nil {
		return nil, err
	}
	var edits []contentline.Edit
	for _, p := range cal.PropsNamed("METHOD") {
		edits = append(edits, contentline.Edit{Start: p.Start, End: p.End})
	}
	if len(edits) == 0 {
		return src, nil
	}
	out, err := contentline.Apply(src, edits...)
	if err != nil {
		return nil, fmt.Errorf("remove invitation method: %w", err)
	}
	return out, nil
}

// ClientScheduled reports whether src leaves scheduling to the client:
// SCHEDULE-AGENT=CLIENT on an event's ORGANIZER (RFC 6638 §7.1), as the
// copies maild answers by mail carry (ADR-0022).
func ClientScheduled(src []byte) bool {
	cal, err := firstCalendar(src)
	if err != nil {
		return false
	}
	for _, event := range cal.Children {
		if event.Name != "VEVENT" {
			continue
		}
		if org := event.Prop("ORGANIZER"); org != nil && strings.EqualFold(org.Param("SCHEDULE-AGENT"), "CLIENT") {
			return true
		}
	}
	return false
}

// ScheduleByClient sets SCHEDULE-AGENT=CLIENT on every event's ORGANIZER
// in src, replacing another agent, so a server that schedules sends no
// message for the copy: maild mails the reply itself (ADR-0022). Only the
// ORGANIZER lines change.
func ScheduleByClient(src []byte) ([]byte, error) {
	cal, err := firstCalendar(src)
	if err != nil {
		return nil, err
	}
	var edits []contentline.Edit
	for _, event := range cal.Children {
		if event.Name != "VEVENT" {
			continue
		}
		org := event.Prop("ORGANIZER")
		if org == nil || org.Param("SCHEDULE-AGENT") == "CLIENT" {
			continue
		}
		p := *org
		p.Params = append(withoutParam(p.Params, "SCHEDULE-AGENT"), contentline.Param{Name: "SCHEDULE-AGENT", Values: []string{"CLIENT"}})
		edits = append(edits, contentline.Edit{Start: org.Start, End: org.End, Text: p.Encode(contentline.LineEnding(src))})
	}
	if len(edits) == 0 {
		return src, nil
	}
	out, err := contentline.Apply(src, edits...)
	if err != nil {
		return nil, fmt.Errorf("set the schedule agent: %w", err)
	}
	return out, nil
}

// withoutParam is params without the ones named name.
func withoutParam(params []contentline.Param, name string) []contentline.Param {
	out := make([]contentline.Param, 0, len(params))
	for _, q := range params {
		if !strings.EqualFold(q.Name, name) {
			out = append(out, q)
		}
	}
	return out
}

// withoutSchedule is p without its SCHEDULE-* parameters, which iTIP
// messages do not carry (RFC 6638 §7); ok is false when it had none.
func withoutSchedule(p contentline.Prop) (contentline.Prop, bool) {
	out := p
	out.Params = nil
	for _, q := range p.Params {
		if !strings.HasPrefix(strings.ToUpper(q.Name), "SCHEDULE-") {
			out.Params = append(out.Params, q)
		}
	}
	return out, len(out.Params) != len(p.Params)
}

// Published returns a published event as a calendar object of the user's
// own: src without its METHOD line, holding one event. The event is the
// one a card shows (Message.Main): the first VEVENT without a
// RECURRENCE-ID, else the first, with the VEVENTs that share its UID (a
// series' overrides). Every other component but the time zones goes: a
// file may publish several events, and a calendar resource holds one UID
// (RFC 4791 §4.1). In the events kept, the ORGANIZER and ATTENDEE lines and
// the alarms go; nothing else changes. With no organizer and no attendee no
// server schedules the copy, and an alarm a sender wrote, which a server
// may act on by mail, does not come along. The answer is that one calendar
// object and nothing else of src: a second VCALENDAR, or any bytes before
// or after the first, are not copied through.
func Published(src []byte) ([]byte, error) {
	cal, err := firstCalendar(src)
	if err != nil {
		return nil, err
	}
	events := cal.ChildrenNamed("VEVENT")
	if len(events) == 0 {
		return nil, errors.New("the calendar holds no event")
	}
	main := events[0]
	for _, event := range events {
		if event.Prop("RECURRENCE-ID") == nil {
			main = event
			break
		}
	}
	// The UID as written: every line of one file writes it the same way.
	uid := main.Prop("UID")
	if uid == nil || uid.Value == "" {
		return nil, errors.New("the event has no UID")
	}
	// Everything outside the chosen calendar goes, whether or not anything
	// inside it changes.
	var edits []contentline.Edit
	if cal.Start > 0 {
		edits = append(edits, contentline.Edit{Start: 0, End: cal.Start})
	}
	if cal.End < len(src) {
		edits = append(edits, contentline.Edit{Start: cal.End, End: len(src)})
	}
	for _, p := range cal.PropsNamed("METHOD") {
		edits = append(edits, contentline.Edit{Start: p.Start, End: p.End})
	}
	for _, child := range cal.Children {
		if child.Name == "VTIMEZONE" {
			continue
		}
		if p := child.Prop("UID"); child.Name != "VEVENT" || p == nil || p.Value != uid.Value {
			edits = append(edits, contentline.Edit{Start: child.Start, End: child.End})
			continue
		}
		for _, name := range []string{"ORGANIZER", "ATTENDEE"} {
			for _, p := range child.PropsNamed(name) {
				edits = append(edits, contentline.Edit{Start: p.Start, End: p.End})
			}
		}
		for _, alarm := range child.ChildrenNamed("VALARM") {
			edits = append(edits, contentline.Edit{Start: alarm.Start, End: alarm.End})
		}
	}
	if len(edits) == 0 {
		return src, nil
	}
	out, err := contentline.Apply(src, edits...)
	if err != nil {
		return nil, fmt.Errorf("make the published event the user's own: %w", err)
	}
	return out, nil
}
