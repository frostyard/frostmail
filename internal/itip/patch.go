package itip

import (
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
