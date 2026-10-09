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

func ruleStarts(event Event, value string, from, to time.Time) []time.Time {
	zone := eventZone(event)
	options, err := rrule.StrToROptionInLocation(value, zone)
	if err != nil {
		return nil
	}
	options.Dtstart = event.Start.In(zone)
	rule, err := rrule.NewRRule(*options)
	if err != nil {
		return nil
	}
	// Include starts at the lower bound, including zero-duration events.
	after := from.Add(-event.End.Sub(event.Start)).Add(-time.Nanosecond)
	return rule.Between(after, to, true)
}

func recurrenceDates(event Event, prop contentline.Prop) []time.Time {
	if strings.EqualFold(prop.Param("VALUE"), "PERIOD") {
		return nil
	}
	reader := timeReader{calendar: &contentline.Component{}, local: eventZone(event)}
	var dates []time.Time
	for value := range strings.SplitSeq(prop.Value, ",") {
		prop.Value = value
		if date, ok := reader.read(&prop); ok {
			dates = append(dates, date.instant)
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
