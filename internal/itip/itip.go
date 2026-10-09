// Package itip reads iTIP messages that arrive in mail (RFC 5546, RFC 6047)
// and writes the user's answers: the REPLY mailed to an organizer and the
// PARTSTAT patch of a stored event (ADR-0019; ADR-0018's patches).
package itip

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/contentline"
	"github.com/frostyard/frostmail/internal/mimex"
)

// Methods, as the API's ITIPMethod names them.
const (
	MethodRequest = "request"
	MethodCancel  = "cancel"
	MethodReply   = "reply"
	MethodPublish = "publish"
	MethodOther   = "other"
)

var (
	// ErrNoInvitation is a message without an iCalendar part.
	ErrNoInvitation = errors.New("itip: no iCalendar part")
	// ErrNoEvent is an iCalendar object without a usable VEVENT.
	ErrNoEvent = errors.New("itip: no event")
	// ErrNotInvited is an attendee the event does not name.
	ErrNotInvited = errors.New("itip: not an attendee")
	// ErrNoOccurrence is a recurrence ID the object has no VEVENT for.
	ErrNoOccurrence = errors.New("itip: no such occurrence")
	// ErrAnswer is an answer other than accepted, declined or tentative.
	ErrAnswer = errors.New("itip: answer is accepted, declined or tentative")
)

// Message is an iTIP object read from mail.
type Message struct {
	// Method is one of the Method constants.
	Method string
	// Source is the iCalendar object as it came.
	Source []byte
	// Events are its VEVENTs in source order, read by calendar.Parse.
	Events []calendar.Event
}

// Main is the event the message is about: the one without a recurrence ID
// (a single event or a series' master), else the first.
func (m *Message) Main() calendar.Event {
	for _, event := range m.Events {
		if event.RecurrenceID == "" {
			return event
		}
	}
	if len(m.Events) > 0 {
		return m.Events[0]
	}
	return calendar.Event{}
}

// FromMessage returns the iCalendar object of a raw mail message: its first
// text/calendar part, else its first application/ics part or part whose
// filename ends in .ics, decoded from its transfer encoding.
func FromMessage(raw []byte) ([]byte, error) {
	var primary, fallback []byte
	var havePrimary, haveFallback bool
	err := mimex.WalkParts(raw, func(p mimex.PartInfo, body io.Reader) error {
		if havePrimary {
			return nil
		}
		calendarPart := p.ContentType == "text/calendar"
		attachment := p.ContentType == "application/ics" || strings.HasSuffix(strings.ToLower(p.Filename), ".ics")
		if !calendarPart && (!attachment || haveFallback) {
			return nil
		}
		data, err := io.ReadAll(body)
		if err != nil {
			return fmt.Errorf("read invitation part: %w", err)
		}
		if calendarPart {
			primary, havePrimary = data, true
		} else {
			fallback, haveFallback = data, true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read invitation message: %w", err)
	}
	if havePrimary {
		return primary, nil
	}
	if haveFallback {
		return fallback, nil
	}
	return nil, ErrNoInvitation
}

// Parse reads an iTIP object. METHOD maps to the Method constants: REQUEST,
// CANCEL, REPLY and PUBLISH to their own, a missing METHOD to publish, and
// any other (ADD, REFRESH, COUNTER, DECLINECOUNTER) to other.
func Parse(src []byte, opts calendar.Options) (*Message, error) {
	cal, err := firstCalendar(src)
	if err != nil {
		return nil, err
	}
	events, err := calendar.Parse(src, opts)
	if err != nil {
		return nil, fmt.Errorf("parse invitation: %w", err)
	}
	if len(events) == 0 {
		return nil, ErrNoEvent
	}
	method := MethodPublish
	if p := cal.Prop("METHOD"); p != nil {
		switch strings.ToUpper(p.Value) {
		case "REQUEST":
			method = MethodRequest
		case "CANCEL":
			method = MethodCancel
		case "REPLY":
			method = MethodReply
		case "PUBLISH":
			method = MethodPublish
		default:
			method = MethodOther
		}
	}
	return &Message{Method: method, Source: src, Events: events}, nil
}

func firstCalendar(src []byte) (*contentline.Component, error) {
	components, err := contentline.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse invitation: %w", err)
	}
	for _, c := range components {
		if c.Name == "VCALENDAR" {
			return c, nil
		}
	}
	return nil, errors.New("parse invitation: no VCALENDAR")
}

func chosenEvents(cal *contentline.Component, recurrenceID string, all bool) ([]*contentline.Component, error) {
	events := cal.ChildrenNamed("VEVENT")
	if recurrenceID != "" {
		for _, event := range events {
			if calendar.VEventRecurrenceID(cal, event, nil) == recurrenceID {
				return []*contentline.Component{event}, nil
			}
		}
		return nil, ErrNoOccurrence
	}
	if len(events) == 0 {
		return nil, ErrNoEvent
	}
	if all {
		return events, nil
	}
	for _, event := range events {
		if event.Prop("RECURRENCE-ID") == nil {
			return []*contentline.Component{event}, nil
		}
	}
	return events[:1], nil
}

func answerPartStat(answer string) (string, error) {
	switch answer {
	case "accepted", "declined", "tentative":
		return strings.ToUpper(answer), nil
	default:
		return "", ErrAnswer
	}
}

func answeredAttendee(event *contentline.Component, address, partStat string) *contentline.Prop {
	for _, prop := range event.PropsNamed("ATTENDEE") {
		email := prop.Value
		if len(email) >= 7 && strings.EqualFold(email[:7], "mailto:") {
			email = email[7:]
		}
		if !strings.EqualFold(email, address) {
			continue
		}
		var params []contentline.Param
		havePartStat := false
		for _, param := range prop.Params {
			switch param.Name {
			case "RSVP":
				continue
			case "PARTSTAT":
				param.Values = []string{partStat}
				havePartStat = true
			}
			params = append(params, param)
		}
		if !havePartStat {
			params = append(params, contentline.Param{Name: "PARTSTAT", Values: []string{partStat}})
		}
		prop.Params = params
		return &prop
	}
	return nil
}
