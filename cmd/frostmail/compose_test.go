package main

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
)

// M8: a part's path in the cache names its message and specifier, and
// nothing else is asked of maild.
func TestPartOf(t *testing.T) {
	for rel, want := range map[string]struct {
		message int64
		part    string
	}{
		"m/12/2.png":         {12, "2"},
		"m/12/1.2.jpeg":      {12, "1.2"},
		"m/7/1.2.3/name.pdf": {7, "1.2.3"},
	} {
		message, part, ok := partOf(rel)
		if !ok || message != want.message || part != want.part {
			t.Errorf("%s = %d, %q, %v", rel, message, part, ok)
		}
	}
	for _, rel := range []string{"", "r/abc.png", "m/x/2.png", "m/1/../../etc/passwd", "m/1/2", "/m/1/2.png", "m/1/2/a/b"} {
		if _, _, ok := partOf(rel); ok {
			t.Errorf("%q read as a part", rel)
		}
	}
}

// M7: a window's client ends with the target it opened on, and follows
// reconnects to it.
func TestBoundClientEndsWithItsTarget(t *testing.T) {
	first, second := &api.Client{}, &api.Client{}
	target, current := conn.Target{Socket: "/run/a"}, first
	client := boundClient(func() conn.Target { return target }, func() *api.Client { return current })
	if got := client(); got != first {
		t.Errorf("on its target: %p", got)
	}
	current = second
	if got := client(); got != second {
		t.Errorf("after a reconnect: %p", got)
	}
	lodge, err := conn.ParseLodge("https://lodge.test")
	if err != nil {
		t.Fatal(err)
	}
	target = conn.Target{Lodge: &lodge}
	if got := client(); got != nil {
		t.Errorf("after a change of target: %p", got)
	}
}
