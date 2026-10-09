package calendar

import (
	"testing"
	"time"
)

func TestWindowsZone(t *testing.T) {
	for win, iana := range map[string]string{
		"Eastern Standard Time":   "America/New_York",
		"W. Europe Standard Time": "Europe/Berlin",
		"Tokyo Standard Time":     "Asia/Tokyo",
		"UTC":                     "Etc/UTC",
	} {
		got, ok := WindowsZone(win)
		if !ok || got != iana {
			t.Errorf("WindowsZone(%q) = %q, %v; want %q", win, got, ok, iana)
		}
		if _, err := time.LoadLocation(got); err != nil {
			t.Errorf("%s: %v", got, err)
		}
	}
	if _, ok := WindowsZone("America/New_York"); ok {
		t.Error("an IANA name is not a Windows name")
	}
	for win, iana := range windowsZones {
		if _, err := time.LoadLocation(iana); err != nil {
			t.Errorf("%s maps to %s, which Go cannot load: %v", win, iana, err)
		}
	}
}
