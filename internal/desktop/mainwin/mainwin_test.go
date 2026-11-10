package mainwin_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("FROSTUI_ICON_THEME", "Adwaita")
	frostui.SetIcons(frostui.NewIcons(nil))
	os.Exit(m.Run())
}

type fixture struct {
	srv  *rpctest.Server
	acct int64
	loop *frostuitest.Loop
	win  *mainwin.Window
	tt   *ui.Tester
}

// newFixture runs the window against a real maild holding n messages.
func newFixture(t *testing.T, n int) *fixture {
	t.Helper()
	srv := rpctest.Start(t)
	ctx := t.Context()
	f := &fixture{srv: srv, loop: frostuitest.NewLoop()}
	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "me@a.test",
			Auth: api.AuthKindPassword, IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"},
			SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"}})
		if err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		f.acct = a.ID
		hs := make([]store.MessageHeader, 0, n)
		for uid := 1; uid <= n; uid++ {
			d := base.Add(time.Duration(uid) * time.Minute)
			hs = append(hs, store.MessageHeader{UID: uint32(uid), Subject: fmt.Sprintf("Message %d", uid),
				MessageID: fmt.Sprintf("%d@x", uid), From: store.Address{Name: "Ann", Addr: "ann@x.test"}, Date: d, InternalDate: d})
		}
		_, err = tx.InsertHeaders(ctx, a.ID, mbs[0].ID, hs)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	f.win = mainwin.New(mainwin.Options{Target: conn.Target{Socket: srv.Socket}, Client: "frostmail test",
		Post: f.loop.Post, Wake: time.Millisecond, Retry: 10 * time.Millisecond})
	f.tt = ui.NewTester(func(c *ui.Context) {
		f.loop.Flush()
		f.win.View(c)
	}, 1200, 780)
	f.win.Start(t.Context())
	return f
}

// until redraws until the window shows what ok wants.
func (f *fixture) until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		f.tt.Frame()
		if ok() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s; the window shows %q", what, f.tt.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func snapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("FROSTMAIL_SNAPSHOT_DIR")
	if dir == "" {
		return
	}
	if err := writePNG(filepath.Join(dir, name+".png"), tt); err != nil {
		t.Error(err)
	}
}

func TestWindowShowsTheMailboxesAndAVirtualList(t *testing.T) {
	f := newFixture(t, 5000)
	f.until(t, "the newest row", func() bool { return f.tt.HasText("Message 5000") })
	for _, want := range []string{"All Inboxes", "me@a.test", "INBOX", "5000 messages"} {
		if !f.tt.HasText(want) {
			t.Errorf("no %q in %q", want, f.tt.Texts())
		}
	}
	snapshot(t, f.tt, "main")

	// Scrolling far: every row in view fills in.
	f.tt.Scroll(400, 400, 0, 84*2500)
	f.until(t, "rows in the middle", func() bool {
		return slices.ContainsFunc(f.tt.Texts(), func(s string) bool { return strings.HasPrefix(s, "Message 25") })
	})
}

func TestStatusBarLeadsWithTheConnectionAndFollowsIt(t *testing.T) {
	f := newFixture(t, 1)
	f.until(t, "the connection", func() bool { return f.tt.HasText("● Local") })
	f.srv.Stop()
	f.until(t, "the lost connection", func() bool { return f.tt.HasText("● Reconnecting to maild…") })
}

func TestPaletteOpensWithBothKeysAndListsBuiltCommands(t *testing.T) {
	f := newFixture(t, 1)
	f.until(t, "the row", func() bool { return f.tt.HasText("Message 1") })
	f.tt.Key(ui.Cmd, ui.KeyK)
	f.tt.Frame()
	if !f.tt.HasText("Get Mail") || !f.tt.HasText("Show/Hide Activity") {
		t.Errorf("Ctrl K's palette shows %q", f.tt.Texts())
	}
	f.tt.Key(0, ui.KeyEscape)
	f.tt.Frame()
	if f.tt.HasText("Show/Hide Activity") {
		t.Error("Escape left the palette open")
	}
	f.tt.Key(ui.Cmd|ui.Shift, ui.KeyP)
	f.tt.Frame()
	if !f.tt.HasText("Show/Hide Activity") {
		t.Errorf("Ctrl+Shift+P's palette shows %q", f.tt.Texts())
	}
}

func TestConsoleShowsMailActivity(t *testing.T) {
	f := newFixture(t, 1)
	f.until(t, "the row", func() bool { return f.tt.HasText("Message 1") })
	f.srv.Activity.Record(activity.Sent(f.acct, 1200*time.Millisecond))
	f.until(t, "the status bar's newest activity", func() bool {
		return slices.ContainsFunc(f.tt.Texts(), func(s string) bool { return strings.Contains(s, "sent a message") })
	})
	f.tt.Key(ui.Cmd, ui.KeyJ)
	f.until(t, "the console's line", func() bool {
		n := 0
		for _, s := range f.tt.Texts() {
			if strings.Contains(s, "sent a message") {
				n++
			}
		}
		return n >= 2 && f.tt.HasText("me@a.test")
	})
	snapshot(t, f.tt, "console")
}
