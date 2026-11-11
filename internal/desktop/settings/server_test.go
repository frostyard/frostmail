package settings_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/settings"
	"github.com/frostyard/frostmail/internal/rpctest"
)

const lodgeURL = "https://lodge.example.ts.net"

// serverFixture is the window with no maild, and a real app.json in a temp
// dir: the Server pane needs neither.
type serverFixture struct {
	*fixture
	mu       sync.Mutex
	steps    []string
	stopErr  error
	startErr error
	gate     chan struct{} // holds StopLocal and StartLocal back while set
	// drafts holds LeaveLocal back while set, as a compose window's save
	// in flight does.
	drafts chan struct{}
}

func (s *serverFixture) log(step string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.steps = append(s.steps, step)
}

func (s *serverFixture) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.steps...)
}

func serverSetup(t *testing.T, facts settings.ServerFacts, stored string, change func(*serverFixture)) *serverFixture {
	t.Helper()
	s := &serverFixture{}
	if change != nil {
		change(s)
	}
	dir := filepath.Join(t.TempDir(), "frostmail")
	cfg := filepath.Join(dir, "app.json")
	if stored != "" {
		if err := conn.WriteAppConfig(cfg, conn.AppConfig{Lodge: stored}); err != nil {
			t.Fatal(err)
		}
	}
	s.fixture = newFixture(t, setup{offline: true, options: func(o *settings.Options) {
		o.ConfigFile, o.Facts = cfg, facts
		o.SetTarget = func(tg conn.Target) { s.log("target " + tg.String()) }
		o.LeaveLocal = func() {
			s.log("leave")
			if s.drafts != nil {
				<-s.drafts
			}
			s.log("left")
		}
		o.StopLocal = func(ctx context.Context) error {
			s.log("stop")
			if s.gate != nil {
				<-s.gate
			}
			return s.stopErr
		}
		o.StartLocal = func(ctx context.Context) error {
			s.log("start")
			if s.gate != nil {
				<-s.gate
			}
			return s.startErr
		}
	}})
	s.config = cfg
	s.show(settings.PaneServer)
	s.sees("maild on this computer")
	return s
}

func (s *serverFixture) stored() string {
	s.t.Helper()
	cfg, err := conn.ReadAppConfig(s.config)
	if err != nil {
		s.t.Fatal(err)
	}
	return cfg.Lodge
}

func (s *serverFixture) chooseLodge(address string) {
	s.t.Helper()
	s.press(s.control(ui.RoleRadio, "maild behind lodge"))
	s.fill("lodge:", address)
}

func TestServerNoteSaysWhatTheChangeDoes(t *testing.T) {
	toLodge := "This computer's maild stops. Its accounts and mail stay on this computer, not synced, until you switch back. Nothing moves to lodge."
	withData := "This computer's maild starts again with the accounts it had. It runs their rules, Send Later and reminders here. If lodge's maild has the same accounts, both run them."
	without := "Mail on lodge keeps running there. If you add the same accounts here, both run their rules, Send Later and reminders."

	t.Run("to lodge", func(t *testing.T) {
		s := serverSetup(t, settings.ServerFacts{Flatpak: true}, "", nil)
		s.notSeen(toLodge)
		s.press(s.control(ui.RoleRadio, "maild behind lodge"))
		s.sees(toLodge)
	})
	t.Run("to local with data", func(t *testing.T) {
		s := serverSetup(t, settings.ServerFacts{Flatpak: true, LocalData: true}, lodgeURL, nil)
		s.press(s.control(ui.RoleRadio, "maild on this computer"))
		s.sees(withData)
	})
	t.Run("to local without data", func(t *testing.T) {
		s := serverSetup(t, settings.ServerFacts{Flatpak: true}, lodgeURL, nil)
		s.press(s.control(ui.RoleRadio, "maild on this computer"))
		s.sees(without)
	})
	t.Run("a new address keeps the mode", func(t *testing.T) {
		s := serverSetup(t, settings.ServerFacts{Flatpak: true}, lodgeURL, nil)
		s.fill("lodge:", "https://other.example.ts.net")
		s.notSeen(toLodge)
		s.notSeen(without)
	})
	t.Run("outside the Flatpak", func(t *testing.T) {
		s := serverSetup(t, settings.ServerFacts{}, "", nil)
		s.press(s.control(ui.RoleRadio, "maild behind lodge"))
		s.notSeen(toLodge)
	})
}

