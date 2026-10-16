// Package calendar reads iCalendar events and expands their occurrences
// (docs/design/pim.md, Calendars; ADR-0018).
package calendar

// WindowsZone returns the IANA zone for a Windows time zone name, such as
// Outlook writes in TZID ("Eastern Standard Time" is America/New_York).
func WindowsZone(name string) (string, bool) {
	z, ok := windowsZones[name]
	return z, ok
}

// WindowsName returns the Windows time zone name whose main IANA zone
// (CLDR's territory 001) is iana, such as Microsoft Graph takes in
// timeZone ("America/New_York" is "Eastern Standard Time"). Of two names
// with the same zone, the first in order wins.
func WindowsName(iana string) (string, bool) {
	best := ""
	for name, z := range windowsZones {
		if z == iana && (best == "" || name < best) {
			best = name
		}
	}
	return best, best != ""
}
