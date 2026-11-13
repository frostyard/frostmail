package settings_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/settings"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// general opens the General pane, once its preferences have loaded.
func (f *fixture) general() {
	f.t.Helper()
	f.show(settings.PaneGeneral)
	f.until("the preferences", func() bool {
		a, ok := f.tt.AccessByRole(ui.RolePopUpButton, "Undo send delay")
		return ok && !a.Disabled
	})
}

func (f *fixture) settings() *api.Settings {
	f.t.Helper()
	s, err := f.c.Settings().Get(f.t.Context(), &api.SettingsGetParams{})
	if err != nil {
		f.t.Fatal(err)
	}
	return s
}

func TestGeneralChoicesReachMaild(t *testing.T) {
	f := newFixture(t, setup{})
	f.general()
	f.choose("New message notifications", "VIPs")
	f.until("the scope", func() bool { return f.settings().NotifyScope == api.NotifyScopeVips })
	f.choose("Undo send delay", "30 Seconds")
	f.until("the delay", func() bool { return f.settings().UndoDelay == 30 })
	f.choose("Blocked mail", "Move it to the Trash")
	f.until("the action", func() bool { b := f.settings().BlockAction; return b != nil && *b == api.BlockActionTrash })
	f.choose("Undo send delay", "Off")
	f.until("off", func() bool { return f.settings().UndoDelay == 0 })
	f.until("the shown delay", func() bool { return f.choice("Undo send delay") == "Off" })
}

// P-211: the list's preview lines, None to 5, 2 until chosen.
func TestGeneralListPreview(t *testing.T) {
	f := newFixture(t, setup{})
	f.general()
	f.until("the default", func() bool { return f.choice("List preview") == "2 Lines" })
	f.choose("List preview", "No Preview")
	f.until("none", func() bool { n := f.settings().PreviewLines; return n != nil && *n == 0 })
	f.until("the shown choice", func() bool { return f.choice("List preview") == "No Preview" })
	f.choose("List preview", "5 Lines")
	f.until("five", func() bool { n := f.settings().PreviewLines; return n != nil && *n == 5 })
	one := int64(1)
	if _, err := f.c.Settings().Set(t.Context(), &api.SettingsSetParams{PreviewLines: &one}); err != nil {
		t.Fatal(err)
	}
	f.until("a change made elsewhere", func() bool { return f.choice("List preview") == "1 Line" })
}

func TestGeneralShowsAChangeMadeElsewhere(t *testing.T) {
	f := newFixture(t, setup{})
	f.general()
	delay := int64(20)
	if _, err := f.c.Settings().Set(t.Context(), &api.SettingsSetParams{UndoDelay: &delay}); err != nil {
		t.Fatal(err)
	}
	f.until("the delay from settings.changed", func() bool { return f.choice("Undo send delay") == "20 Seconds" })
}

func TestGeneralNotifiesForASmartMailbox(t *testing.T) {
	f := newFixture(t, setup{})
	sm, err := f.c.Smart().Create(t.Context(), &api.SmartCreateParams{Name: "Receipts", Conditions: api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{}}})
	if err != nil {
		t.Fatal(err)
	}
	f.general()
	f.until("the smart mailbox", func() bool {
		f.press(f.control(ui.RolePopUpButton, "New message notifications"))
		if f.tt.HasText("Smart Mailbox: Receipts") {
			return true
		}
		f.tt.Key(0, ui.KeyEscape)
		return false
	})
	f.click("Smart Mailbox: Receipts")
	f.until("the smart scope", func() bool {
		s := f.settings()
		return s.NotifyScope == api.NotifyScopeSmart && s.NotifySmartID != nil && *s.NotifySmartID == sm.ID
	})
	f.until("the shown scope", func() bool { return f.choice("New message notifications") == "Smart Mailbox: Receipts" })
}

func TestFlagNamesSaveOnEnterAndWhenLeft(t *testing.T) {
	f := newFixture(t, setup{})
	f.general()
	if a := f.control(ui.RoleTextField, "Flag 1 name"); a.Placeholder != "Red" {
		t.Errorf("flag 1's field = %+v", a)
	}
	f.fill("Flag 7 name", strings.Repeat("a", 50))
	if got := f.value("Flag 7 name"); len(got) != 40 {
		t.Errorf("flag 7's name is %d long", len(got))
	}
	f.fill("Flag 7 name", "")
	f.fill("Flag 1 name", " Urgent ")
	f.tt.Key(0, ui.KeyEnter)
	f.until("the name on Enter", func() bool { return f.settings().FlagNames[0] == "Urgent" })

	f.fill("Flag 2 name", "Later")
	f.click("Flag 3 name") // leaving the field saves it
	f.until("the name when left", func() bool {
		return slices.Equal(f.settings().FlagNames, []string{"Urgent", "Later", "", "", "", "", ""})
	})
}

func TestBlockedSendersListsAndUnblocks(t *testing.T) {
	f := newFixture(t, setup{})
	f.general()
	f.sees("None")
	if _, err := f.c.Block().Add(t.Context(), &api.BlockAddParams{Addresses: []string{"spam@y.test"}}); err != nil {
		t.Fatal(err)
	}
	f.sees("spam@y.test")
	f.notSeen("None")
	f.click("Unblock spam@y.test")
	f.sees("None")
	list, err := f.c.Block().List(t.Context(), &api.BlockListParams{})
	if err != nil || len(list) != 0 {
		t.Errorf("block.list = %v, %v", list, err)
	}
}

