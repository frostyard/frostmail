package calendar

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

// ErrWholeSeries is DeleteOccurrence's answer to All Future Events from a
// series' first occurrence: that deletes the series, so its object.
var ErrWholeSeries = errors.New("calendar: from its first occurrence, the change takes the whole series")

// patchFuture splits the series before one of its occurrences (ADR-0031,
// All Future Events): the old object ends before it, and split, a new
// object under newUID, starts at it with c applied.
func patchFuture(src []byte, p occurrencePatch, c EventChange, newUID string, now time.Time) (patched, split []byte, err error) {
	if p.start.Equal(p.event.Start) {
		patched, err = patchAll(src, p, c, now)
		return patched, nil, err
	}
	if newUID == "" {
		return nil, nil, errors.New("patch future: new UID is required")
	}
	split, err = splitFuture(src, p, c, newUID, now)
	if err != nil {
		return nil, nil, err
	}
	patched, err = deleteFuture(src, p, now)
	if err != nil {
		return nil, nil, err
	}
	return patched, split, nil
}

// deleteFuture ends the series before one of its occurrences.
func deleteFuture(src []byte, p occurrencePatch, now time.Time) ([]byte, error) {
	if p.start.Equal(p.event.Start) {
		return nil, ErrWholeSeries
	}
	fields := eventRevisionFields(p.master, now, true)
	if rule := p.master.Prop("RRULE"); rule != nil {
		fields = append(fields, contentline.Prop{Name: "RRULE", Value: endFutureRule(rule.Value, p.untilBefore())})
	}
	eol := contentline.LineEnding(src)
	edits := eventPropertyEdits(src, p.master, fields, eol)
	edits = append(edits, p.futureDateEdits(false, eol)...)
	edits = append(edits, p.futureOverrideEdits(src, false, "", eol)...)
	out, err := contentline.Apply(src, edits...)
	if err != nil {
		return nil, fmt.Errorf("delete future: %w", err)
	}
	return out, nil
}

func (p occurrencePatch) untilBefore() string {
	frame := p.frame()
	if frame.allDay {
		return p.start.AddDate(0, 0, -1).UTC().Format("20060102")
	}
	if frame.utc || frame.tzid != "" {
		return p.start.Add(-time.Second).UTC().Format("20060102T150405Z")
	}
	return civilTime(p.start, frame).Add(-time.Second).Format("20060102T150405")
}

// Replace the first end condition in place and discard any others.
func endFutureRule(rule, until string) string {
	var parts []string
	ended := false
	for part := range strings.SplitSeq(rule, ";") {
		name, _, _ := strings.Cut(part, "=")
		if strings.EqualFold(name, "COUNT") || strings.EqualFold(name, "UNTIL") {
			if !ended {
				parts = append(parts, "UNTIL="+until)
				ended = true
			}
			continue
		}
		parts = append(parts, part)
	}
	if !ended {
		parts = append(parts, "UNTIL="+until)
	}
	return strings.Join(parts, ";")
}

func splitFuture(src []byte, p occurrencePatch, c EventChange, uid string, now time.Time) ([]byte, error) {
	fields, err := p.futureFields(uid, now)
	if err != nil {
		return nil, fmt.Errorf("split future: %w", err)
	}
	eol := contentline.LineEnding(src)
	edits := eventPropertyEdits(src, p.master, fields, eol)
	edits = append(edits, p.futureDateEdits(true, eol)...)
	edits = append(edits, p.futureOverrideEdits(src, true, uid, eol)...)
	out, err := contentline.Apply(src, edits...)
	if err != nil {
		return nil, fmt.Errorf("split future: %w", err)
	}
	first, err := readOccurrencePatch(out, RecurrenceKey(p.start, p.event.AllDay))
	if err != nil {
		return nil, err
	}
	return patchAll(out, first, c, now)
}

func (p occurrencePatch) futureFields(uid string, now time.Time) ([]contentline.Prop, error) {
	fields := []contentline.Prop{p.timeProp("DTSTART", p.start),
		{Name: "UID", Value: contentline.EscapeText(uid)},
		{Name: "CREATED", Value: now.UTC().Format("20060102T150405Z")}}
	fields = append(fields, eventRevisionFields(p.master, now, false)...)
	if p.master.Prop("DTEND") != nil {
		fields = append(fields, p.timeProp("DTEND", p.start.Add(p.event.End.Sub(p.event.Start))))
	}
	rule := propertyValue(p.master, "RRULE")
	parts := strings.Split(rule, ";")
	for i, part := range parts {
		name, value, _ := strings.Cut(part, "=")
		if !strings.EqualFold(name, "COUNT") {
			continue
		}
		count, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("recurrence count: %w", err)
		}
		count -= len(ruleStarts(p.event, rule, p.event.Start, p.start))
		remaining := ""
		if count > 0 {
			parts[i] = "COUNT=" + strconv.Itoa(count)
			remaining = strings.Join(parts, ";")
		}
		fields = append(fields, contentline.Prop{Name: "RRULE", Value: remaining})
		break
	}
	return fields, nil
}

func (p occurrencePatch) futureDateEdits(future bool, eol string) []contentline.Edit {
	var edits []contentline.Edit
	for _, prop := range p.master.Props {
		if (prop.Name != "RDATE" && prop.Name != "EXDATE") || strings.EqualFold(prop.Param("VALUE"), "PERIOD") {
			continue
		}
		dates := recurrenceDates(p.event, prop)
		var values []string
		for _, date := range dates {
			if date.Before(p.start) != future {
				values = append(values, p.timeProp(prop.Name, date).Value)
			}
		}
		if len(values) == len(dates) {
			continue
		}
		edit := contentline.Edit{Start: prop.Start, End: prop.End}
		if len(values) > 0 {
			field := p.timeProp(prop.Name, p.start)
			field.Value = strings.Join(values, ",")
			edit.Text = field.Encode(eol)
		}
		edits = append(edits, edit)
	}
	return edits
}

func (p occurrencePatch) futureOverrideEdits(src []byte, future bool, uid, eol string) []contentline.Edit {
	var edits []contentline.Edit
	reader := timeReader{calendar: p.cal, local: time.Local}
	for _, component := range p.cal.ChildrenNamed("VEVENT") {
		id, ok := reader.read(component.Prop("RECURRENCE-ID"))
		if !ok {
			continue
		}
		if id.instant.Before(p.start) == future {
			edits = append(edits, contentline.Edit{Start: component.Start, End: component.End})
		} else if future {
			fields := []contentline.Prop{{Name: "UID", Value: contentline.EscapeText(uid)}}
			edits = append(edits, eventPropertyEdits(src, component, fields, eol)...)
		}
	}
	return edits
}