func TestServerSaveIsOffUntilTheChoiceDiffers(t *testing.T) {
	s := serverSetup(t, settings.ServerFacts{}, "", nil)
	if !s.disabled("Save") {
		t.Error("Save works with no change")
	}
	s.press(s.control(ui.RoleRadio, "maild behind lodge"))
	if !s.disabled("Save") {
		t.Error("Save works with no address")
	}
	if a := s.control(ui.RoleTextField, "lodge:"); a.Placeholder != "https://lodge.example.ts.net" {
		t.Errorf("the placeholder is %q", a.Placeholder)
	}
	s.fill("lodge:", lodgeURL)
	if s.disabled("Save") {
		t.Error("Save is off with a new choice")
	}
}

func TestChangeToLodgeWritesThenStopsThenMoves(t *testing.T) {
	s := serverSetup(t, settings.ServerFacts{Flatpak: true}, "", nil)
	s.chooseLodge(lodgeURL)
	s.click("Save")
	s.until("the move", func() bool { return len(s.seen()) == 4 })
	if got := s.seen(); !slices.Equal(got, []string{"leave", "left", "stop", "target lodge.example.ts.net"}) {
		t.Errorf("steps = %q", got)
	}
	if s.stored() != lodgeURL {
		t.Errorf("app.json says %q", s.stored())
	}
	s.until("the stored choice shown", func() bool { return s.control(ui.RoleRadio, "maild behind lodge").Checked && s.disabled("Save") })
}

// M7: a compose window's save still running holds the change to lodge
// back, so that this computer's maild stops only after its drafts saved.
func TestChangeToLodgeStopsMaildAfterTheDraftsSaved(t *testing.T) {
	s := serverSetup(t, settings.ServerFacts{Flatpak: true}, "", func(s *serverFixture) { s.drafts = make(chan struct{}) })
	s.chooseLodge(lodgeURL)
	s.click("Save")
	s.until("the drafts saving", func() bool { return slices.Contains(s.seen(), "leave") })
	if got := s.seen(); slices.Contains(got, "stop") {
		t.Fatalf("maild stopped while a draft saved: %q", got)
	}
	close(s.drafts)
	s.until("the move", func() bool { return len(s.seen()) == 4 })
	if got := s.seen(); !slices.Equal(got, []string{"leave", "left", "stop", "target lodge.example.ts.net"}) {
		t.Errorf("steps = %q", got)
	}
}

// M7: a change to this computer leaves lodge's maild running, so its
// compose windows close with the move (SetTarget), not before.
func TestChangeToLocalLeavesNothingFirst(t *testing.T) {
	s := serverSetup(t, settings.ServerFacts{Flatpak: true}, lodgeURL, nil)
	s.press(s.control(ui.RoleRadio, "maild on this computer"))
	s.click("Save")
	s.until("the move", func() bool { return len(s.seen()) == 2 })
	if got := s.seen(); slices.Contains(got, "leave") {
		t.Errorf("steps = %q", got)
	}
}

func TestAMaildThatDoesNotStopKeepsThisComputer(t *testing.T) {
	s := serverSetup(t, settings.ServerFacts{Flatpak: true}, "", func(s *serverFixture) { s.stopErr = errors.New("maild still holds the lock") })
	s.chooseLodge(lodgeURL)
	s.click("Save")
	s.sees("This computer's maild did not stop. Frostmail still uses it.")
	if got := s.seen(); !slices.Equal(got, []string{"leave", "left", "stop", "start"}) {
		t.Errorf("steps = %q, want stop then start and no move", got)
	}
	if s.stored() != "" {
		t.Errorf("app.json says %q after the write back", s.stored())
	}
	s.until("the stored choice shown", func() bool { return s.control(ui.RoleRadio, "maild on this computer").Checked })
}

func TestChangeToLocalStartsThenMoves(t *testing.T) {
	s := serverSetup(t, settings.ServerFacts{Flatpak: true}, lodgeURL, nil)
	s.press(s.control(ui.RoleRadio, "maild on this computer"))
	s.click("Save")
	s.until("the move", func() bool { return len(s.seen()) == 2 })
	if got := s.seen(); got[0] != "start" || got[1] != "target Local" {
		t.Errorf("steps = %q", got)
	}
	if s.stored() != "" {
		t.Errorf("app.json says %q", s.stored())
	}
}

