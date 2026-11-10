package mainwin_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// Ctrl+1–4 and the module bar switch modules, and the palette follows.
func TestModulesSwitchByKeyAndBar(t *testing.T) {
	f := newFixture(t, 1)
	f.until(t, "the row", func() bool { return f.tt.HasText("Message 1") })
	f.tt.Key(ui.Cmd, ui.Key2)
	f.until(t, "Calendar", func() bool { _, ok := f.tt.Access("Calendars"); return ok })
	if f.tt.HasText("Sort by Date") {
		t.Errorf("the list shows in Calendar: %q", f.tt.Texts())
	}
	f.tt.Key(ui.Cmd, ui.KeyK)
	f.tt.Frame()
	if !f.tt.HasText("New Event") {
		t.Errorf("the palette in Calendar shows %q", f.tt.Texts())
	}
	f.tt.Key(0, ui.KeyEscape)
	f.tt.Frame()
	f.tt.Key(ui.Cmd, ui.Key3)
	f.until(t, "People", func() bool { _, ok := f.tt.Access("Address Books"); return ok })
	if err := f.tt.Click("Tasks"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "Tasks", func() bool { return f.tt.HasText("All Tasks") })
	f.tt.Key(ui.Cmd, ui.Key1)
	f.until(t, "Mail", func() bool { return f.tt.HasText("Message 1") })
}

// The To-Do Bar shows beside the reader, in Mail.
func TestToDoBarToggles(t *testing.T) {
	f := newFixture(t, 1)
	f.until(t, "the row", func() bool { return f.tt.HasText("Message 1") })
	if _, ok := f.tt.Access("To-Do Bar"); !ok {
		t.Fatalf("no button in %q", f.tt.Texts())
	}
	if err := f.tt.Click("To-Do Bar"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the bar", func() bool { return f.tt.HasText("Upcoming") })
}

// Pane widths and the list's choices last between runs.
func TestLayoutLastsBetweenRuns(t *testing.T) {
	srv := rpctest.Start(t)
	file := filepath.Join(t.TempDir(), "window.json")
	start := func() (*mainwin.Window, *ui.Tester) {
		loop := frostuitest.NewLoop()
		win := mainwin.New(mainwin.Options{Target: conn.Target{Socket: srv.Socket}, Client: "frostmail test", Post: loop.Post,
			Wake: time.Millisecond, Retry: 10 * time.Millisecond, StateFile: file})
		tt := ui.NewTester(func(c *ui.Context) {
			loop.Flush()
			win.View(c)
		}, 1200, 780)
		win.Start(t.Context())
		return win, tt
	}
	win, tt := start()
	waitFor(t, tt, "the panes", func() bool { return tt.HasText("Sort by Date") })
	if err := tt.Click("To-Do Bar"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, tt, "the bar", func() bool { return tt.HasText("Upcoming") })
	win.SaveState()

	_, tt2 := start()
	waitFor(t, tt2, "the bar again", func() bool { return tt2.HasText("Upcoming") })
}
