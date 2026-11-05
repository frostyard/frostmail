package problemrec

import (
	"testing"

	"github.com/frostyard/std/problems"
)

// Only a release build records: the tag's version without its v. Every
// placeholder stays as it is and gives an inert recorder.
func TestVersionRecordsOnlyReleases(t *testing.T) {
	for _, tc := range []struct {
		build, want string
		active      bool
	}{
		{"v0.4.1", "0.4.1", true},
		{"0.4.1", "0.4.1", true},
		{"v0.4.1-rc.2", "0.4.1-rc.2", true},
		{"dev", "dev", false},
		{"flatpak", "flatpak", false},
		{"0.0.0-c0ffee1-snapshot", "0.0.0-c0ffee1-snapshot", false},
		{"v0.4.1-3-gabc1234", "0.4.1-3-gabc1234", false},
		{"v0.4.1-dirty", "0.4.1-dirty", false},
		{"c0ffee1", "c0ffee1", false},
		{"vv0.4.1", "v0.4.1", false},
		{"", "", false},
	} {
		if got := Version(tc.build); got != tc.want {
			t.Errorf("Version(%q) = %q, want %q", tc.build, got, tc.want)
		}
		err := New(tc.build, problems.ComponentDaemon).Err()
		if (err == nil) != tc.active {
			t.Errorf("New(%q).Err() = %v, want active %v", tc.build, err, tc.active)
		}
	}
}
