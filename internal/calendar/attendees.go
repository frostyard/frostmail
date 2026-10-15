package calendar

import (
	"errors"
	"strings"

	"github.com/frostyard/frostmail/internal/contentline"
)

// Invitees (ADR-0035): an EventChange's Attendees replace an event's
// ATTENDEE lines, which the server reads to tell invitees, so a line that
// stays is left as it was.

// roleParams are an Attendee's Role as ROLE values.
var roleParams = map[string]string{
	"": "REQ-PARTICIPANT", "required": "REQ-PARTICIPANT", "optional": "OPT-PARTICIPANT",
	"chair": "CHAIR", "none": "NON-PARTICIPANT",
}

// invitees are the change's attendees by address, in order, without
// repeats.
func invitees(c EventChange) ([]Attendee, map[string]Attendee, error) {
	var list []Attendee
	byEmail := map[string]Attendee{}
	for _, a := range *c.Attendees {
		a.Email = strings.ToLower(strings.TrimSpace(a.Email))
		if a.Email == "" || strings.ContainsAny(a.Email, " \t\r\n:;,<>") || !strings.Contains(a.Email, "@") {
			return nil, nil, errors.New("an invitee needs an address")
		}
		if _, ok := roleParams[a.Role]; !ok {
			return nil, nil, errors.New("an invitee's role is required, optional, chair or none")
		}
		if _, ok := byEmail[a.Email]; ok {
			continue
		}
		byEmail[a.Email] = a
		list = append(list, a)
	}
	return list, byEmail, nil
}

// eventAttendeeEdits rewrites the event's invitees to c.Attendees.
func eventAttendeeEdits(src []byte, event *contentline.Component, c EventChange, eol string) ([]contentline.Edit, error) {
	if c.Attendees == nil {
		return nil, nil
	}
	list, want, err := invitees(c)
	if err != nil {
		return nil, err
	}
	organizer := event.Prop("ORGANIZER")
	organizerEmail := ""
	if organizer != nil {
		organizerEmail = PersonEmail(*organizer)
	}
	var edits []contentline.Edit
	kept := map[string]bool{}
	at := -1
	if organizer != nil {
		at = organizer.End
	}
	for _, p := range event.PropsNamed("ATTENDEE") {
		at = max(at, p.End)
		email := PersonEmail(p)
		if organizerEmail != "" && email == organizerEmail {
			continue
		}
		a, ok := want[email]
		if !ok || kept[email] {
			edits = append(edits, contentline.Edit{Start: p.Start, End: p.End})
			continue
		}
		kept[email] = true
		role := strings.ToUpper(p.Param("ROLE"))
		if role == "" {
			role = "REQ-PARTICIPANT"
		}
		if role != roleParams[a.Role] {
			edits = append(edits, contentline.Edit{Start: p.Start, End: p.End, Text: withRole(p, roleParams[a.Role]).Encode(eol)})
		}
	}
	var added strings.Builder
	for _, a := range list {
		if !kept[a.Email] {
			added.WriteString(attendeeProp(a).Encode(eol))
		}
	}
	if added.Len() == 0 {
		return edits, nil
	}
	text := added.String()
	if organizer == nil {
		if c.Organizer == nil || c.Organizer.Email == "" {
			return nil, errors.New("invitees need an organizer")
		}
		text = organizerProp(*c.Organizer).Encode(eol) + text
	}
	if at < 0 {
		at = todoEndStart(src, event.End)
		if len(event.Children) > 0 {
			at = event.Children[0].Start
		}
	}
	return append(edits, contentline.Edit{Start: at, End: at, Text: text}), nil
}

// newEventPeople is a new event's ORGANIZER and ATTENDEE lines.
func newEventPeople(c EventChange, eol string) (string, error) {
	if c.Attendees == nil || len(*c.Attendees) == 0 {
		return "", nil
	}
	list, _, err := invitees(c)
	if err != nil {
		return "", err
	}
	if c.Organizer == nil || c.Organizer.Email == "" {
		return "", errors.New("invitees need an organizer")
	}
	var out strings.Builder
	out.WriteString(organizerProp(*c.Organizer).Encode(eol))
	for _, a := range list {
		out.WriteString(attendeeProp(a).Encode(eol))
	}
	return out.String(), nil
}

func attendeeProp(a Attendee) contentline.Prop {
	var params []contentline.Param
	if a.Name != "" {
		params = append(params, contentline.Param{Name: "CN", Values: []string{a.Name}})
	}
	params = append(params, contentline.Param{Name: "ROLE", Values: []string{roleParams[a.Role]}},
		contentline.Param{Name: "PARTSTAT", Values: []string{"NEEDS-ACTION"}},
		contentline.Param{Name: "RSVP", Values: []string{"TRUE"}})
	return contentline.Prop{Name: "ATTENDEE", Params: params, Value: "mailto:" + a.Email}
}

func organizerProp(a Attendee) contentline.Prop {
	var params []contentline.Param
	if a.Name != "" {
		params = append(params, contentline.Param{Name: "CN", Values: []string{a.Name}})
	}
	return contentline.Prop{Name: "ORGANIZER", Params: params, Value: "mailto:" + strings.ToLower(strings.TrimSpace(a.Email))}
}

// withRole is p with its ROLE set to role, in place when it has one.
func withRole(p contentline.Prop, role string) contentline.Prop {
	out := p
	out.Params = nil
	set := false
	for _, q := range p.Params {
		if strings.EqualFold(q.Name, "ROLE") {
			if set {
				continue
			}
			q = contentline.Param{Name: q.Name, Values: []string{role}}
			set = true
		}
		out.Params = append(out.Params, q)
	}
	if !set {
		out.Params = append(out.Params, contentline.Param{Name: "ROLE", Values: []string{role}})
	}
	return out
}

// sameInvitees reports whether two VEVENTs invite the same people in the
// same roles: an override whose invitees were the master's follows an
// All Events change of them (ADR-0031).
func sameInvitees(a, b Event) bool {
	roles := func(e Event) map[string]string {
		out := map[string]string{}
		for _, p := range e.Attendees {
			out[p.Email] = p.Role
		}
		return out
	}
	ra, rb := roles(a), roles(b)
	if len(ra) != len(rb) {
		return false
	}
	for email, role := range ra {
		if r, ok := rb[email]; !ok || r != role {
			return false
		}
	}
	return true
}
