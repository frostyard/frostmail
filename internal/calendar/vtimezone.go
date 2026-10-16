package calendar

import (
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

// vtimezoneEnd is where the generated rules stop looking: the last year
// 32-bit clients and most servers expand.
var vtimezoneEnd = time.Date(2038, 1, 1, 0, 0, 0, 0, time.UTC)

// transition is one change of a zone's offset.
type transition struct {
	at       time.Time // the instant, in UTC
	from, to int       // offsets in seconds east of UTC
	name     string    // the abbreviation after it
	dst      bool      // daylight time after it
}

// local is the wall time the transition happens at, before it.
func (t transition) local() time.Time {
	return t.at.In(time.FixedZone("", t.from))
}

// VTimezone writes the VTIMEZONE of an IANA zone for events from from on
// (ADR-0033), with lines ending in eol. It walks the zone's transitions
// from the year before from to 2037 in maild's tzdata. Standard and
// daylight time are each a component with a yearly RRULE when their
// transitions keep one (the same month, n-th or last weekday and wall
// time); otherwise each transition is an RDATE. Transitions between other
// offsets or under another name are components of their own. A zone
// without transitions then is one STANDARD.
func VTimezone(name string, from time.Time, eol string) (string, error) {
	if name == "" || name == "Local" {
		return "", fmt.Errorf("vtimezone: %q is not a zone name", name)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return "", fmt.Errorf("vtimezone: %w", err)
	}
	start := time.Date(from.Year()-1, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := transitions(loc, start, vtimezoneEnd)
	lines := []string{"BEGIN:VTIMEZONE", "TZID:" + contentline.EscapeText(name), "X-LIC-LOCATION:" + contentline.EscapeText(name)}
	if len(ts) == 0 {
		abbr, offset := start.In(loc).Zone()
		lines = append(lines, "BEGIN:STANDARD", "DTSTART:19700101T000000", "TZOFFSETFROM:"+formatOffset(offset),
			"TZOFFSETTO:"+formatOffset(offset), "TZNAME:"+contentline.EscapeText(abbr), "END:STANDARD")
	}
	for _, group := range observances(ts) {
		lines = append(lines, observance(group)...)
	}
	lines = append(lines, "END:VTIMEZONE")
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(contentline.Fold(l, eol))
	}
	return b.String(), nil
}

// observances groups transitions by what one component can say: daylight
// or standard, between the same offsets, under the same name; the groups
// come in the order of their first transitions.
func observances(ts []transition) [][]transition {
	type key struct {
		dst      bool
		from, to int
		name     string
	}
	at := map[key]int{}
	var out [][]transition
	for _, t := range ts {
		k := key{t.dst, t.from, t.to, t.name}
		i, ok := at[k]
		if !ok {
			i = len(out)
			at[k] = i
			out = append(out, nil)
		}
		out[i] = append(out[i], t)
	}
	return out
}

// transitions lists a zone's offset changes in [from, to).
func transitions(loc *time.Location, from, to time.Time) []transition {
	var out []transition
	t := from.In(loc)
	for {
		_, end := t.ZoneBounds()
		if !end.IsZero() && !end.After(t) {
			// Go's rule-based periods (the slim tzdata it embeds and reads
			// on Windows) end the last day of a leap year before it begins
			// (Go 1.27): step over that day an hour at a time.
			end = t.Add(time.Hour)
		}
		if end.IsZero() || !end.Before(to) {
			return out
		}
		_, before := t.Zone()
		after := end.In(loc)
		name, offset := after.Zone()
		if offset != before {
			out = append(out, transition{at: end.UTC(), from: before, to: offset, name: name, dst: after.IsDST()})
		}
		t = after
	}
}

// observance writes one component (DAYLIGHT or STANDARD) for transitions
// of one kind: with an RRULE when they keep a yearly rule, else RDATEs.
func observance(ts []transition) []string {
	if len(ts) == 0 {
		return nil
	}
	first := ts[0]
	kind := "STANDARD"
	if first.dst {
		kind = "DAYLIGHT"
	}
	lines := []string{"BEGIN:" + kind, "DTSTART:" + first.local().Format("20060102T150405"),
		"TZOFFSETFROM:" + formatOffset(first.from), "TZOFFSETTO:" + formatOffset(first.to),
		"TZNAME:" + contentline.EscapeText(first.name)}
	if rule, ok := yearlyRule(ts); ok {
		lines = append(lines, "RRULE:"+rule)
	} else {
		for _, t := range ts[1:] {
			lines = append(lines, "RDATE:"+t.local().Format("20060102T150405"))
		}
	}
	return append(lines, "END:"+kind)
}

var weekdays = [...]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}

// yearlyRule is the RRULE transitions keep, one a year to the end of the
// span: the same offsets, month, weekday and wall time, on the same n-th
// (1 to 4) or last weekday of the month.
func yearlyRule(ts []transition) (string, bool) {
	first := ts[0].local()
	// A rule must run to the end of the span, or it would go on past the
	// last transition: a few that happen to match are RDATEs.
	if ts[len(ts)-1].local().Year() != vtimezoneEnd.Year()-1 {
		return "", false
	}
	allLast, allNth := true, true
	nth := (first.Day()-1)/7 + 1
	for i, t := range ts {
		l := t.local()
		if t.from != ts[0].from || t.to != ts[0].to || l.Month() != first.Month() || l.Weekday() != first.Weekday() ||
			l.Hour() != first.Hour() || l.Minute() != first.Minute() || l.Second() != first.Second() ||
			l.Year() != first.Year()+i {
			return "", false
		}
		daysInMonth := time.Date(l.Year(), l.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
		allLast = allLast && l.Day()+7 > daysInMonth
		allNth = allNth && (l.Day()-1)/7+1 == nth
	}
	day := weekdays[first.Weekday()]
	switch {
	case allNth && nth <= 4:
		return fmt.Sprintf("FREQ=YEARLY;BYMONTH=%d;BYDAY=%d%s", first.Month(), nth, day), true
	case allLast:
		return fmt.Sprintf("FREQ=YEARLY;BYMONTH=%d;BYDAY=-1%s", first.Month(), day), true
	}
	return "", false
}

// formatOffset writes seconds east of UTC as ±HHMM, or ±HHMMSS.
func formatOffset(seconds int) string {
	sign := '+'
	if seconds < 0 {
		sign, seconds = '-', -seconds
	}
	s := fmt.Sprintf("%c%02d%02d", sign, seconds/3600, seconds/60%60)
	if seconds%60 != 0 {
		s += fmt.Sprintf("%02d", seconds%60)
	}
	return s
}
