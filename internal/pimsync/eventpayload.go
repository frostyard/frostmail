package pimsync

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/contentline"
)

// EventEdit is one change calendar.update or calendar.delete made to an
// event's object: a patch of the event (no RecurrenceID), or of one of a
// series' occurrences in a scope, or a delete of occurrences.
type EventEdit struct {
	RecurrenceID string               `json:"recurrenceId,omitzero"`
	Scope        string               `json:"scope,omitzero"` // this, future or all
	Change       calendar.EventChange `json:"change,omitzero"`
	NewUID       string               `json:"newUid,omitzero"` // a split's new series, written by an op of its own
	Delete       bool                 `json:"delete,omitzero"`
}

// EventPayload is an event write's payload (ADR-0030; docs/design/pim.md,
// Editing). A put holds its edits in order and, by component and property,
// the lines the source had before the first one; or Whole, a source
// written whole (a restore), which cannot be applied again. A delete holds
// the event's summary, for calendar.refused, and, for a move, the put to
// the other calendar it waits for.
type EventPayload struct {
	Edits   []EventEdit         `json:"event,omitzero"`
	Base    map[string][]string `json:"base,omitzero"`
	Whole   bool                `json:"whole,omitzero"`
	Summary string              `json:"summary,omitzero"`
	After   int64               `json:"after,omitzero"`
}

// ParseEventPayload reads a write's payload; ok is false for a put that is
// no edit (a creation, an answer).
func ParseEventPayload(payload string) (EventPayload, bool) {
	var p EventPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return EventPayload{}, false
	}
	return p, len(p.Edits) > 0 || p.Whole
}

// Encode is the payload as stored.
func (p EventPayload) Encode() (string, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("event payload: %w", err)
	}
	return string(b), nil
}

// Scope is an EventEdit's scope as calendar takes it.
func Scope(name string) (calendar.Scope, bool) {
	switch name {
	case "this":
		return calendar.ScopeThis, true
	case "future":
		return calendar.ScopeFuture, true
	case "all":
		return calendar.ScopeAll, true
	}
	return 0, false
}

// seriesShape are the properties that say which occurrences a series
// has: an edit of one depends on them.
var seriesShape = []string{"DTSTART", "DTEND", "DURATION", "RRULE", "RDATE", "EXDATE"}

// Extend adds an edit made on src, recording the base of what it touches
// that no earlier edit touched: for an event, the properties it rewrites;
// for a series, those and its shape in every VEVENT, and which VEVENTs
// the object has.
func (p EventPayload) Extend(src []byte, e EventEdit) (EventPayload, error) {
	if p.Whole {
		return p, nil
	}
	names, series := eventTouched(e)
	lines, err := eventLines(src, names, series || e.RecurrenceID != "")
	if err != nil {
		return p, err
	}
	out := EventPayload{Edits: append(slices.Clone(p.Edits), e), Base: maps.Clone(p.Base)}
	if out.Base == nil {
		out.Base = map[string][]string{}
	}
	for key, l := range lines {
		if _, ok := out.Base[key]; !ok {
			out.Base[key] = l
		}
	}
	return out, nil
}

