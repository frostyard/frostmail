package calendar

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/teambition/rrule-go"

	"github.com/frostyard/frostmail/internal/contentline"
)

// EventChange is a change to an event (calendar.update's EventFields,
// ADR-0030): nil fields keep their values. A change of timing gives all
// of it: AllDay, and Start, End and Zone for a timed event or StartDate
// and EndDate for an all-day one. Its JSON is how a put's payload keeps
// it, to apply again after a 412.
type EventChange struct {
	Summary     *string    `json:"summary,omitzero"`
	Location    *string    `json:"location,omitzero"`
	Description *string    `json:"description,omitzero"`
	AllDay      *bool      `json:"allDay,omitzero"`
	Start       *time.Time `json:"start,omitzero"` // timed: instants, written in Zone; End after Start
	End         *time.Time `json:"end,omitzero"`
	Zone        *string    `json:"zone,omitzero"`        // an IANA zone name, or UTC
	StartDate   *string    `json:"startDate,omitzero"`   // all-day: YYYY-MM-DD
	EndDate     *string    `json:"endDate,omitzero"`     // all-day: the day after the last
	RRule       *string    `json:"rrule,omitzero"`       // the RRULE value; "" removes RRULE
	Alarms      *[]int     `json:"alarms,omitzero"`      // minutes before the start (negative: after); replaces the DISPLAY and AUDIO alarms
	Transparent *bool      `json:"transparent,omitzero"` // TRANSP:TRANSPARENT, or OPAQUE when false
}

// PatchEvent applies c to the single event, or the master of the series,
// in src: its VEVENT without RECURRENCE-ID, rewriting only the lines c
// touches (ADR-0018, ADR-0030, ADR-0033).
func PatchEvent(src []byte, c EventChange, now time.Time) ([]byte, error) {
	cal, event, err := eventPatchMaster(src)
	if err != nil {
		return nil, err
	}
	return patchEventComponent(src, cal, event, c, now)
}

func patchEventComponent(src []byte, cal, event *contentline.Component, c EventChange, now time.Time) ([]byte, error) {
	var timing []contentline.Prop
	var err error
	if c.AllDay != nil || c.Start != nil || c.End != nil || c.Zone != nil || c.StartDate != nil || c.EndDate != nil {
		timing, err = eventTiming(c)
		if err != nil {
			return nil, fmt.Errorf("patch event: %w", err)
		}
	}
	fields, err := eventFields(c, timing)
	if err != nil {
		return nil, fmt.Errorf("patch event: %w", err)
	}
	if len(fields) == 0 && c.Alarms == nil {
		return src, nil
	}
	fields = append(fields, eventRevisionFields(event, now, len(timing) > 0 || c.RRule != nil)...)
	eol := contentline.LineEnding(src)
	zone, err := eventNewZone(cal, c, eol)
	if err != nil {
		return nil, fmt.Errorf("patch event: %w", err)
	}
	edits := eventPropertyEdits(src, event, fields, eol)
	if len(timing) > 0 {
		edits = append(edits, eventFieldEdits(event, contentline.Prop{Name: "DURATION"}, 0, eol)...)
	}
	if zone != "" {
		at := cal.ChildrenNamed("VEVENT")[0].Start
		edits = append(edits, contentline.Edit{Start: at, End: at, Text: zone})
	}
	edits = append(edits, eventAlarmEdits(src, event, c, eol)...)
	out, err := contentline.Apply(src, edits...)
	if err != nil {
		return nil, fmt.Errorf("patch event: %w", err)
	}
	return out, nil
}

func eventRevisionFields(event *contentline.Component, now time.Time, increment bool) []contentline.Prop {
	var fields []contentline.Prop
	if increment {
		sequence, _ := strconv.Atoi(propertyValue(event, "SEQUENCE"))
		fields = append(fields, contentline.Prop{Name: "SEQUENCE", Value: strconv.Itoa(sequence + 1)})
	}
	stamp := now.UTC().Format("20060102T150405Z")
	return append(fields, contentline.Prop{Name: "DTSTAMP", Value: stamp}, contentline.Prop{Name: "LAST-MODIFIED", Value: stamp})
}

