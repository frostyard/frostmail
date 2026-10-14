package calendar

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

// MaxSequence is the highest SEQUENCE among the first VCALENDAR's
// VEVENTs; 0 when none has one or the source does not parse.
func MaxSequence(src []byte) int {
	components, err := contentline.Parse(src)
	if err != nil {
		return 0
	}
	most := 0
	for _, cal := range components {
		if cal.Name != "VCALENDAR" {
			continue
		}
		for _, v := range cal.ChildrenNamed("VEVENT") {
			if n, err := strconv.Atoi(strings.TrimSpace(propertyValue(v, "SEQUENCE"))); err == nil && n > most {
				most = n
			}
		}
		break
	}
	return most
}

// RaiseSequence makes every VEVENT's SEQUENCE in src one more than the
// higher of its own and floor. A source written back over a later version
// (a restore, ADR-0032) must not go back in SEQUENCE: Google's CalDAV
// refuses that with 409, and clients read a lower SEQUENCE as outdated.
func RaiseSequence(src []byte, floor int) ([]byte, error) {
	components, err := contentline.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("raise sequence: %w", err)
	}
	eol := contentline.LineEnding(src)
	var edits []contentline.Edit
	for _, cal := range components {
		if cal.Name != "VCALENDAR" {
			continue
		}
		for _, v := range cal.ChildrenNamed("VEVENT") {
			own, _ := strconv.Atoi(strings.TrimSpace(propertyValue(v, "SEQUENCE")))
			next := strconv.Itoa(max(own, floor) + 1)
			edits = append(edits, eventPropertyEdits(src, v, []contentline.Prop{{Name: "SEQUENCE", Value: next}}, eol)...)
		}
		break
	}
	out, err := contentline.Apply(src, edits...)
	if err != nil {
		return nil, fmt.Errorf("raise sequence: %w", err)
	}
	return out, nil
}

// KeepOverrides is restored with a plain override, made from its master
// as This Event makes one, for each occurrence that current overrides and
// restored does not. Google's CalDAV keeps an exception its PUT leaves
// out, so a restore written there must reset the exception instead.
func KeepOverrides(restored, current []byte, now time.Time) ([]byte, error) {
	components, err := contentline.Parse(current)
	if err != nil {
		return nil, fmt.Errorf("keep overrides: %w", err)
	}
	have := map[string]bool{}
	for _, e := range mustEvents(restored) {
		have[e.RecurrenceID] = true
	}
	out := restored
	for _, cal := range components {
		if cal.Name != "VCALENDAR" {
			continue
		}
		for _, v := range cal.ChildrenNamed("VEVENT") {
			key := VEventRecurrenceID(cal, v, time.Local)
			if key == "" || have[key] {
				continue
			}
			next, _, err := PatchOccurrence(out, key, ScopeThis, EventChange{}, "", now)
			if err != nil {
				continue // the restored series has no such occurrence
			}
			out = next
			have[key] = true
		}
		break
	}
	return out, nil
}

func mustEvents(src []byte) []Event {
	events, _ := Parse(src, Options{Local: time.Local})
	return events
}
