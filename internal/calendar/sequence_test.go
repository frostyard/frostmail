package calendar

import (
	"strings"
	"testing"
)

func TestRaiseSequence(t *testing.T) {
	if got := MaxSequence([]byte(googleSeries)); got != 0 {
		t.Errorf("MaxSequence = %d", got)
	}
	raised, err := RaiseSequence([]byte(googleSeries), 3)
	if err != nil {
		t.Fatal(err)
	}
	// Every VEVENT, the master and its override, goes past the floor, and
	// nothing else changes.
	same(t, string(raised), strings.ReplaceAll(googleSeries, "SEQUENCE:0\r\n", "SEQUENCE:4\r\n"))
	if got := MaxSequence(raised); got != 4 {
		t.Errorf("MaxSequence after = %d", got)
	}
	// A VEVENT without SEQUENCE gets one, before its alarms.
	bare := strings.Replace(googleEvent, "SEQUENCE:0\r\n", "", 1)
	raised, err = RaiseSequence([]byte(bare), 0)
	if err != nil || !strings.Contains(string(raised), "SEQUENCE:1\r\nBEGIN:VALARM") {
		t.Errorf("RaiseSequence of a bare event = %v\n%s", err, raised)
	}
}
