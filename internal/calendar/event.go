package calendar

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

// Event is one VEVENT: a single event, a series' master, or an override of
// one occurrence of a series (RecurrenceID set).
type Event struct {
	UID string
	// RecurrenceID is "" for a single event or a master; for an override,
	// the start of the occurrence it replaces, formatted as RecurrenceKey.
	RecurrenceID string
	Summary      string
	Location     string
	Description  string
	AllDay       bool
	// Start and End: for a timed event, instants in UTC (End exclusive);
	// for an all-day event, midnight UTC of the first day and of the day
	// after the last.
	Start, End time.Time
	// TZID is the IANA zone DTSTART was resolved to: "UTC" for UTC times,
	// "" for floating times and all-day events.
	TZID string
	// Floating is a timed event without a zone, read in Options.Local.
	Floating bool
	// Recurrence is the RRULE, RDATE and EXDATE content lines as written,
	// unfolded and joined by "\n"; "" for an event that is not a series.
	Recurrence  string
	Status      string // confirmed, tentative or cancelled
	Transparent bool   // TRANSP:TRANSPARENT: does not count as busy
	Organizer   *Attendee
	Attendees   []Attendee // without the organizer
	// PartStat is the user's answer when one of Options.UserEmails is an
	// attendee: needsaction, accepted, declined, tentative, delegated; "".
	PartStat string
	Sequence int
	Alarms   []Alarm
	// Zone is the location DTSTART is read in, for expanding the series
	// in its own wall time: the TZID's zone, Options.Local when floating,
	// UTC for UTC and all-day events.
	Zone *time.Location
}

// Attendee is an ORGANIZER or ATTENDEE.
type Attendee struct {
	Email    string // lowercased, without mailto:
	Name     string // CN
	Role     string // chair, required, optional, none
	PartStat string // needsaction, accepted, declined, tentative, delegated
}

// Alarm is a DISPLAY or AUDIO VALARM.
type Alarm struct {
	Action string // display or audio
	// Offset is relative to the occurrence's start, or its end when
	// RelatedEnd; negative is before. Unused when At is set.
	Offset     time.Duration
	RelatedEnd bool
	At         time.Time // an absolute trigger, in UTC; zero for a relative one
}

// Options say how to read an object's events.
type Options struct {
	// Local is the zone of floating times; nil means time.Local.
	Local *time.Location
	// UserEmails are the account's own addresses, lowercased: the attendee
	// with one of them is the user.
	UserEmails []string
}

// Parse reads usable VEVENTs from the first VCALENDAR, resolving times,
// people and supported alarms while retaining recurrence content lines.
func Parse(raw []byte, opts Options) ([]Event, error) {
	components, err := contentline.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse calendar: %w", err)
	}
	if opts.Local == nil {
		opts.Local = time.Local
	}
	for _, c := range components {
		if c.Name != "VCALENDAR" {
			continue
		}
		reader := timeReader{calendar: c, local: opts.Local}
		var events []Event
		for _, component := range c.ChildrenNamed("VEVENT") {
			if event, ok := readEvent(component, reader, raw, opts.UserEmails); ok {
				events = append(events, event)
			}
		}
		return events, nil
	}
	return nil, errors.New("parse calendar: no VCALENDAR")
}

