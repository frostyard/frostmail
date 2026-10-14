package calendar

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

// patchAll changes the whole series from one of its occurrences
// (ADR-0031, All Events).
func patchAll(src []byte, p occurrencePatch, c EventChange, now time.Time) ([]byte, error) {
	plan, err := planAll(p, c)
	if err != nil {
		return nil, fmt.Errorf("patch all: %w", err)
	}
	if c == (EventChange{}) {
		return src, nil
	}
	if err := plan.prepare(src, c); err != nil {
		return nil, fmt.Errorf("patch all: %w", err)
	}
	out, err := contentline.Apply(src, plan.edits...)
	if err != nil {
		return nil, fmt.Errorf("patch all: %w", err)
	}
	out, err = PatchEvent(out, plan.change, now)
	if err != nil {
		return nil, err
	}
	for _, override := range plan.overrides {
		out, err = patchAllOverride(out, override, now)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

type allOverride struct {
	id     string
	change EventChange
}

type allPlan struct {
	old       occurrencePatch
	frame     occurrenceFrame
	target    time.Time
	length    time.Duration
	timing    bool
	moved     bool
	change    EventChange
	series    Event
	edits     []contentline.Edit
	overrides []allOverride
}

func planAll(p occurrencePatch, c EventChange) (allPlan, error) {
	plan := allPlan{old: p, frame: p.frame(), target: p.start,
		length: p.event.End.Sub(p.event.Start), change: c}
	plan.timing = c.AllDay != nil || c.Start != nil || c.End != nil || c.Zone != nil || c.StartDate != nil || c.EndDate != nil
	if plan.timing {
		if _, err := eventTiming(c); err != nil {
			return plan, err
		}
		plan.frame = occurrenceFrame{allDay: *c.AllDay, zone: time.UTC}
		if *c.AllDay {
			plan.target, _ = time.Parse(time.DateOnly, *c.StartDate)
			end, _ := time.Parse(time.DateOnly, *c.EndDate)
			plan.length = end.Sub(plan.target)
		} else {
			plan.frame.zone, _ = time.LoadLocation(*c.Zone)
			plan.frame.utc = *c.Zone == "UTC"
			if !plan.frame.utc {
				plan.frame.tzid = *c.Zone
			}
			plan.target, plan.length = *c.Start, c.End.Sub(*c.Start)
		}
	}
	if _, err := eventFields(c, nil); err != nil {
		return plan, err
	}
	oldFrame := p.frame()
	plan.moved = !plan.shift(p.event.Start).Equal(p.event.Start) || oldFrame.allDay != plan.frame.allDay ||
		oldFrame.utc != plan.frame.utc || oldFrame.tzid != plan.frame.tzid
	if plan.timing {
		plan.change = plan.withTiming(c, plan.shift(p.event.Start))
	}
	return plan, nil
}

// Civil clocks in UTC make the offset independent of either date's DST.
func civilTime(instant time.Time, frame occurrenceFrame) time.Time {
	wall := instant.In(frame.zone)
	if frame.allDay {
		return time.Date(wall.Year(), wall.Month(), wall.Day(), 0, 0, 0, 0, time.UTC)
	}
	return time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), time.UTC)
}

func (p allPlan) shift(instant time.Time) time.Time {
	if !p.timing {
		return instant
	}
	oldFrame := p.old.frame()
	original, value := civilTime(p.old.start, oldFrame), civilTime(instant, oldFrame)
	target := civilTime(p.target, p.frame)
	if p.frame.allDay {
		original = original.Truncate(24 * time.Hour)
		value = value.Truncate(24 * time.Hour)
	}
	wall := target.Add(value.Sub(original))
	return time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(), p.frame.zone)
}

func (p allPlan) withTiming(c EventChange, start time.Time) EventChange {
	allDay := p.frame.allDay
	c.AllDay, c.Start, c.End, c.Zone, c.StartDate, c.EndDate = &allDay, nil, nil, nil, nil, nil
	end := start.Add(p.length)
	if allDay {
		first, last := start.UTC().Format(time.DateOnly), end.UTC().Format(time.DateOnly)
		c.StartDate, c.EndDate = &first, &last
	} else {
		zone := p.frame.tzid
		if p.frame.utc {
			zone = "UTC"
		}
		c.Start, c.End, c.Zone = &start, &end, &zone
	}
	return c
}

func (p *allPlan) prepare(src []byte, c EventChange) error {
	if err := p.shiftUntil(); err != nil {
		return err
	}
	p.series = p.old.event
	p.series.Start = p.shift(p.old.event.Start)
	p.series.End = p.series.Start.Add(p.length)
	p.series.AllDay, p.series.Zone = p.frame.allDay, p.frame.zone
	p.series.Recurrence = ""
	rule := propertyValue(p.old.master, "RRULE")
	if p.change.RRule != nil {
		rule = *p.change.RRule
	}
	if rule != "" {
		p.series.Recurrence = "RRULE:" + rule + "\n"
	}
	// A series of RDATEs alone has occurrences too.
	for _, prop := range p.old.master.PropsNamed("RDATE") {
		if dates := recurrenceDates(p.old.event, prop); len(dates) > 0 {
			p.series.Recurrence += p.dateProp(prop.Name, dates).Encode("\n")
		}
	}
	eol := contentline.LineEnding(src)
	if err := p.planDates(eol); err != nil {
		return err
	}
	for _, component := range p.old.cal.ChildrenNamed("VEVENT") {
		if err := p.planOverride(src, component, c, eol); err != nil {
			return err
		}
	}
	return nil
}

