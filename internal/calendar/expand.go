package calendar

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
	"github.com/teambition/rrule-go"
)

// Occurrence is one occurrence of an event.
type Occurrence struct {
	// Event is the index, in the events given to Expand, of the single
	// event, the series' master, or the override that replaced this
	// occurrence.
	Event int
	// RecurrenceID is RecurrenceKey of the occurrence's original start; ""
	// for a single event.
	RecurrenceID string
	Start, End   time.Time // as Event's: instants, or UTC midnights for all-day
	AllDay       bool
}

// Expand returns the occurrences of one calendar object's events that
// overlap [from, to), ordered by start and event index. Series expand in
// their own zones, with overrides replacing their original occurrences.
func Expand(events []Event, from, to time.Time) ([]Occurrence, error) {
	if !from.Before(to) {
		return nil, nil
	}
	overrides := make(map[string]map[string]int)
	for i, event := range events {
		if event.RecurrenceID == "" {
			continue
		}
		if overrides[event.UID] == nil {
			overrides[event.UID] = make(map[string]int)
		}
		overrides[event.UID][event.RecurrenceID] = i
	}
	var occurrences []Occurrence
	for i, event := range events {
		if event.RecurrenceID != "" || event.Recurrence == "" {
			if event.RecurrenceID != "" && overrides[event.UID][event.RecurrenceID] != i {
				continue
			}
			occ := occurrence(event, i, event.Start, event.RecurrenceID)
			if overlaps(occ, from, to) {
				occurrences = append(occurrences, occ)
			}
			continue
		}
		for id, start := range seriesStarts(event, from, to) {
			if _, replaced := overrides[event.UID][id]; replaced {
				continue
			}
			occ := occurrence(event, i, start, id)
			if overlaps(occ, from, to) {
				occurrences = append(occurrences, occ)
			}
		}
	}
	slices.SortFunc(occurrences, func(a, b Occurrence) int {
		if order := a.Start.Compare(b.Start); order != 0 {
			return order
		}
		return cmp.Compare(a.Event, b.Event)
	})
	return occurrences, nil
}

func occurrence(event Event, index int, start time.Time, id string) Occurrence {
	return Occurrence{
		Event: index, RecurrenceID: id, AllDay: event.AllDay,
		Start: start.UTC(), End: start.Add(event.End.Sub(event.Start)).UTC(),
	}
}

func overlaps(occ Occurrence, from, to time.Time) bool {
	if occ.Start.Equal(occ.End) {
		return !occ.Start.Before(from) && occ.Start.Before(to)
	}
	return occ.Start.Before(to) && occ.End.After(from)
}

func seriesStarts(event Event, from, to time.Time) map[string]time.Time {
	starts := map[string]time.Time{RecurrenceKey(event.Start, event.AllDay): event.Start.UTC()}
	excluded := make(map[string]time.Time)
	components, err := contentline.Parse([]byte("BEGIN:VEVENT\n" + event.Recurrence + "\nEND:VEVENT\n"))
	if err != nil || len(components) == 0 {
		return starts
	}
	for _, prop := range components[0].Props {
		var values []time.Time
		destination := starts
		switch prop.Name {
		case "RRULE":
			values = ruleStarts(event, prop.Value, from, to)
		case "RDATE", "EXDATE":
			values = recurrenceDates(event, prop)
			if prop.Name == "EXDATE" {
				destination = excluded
			}
		}
		for _, start := range values {
			destination[RecurrenceKey(start, event.AllDay)] = start.UTC()
		}
	}
	for id := range excluded {
		delete(starts, id)
	}
	return starts
}

// maxRuleStarts bounds the starts one RRULE generates, counted from
// DTSTART: a hostile or mistaken rule must not stall indexing.
const maxRuleStarts = 200_000

func ruleStarts(event Event, value string, from, to time.Time) []time.Time {
	zone := eventZone(event)
	options, err := rrule.StrToROptionInLocation(value, zone)
	if err != nil {
		return nil
	}
	// Calendar apps do not make series repeating more than hourly, and
	// those iterate from DTSTART for too long: read them as single events.
	if options.Freq == rrule.MINUTELY || options.Freq == rrule.SECONDLY {
		return nil
	}
	options.Dtstart = event.Start.In(zone)
	rule, err := rrule.NewRRule(*options)
	if err != nil {
		return nil
	}
	// Include starts at the lower bound, including zero-duration events.
	after := from.Add(-event.End.Sub(event.Start))
	var starts []time.Time
	next := rule.Iterator()
	for range maxRuleStarts {
		start, ok := next()
		if !ok || !start.Before(to) {
			break
		}
		if !start.Before(after) {
			starts = append(starts, start)
		}
	}
	return starts
}

func recurrenceDates(event Event, prop contentline.Prop) []time.Time {
	if strings.EqualFold(prop.Param("VALUE"), "PERIOD") {
		return nil
	}
	// A TZID defined only by the object's VTIMEZONE is the one DTSTART
	// used, in practice: read it in the event's zone rather than UTC.
	zone := eventZone(event)
	if tzid := prop.Param("TZID"); tzid != "" {
		if named, ok := namedZone(tzid); ok {
			zone = named
		}
	}
	var dates []time.Time
	for value := range strings.SplitSeq(prop.Value, ",") {
		value = strings.TrimSpace(value)
		var date time.Time
		var err error
		switch {
		case strings.EqualFold(prop.Param("VALUE"), "DATE") || len(value) == 8:
			date, err = time.ParseInLocation("20060102", value, time.UTC)
		case strings.HasSuffix(value, "Z"):
			date, err = time.Parse("20060102T150405Z", value)
		default:
			date, err = time.ParseInLocation("20060102T150405", value, zone)
		}
		if err == nil {
			dates = append(dates, date)
		}
	}
	return dates
}

func eventZone(event Event) *time.Location {
	if event.Zone != nil {
		return event.Zone
	}
	return time.UTC
}
