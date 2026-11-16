package main

import (
	"slices"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
)

func TestTrayStatus(t *testing.T) {
	for n, want := range map[int64]string{0: "0 unread", 1: "1 unread", 12: "12 unread"} {
		if got := trayStatus(n); got != want {
			t.Errorf("trayStatus(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestTrayItemsNewMessage(t *testing.T) {
	var made int
	items := trayItems(func() { made++ })
	if len(items) != 1 {
		t.Fatalf("the tray has %d items, want 1", len(items))
	}
	item := items[0]
	if item.ID != "frostmail.new-message" || item.Label != "New Message" {
		t.Errorf("the item is %q, %q", item.ID, item.Label)
	}
	if item.Click == nil {
		t.Fatal("the item has no click")
	}
	item.Click()
	if made != 1 {
		t.Errorf("the click started %d messages, want 1", made)
	}
}

// queue is the UI thread's queue, as poster.post fills it.
type queue struct {
	posted []func()
}

func (q *queue) post(fn func()) { q.posted = append(q.posted, fn) }

// run runs what was posted, as the UI thread would.
func (q *queue) run() {
	posted := q.posted
	q.posted = nil
	for _, fn := range posted {
		fn()
	}
}

// fakeTray is the tray's status, without an icon.
type fakeTray struct {
	status    string
	dot       bool
	enabled   bool
	closes    int
	destroyed int
}

func (f *fakeTray) SetStatus(line string) { f.status = line }

func (f *fakeTray) SetDot(on bool) { f.dot = on }

func (f *fakeTray) CloseToTray(_ *mygo.CloseEvent, enabled bool) bool {
	f.closes++
	f.enabled = enabled
	return enabled
}

func (f *fakeTray) Destroy() { f.destroyed++ }

func TestTrayCountCoalesces(t *testing.T) {
	var n atomic.Int64
	q := &queue{}
	tc := &trayCount{post: q.post, count: n.Load}
	icon := &fakeTray{}
	tc.attach(icon)
	if icon.status != "0 unread" || icon.dot {
		t.Errorf("the attached icon shows %q, dot %v, want the count at once", icon.status, icon.dot)
	}

	n.Store(5)
	tc.Changed()
	tc.Changed()
	tc.Changed()
	if len(q.posted) != 1 {
		t.Fatalf("3 changes posted %d updates, want 1", len(q.posted))
	}
	q.run()
	if icon.status != "5 unread" || !icon.dot {
		t.Errorf("the tray shows %q, dot %v, want 5 unread with the dot", icon.status, icon.dot)
	}

	n.Store(0)
	tc.Changed()
	if len(q.posted) != 1 {
		t.Fatalf("a change after the update posted %d updates, want 1", len(q.posted))
	}
	q.run()
	if icon.status != "0 unread" || icon.dot {
		t.Errorf("the tray shows %q, dot %v, want nothing unread", icon.status, icon.dot)
	}
}

func TestTrayCountWithoutAnIcon(t *testing.T) {
	n := int64(3)
	q := &queue{}
	tc := &trayCount{post: q.post, count: func() int64 { return n }}
	tc.Changed() // no icon yet: nothing to show, and no panic
	q.run()
	if len(q.posted) != 0 {
		t.Errorf("the update posted %d more", len(q.posted))
	}
	tc.Changed()
	if len(q.posted) != 1 {
		t.Errorf("a change after the update posted %d updates, want 1", len(q.posted))
	}
}

// closer is frostui's Tray side of the close, without a tray.
type closer struct {
	event   *mygo.CloseEvent
	enabled bool
	calls   int
}

func (c *closer) CloseToTray(e *mygo.CloseEvent, enabled bool) bool {
	c.calls++
	c.event, c.enabled = e, enabled
	return enabled
}

func TestCloseToTrayPassesTheSetting(t *testing.T) {
	c := &closer{}
	e := &mygo.CloseEvent{}
	on := false
	listener := closeToTray(c, func() bool { return on })
	listener(e)
	if c.event != e || c.enabled {
		t.Errorf("the close passed %p, enabled %v; want the event, off", c.event, c.enabled)
	}
	on = true
	listener(e)
	if c.event != e || !c.enabled {
		t.Errorf("the close passed %p, enabled %v; want the event, on", c.event, c.enabled)
	}
	if c.calls != 2 {
		t.Errorf("the tray decided %d closes, want 2", c.calls)
	}
}

// fakeWindow keeps the main window's listeners, as mygo.Window does.
type fakeWindow struct {
	onClose  []func(*mygo.CloseEvent)
	onClosed []func()
}

func (w *fakeWindow) OnClose(fn func(*mygo.CloseEvent)) func() {
	w.onClose = append(w.onClose, fn)
	return func() {}
}

func (w *fakeWindow) OnClosed(fn func()) func() {
	w.onClosed = append(w.onClosed, fn)
	return func() {}
}

// close and closed run the listeners, as the app's window does.
func (w *fakeWindow) close(e *mygo.CloseEvent) {
	for _, fn := range w.onClose {
		fn(e)
	}
}

func (w *fakeWindow) closed() {
	for _, fn := range w.onClosed {
		fn()
	}
}

// fakeSetting is this computer's close-to-tray setting.
type fakeSetting struct {
	on   bool
	seen []bool
}

func (s *fakeSetting) CloseToTray() bool { return s.on }

func (s *fakeSetting) SetTray(shown bool) { s.seen = append(s.seen, shown) }

func TestWireTrayTakesTheTrayWithTheWindow(t *testing.T) {
	var n atomic.Int64
	n.Store(2)
	q := &queue{}
	tc := &trayCount{post: q.post, count: n.Load}
	win := &fakeWindow{}
	tray := &fakeTray{}
	setting := &fakeSetting{}
	wireTray(win, tray, tc, setting)

	if len(setting.seen) != 1 || !setting.seen[0] {
		t.Fatalf("SetTray got %v, want [true]", setting.seen)
	}
	if tray.status != "2 unread" || !tray.dot {
		t.Errorf("the tray shows %q, dot %v, want 2 unread with the dot", tray.status, tray.dot)
	}

	setting.on = true
	win.close(&mygo.CloseEvent{})
	if tray.closes != 1 || !tray.enabled {
		t.Errorf("the close passed enabled %v, %d closes; want this computer's on", tray.enabled, tray.closes)
	}

	// The window's own close, after a close the tray did not take, takes
	// the icon away: Open must not show a window that is gone.
	win.closed()
	if tray.destroyed != 1 {
		t.Errorf("the tray was destroyed %d times, want 1", tray.destroyed)
	}
	if len(setting.seen) != 2 || setting.seen[1] {
		t.Errorf("SetTray got %v, want [true false]", setting.seen)
	}
}

// fakeRaiser records what raise calls, in order.
type fakeRaiser struct {
	calls []string
}

func (r *fakeRaiser) Show()    { r.calls = append(r.calls, "show") }
func (r *fakeRaiser) Restore() { r.calls = append(r.calls, "restore") }
func (r *fakeRaiser) Focus()   { r.calls = append(r.calls, "focus") }

func TestRaiseShowsTheHiddenWindow(t *testing.T) {
	r := &fakeRaiser{}
	raise(r)
	if want := []string{"show", "restore", "focus"}; !slices.Equal(r.calls, want) {
		t.Errorf("raise called %v, want %v: a window close to tray hid is shown first", r.calls, want)
	}
}