func (p *allPlan) shiftUntil() error {
	if !p.moved || p.change.RRule != nil {
		return nil
	}
	parts := strings.Split(propertyValue(p.old.master, "RRULE"), ";")
	for i, part := range parts {
		value, ok := strings.CutPrefix(part, "UNTIL=")
		if !ok {
			continue
		}
		dates := recurrenceDates(p.old.event, contentline.Prop{Value: value})
		if len(dates) != 1 {
			return errors.New("invalid recurrence until")
		}
		layout := "20060102T150405Z"
		if p.frame.allDay {
			layout = "20060102"
		}
		parts[i] = "UNTIL=" + p.shift(dates[0]).UTC().Format(layout)
		rule := strings.Join(parts, ";")
		p.change.RRule = &rule
	}
	return nil
}

// member checks the changed series without exclusions, including DTSTART.
func (p allPlan) member(start time.Time) (bool, error) {
	if !p.moved && p.change.RRule == nil {
		return true, nil
	}
	if p.series.Recurrence == "" {
		return false, nil
	}
	occurrences, err := Expand([]Event{p.series}, start.Add(-24*time.Hour), start.Add(24*time.Hour))
	if err != nil {
		return false, fmt.Errorf("expand changed series: %w", err)
	}
	key := RecurrenceKey(start, p.frame.allDay)
	for _, occurrence := range occurrences {
		if occurrence.RecurrenceID == key {
			return true, nil
		}
	}
	return false, nil
}

func (p allPlan) dateProp(name string, dates []time.Time) contentline.Prop {
	prop := p.frame.timeProp(name, p.shift(dates[0]))
	values := make([]string, 0, len(dates))
	for _, date := range dates {
		values = append(values, p.frame.timeProp(name, p.shift(date)).Value)
	}
	prop.Value = strings.Join(values, ",")
	return prop
}

func (p *allPlan) planDates(eol string) error {
	for _, prop := range p.old.master.Props {
		if (prop.Name != "EXDATE" && prop.Name != "RDATE") || strings.EqualFold(prop.Param("VALUE"), "PERIOD") {
			continue
		}
		dates := recurrenceDates(p.old.event, prop)
		var kept []time.Time
		for _, date := range dates {
			member := true
			if prop.Name == "EXDATE" {
				var err error
				member, err = p.member(p.shift(date))
				if err != nil {
					return err
				}
			}
			if member {
				kept = append(kept, date)
			}
		}
		if !p.moved && len(kept) == len(dates) {
			continue
		}
		edit := contentline.Edit{Start: prop.Start, End: prop.End}
		if len(kept) > 0 {
			edit.Text = p.dateProp(prop.Name, kept).Encode(eol)
		}
		p.edits = append(p.edits, edit)
	}
	return nil
}

func (p *allPlan) planOverride(src []byte, component *contentline.Component, c EventChange, eol string) error {
	reader := timeReader{calendar: p.old.cal, local: time.Local}
	prop := component.Prop("RECURRENCE-ID")
	id, ok := reader.read(prop)
	if !ok {
		return nil
	}
	shifted := p.shift(id.instant)
	member, err := p.member(shifted)
	if err != nil {
		return err
	}
	if !member {
		p.edits = append(p.edits, contentline.Edit{Start: component.Start, End: component.End})
		return nil
	}
	if p.moved {
		p.edits = append(p.edits, contentline.Edit{Start: prop.Start, End: prop.End,
			Text: p.frame.timeProp("RECURRENCE-ID", shifted).Encode(eol)})
	}
	event, ok := readEvent(component, reader, src, nil)
	if !ok {
		return nil
	}
	change := inheritedChange(p.old.event, event, c)
	oldLength := p.old.event.End.Sub(p.old.event.Start)
	if (p.moved || p.length != oldLength) && event.Start.Equal(id.instant) && event.End.Sub(event.Start) == oldLength {
		change = p.withTiming(change, shifted)
	}
	if change != (EventChange{}) {
		key := VEventRecurrenceID(p.old.cal, component, time.Local)
		if p.moved {
			key = RecurrenceKey(shifted, p.frame.allDay)
		}
		p.overrides = append(p.overrides, allOverride{id: key, change: change})
	}
	return nil
}

func inheritedChange(master, override Event, c EventChange) EventChange {
	var change EventChange
	if master.Summary == override.Summary {
		change.Summary = c.Summary
	}
	if master.Location == override.Location {
		change.Location = c.Location
	}
	if master.Description == override.Description {
		change.Description = c.Description
	}
	if master.Transparent == override.Transparent {
		change.Transparent = c.Transparent
	}
	if slices.Equal(master.Alarms, override.Alarms) {
		change.Alarms = c.Alarms
	}
	return change
}

func patchAllOverride(src []byte, override allOverride, now time.Time) ([]byte, error) {
	cal, _, err := eventPatchMaster(src)
	if err != nil {
		return nil, err
	}
	for _, component := range cal.ChildrenNamed("VEVENT") {
		if VEventRecurrenceID(cal, component, time.Local) == override.id {
			return patchEventComponent(src, cal, component, override.change, now)
		}
	}
	return nil, errors.New("patch all: override disappeared")
}
