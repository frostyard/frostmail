package itip

import (
	"fmt"

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
