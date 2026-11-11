package settings_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/settings"
)

// mainFixture draws the first run, as the main window does, on a fixture's
// window.
func mainFixture(t *testing.T, s setup) (*fixture, *ui.Tester) {
	t.Helper()
	f := newFixture(t, s)
	tt := ui.NewTester(func(c *ui.Context) {
		f.loop.Flush()
		if f.win.NeedsFirstRun() {
			f.win.FirstRun(c)
		}
	}, 900, 700)
	return f, tt
}

func untilOn(t *testing.T, f *fixture, tt *ui.Tester, what string, ok func() bool) {
	t.Helper()
	f.until(what, func() bool { tt.Frame(); return ok() })
}

func (f *fixture) panesOpened() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.panes)
}

func TestFirstRunOpensAccountsOncePerConnection(t *testing.T) {
	f, tt := mainFixture(t, setup{})
	untilOn(t, f, tt, "the welcome", func() bool { return tt.HasText("Welcome to Frostmail") })
	untilOn(t, f, tt, "accounts opened", func() bool { return slices.Equal(f.panesOpened(), []string{settings.PaneAccounts}) })
	if !tt.HasText("Add a mail account to start. Frostmail keeps its mail on this computer.") {
		t.Errorf("the welcome says %q", tt.Texts())
	}
	tt.Frame()
	if got := f.panesOpened(); len(got) != 1 {
		t.Errorf("opened again: %q", got)
	}

	// A new connection that still finds none opens it again.
	f.win.Disconnect()
	untilOn(t, f, tt, "the end of the welcome", func() bool { return !f.win.NeedsFirstRun() })
	f.connect()
	untilOn(t, f, tt, "accounts opened again", func() bool { return len(f.panesOpened()) == 2 })
}

func TestFirstRunButtons(t *testing.T) {
	f, tt := mainFixture(t, setup{})
	untilOn(t, f, tt, "the welcome", func() bool { return tt.HasText("Welcome to Frostmail") })
	untilOn(t, f, tt, "accounts opened", func() bool { return len(f.panesOpened()) == 1 })
	if err := tt.Click("Add Account…"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Connect to lodge…"); err != nil {
		t.Fatal(err)
	}
	if got := f.panesOpened(); !slices.Equal(got, []string{settings.PaneAccounts, settings.PaneAccounts, settings.PaneServer}) {
		t.Errorf("panes = %q", got)
	}
}

func TestFirstRunOnLodge(t *testing.T) {
	f, tt := mainFixture(t, setup{})
	f.inLodge()
	untilOn(t, f, tt, "the welcome", func() bool { return tt.HasText("Welcome to Frostmail") })
	if !tt.HasText("Add a mail account to start.") || tt.HasText("keeps its mail on this computer") || tt.HasText("Connect to lodge…") {
		t.Errorf("the welcome on lodge shows %q", tt.Texts())
	}
}

func TestAnAccountEndsTheWelcomeAndRemovingTheLastDoesNotReopen(t *testing.T) {
	f, tt := mainFixture(t, setup{first: func(f *fixture) { f.addIMAP("me@a.test") }})
	f.until("the accounts", func() bool { return f.tt.HasText("me@a.test") })
	tt.Frame()
	if f.win.NeedsFirstRun() {
		t.Fatal("first run with an account")
	}
	list := f.listed()
	if err := f.c.Account().Delete(t.Context(), &api.AccountDeleteParams{ID: list[0].ID}); err != nil {
		t.Fatal(err)
	}
	untilOn(t, f, tt, "the welcome", func() bool { return tt.HasText("Welcome to Frostmail") })
	tt.Frame()
	if got := f.panesOpened(); len(got) != 0 {
		t.Errorf("removing the last account opened %q", got)
	}
	f.addIMAP("next@a.test")
	untilOn(t, f, tt, "the modules", func() bool { return !f.win.NeedsFirstRun() })
}

func TestMoveNoteShowsAndDismissesForGood(t *testing.T) {
	note := "This computer also has mail from a Frostmail installed outside Flatpak."
	facts := settings.ServerFacts{Flatpak: true, HostMail: true}
	f, tt := mainFixture(t, setup{options: func(o *settings.Options) { o.Facts = facts }})
	untilOn(t, f, tt, "the note", func() bool { return tt.HasText(note) })
	writeSnapshot(t, tt, "firstrun", 900)
	for _, want := range []string{"systemctl --user disable --now maild.service", "mailctl export ~/frostmail-export",
		"flatpak run --command=mailctl org.frostyard.Frostmail import ~/frostmail-export",
		"Then open Frostmail again. The folder ~/frostmail-export holds your passwords in plain text: delete it once your accounts sync."} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if err := tt.Click("Dismiss"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText(note) {
		t.Error("the note stays after Dismiss")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(f.config), "move-note-dismissed")); err != nil {
		t.Error(err)
	}

	// On this computer it does not come back.
	second := newFixtureAt(t, f.config, facts)
	secondTT := ui.NewTester(func(c *ui.Context) {
		second.loop.Flush()
		if second.win.NeedsFirstRun() {
			second.win.FirstRun(c)
		}
	}, 900, 700)
	untilOn(t, second, secondTT, "the welcome", func() bool { return secondTT.HasText("Welcome to Frostmail") })
	if secondTT.HasText(note) {
		t.Error("the note came back")
	}
}

// newFixtureAt is a fixture over an existing app.json's directory.
func newFixtureAt(t *testing.T, config string, facts settings.ServerFacts) *fixture {
	t.Helper()
	return newFixture(t, setup{options: func(o *settings.Options) { o.ConfigFile, o.Facts = config, facts }})
}

func TestNoMoveNoteOutsideTheFlatpakOrOnLodge(t *testing.T) {
	note := "This computer also has mail"
	f, tt := mainFixture(t, setup{options: func(o *settings.Options) { o.Facts = settings.ServerFacts{HostMail: true} }})
	untilOn(t, f, tt, "the welcome", func() bool { return tt.HasText("Welcome to Frostmail") })
	if tt.HasText(note) {
		t.Error("a note outside the Flatpak")
	}
	g, gt := mainFixture(t, setup{options: func(o *settings.Options) { o.Facts = settings.ServerFacts{Flatpak: true, HostMail: true} }})
	g.inLodge()
	untilOn(t, g, gt, "the welcome", func() bool { return gt.HasText("Welcome to Frostmail") })
	if gt.HasText(note) {
		t.Error("a note on lodge")
	}
	h, ht := mainFixture(t, setup{options: func(o *settings.Options) { o.Facts = settings.ServerFacts{Flatpak: true} }})
	untilOn(t, h, ht, "the welcome", func() bool { return ht.HasText("Welcome to Frostmail") })
	if ht.HasText(note) {
		t.Error("a note with no host mail")
	}
}

func TestFirstRunOpensTheLocalPaneWithoutAnOpener(t *testing.T) {
	f, tt := mainFixture(t, setup{options: func(o *settings.Options) { o.OpenSettings = nil }})
	untilOn(t, f, tt, "the welcome", func() bool { return tt.HasText("Welcome to Frostmail") })
	if err := tt.Click("Connect to lodge…"); err != nil {
		t.Fatal(err)
	}
	f.sees("maild on this computer")
}
