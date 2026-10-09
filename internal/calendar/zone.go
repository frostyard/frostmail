package calendar

import (
	"fmt"
	"strconv"
	"time"
	_ "time/tzdata" // Resolve IANA zones even without system zoneinfo.

	"github.com/frostyard/frostmail/internal/contentline"
)

// resolveZone tries IANA, Windows and object-local zone definitions in order.
func resolveZone(tzid string, calendar *contentline.Component) *time.Location {
	if zone, ok := namedZone(tzid); ok {
		return zone
	}
	for _, component := range calendar.ChildrenNamed("VTIMEZONE") {
		if propertyText(component, "TZID") != tzid {
			continue
		}
		if zone, ok := loadZone(propertyText(component, "X-LIC-LOCATION")); ok {
			return zone
		}
		if zone, ok := fixedZone(component); ok {
			return zone
		}
	}
	return time.UTC
}

// namedZone resolves an IANA or Windows zone name.
func namedZone(tzid string) (*time.Location, bool) {
	if zone, ok := loadZone(tzid); ok {
		return zone, true
	}
	if name, ok := WindowsZone(tzid); ok {
		return loadZone(name)
	}
	return nil, false
}

func loadZone(name string) (*time.Location, bool) {
	if name == "" || name == "Local" {
		return nil, false
	}
	zone, err := time.LoadLocation(name)
	return zone, err == nil
}

// fixedZone uses the latest STANDARD observance, or DAYLIGHT when absent.
func fixedZone(c *contentline.Component) (*time.Location, bool) {
	observances := c.ChildrenNamed("STANDARD")
	if len(observances) == 0 {
		observances = c.ChildrenNamed("DAYLIGHT")
	}
	var latest *contentline.Component
	var latestStart time.Time
	for _, observance := range observances {
		start, err := time.Parse("20060102T150405", propertyValue(observance, "DTSTART"))
		if err == nil && (latest == nil || start.After(latestStart)) {
			latest, latestStart = observance, start
		}
	}
	if latest == nil {
		return nil, false
	}
	offset, ok := zoneOffset(propertyValue(latest, "TZOFFSETTO"))
	if !ok {
		return nil, false
	}
	sign, absolute := '+', offset
	if offset < 0 {
		sign, absolute = '-', -offset
	}
	name := fmt.Sprintf("UTC%c%02d:%02d", sign, absolute/3600, absolute/60%60)
	return time.FixedZone(name, offset), true
}

func zoneOffset(value string) (int, bool) {
	if (len(value) != 5 && len(value) != 7) || (value[0] != '+' && value[0] != '-') {
		return 0, false
	}
	units := [...]int{3600, 60, 1}
	offset := 0
	for i := 1; i < len(value); i += 2 {
		if value[i] < '0' || value[i] > '9' || value[i+1] < '0' || value[i+1] > '9' {
			return 0, false
		}
		n, err := strconv.Atoi(value[i : i+2])
		if err != nil || (i == 1 && n > 23) || (i > 1 && n > 59) {
			return 0, false
		}
		offset += n * units[(i-1)/2]
	}
	if value[0] == '-' {
		offset = -offset
	}
	return offset, true
}

// ZoneNamed returns the zone an Event's TZID names: an IANA zone, UTC, or
// a fixed offset written UTC±hh:mm; local for "" (floating). A name it
// cannot read is UTC.
func ZoneNamed(tzid string, local *time.Location) *time.Location {
	if tzid == "" {
		return local
	}
	if zone, ok := loadZone(tzid); ok {
		return zone
	}
	if len(tzid) == len("UTC+05:30") && tzid[:3] == "UTC" && tzid[6] == ':' {
		if offset, ok := zoneOffset(tzid[3:6] + tzid[7:]); ok {
			return time.FixedZone(tzid, offset)
		}
	}
	return time.UTC
}
