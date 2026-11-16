//go:build !windows

package launcher

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// call is one signal the fake bus was asked to emit.
type call struct {
	path   dbus.ObjectPath
	name   string
	values []any
}

// fake stands in for the session bus: it records every emit and fails on
// demand.
type fake struct {
	mu    sync.Mutex
	calls []call
	sent  chan call

	fail bool
}

func newFake() *fake { return &fake{sent: make(chan call, 64)} }

func (f *fake) emit(path dbus.ObjectPath, name string, values ...any) error {
	c := call{path: path, name: name, values: values}
	f.mu.Lock()
	f.calls = append(f.calls, c)
	fail := f.fail
	f.mu.Unlock()
	f.sent <- c
	if fail {
		return errors.New("no listener")
	}
	return nil
}

// all is every emit so far, in order.
func (f *fake) all() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]call(nil), f.calls...)
}

// next returns the next emit, or fails after a few seconds.
func next(t *testing.T, f *fake) call {
	t.Helper()
	select {
	case c := <-f.sent:
		return c
	case <-time.After(5 * time.Second):
		t.Fatalf("no signal within 5 s; %d sent", len(f.all()))
		return call{}
	}
}

// count reads the count of one signal body.
func count(t *testing.T, c call) int64 {
	t.Helper()
	hints := hints(t, c)
	n, ok := hints["count"].Value().(int64)
	if !ok {
		t.Fatalf("count is %#v, not an int64", hints["count"].Value())
	}
	return n
}

// visible reads whether one signal body shows the count.
func visible(t *testing.T, c call) bool {
	t.Helper()
	hints := hints(t, c)
	v, ok := hints["count-visible"].Value().(bool)
	if !ok {
		t.Fatalf("count-visible is %#v, not a bool", hints["count-visible"].Value())
	}
	return v
}

// hints checks the body's app URI and returns its hints.
func hints(t *testing.T, c call) map[string]dbus.Variant {
	t.Helper()
	if len(c.values) != 2 {
		t.Fatalf("the signal body is %#v, not 2 values", c.values)
	}
	if got, ok := c.values[0].(string); !ok || got != "application://org.frostyard.Frostmail.desktop" {
		t.Fatalf("the app URI is %#v", c.values[0])
	}
	m, ok := c.values[1].(map[string]dbus.Variant)
	if !ok {
		t.Fatalf("the hints are %#v, not a map of variants", c.values[1])
	}
	return m
}

// discard is a logger that keeps nothing.
func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestUpdateShowsTheCount(t *testing.T) {
	f := newFake()
	var n atomic.Int64
	n.Store(3)
	e := newEntry(discard(), n.Load, f.emit, nil)
	defer e.Close()
	e.Changed()
	c := next(t, f)
	if want := dbus.ObjectPath("/com/canonical/unity/launcherentry/3122036172"); c.path != want {
		t.Errorf("path = %q, want %q", c.path, want)
	}
	if want := "com.canonical.Unity.LauncherEntry.Update"; c.name != want {
		t.Errorf("signal = %q, want %q", c.name, want)
	}
	if got := count(t, c); got != 3 {
		t.Errorf("count = %d, want 3", got)
	}
	if !visible(t, c) {
		t.Error("the count is not visible")
	}
}

func TestZeroHidesTheCount(t *testing.T) {
	for _, n := range []int64{0, -1} { // a negative count shows as 0
		f := newFake()
		var c0 atomic.Int64
		c0.Store(n)
		e := newEntry(discard(), c0.Load, f.emit, nil)
		e.Changed()
		got := next(t, f)
		if v := count(t, got); v != 0 {
			t.Errorf("count %d shows as %d, want 0", n, v)
		}
		if visible(t, got) {
			t.Errorf("count %d shows the count", n)
		}
		e.Close()
	}
}

func TestTheSameCountSendsNothing(t *testing.T) {
	f := newFake()
	var n atomic.Int64
	n.Store(3)
	e := newEntry(discard(), n.Load, f.emit, nil)
	defer e.Close()
	e.Changed()
	if got := count(t, next(t, f)); got != 3 {
		t.Fatalf("count = %d, want 3", got)
	}
	e.Changed() // the same count: the dock already shows it
	n.Store(2)
	e.Changed()
	if got := count(t, next(t, f)); got != 2 {
		t.Errorf("the next signal is %d, want 2: the same count was sent again", got)
	}
	if got := len(f.all()); got != 2 {
		t.Errorf("%d signals, want 2", got)
	}
}

func TestTheLatestCountWins(t *testing.T) {
	f := newFake()
	var n atomic.Int64
	e := newEntry(discard(), n.Load, f.emit, nil)
	defer e.Close()
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			n.Store(int64(i + 1))
			e.Changed()
		})
	}
	wg.Wait()
	n.Store(42)
	e.Changed()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case c := <-f.sent:
			if count(t, c) != 42 {
				continue
			}
		case <-deadline:
			t.Fatalf("no signal with the final count; %d sent", len(f.all()))
		}
		break
	}
	if all := f.all(); count(t, all[len(all)-1]) != 42 {
		t.Errorf("the last signal is %d, want 42", count(t, all[len(all)-1]))
	}
}

func TestCloseClearsTheBadge(t *testing.T) {
	f := newFake()
	var n atomic.Int64
	n.Store(5)
	var closed atomic.Int32
	e := newEntry(discard(), n.Load, f.emit, func() error {
		closed.Add(1)
		return nil
	})
	e.Changed()
	if got := count(t, next(t, f)); got != 5 {
		t.Fatalf("count = %d, want 5", got)
	}
	e.Close()
	all := f.all()
	last := all[len(all)-1]
	if got := count(t, last); got != 0 {
		t.Errorf("the cleared count is %d, want 0", got)
	}
	if visible(t, last) {
		t.Error("the cleared count is visible")
	}
	if got := closed.Load(); got != 1 {
		t.Errorf("the bus was closed %d times, want 1", got)
	}
	e.Close() // twice is safe, and closes nothing again
	e.Changed()
	if got := closed.Load(); got != 1 {
		t.Errorf("the second Close closed the bus %d times, want 1", got)
	}
	if got := len(f.all()); got != len(all) {
		t.Errorf("a Close or a Changed after Close sent %d more signals", got-len(all))
	}
}

func TestFailedEmitsLogOnce(t *testing.T) {
	f := newFake()
	f.fail = true
	var n atomic.Int64
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	e := newEntry(log, n.Load, f.emit, nil)
	defer e.Close()
	for i := range 3 {
		n.Store(int64(i + 1))
		e.Changed()
		next(t, f)
	}
	if got := strings.Count(buf.String(), "launcher count"); got != 1 {
		t.Errorf("%d log records, want 1: %q", got, buf.String())
	}
}

func TestInertEntryDoesNothing(t *testing.T) {
	var calls atomic.Int32
	e := newEntry(discard(), func() int64 { calls.Add(1); return 1 }, nil, nil)
	e.Changed()
	e.Close()
	e.Close()
	e.Changed()
	if got := calls.Load(); got != 0 {
		t.Errorf("the count was read %d times", got)
	}
}

func TestPathIsLibunitys(t *testing.T) {
	// Computed with glib's own g_str_hash, through python ctypes.
	if got := strHash("application://org.frostyard.Frostmail.desktop"); got != 3122036172 {
		t.Errorf("strHash = %d, want 3122036172", got)
	}
	if want := dbus.ObjectPath("/com/canonical/unity/launcherentry/3122036172"); entryPath != want {
		t.Errorf("entryPath = %q, want %q", entryPath, want)
	}
}
