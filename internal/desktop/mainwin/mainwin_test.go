package mainwin_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("FROSTUI_ICON_THEME", "Adwaita")
	frostui.SetIcons(frostui.NewIcons(icons.Embedded()))
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
	for _, want := range []string{"All Inboxes", "me@a.test", "Inbox", "5000 messages"} {
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
	f.until(t, "the connection", func() bool { return f.tt.HasText("Local") && f.tt.HasText("Message 1") })
	f.srv.Stop()
	f.until(t, "the lost connection", func() bool {
		_, ok := f.tt.Access("Connection: Local")
		return ok && f.tt.HasText("Reconnecting to maild…")
	})
}

// No maild yet: the connecting screen, with what the app tried.
func TestConnectingScreenBeforeTheFirstConnection(t *testing.T) {
	loop := frostuitest.NewLoop()
	win := mainwin.New(mainwin.Options{Target: conn.Target{Socket: filepath.Join(t.TempDir(), "none.sock")}, Client: "frostmail test",
		Post: loop.Post, Wake: time.Millisecond, Retry: 10 * time.Millisecond})
	tt := ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		win.View(c)
	}, 1200, 780)
	win.Start(t.Context())
	deadline := time.Now().Add(15 * time.Second)
	for !tt.HasText("Retry") {
		tt.Frame()
		if time.Now().After(deadline) {
			t.Fatalf("no connecting screen in %q", tt.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
	for _, want := range []string{"Connecting to maild…", "Details"} {
		if !tt.HasText(want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	if tt.HasText("Mailboxes") {
		t.Errorf("the panes show behind the connecting screen: %q", tt.Texts())
	}
}

func TestPaletteOpensWithBothKeysAndListsBuiltCommands(t *testing.T) {
	f := newFixture(t, 1)
	f.until(t, "the row", func() bool { return f.tt.HasText("Message 1") })
	f.tt.Key(ui.Cmd, ui.KeyK)
	f.tt.Frame()
	if !f.tt.HasText("Mark as Spam/Not Spam") {
		t.Errorf("Ctrl K's palette shows %q", f.tt.Texts())
	}
	f.tt.Key(0, ui.KeyEscape)
	f.tt.Frame()
	if f.tt.HasText("Mark as Spam/Not Spam") {
		t.Error("Escape left the palette open")
	}
	f.tt.Key(ui.Cmd|ui.Shift, ui.KeyP)
	f.tt.Frame()
	if !f.tt.HasText("Mark as Spam/Not Spam") {
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

// ADR-0049: losing maild's socket is a problem record, once.
func TestLosingTheSocketIsRecorded(t *testing.T) {
	srv := rpctest.Start(t)
	loop := frostuitest.NewLoop()
	var lost atomic.Int32
	win := mainwin.New(mainwin.Options{Target: conn.Target{Socket: srv.Socket}, Client: "frostmail test",
		Post: loop.Post, Wake: time.Millisecond, Retry: 10 * time.Millisecond, SocketLost: func(error) { lost.Add(1) }})
	tt := ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		win.View(c)
	}, 1200, 780)
	win.Start(t.Context())
	wait := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for !ok() {
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s: %q", what, tt.Texts())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	wait("the panes", func() bool { return tt.HasText("Mailboxes") })
	srv.Stop()
	wait("the record", func() bool { return lost.Load() == 1 })
	for range 50 { // the retries that follow are not new losses
		tt.Frame()
		time.Sleep(5 * time.Millisecond)
	}
	if n := lost.Load(); n != 1 {
		t.Errorf("recorded %d times", n)
	}
}

// The window controls take room from the top bar, which a Tester has none
// of unless told: the reader's actions must still show at the smallest
// window, with the room GTK reserves.
func TestReaderActionsShowBesideTheWindowControls(t *testing.T) {
	f := newFixture(t, 1)
	f.until(t, "the row", func() bool { return f.tt.HasText("Message 1") })
	for _, width := range []int{1200, 800} {
		f.tt.SetSize(width, 600)
		f.tt.SetTitleBar(ui.TitleBar{Height: 52, Right: 224})
		f.tt.Frame()
		f.tt.Frame()
		for _, name := range []string{"Reply (Ctrl+R)", "Reply All (Ctrl+Shift+R)", "Forward (Ctrl+Shift+F)", "Mark as Read or Unread", "Move"} {
			info, ok := f.tt.Access(name)
			if !ok || info.Bounds.W < 20 || info.Bounds.X+info.Bounds.W > float32(width) {
				t.Errorf("at %d wide, %q is at %+v, %v", width, name, info.Bounds, ok)
			}
		}
	}
}
