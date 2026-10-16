package calendar

import "testing"

func TestLocalZoneTakesTZ(t *testing.T) {
	t.Setenv("TZ", ":Europe/Berlin")
	if got := LocalZone().String(); got != "Europe/Berlin" {
		t.Errorf("LocalZone with TZ = %s", got)
	}
	t.Setenv("TZ", "Nowhere/Special")
	if got := LocalZone(); got == nil {
		t.Fatal("LocalZone with an unknown TZ is nil")
	}
}

// The system's zone comes back under its IANA name: Linux's
// /etc/localtime link, or Windows' registry name through the zone table.
func TestLocalZoneNamesTheSystemZone(t *testing.T) {
	t.Setenv("TZ", "")
	name := systemZoneName()
	if name == "" {
		t.Skip("the system names no zone")
	}
	got := LocalZone().String()
	if got == "Local" {
		t.Fatalf("LocalZone = Local, though the system says %q", name)
	}
	if _, ok := loadZone(got); !ok {
		t.Errorf("LocalZone = %q, which does not load", got)
	}
}