// RecurrenceKey formats an occurrence's original start as a UTC date for
// all-day events, or a UTC instant with millisecond precision otherwise.
func RecurrenceKey(t time.Time, allDay bool) string {
	if allDay {
		return t.UTC().Format("2006-01-02")
	}
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func readEvent(c *contentline.Component, reader timeReader, raw []byte, emails []string) (Event, bool) {
	start, ok := reader.read(c.Prop("DTSTART"))
	if !ok {
		return Event{}, false
	}
	event := Event{
		UID:     strings.TrimSpace(propertyValue(c, "UID")),
		Summary: propertyText(c, "SUMMARY"), Location: propertyText(c, "LOCATION"),
		Description: propertyText(c, "DESCRIPTION"), AllDay: start.allDay,
		Start: start.instant, TZID: start.tzid, Zone: start.zone, Floating: start.floating,
		Status: "confirmed", Transparent: strings.EqualFold(propertyValue(c, "TRANSP"), "TRANSPARENT"),
		Recurrence: recurrenceLines(c, raw),
	}
	switch strings.ToUpper(propertyValue(c, "STATUS")) {
	case "TENTATIVE":
		event.Status = "tentative"
	case "CANCELLED":
		event.Status = "cancelled"
	}
	if sequence, err := strconv.Atoi(propertyValue(c, "SEQUENCE")); err == nil {
		event.Sequence = sequence
	}
	event.End = readEnd(c, reader, start)
	if id, ok := reader.read(c.Prop("RECURRENCE-ID")); ok {
		event.RecurrenceID = RecurrenceKey(id.instant, id.allDay)
	}
	readPeople(c, &event, emails)
	event.Alarms = readAlarms(c, reader)
	return event, true
}

func propertyValue(c *contentline.Component, name string) string {
	if p := c.Prop(name); p != nil {
		return p.Value
	}
	return ""
}

func propertyText(c *contentline.Component, name string) string {
	if p := c.Prop(name); p != nil {
		return p.Text()
	}
	return ""
}

type timeReader struct {
	calendar *contentline.Component
	local    *time.Location
}

type eventTime struct {
	instant          time.Time
	zone             *time.Location
	tzid             string
	allDay, floating bool
}

func (r timeReader) read(p *contentline.Prop) (eventTime, bool) {
	if p == nil {
		return eventTime{}, false
	}
	value := p.Value
	result := eventTime{zone: time.UTC}
	layout := "20060102T150405"
	switch {
	case strings.EqualFold(p.Param("VALUE"), "DATE") || len(value) == 8:
		result.allDay = true
		layout = "20060102"
	case strings.HasSuffix(value, "Z"):
		result.tzid = "UTC"
		layout += "Z"
	case p.Param("TZID") != "":
		result.zone = resolveZone(p.Param("TZID"), r.calendar)
		result.tzid = result.zone.String()
	default:
		result.zone, result.floating = r.local, true
	}
	instant, err := time.ParseInLocation(layout, value, result.zone)
	if err != nil {
		return eventTime{}, false
	}
	result.instant = instant.UTC()
	return result, true
}

func readEnd(c *contentline.Component, reader timeReader, start eventTime) time.Time {
	if end, ok := reader.read(c.Prop("DTEND")); ok {
		return end.instant
	}
	if duration, ok := parseDuration(propertyValue(c, "DURATION")); ok {
		return start.instant.Add(duration)
	}
	if start.allDay {
		return start.instant.AddDate(0, 0, 1)
	}
	return start.instant
}

func recurrenceLines(c *contentline.Component, raw []byte) string {
	var lines []string
	unfold := strings.NewReplacer("\r\n ", "", "\r\n\t", "", "\n ", "", "\n\t", "")
	for _, p := range c.Props {
		switch p.Name {
		case "RRULE", "RDATE", "EXDATE":
			line := strings.TrimRight(string(raw[p.Start:p.End]), "\r\n")
			lines = append(lines, unfold.Replace(line))
		}
	}
	return strings.Join(lines, "\n")
}

func readPeople(c *contentline.Component, event *Event, emails []string) {
	if p := c.Prop("ORGANIZER"); p != nil {
		event.Organizer = &Attendee{
			Email: personEmail(p.Value), Name: p.Param("CN"), Role: "chair", PartStat: "accepted",
		}
	}
	for _, p := range c.PropsNamed("ATTENDEE") {
		attendee := readAttendee(p)
		if organizer := event.Organizer; organizer != nil && organizer.Email == attendee.Email {
			if p.Param("PARTSTAT") != "" {
				organizer.PartStat = attendee.PartStat
			}
			if organizer.Name == "" {
				organizer.Name = attendee.Name
			}
			continue
		}
		event.Attendees = append(event.Attendees, attendee)
		if event.PartStat == "" && slices.Contains(emails, attendee.Email) {
			event.PartStat = attendee.PartStat
		}
	}
}

func personEmail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= len("mailto:") && strings.EqualFold(value[:len("mailto:")], "mailto:") {
		value = value[len("mailto:"):]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func readAttendee(p contentline.Prop) Attendee {
	attendee := Attendee{Email: personEmail(p.Value), Name: p.Param("CN"), Role: "required", PartStat: "needsaction"}
	switch strings.ToUpper(p.Param("ROLE")) {
	case "CHAIR":
		attendee.Role = "chair"
	case "OPT-PARTICIPANT":
		attendee.Role = "optional"
	case "NON-PARTICIPANT":
		attendee.Role = "none"
	}
	switch strings.ToUpper(p.Param("PARTSTAT")) {
	case "ACCEPTED", "DECLINED", "TENTATIVE", "DELEGATED":
		attendee.PartStat = strings.ToLower(p.Param("PARTSTAT"))
	}
	return attendee
}

func readAlarms(c *contentline.Component, reader timeReader) []Alarm {
	var alarms []Alarm
	for _, component := range c.ChildrenNamed("VALARM") {
		action := strings.ToLower(propertyValue(component, "ACTION"))
		trigger := component.Prop("TRIGGER")
		if (action != "display" && action != "audio") || trigger == nil {
			continue
		}
		alarm := Alarm{Action: action}
		if strings.EqualFold(trigger.Param("VALUE"), "DATE-TIME") {
			at, ok := reader.read(trigger)
			if !ok || at.allDay {
				continue
			}
			alarm.At = at.instant
		} else {
			offset, ok := parseDuration(trigger.Value)
			if !ok {
				continue
			}
			alarm.Offset = offset
			alarm.RelatedEnd = strings.EqualFold(trigger.Param("RELATED"), "END")
		}
		alarms = append(alarms, alarm)
	}
	return alarms
}

var durationPattern = regexp.MustCompile(`^([+-]?)P(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)

func parseDuration(value string) (time.Duration, bool) {
	parts := durationPattern.FindStringSubmatch(value)
	if parts == nil || strings.HasSuffix(value, "T") {
		return 0, false
	}
	units := [...]time.Duration{7 * 24 * time.Hour, 24 * time.Hour, time.Hour, time.Minute, time.Second}
	var total time.Duration
	have := false
	for i, unit := range units {
		if parts[i+2] == "" {
			continue
		}
		have = true
		n, err := strconv.ParseInt(parts[i+2], 10, 64)
		if err != nil || n > (1<<63-1-int64(total))/int64(unit) {
			return 0, false
		}
		total += time.Duration(n) * unit
	}
	if parts[1] == "-" {
		total = -total
	}
	return total, have
}