// NewEvent writes a calendar object holding one new VEVENT, and the
// VTIMEZONE of its zone.
func NewEvent(uid string, c EventChange, now time.Time) ([]byte, error) {
	timing, err := eventTiming(c)
	if err != nil {
		return nil, fmt.Errorf("new event: %w", err)
	}
	fields, err := eventFields(c, nil)
	if err != nil {
		return nil, fmt.Errorf("new event: %w", err)
	}
	const eol = "\r\n"
	zone, err := eventNewZone(nil, c, eol)
	if err != nil {
		return nil, fmt.Errorf("new event: %w", err)
	}
	stamp := now.UTC().Format("20060102T150405Z")
	var out strings.Builder
	out.WriteString(encodeEventProps(eol, contentline.Prop{Name: "BEGIN", Value: "VCALENDAR"},
		contentline.Prop{Name: "VERSION", Value: "2.0"}, contentline.Prop{Name: "PRODID", Value: "-//Frostyard//Frostmail//EN"}))
	out.WriteString(zone)
	out.WriteString(encodeEventProps(eol, contentline.Prop{Name: "BEGIN", Value: "VEVENT"},
		contentline.Prop{Name: "UID", Value: contentline.EscapeText(uid)}, contentline.Prop{Name: "DTSTAMP", Value: stamp},
		contentline.Prop{Name: "CREATED", Value: stamp}, contentline.Prop{Name: "LAST-MODIFIED", Value: stamp},
		contentline.Prop{Name: "SEQUENCE", Value: "0"}))
	out.WriteString(encodeEventProps(eol, timing...))
	for _, field := range fields {
		if field.Value != "" {
			out.WriteString(field.Encode(eol))
		}
	}
	out.WriteString(eventAlarms(c, "", eol))
	out.WriteString(encodeEventProps(eol, contentline.Prop{Name: "END", Value: "VEVENT"},
		contentline.Prop{Name: "END", Value: "VCALENDAR"}))
	return []byte(out.String()), nil
}

func eventPatchMaster(src []byte) (*contentline.Component, *contentline.Component, error) {
	components, err := contentline.Parse(src)
	if err != nil {
		return nil, nil, fmt.Errorf("patch event: %w", err)
	}
	for _, cal := range components {
		if cal.Name != "VCALENDAR" {
			continue
		}
		for _, event := range cal.ChildrenNamed("VEVENT") {
			if event.Prop("RECURRENCE-ID") == nil {
				return cal, event, nil
			}
		}
		return nil, nil, errors.New("patch event: no master VEVENT")
	}
	return nil, nil, errors.New("patch event: no VCALENDAR")
}

func eventTiming(c EventChange) ([]contentline.Prop, error) {
	if c.AllDay == nil {
		return nil, errors.New("event timing requires all-day flag")
	}
	if *c.AllDay {
		return eventDateTiming(c)
	}
	if c.Start == nil || c.End == nil || c.Zone == nil || *c.Zone == "" || *c.Zone == "Local" {
		return nil, errors.New("event timing requires start, end and explicit zone")
	}
	if !c.End.After(*c.Start) {
		return nil, errors.New("event end must be after start")
	}
	loc, err := time.LoadLocation(*c.Zone)
	if err != nil {
		return nil, fmt.Errorf("event zone: %w", err)
	}
	layout := "20060102T150405"
	var params []contentline.Param
	if *c.Zone == "UTC" {
		layout += "Z"
	} else {
		params = []contentline.Param{{Name: "TZID", Values: []string{*c.Zone}}}
	}
	return []contentline.Prop{
		{Name: "DTSTART", Params: params, Value: c.Start.In(loc).Format(layout)},
		{Name: "DTEND", Params: params, Value: c.End.In(loc).Format(layout)},
	}, nil
}

func eventDateTiming(c EventChange) ([]contentline.Prop, error) {
	if c.StartDate == nil || c.EndDate == nil {
		return nil, errors.New("all-day event requires start and end dates")
	}
	start, err := time.Parse(time.DateOnly, *c.StartDate)
	if err != nil {
		return nil, fmt.Errorf("event start date: %w", err)
	}
	end, err := time.Parse(time.DateOnly, *c.EndDate)
	if err != nil {
		return nil, fmt.Errorf("event end date: %w", err)
	}
	if !end.After(start) {
		return nil, errors.New("event end date must be after start date")
	}
	params := []contentline.Param{{Name: "VALUE", Values: []string{"DATE"}}}
	return []contentline.Prop{
		{Name: "DTSTART", Params: params, Value: start.Format("20060102")},
		{Name: "DTEND", Params: params, Value: end.Format("20060102")},
	}, nil
}

