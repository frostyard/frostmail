package calendar

import (
	"os"
	"strings"
	"time"
)

// LocalZone is the system's zone under its IANA name, which writes to
// Graph and the editor's defaults name: $TZ when it names a zone, else
// the system's setting (the /etc/localtime link on Linux, the registry's
// time zone on Windows, through the Windows zone table). time.Local, whose
// name is "Local", when neither names one.
func LocalZone() *time.Location {
	if name := strings.TrimPrefix(os.Getenv("TZ"), ":"); name != "" {
		if loc, ok := loadZone(name); ok {
			return loc
		}
	}
	if name := systemZoneName(); name != "" {
		if loc, ok := namedZone(name); ok {
			return loc
		}
	}
	return time.Local
}
