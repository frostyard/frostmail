package graph

import (
	"time"

	"github.com/frostyard/frostmail/internal/calendar"
)

// Series names a series' occurrences as the derived source does, by
// calendar.RecurrenceKey of their original starts, for writes to one
// occurrence.
type Series struct {
	allDay     bool
	loc        *time.Location
	exceptions map[string]string // keys to IDs
}

// SeriesOf reads a master (or a single event) and its exceptions.
func SeriesOf(master Event) (Series, error) {
	loc, err := time.LoadLocation(seriesZone(master))
	if err != nil {
		return Series{}, err
	}
	start, err := parseGraphTime(master.Start)
	if err != nil {
		return Series{}, err
	}
	s := Series{allDay: master.IsAllDay, loc: loc, exceptions: map[string]string{}}
	for _, ex := range master.ExceptionOccurrences {
		at, err := exceptionStart(ex, start, loc, master.IsAllDay)
		if err != nil {
			return Series{}, err
		}
		s.exceptions[calendar.RecurrenceKey(at, master.IsAllDay)] = ex.ID
	}
	return s, nil
}

// Exception is the ID of the exception an occurrence key names.
func (s Series) Exception(key string) (string, bool) {
	id, ok := s.exceptions[key]
	return id, ok
}

// Key is an unchanged occurrence's key, from its start (Instances).
func (s Series) Key(occurrence Event) (string, error) {
	at, err := parseGraphTime(occurrence.Start)
	if err != nil {
		return "", err
	}
	if s.allDay {
		at = allDayDate(at, s.loc)
	}
	return calendar.RecurrenceKey(at, s.allDay), nil
}