// failingSettings refuses settings.set while told to.
type failingSettings struct {
	api.SettingsService
	fail *atomic.Bool
}

func (s failingSettings) Set(ctx context.Context, p *api.SettingsSetParams) (*api.Settings, error) {
	if s.fail.Load() {
		return nil, errors.New("the disk is full")
	}
	return s.SettingsService.Set(ctx, p)
}

func TestGeneralErrorStaysUntilTheNextChangeSucceeds(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	f := newFixture(t, setup{server: rpctest.Options{Services: func(s *api.Services) { s.Settings = failingSettings{s.Settings, &fail} }}})
	f.general()
	f.choose("Undo send delay", "20 Seconds")
	f.until("the error", func() bool { return f.tt.HasText("the disk is full") })
	fail.Store(false)
	f.choose("Undo send delay", "30 Seconds")
	f.until("the error's end", func() bool { return !f.tt.HasText("the disk is full") })
}

func TestAnnouncementsReachTheNotifier(t *testing.T) {
	var mu sync.Mutex
	var got []api.Announcement
	f := newFixture(t, setup{options: func(o *settings.Options) {
		o.Announce = func(account int64, notes []api.Announcement) {
			mu.Lock()
			defer mu.Unlock()
			if account == 7 {
				got = append(got, notes...)
			}
		}
	}})
	f.events.Deliver(api.MessageAnnounced{AccountID: 7, Notes: []api.Announcement{{Title: "New mail", Body: "From Ann"}}})
	f.until("the note", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 1 && got[0].Title == "New mail"
	})
}

// A stored change coming back must not take the text being typed.
func TestFlagNamesKeepTheFieldBeingTyped(t *testing.T) {
	f := newFixture(t, setup{})
	f.general()
	f.fill("Flag 2 name", "Half typed")
	names := []string{"Elsewhere", "", "", "", "", "", ""}
	if _, err := f.c.Settings().Set(t.Context(), &api.SettingsSetParams{FlagNames: names}); err != nil {
		t.Fatal(err)
	}
	f.until("the other field to follow", func() bool { return f.value("Flag 1 name") == "Elsewhere" })
	if got := f.value("Flag 2 name"); got != "Half typed" {
		t.Errorf("the field being typed in holds %q", got)
	}
}

// sameNames shows every blocked sender under one name, as two addresses of
// one company can be.
type sameNames struct {
	api.BlockService
	removed *atomic.Int32
}

func (s sameNames) List(ctx context.Context, p *api.BlockListParams) ([]api.BlockedSender, error) {
	list, err := s.BlockService.List(ctx, p)
	for i := range list {
		list[i].Name = "Spam Co"
	}
	return list, err
}

func (s sameNames) Remove(ctx context.Context, p *api.BlockRemoveParams) error {
	s.removed.Add(1)
	return s.BlockService.Remove(ctx, p)
}

// frostui keys a row by its title: when a row leaves under a press, the next
// sender of that name must not take the click.
func TestUnblockGoesToTheAddressPressed(t *testing.T) {
	var removed atomic.Int32
	f := newFixture(t, setup{server: rpctest.Options{Services: func(s *api.Services) { s.Block = sameNames{s.Block, &removed} }}})
	f.general()
	if _, err := f.c.Block().Add(t.Context(), &api.BlockAddParams{Addresses: []string{"a@y.test", "b@y.test"}}); err != nil {
		t.Fatal(err)
	}
	f.sees("b@y.test")
	first := f.control(ui.RoleButton, "Unblock a@y.test")
	x, y := first.Bounds.X+first.Bounds.W/2, first.Bounds.Y+first.Bounds.H/2
	f.tt.Press(x, y)
	if err := f.c.Block().Remove(t.Context(), &api.BlockRemoveParams{Addresses: []string{"a@y.test"}}); err != nil {
		t.Fatal(err)
	}
	f.until("the row's end", func() bool { return !f.tt.HasText("a@y.test") })
	f.tt.Release(x, y)
	// A change made after the release: a click, had there been one, went first.
	f.choose("Undo send delay", "20 Seconds")
	f.until("the delay", func() bool { return f.settings().UndoDelay == 20 })
	if n := removed.Load(); n != 1 {
		t.Errorf("block.remove ran %d times, want the test's own", n)
	}
	f.sees("b@y.test")
}

func TestFlagNamesSaveWhenFocusMovesBackward(t *testing.T) {
	for _, keyboard := range []bool{false, true} {
		name := "click"
		if keyboard {
			name = "Shift+Tab"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t, setup{})
			f.general()
			f.fill("Flag 3 name", "Follow up")
			if keyboard {
				f.tt.Key(ui.Shift, ui.KeyTab)
			} else {
				f.click("Flag 2 name")
			}
			f.until("the name after moving backward", func() bool {
				return f.settings().FlagNames[2] == "Follow up"
			})
			f.fill("Flag 2 name", "Next")
			f.tt.Key(0, ui.KeyEnter)
			f.until("both names", func() bool {
				names := f.settings().FlagNames
				return names[1] == "Next" && names[2] == "Follow up"
			})
		})
	}
}
