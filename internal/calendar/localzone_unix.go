//go:build !windows

package calendar

import (
	"os"
	"strings"
)

// systemZoneName is the zone /etc/localtime links to, or "".
func systemZoneName() string {
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return ""
	}
	_, name, _ := strings.Cut(target, "zoneinfo/")
	return name
}
