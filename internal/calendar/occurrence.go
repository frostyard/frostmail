package calendar

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

// Scope says what a change to one occurrence of a series changes
// (ADR-0031).
type Scope int

// The scopes, as calendar.update and calendar.delete take them.
const (
	ScopeThis   Scope = iota // the occurrence: an override, or an EXDATE when deleting
	ScopeFuture              // the occurrence and those after it: the series splits
	ScopeAll                 // the whole series: its master
)

// PatchOccurrence applies c to one occurrence of the series in src, in
// scope (ADR-0031). recurrenceID names the occurrence as Parse and Expand
// do (RecurrenceKey of its original start). For ScopeFuture, split is the
// new series' object, under newUID; it is nil otherwise.
func PatchOccurrence(src []byte, recurrenceID string, scope Scope, c EventChange, newUID string, now time.Time) (patched, split []byte, err error) {
	p, err := readSeriesOccurrencePatch(src, recurrenceID)
	if err != nil {
		return nil, nil, err
	}
	switch scope {
	case ScopeThis:
	case ScopeAll:
		patched, err = patchAll(src, p, c, now)
		return patched, nil, err
	case ScopeFuture:
		return patchFuture(src, p, c, newUID, now)
	default:
		return nil, nil, errors.New("patch occurrence: invalid scope")
	}
	if c.RRule != nil {
		return nil, nil, errors.New("patch occurrence: one occurrence cannot repeat")
	}
	if p.override == nil {
		src, err = contentline.Apply(src, contentline.Edit{
			Start: p.master.End, End: p.master.End, Text: p.overrideText(src),
		})
		if err != nil {
			return nil, nil, fmt.Errorf("patch occurrence: %w", err)
		}
		p, err = readOccurrencePatch(src, recurrenceID)
		if err != nil {
			return nil, nil, err
		}
	}
	patched, err = patchEventComponent(src, p.cal, p.override, c, now)
	return patched, nil, err
}

// DeleteOccurrence deletes one occurrence (ScopeThis) or the series from
// it on (ScopeFuture). Deleting a whole series deletes its object, which
// is not a patch.
func DeleteOccurrence(src []byte, recurrenceID string, scope Scope, now time.Time) ([]byte, error) {
	p, err := readSeriesOccurrencePatch(src, recurrenceID)
	if err != nil {
		return nil, err
	}
	switch scope {
	case ScopeThis:
	case ScopeFuture:
		return deleteFuture(src, p, now)
	default:
		return nil, errors.New("delete occurrence: only this or future occurrences can be patched")
	}
	eol := contentline.LineEnding(src)
	at := todoEndStart(src, p.master.End)
	if len(p.master.Children) > 0 {
		at = p.master.Children[0].Start
	}
	edits := []contentline.Edit{{Start: at, End: at, Text: p.timeProp("EXDATE", p.start).Encode(eol)}}
	edits = append(edits, eventPropertyEdits(src, p.master, eventRevisionFields(p.master, now, true), eol)...)
	if p.override != nil {
		edits = append(edits, contentline.Edit{Start: p.override.Start, End: p.override.End})
	}
	out, err := contentline.Apply(src, edits...)
	if err != nil {
		return nil, fmt.Errorf("delete occurrence: %w", err)
	}
	return out, nil
}

type occurrencePatch struct {
	cal, master, override *contentline.Component
	event                 Event
	start                 time.Time
}

func readSeriesOccurrencePatch(src []byte, id string) (occurrencePatch, error) {
	p, err := readOccurrencePatch(src, id)
	if err == nil && p.master.Prop("RRULE") == nil && p.master.Prop("RDATE") == nil {
		return p, errors.New("occurrence: master is not a usable series")
	}
	return p, err
}

func readOccurrencePatch(src []byte, id string) (occurrencePatch, error) {
	var p occurrencePatch
	cal, master, err := eventPatchMaster(src)
	if err != nil {
		return p, err
	}
	event, ok := readEvent(master, timeReader{calendar: cal, local: time.Local}, src, nil)
	if !ok {
		return p, errors.New("occurrence: master is not a usable series")
	}
	layout := time.RFC3339Nano
	if event.AllDay {
		layout = time.DateOnly
	}
	start, err := time.Parse(layout, id)
	if err != nil {
		return p, fmt.Errorf("occurrence: invalid recurrence ID: %w", err)
	}
	if RecurrenceKey(start, event.AllDay) != id {
		return p, errors.New("occurrence: recurrence ID is not in canonical form")
	}
	p = occurrencePatch{cal: cal, master: master, event: event, start: start}
	// A split can exhaust its COUNT and leave only the new master's start.
	if master.Prop("RRULE") == nil && master.Prop("RDATE") == nil && start.Equal(event.Start) {
		return p, nil
	}
	for _, component := range cal.ChildrenNamed("VEVENT") {
		if VEventRecurrenceID(cal, component, time.Local) == id {
			p.override = component
			return p, nil
		}
	}
	occurrences, err := Expand([]Event{event}, start.Add(-24*time.Hour), start.Add(24*time.Hour))
	if err != nil {
		return p, fmt.Errorf("occurrence: expand series: %w", err)
	}
	for _, occurrence := range occurrences {
		if occurrence.RecurrenceID == id {
			return p, nil
		}
	}
	return p, errors.New("occurrence: no occurrence with that recurrence ID")
}

// timeProp retains DTSTART's date, UTC, zoned or floating representation.
func (p occurrencePatch) timeProp(name string, instant time.Time) contentline.Prop {
	return p.frame().timeProp(name, instant)
}

type occurrenceFrame struct {
	allDay bool
	utc    bool
	tzid   string
	zone   *time.Location
}

func (p occurrencePatch) frame() occurrenceFrame {
	start := p.master.Prop("DTSTART")
	return occurrenceFrame{allDay: p.event.AllDay, utc: strings.HasSuffix(start.Value, "Z"),
		tzid: start.Param("TZID"), zone: eventZone(p.event)}
}

func (f occurrenceFrame) timeProp(name string, instant time.Time) contentline.Prop {
	prop := contentline.Prop{Name: name}
	switch {
	case f.allDay:
		prop.Params = []contentline.Param{{Name: "VALUE", Values: []string{"DATE"}}}
		prop.Value = instant.UTC().Format("20060102")
	case f.utc:
		prop.Value = instant.UTC().Format("20060102T150405Z")
	default:
		if f.tzid != "" {
			prop.Params = []contentline.Param{{Name: "TZID", Values: []string{f.tzid}}}
		}
		prop.Value = instant.In(f.zone).Format("20060102T150405")
	}
	return prop
}

func (p occurrencePatch) overrideText(src []byte) string {
	eol := contentline.LineEnding(src)
	var out strings.Builder
	out.WriteString("BEGIN:VEVENT" + eol)
	for _, prop := range p.master.Props {
		switch prop.Name {
		case "RRULE", "RDATE", "EXDATE":
			continue
		case "DTSTART":
			out.WriteString(p.timeProp("DTSTART", p.start).Encode(eol))
			out.WriteString(p.timeProp("RECURRENCE-ID", p.start).Encode(eol))
		case "DTEND":
			out.WriteString(p.timeProp("DTEND", p.start.Add(p.event.End.Sub(p.event.Start))).Encode(eol))
		default:
			out.Write(src[prop.Start:prop.End])
		}
	}
	for _, child := range p.master.Children {
		out.Write(src[child.Start:child.End])
	}
	out.WriteString("END:VEVENT" + eol)
	return out.String()
}