// Fits reports whether another client left everything the edits touch as
// it was: the server's lines equal the base.
func (p EventPayload) Fits(server []byte) bool {
	if p.Whole {
		return false
	}
	var names []string
	all := false
	for key := range p.Base {
		if key == objectEvents {
			all = true
		} else if _, name, ok := strings.Cut(key, "/"); ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	lines, err := eventLines(server, slices.Compact(names), all)
	if err != nil {
		return false
	}
	for key, base := range p.Base {
		if !slices.Equal(lines[key], base) {
			return false
		}
	}
	return true
}

// Apply makes the edits again, in order, on the server's source. A split's
// new series is its own op, so only the old one is kept here.
func (p EventPayload) Apply(server []byte, now time.Time) ([]byte, error) {
	out := server
	for _, e := range p.Edits {
		var err error
		if e.RecurrenceID == "" {
			out, err = calendar.PatchEvent(out, e.Change, now)
		} else if scope, ok := Scope(e.Scope); !ok {
			err = fmt.Errorf("event payload: scope %q", e.Scope)
		} else if e.Delete {
			out, err = calendar.DeleteOccurrence(out, e.RecurrenceID, scope, now)
		} else {
			out, _, err = calendar.PatchOccurrence(out, e.RecurrenceID, scope, e.Change, e.NewUID, now)
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// eventTouched are the properties an edit rewrites, and whether it is one
// of a series (scoped).
func eventTouched(e EventEdit) ([]string, bool) {
	c := e.Change
	var out []string
	add := func(set bool, names ...string) {
		if set {
			out = append(out, names...)
		}
	}
	add(c.Summary != nil, "SUMMARY")
	add(c.Location != nil, "LOCATION")
	add(c.Description != nil, "DESCRIPTION")
	add(c.AllDay != nil || c.Start != nil || c.End != nil || c.Zone != nil || c.StartDate != nil || c.EndDate != nil,
		"DTSTART", "DTEND", "DURATION")
	add(c.RRule != nil, "RRULE")
	add(c.Transparent != nil, "TRANSP")
	add(c.Alarms != nil, alarmLines)
	series := e.RecurrenceID != ""
	add(series, seriesShape...)
	add(series, "RECURRENCE-ID")
	slices.Sort(out)
	return slices.Compact(out), series
}

const (
	// alarmLines names a VEVENT's DISPLAY and AUDIO alarms, the ones its
	// alerts replace, in a base.
	alarmLines = "VALARM"
	// objectEvents names the list of an object's VEVENTs in a base.
	objectEvents = "VEVENTS"
)

// eventLines are the unfolded lines of the named properties, by
// "<component>/<name>": the component is VEVENT for the event or master,
// or VEVENT;RECURRENCE-ID=<key> for an override, which only all reads.
// With all, objectEvents lists the components.
func eventLines(src []byte, names []string, all bool) (map[string][]string, error) {
	components, err := contentline.Parse(src)
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(components, func(c *contentline.Component) bool { return c.Name == "VCALENDAR" })
	if i < 0 {
		return nil, errors.New("no vcalendar component")
	}
	cal := components[i]
	out := map[string][]string{}
	var keys []string
	for _, v := range cal.ChildrenNamed("VEVENT") {
		key := "VEVENT"
		if id := calendar.VEventRecurrenceID(cal, v, time.Local); id != "" {
			key += ";RECURRENCE-ID=" + id
		} else if v.Prop("RECURRENCE-ID") != nil {
			continue
		}
		if key != "VEVENT" && !all || slices.Contains(keys, key) {
			continue
		}
		keys = append(keys, key)
		for _, name := range names {
			out[key+"/"+name] = componentLines(v, name)
		}
	}
	if all {
		slices.Sort(keys)
		out[objectEvents] = keys
	}
	return out, nil
}

func componentLines(v *contentline.Component, name string) []string {
	unfold := func(p contentline.Prop) string {
		return strings.ReplaceAll(strings.TrimSuffix(p.Encode("\n"), "\n"), "\n ", "")
	}
	var out []string
	if name != alarmLines {
		for _, p := range v.PropsNamed(name) {
			out = append(out, unfold(p))
		}
		return out
	}
	for _, alarm := range v.ChildrenNamed("VALARM") {
		action := alarm.Prop("ACTION")
		if action == nil || !strings.EqualFold(action.Value, "DISPLAY") && !strings.EqualFold(action.Value, "AUDIO") {
			continue
		}
		out = append(out, "BEGIN:VALARM")
		for _, p := range alarm.Props {
			out = append(out, unfold(p))
		}
		out = append(out, "END:VALARM")
	}
	return out
}
