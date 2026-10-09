package calendar

import (
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

func TestVEventRecurrenceID(t *testing.T) {
	src := []byte("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:s\r\nDTSTART;TZID=Europe/Berlin:20261008T090000\r\nRRULE:FREQ=DAILY\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:s\r\nRECURRENCE-ID;TZID=Europe/Berlin:20261009T090000\r\nDTSTART;TZID=Europe/Berlin:20261009T100000\r\nEND:VEVENT\r\n" +
		"BEGIN:VEVENT\r\nUID:d\r\nRECURRENCE-ID;VALUE=DATE:20261010\r\nDTSTART;VALUE=DATE:20261011\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	cs, err := contentline.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	cal := cs[0]
	var got []string
	for _, v := range cal.ChildrenNamed("VEVENT") {
		got = append(got, VEventRecurrenceID(cal, v, time.UTC))
	}
	want := []string{"", "2026-10-09T07:00:00.000Z", "2026-10-10"}
	if !slices.Equal(got, want) {
		t.Errorf("keys = %q, want %q", got, want)
	}
	events, _ := Parse(src, Options{Local: time.UTC})
	if events[1].RecurrenceID != got[1] {
		t.Errorf("Parse gives %q", events[1].RecurrenceID)
	}
}