// eventFields orders changed properties; an empty value means deletion.
func eventFields(c EventChange, timing []contentline.Prop) ([]contentline.Prop, error) {
	var fields []contentline.Prop
	for _, field := range []struct {
		name  string
		value *string
	}{{"SUMMARY", c.Summary}, {"LOCATION", c.Location}, {"DESCRIPTION", c.Description}} {
		if field.value != nil {
			fields = append(fields, contentline.Prop{Name: field.name, Value: contentline.EscapeText(*field.value)})
		}
	}
	fields = append(fields, timing...)
	if c.RRule != nil {
		if *c.RRule != "" {
			if _, err := rrule.StrToROption(*c.RRule); err != nil {
				return nil, fmt.Errorf("event recurrence rule: %w", err)
			}
		}
		fields = append(fields, contentline.Prop{Name: "RRULE", Value: *c.RRule})
	}
	if c.Transparent != nil {
		value := "OPAQUE"
		if *c.Transparent {
			value = "TRANSPARENT"
		}
		fields = append(fields, contentline.Prop{Name: "TRANSP", Value: value})
	}
	return fields, nil
}

// eventNewZone is called after timing validation; nil cal creates a new object.
func eventNewZone(cal *contentline.Component, c EventChange, eol string) (string, error) {
	if c.AllDay == nil || *c.AllDay || *c.Zone == "UTC" {
		return "", nil
	}
	if cal != nil {
		for _, zone := range cal.ChildrenNamed("VTIMEZONE") {
			if propertyText(zone, "TZID") == *c.Zone {
				return "", nil
			}
		}
	}
	return VTimezone(*c.Zone, *c.Start, eol)
}

func eventPropertyEdits(src []byte, event *contentline.Component, fields []contentline.Prop, eol string) []contentline.Edit {
	at := todoEndStart(src, event.End)
	if len(event.Children) > 0 {
		at = event.Children[0].Start
	}
	var edits []contentline.Edit
	for _, field := range fields {
		edits = append(edits, eventFieldEdits(event, field, at, eol)...)
	}
	return edits
}

func eventFieldEdits(event *contentline.Component, field contentline.Prop, at int, eol string) []contentline.Edit {
	text := ""
	if field.Value != "" {
		text = field.Encode(eol)
	}
	props := event.PropsNamed(field.Name)
	if len(props) == 0 {
		if text == "" {
			return nil
		}
		return []contentline.Edit{{Start: at, End: at, Text: text}}
	}
	var edits []contentline.Edit
	for _, prop := range props {
		edits = append(edits, contentline.Edit{Start: prop.Start, End: prop.End, Text: text})
		text = "" // Remove repeats after replacing the first property.
	}
	return edits
}

func eventAlarmEdits(src []byte, event *contentline.Component, c EventChange, eol string) []contentline.Edit {
	if c.Alarms == nil {
		return nil
	}
	var edits []contentline.Edit
	for _, alarm := range event.ChildrenNamed("VALARM") {
		switch strings.ToUpper(propertyValue(alarm, "ACTION")) {
		case "DISPLAY", "AUDIO":
			edits = append(edits, contentline.Edit{Start: alarm.Start, End: alarm.End})
		}
	}
	at := todoEndStart(src, event.End)
	return append(edits, contentline.Edit{Start: at, End: at, Text: eventAlarms(c, propertyText(event, "SUMMARY"), eol)})
}

func eventAlarms(c EventChange, summary, eol string) string {
	if c.Alarms == nil {
		return ""
	}
	if c.Summary != nil {
		summary = *c.Summary
	}
	if summary == "" {
		summary = "Reminder"
	}
	var out strings.Builder
	for _, minutes := range *c.Alarms {
		out.WriteString(encodeEventProps(eol, contentline.Prop{Name: "BEGIN", Value: "VALARM"},
			contentline.Prop{Name: "ACTION", Value: "DISPLAY"},
			contentline.Prop{Name: "DESCRIPTION", Value: contentline.EscapeText(summary)},
			contentline.Prop{Name: "TRIGGER", Value: eventAlarmDuration(minutes)},
			contentline.Prop{Name: "END", Value: "VALARM"}))
	}
	return out.String()
}

func eventAlarmDuration(minutes int) string {
	if minutes == 0 {
		return "PT0S"
	}
	prefix, magnitude := "-P", uint(minutes)
	if minutes < 0 {
		// Avoid overflowing when minutes is the smallest int.
		prefix, magnitude = "P", uint(-(minutes+1))+1
	}
	var out strings.Builder
	out.WriteString(prefix)
	if days := magnitude / 1440; days > 0 {
		fmt.Fprintf(&out, "%dD", days)
	}
	if rest := magnitude % 1440; rest > 0 {
		out.WriteByte('T')
		if hours := rest / 60; hours > 0 {
			fmt.Fprintf(&out, "%dH", hours)
		}
		if mins := rest % 60; mins > 0 {
			fmt.Fprintf(&out, "%dM", mins)
		}
	}
	return out.String()
}

func encodeEventProps(eol string, props ...contentline.Prop) string {
	var out strings.Builder
	for _, prop := range props {
		out.WriteString(prop.Encode(eol))
	}
	return out.String()
}