func TestAMaildThatDoesNotStartStaysLocalAndSaysWhy(t *testing.T) {
	s := serverSetup(t, settings.ServerFacts{Flatpak: true}, lodgeURL, func(s *serverFixture) { s.startErr = errors.New("see maild.log") })
	s.press(s.control(ui.RoleRadio, "maild on this computer"))
	s.click("Save")
	s.sees("This computer's maild did not start: see maild.log")
	if got := s.seen(); len(got) != 2 || got[1] != "target Local" {
		t.Errorf("steps = %q: the session must try this computer", got)
	}
	if s.stored() != "" {
		t.Errorf("app.json says %q; the mode stays local", s.stored())
	}
}

func TestChangeOutsideTheFlatpakOnlyWritesAndMoves(t *testing.T) {
	s := serverSetup(t, settings.ServerFacts{}, "", nil)
	s.chooseLodge(lodgeURL)
	s.click("Save")
	s.until("the move", func() bool { return len(s.seen()) == 1 })
	if got := s.seen(); got[0] != "target lodge.example.ts.net" {
		t.Errorf("steps = %q", got)
	}
	if s.stored() != lodgeURL {
		t.Errorf("app.json says %q", s.stored())
	}
}

func TestANewLodgeAddressOnlyWritesAndMoves(t *testing.T) {
	s := serverSetup(t, settings.ServerFacts{Flatpak: true}, lodgeURL, nil)
	s.fill("lodge:", "https://other.example.ts.net")
	s.click("Save")
	s.until("the move", func() bool { return len(s.seen()) == 1 })
	if got := s.seen(); got[0] != "target other.example.ts.net" {
		t.Errorf("steps = %q", got)
	}
}

func TestABadAddressChangesNothing(t *testing.T) {
	s := serverSetup(t, settings.ServerFacts{Flatpak: true}, "", nil)
	s.chooseLodge("ftp://lodge.example.ts.net")
	s.click("Save")
	s.until("the error", func() bool { _, ok := s.tt.AccessByRole(ui.RoleAlert, ""); return ok })
	if len(s.seen()) != 0 || s.stored() != "" {
		t.Errorf("steps %q, stored %q", s.seen(), s.stored())
	}
	// After a failed save too, the pane shows the stored choice.
	s.until("the stored choice shown", func() bool { return s.control(ui.RoleRadio, "maild on this computer").Checked })
}

func TestFactsAreReadAfreshWhenThePaneOpens(t *testing.T) {
	var mu sync.Mutex
	facts := settings.ServerFacts{Flatpak: true}
	f := newFixture(t, setup{offline: true, options: func(o *settings.Options) {
		o.LoadFacts = func() settings.ServerFacts { mu.Lock(); defer mu.Unlock(); return facts }
		if err := conn.WriteAppConfig(o.ConfigFile, conn.AppConfig{Lodge: lodgeURL}); err != nil {
			t.Fatal(err)
		}
	}})
	f.show(settings.PaneServer)
	f.press(f.control(ui.RoleRadio, "maild on this computer"))
	f.sees("Mail on lodge keeps running there.")
	f.show(settings.PaneGeneral)
	mu.Lock()
	facts.LocalData = true
	mu.Unlock()
	f.show(settings.PaneServer)
	f.tt.Frame()
	f.press(f.control(ui.RoleRadio, "maild on this computer"))
	f.sees("This computer's maild starts again with the accounts it had.")
}

func TestSaveShowsWhatItDoesWhileItRuns(t *testing.T) {
	gate := make(chan struct{})
	s := serverSetup(t, settings.ServerFacts{Flatpak: true}, "", func(s *serverFixture) { s.gate = gate })
	s.chooseLodge(lodgeURL)
	s.click("Save")
	s.sees("Stopping this computer's maild…")
	if !s.disabled("Save") {
		t.Error("a second save can start")
	}
	close(gate)
	s.until("the end of the save", func() bool { return !s.tt.HasText("Stopping this computer's maild…") })
}

