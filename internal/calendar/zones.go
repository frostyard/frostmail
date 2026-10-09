// Package calendar reads iCalendar events and expands their occurrences
// (docs/design/pim.md, Calendars; ADR-0018).
package calendar

// WindowsZone returns the IANA zone for a Windows time zone name, such as
// Outlook writes in TZID ("Eastern Standard Time" is America/New_York).
func WindowsZone(name string) (string, bool) {
	z, ok := windowsZones[name]
	return z, ok
}
