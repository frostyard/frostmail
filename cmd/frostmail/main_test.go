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

// P-805: a notification's click and its Reply are different arguments.
func TestMessageArgs(t *testing.T) {
	open := []string{"--open-message", "7"}
	reply := []string{"--reply-message", "9"}
	if id, ok := openMessageArg(open); !ok || id != 7 {
		t.Errorf("openMessageArg(%q) = %d, %v", open, id, ok)
	}
	if id, ok := replyMessageArg(reply); !ok || id != 9 {
		t.Errorf("replyMessageArg(%q) = %d, %v", reply, id, ok)
	}
	if _, ok := replyMessageArg(open); ok {
		t.Error("--open-message started a reply")
	}
	if _, ok := openMessageArg(reply); ok {
		t.Error("--reply-message opened a message")
	}
	for _, bad := range [][]string{{"--reply-message"}, {"--reply-message", "0"}, {"--reply-message", "-3"}, {"--reply-message", "x"}} {
		if id, ok := replyMessageArg(bad); ok {
			t.Errorf("replyMessageArg(%q) = %d", bad, id)
		}
	}
}
