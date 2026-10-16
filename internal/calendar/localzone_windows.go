package calendar

import (
	// Windows has no zoneinfo of its own: maild and mailctl carry Go's
	// (ADR-0038).
	_ "time/tzdata"

	"golang.org/x/sys/windows/registry"
)

// systemZoneName is the Windows zone the system is set to (such as
// "Eastern Standard Time"), which namedZone maps to IANA, or "".
func systemZoneName() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\TimeZoneInformation`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer func() { _ = k.Close() }()
	name, _, err := k.GetStringValue("TimeZoneKeyName")
	if err != nil {
		return ""
	}
	return name
}