func TestStartingStatusWhileItRuns(t *testing.T) {
	gate := make(chan struct{})
	s := serverSetup(t, settings.ServerFacts{Flatpak: true}, lodgeURL, func(s *serverFixture) { s.gate = gate })
	s.press(s.control(ui.RoleRadio, "maild on this computer"))
	s.click("Save")
	s.sees("Starting this computer's maild…")
	close(gate)
	s.until("the end of the save", func() bool { return !s.tt.HasText("Starting this computer's maild…") })
}

func TestASaveOutlivesItsConnection(t *testing.T) {
	gate := make(chan struct{})
	s := serverSetup(t, settings.ServerFacts{Flatpak: true}, "", func(s *serverFixture) { s.gate = gate; s.stopErr = errors.New("refused") })
	s.srv = rpctest.Start(t)
	s.connect()
	s.chooseLodge(lodgeURL)
	s.click("Save")
	s.sees("Stopping this computer's maild…")
	s.win.Disconnect()
	s.connect()
	s.sees("Stopping this computer's maild…")
	close(gate)
	s.sees("This computer's maild did not stop. Frostmail still uses it.")
	s.win.Disconnect()
	s.connect()
	s.sees("This computer's maild did not stop. Frostmail still uses it.")
}

func TestWithoutAMaildOnlyTheServerPaneWorks(t *testing.T) {
	f := newFixture(t, setup{offline: true})
	f.sees("Connecting to maild…")
	f.notSeen("Email Address:")
	f.click("Server")
	f.sees("maild on this computer")
	f.click("General")
	f.sees("Connecting to maild…")
	f.record(func() { f.state.Err = errors.New("connection refused") })
	f.sees("Reconnecting to maild… (connection refused)")
}

func TestReconnectingKeepsThePane(t *testing.T) {
	f := newFixture(t, setup{})
	f.show(settings.PaneRules)
	f.sees("No Rules")
	f.win.Disconnect()
	f.sees("Reconnecting to maild…")
	f.connect()
	f.sees("No Rules")
	if a, ok := f.tt.AccessByRole(ui.RoleList, "Rules"); !ok || a.Name != "Rules" {
		t.Errorf("the Rules pane did not come back: %+v, %v", a, ok)
	}
}

func TestFactsFrom(t *testing.T) {
	home, data := t.TempDir(), t.TempDir()
	touch := func(path string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env := map[string]string{"HOME": home, "FROSTMAIL_CACHE_DIR": data, "FROSTMAIL_DATA_DIR": data}
	get := func(key string) string { return env[key] }
	if f := settings.FactsFrom(get); f != (settings.ServerFacts{}) {
		t.Errorf("no facts = %+v", f)
	}
	touch(filepath.Join(data, "frostmail.db"))
	if f := settings.FactsFrom(get); !f.LocalData || f.Flatpak {
		t.Errorf("data = %+v", f)
	}
	touch(filepath.Join(home, ".local", "share", "frostmail", "frostmail.db"))
	if f := settings.FactsFrom(get); f.HostMail {
		t.Errorf("outside the Flatpak the app does not ask: %+v", f)
	}
	env["FLATPAK_ID"] = "org.frostyard.Frostmail"
	if f := settings.FactsFrom(get); !f.Flatpak || !f.HostMail {
		t.Errorf("in the Flatpak = %+v", f)
	}
	env["FROSTMAIL_SOCKET"] = "/dev.sock"
	if f := settings.FactsFrom(get); f.HostMail {
		t.Errorf("the development bundle's maild is the host's: %+v", f)
	}
	delete(env, "FROSTMAIL_SOCKET")
	env["FLATPAK_ID"] = "org.other.App"
	if f := settings.FactsFrom(get); f.Flatpak || f.HostMail {
		t.Errorf("another app's Flatpak = %+v", f)
	}
}

// A field takes its input as it is drawn: text that arrives in the frame of
// Enter, as an input method's commit does, is part of what Enter saves.
func TestEnterSavesTheTextOfItsOwnFrame(t *testing.T) {
	s := serverSetup(t, settings.ServerFacts{}, "", nil)
	s.chooseLodge("https://lodge.example")
	s.tt.Compose(".ts.net", 7)
	s.tt.Key(0, ui.KeyEnter)
	s.until("the move", func() bool { return len(s.seen()) == 1 })
	if s.stored() != lodgeURL {
		t.Errorf("app.json says %q", s.stored())
	}
}
