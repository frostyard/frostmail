package main

import (
	"testing"

	"github.com/frostyard/frostmail/internal/config"
)

// M10: the development run is an instance of its own, so a second start of
// it never hands its arguments to the user's app; the app itself keeps
// its ID.
func TestAppNameGivesTheDevelopmentRunItsOwnInstance(t *testing.T) {
	if got := appName(func(string) string { return "" }); got != config.AppID {
		t.Errorf("the app's name = %q, want %q", got, config.AppID)
	}
	dev := appName(func(k string) string {
		if k == "FROSTMAIL_INSTANCE" {
			return "dev"
		}
		return ""
	})
	if dev == config.AppID || dev == "" {
		t.Errorf("the development run's name = %q, the app's", dev)
	}
}
