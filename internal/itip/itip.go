// Package itip reads iTIP messages that arrive in mail (RFC 5546, RFC 6047)
// and writes the user's answers: the REPLY mailed to an organizer and the
// PARTSTAT patch of a stored event (ADR-0019; ADR-0018's patches). Task
// T-0087 builds it.
package itip

import (
	"errors"
	"time"

	"github.com/frostyard/frostmail/internal/calendar"
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
	panic("Task T-0087 builds it")
}

// FromMessage returns the iCalendar object of a raw mail message: its first
// text/calendar part, else its first application/ics part or part whose
// filename ends in .ics, decoded from its transfer encoding.
func FromMessage(_ []byte) ([]byte, error) {
	return nil, errors.New("task T-0087 builds it")
}

// Parse reads an iTIP object. METHOD maps to the Method constants: REQUEST,
// CANCEL, REPLY and PUBLISH to their own, a missing METHOD to publish, and
// any other (ADD, REFRESH, COUNTER, DECLINECOUNTER) to other.
func Parse(_ []byte, _ calendar.Options) (*Message, error) {
	return nil, errors.New("task T-0087 builds it")
}

// Reply writes the iTIP REPLY that tells the organizer attendee's answer
// (accepted, declined or tentative) to the event in src, an invitation or a
// stored event: its single event or master when recurrenceID is "", else
// the VEVENT with that recurrence ID (as calendar.Event.RecurrenceID
// formats it). comment, when not empty, is a note to the organizer.
func Reply(_ []byte, _, _, _, _ string, _ time.Time) ([]byte, error) {
	return nil, errors.New("task T-0087 builds it")
}

// SetPartStat patches attendee's PARTSTAT to answer in src, changing only
// the attendee's ATTENDEE lines: in every VEVENT when recurrenceID is "",
// else in the VEVENT with that recurrence ID.
func SetPartStat(_ []byte, _, _, _ string) ([]byte, error) {
	return nil, errors.New("task T-0087 builds it")
}

// ForCalendar returns an invitation as a calendar object to store: src
// without its METHOD line, which calendar collections do not take.
func ForCalendar(_ []byte) ([]byte, error) {
	return nil, errors.New("task T-0087 builds it")
}
