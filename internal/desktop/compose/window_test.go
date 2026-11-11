package compose_test

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostui"
	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/compose"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/smtpx/smtpxtest"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("FROSTUI_ICON_THEME", "Adwaita")
	frostui.SetIcons(frostui.NewIcons(icons.Embedded()))
	os.Exit(m.Run())
}

type fixture struct {
	c *api.Client
	// current is what the window's Client answers, on the test's goroutine.
	current *api.Client
	srv     *rpctest.Server
	hidden  atomic.Bool
	acct    int64
	sink    *smtpxtest.Server
	loop    *frostuitest.Loop
	win     *compose.Window
	tt      *ui.Tester
	closed  atomic.Bool
	shown   atomic.Bool
	kept    atomic.Int32
	sent    atomic.Pointer[api.OutboxItem]
	picked  []string
	// loadPart is the window's LoadPart; nil reads no image.
	loadPart func(*api.Client, string) ([]byte, error)
}

func start(t *testing.T) *fixture {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	raw := "From: Ann <ann@x.test>\r\nTo: me@x.test\r\nSubject: Plans\r\nMessage-ID: <a@x.test>\r\nDate: Mon, 09 Nov 2026 09:41:00 +0000\r\n\r\nFirst line\r\nSecond line\r\n"
	if _, err := mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{Flags: []imap.Flag{imap.FlagSeen}}); err != nil {
		t.Fatal(err)
	}
	sink := smtpxtest.Start(t, imapxtest.Password)
	cfg := mailsync.Config{PollInterval: time.Hour, IdleMax: time.Minute, MinBackoff: 50 * time.Millisecond, Chunk: 50, NoPrefetch: true,
		DraftQuiet: 20 * time.Millisecond, SendRetry: 50 * time.Millisecond}
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg})
	c := srv.Dial(t)
	ctx := t.Context()
	o := mem.DialOptions()
	a, err := c.Account().Create(ctx, &api.AccountCreateParams{Kind: api.AccountKindIMAP, Email: "me@x.test", DisplayName: "Me", Auth: api.AuthKindPassword,
		IMAP: &api.ServerConfig{Host: o.Host, Port: int64(o.Port), TLS: o.TLS, Username: o.Username}, SMTP: sink.Config(o.Username)})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: imapxtest.Password}); err != nil {
		t.Fatal(err)
	}
	f := &fixture{c: c, current: c, srv: srv, acct: a.ID, sink: sink, loop: frostuitest.NewLoop()}
	deadline := time.Now().Add(15 * time.Second)
	for {
		s, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{1}})
		if err == nil && len(s) == 1 {
			return f
		}
		if time.Now().After(deadline) {
			t.Fatal("the message never synced")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *fixture) open(t *testing.T, d *api.Draft, fresh bool, bytesMode bool) {
	t.Helper()
	f.win = compose.New(compose.Options{Client: func() *api.Client { return f.current }, Draft: d.ID, Fresh: fresh, Post: f.loop.Post,
		Close:        func() { f.closed.Store(true) },
		Hide:         func() { f.hidden.Store(true) },
		Show:         func() { f.shown.Store(true) },
		OpenFiles:    func() ([]string, error) { return f.picked, nil },
		FilesAsBytes: func() bool { return bytesMode },
		LoadPart:     f.loadPart,
		Sent:         func(i *api.OutboxItem) { f.sent.Store(i) },
		Kept:         func() { f.kept.Add(1) },
	})
	f.tt = ui.NewTester(func(c *ui.Context) {
		f.loop.Flush()
		f.win.View(c)
	}, 720, 560)
	f.win.Start(t.Context())
	f.until(t, "the draft", func() bool { _, ok := f.tt.Access("Message body"); return ok })
}

// hold stands between a client and maild, keeping back the calls of one
// method until free.
type hold struct {
	arrived atomic.Bool
	release chan struct{}
	once    sync.Once
}

func (h *hold) reached() bool { return h.arrived.Load() }
func (h *hold) free()         { h.once.Do(func() { close(h.release) }) }

// holdCalls makes the window's client one whose calls of method wait.
func (f *fixture) holdCalls(t *testing.T, method string) *hold {
	t.Helper()
	h := &hold{release: make(chan struct{})}
	t.Cleanup(h.free)
	client, front := net.Pipe()
	back, served := net.Pipe()
	go f.srv.RPC.ServeConn(t.Context(), served, f.srv.Router, api.Caller{Kind: api.CallerKindLocal})
	go func() { _, _ = io.Copy(front, back) }()
	go func() {
		lines := bufio.NewScanner(front)
		lines.Buffer(make([]byte, 64<<10), 16<<20)
		for lines.Scan() {
			if bytes.Contains(lines.Bytes(), []byte(`"`+method+`"`)) {
				h.arrived.Store(true)
				<-h.release
			}
			if _, err := back.Write(append(lines.Bytes(), '\n')); err != nil {
				return
			}
		}
	}()
	c := api.NewClient(client)
	t.Cleanup(func() { _ = c.Close() })
	if _, err := c.RPC().Hello(t.Context(), &api.RPCHelloParams{Protocol: api.Protocol, Client: "held"}); err != nil {
		t.Fatal(err)
	}
	f.current = c
	return h
}

func (f *fixture) subject(t *testing.T, d *api.Draft) string {
	t.Helper()
	got, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
	if err != nil {
		return ""
	}
	return got.Content.Subject
}

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

func (f *fixture) draft(t *testing.T, p *api.DraftCreateParams) *api.Draft {
	t.Helper()
	d, err := f.c.Draft().Create(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f *fixture) focus(t *testing.T, name string) {
	t.Helper()
	info, ok := f.tt.Access(name)
	if !ok {
		t.Fatalf("no %q in %q", name, f.tt.Texts())
	}
	f.tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
	f.tt.Frame()
}

func (f *fixture) addRecipient(t *testing.T, address string) {
	t.Helper()
	f.focus(t, "To")
	f.tt.Type(address)
	f.tt.Key(0, ui.KeyEnter)
	f.until(t, "the token", func() bool { return f.tt.HasText(address) })
}

func (f *fixture) sendAndGet(t *testing.T) *mail.Message {
	t.Helper()
	f.tt.Key(ui.Cmd, ui.KeyEnter)
	f.until(t, "the window closing", f.closed.Load)
	got := f.sink.WaitMessages(t, 1, 15*time.Second)
	m, err := mail.ReadMessage(bytes.NewReader(got[0].Data))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func contentType(t *testing.T, h mail.Header) (string, map[string]string) {
	t.Helper()
	mt, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	return mt, params
}

// P-501: a new message with a file goes out as multipart/mixed.
func TestNewMessageWithAttachment(t *testing.T) {
	f := start(t)
	path := filepath.Join(t.TempDir(), "notes.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4 test"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.picked = []string{path}
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew}), true, false)
	f.addRecipient(t, "bo@x.test")
	// From the body: MyGo's one-line inputs keep Ctrl+Shift+A as select all.
	f.focus(t, "Message body")
	f.tt.Key(ui.Cmd|ui.Shift, ui.KeyA)
	f.until(t, "the chip", func() bool { return f.tt.HasText("notes.pdf") && f.tt.HasText("1 file, 13 B") })
	m := f.sendAndGet(t)
	mt, params := contentType(t, m.Header)
	if mt != "multipart/mixed" {
		t.Fatalf("Content-Type %s", mt)
	}
	r := multipart.NewReader(m.Body, params["boundary"])
	var names []string
	for {
		p, err := r.NextPart()
		if err != nil {
			break
		}
		names = append(names, p.FileName())
	}
	if !slices.Contains(names, "notes.pdf") {
		t.Errorf("parts %q", names)
	}
	if f.sent.Load() == nil {
		t.Error("the main window was not told")
	}
}

// Attaching by bytes, as behind lodge and in the Flatpak.
func TestAttachByBytes(t *testing.T) {
	f := start(t)
	path := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 4<<20), 0o600); err != nil {
		t.Fatal(err)
	}
	f.picked = []string{path}
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew}), true, true)
	f.tt.Key(ui.Cmd|ui.Shift, ui.KeyA)
	f.until(t, "the chip in two chunks", func() bool { return f.tt.HasText("photo.png") && f.tt.HasText("1 file, 4.2 MB") })
}

// A reply answers its sender and quotes the message.
func TestReplyGoesToTheSenderWithItsQuote(t *testing.T) {
	f := start(t)
	source := int64(1)
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindReply, SourceID: &source}), true, false)
	m := f.sendAndGet(t)
	if to := m.Header.Get("To"); !strings.Contains(to, "ann@x.test") {
		t.Errorf("To %q", to)
	}
	if ref := m.Header.Get("In-Reply-To"); ref != "<a@x.test>" {
		t.Errorf("In-Reply-To %q", ref)
	}
	if mt, _ := contentType(t, m.Header); mt != "multipart/alternative" {
		t.Errorf("Content-Type %s", mt)
	}
}

// P-508: only the selection is quoted, and it survives the editor.
func TestReplyWithSelectionKeepsOnlyTheSelection(t *testing.T) {
	f := start(t)
	source, quote := int64(1), "Second line"
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindReply, SourceID: &source, Quote: &quote})
	f.open(t, d, true, false)
	f.focus(t, "Subject")
	f.tt.Type("x")
	f.until(t, "the save", func() bool {
		got, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
		return err == nil && strings.HasSuffix(got.Content.Subject, "x")
	})
	got, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Content.HTML, "Second line") || strings.Contains(got.Content.HTML, "First line") {
		t.Errorf("html %q", got.Content.HTML)
	}
	if !strings.Contains(got.Content.HTML, `<blockquote type="cite">`) {
		t.Errorf("the quote was lost: %q", got.Content.HTML)
	}
}

// A forward carries the source as quoted text.
func TestForward(t *testing.T) {
	f := start(t)
	source := int64(1)
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindForward, SourceID: &source}), true, false)
	f.addRecipient(t, "cy@x.test")
	m := f.sendAndGet(t)
	if s := m.Header.Get("Subject"); !strings.HasPrefix(s, "Fwd:") {
		t.Errorf("Subject %q", s)
	}
}

func TestSendNeedsAValidRecipient(t *testing.T) {
	f := start(t)
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew}), true, false)
	f.tt.Key(ui.Cmd, ui.KeyEnter)
	f.tt.Frame()
	if f.closed.Load() {
		t.Fatal("sent with no recipient")
	}
	f.focus(t, "To")
	f.tt.Type("not-an-address")
	f.tt.Key(0, ui.KeyEnter)
	f.until(t, "the token", func() bool { return f.tt.HasText("not-an-address") })
	f.tt.Key(ui.Cmd, ui.KeyEnter)
	f.tt.Frame()
	if f.closed.Load() {
		t.Fatal("sent with an invalid recipient")
	}
}

// Plan 0014 Phase 2: the reminder holds the send back.
func TestAttachmentReminderHoldsTheSend(t *testing.T) {
	f := start(t)
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew}), true, false)
	f.addRecipient(t, "bo@x.test")
	f.focus(t, "Message body")
	f.tt.Type("I attached it")
	f.tt.Frame()
	f.tt.Key(ui.Cmd, ui.KeyEnter)
	f.until(t, "the question", func() bool { return f.tt.HasText("Send without an attachment?") })
	if f.closed.Load() {
		t.Fatal("sent past the reminder")
	}
	if err := f.tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the sheet closed", func() bool { return !f.tt.HasText("Send without an attachment?") })
	if f.closed.Load() {
		t.Fatal("Cancel sent the message")
	}
	f.tt.Key(ui.Cmd, ui.KeyEnter)
	f.until(t, "the question again", func() bool { return f.tt.HasText("Send without an attachment?") })
	if err := f.tt.Click("Send Anyway"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the window closing", f.closed.Load)
}

func TestClosingAFreshUntouchedDraftDeletesIt(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, true, false)
	f.win.RequestClose()
	f.until(t, "the draft gone", func() bool {
		_, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
		return err != nil
	})
}

func TestClosingAnEditedDraftSavesIt(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, true, false)
	f.focus(t, "Subject")
	f.tt.Type("Kept")
	f.tt.Frame()
	f.win.RequestClose()
	f.until(t, "the saved subject", func() bool {
		got, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
		return err == nil && got.Content.Subject == "Kept"
	})
}

// M28: a save on closing that fails keeps the window, shown again and
// saying so, as closing it would lose the edits: draft.update fails here as
// the draft no longer exists. Close Anyway lets them go.
func TestAFailedSaveOnClosingKeepsTheWindow(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, true, false)
	f.focus(t, "Subject")
	f.tt.Type("Lost")
	f.tt.Frame()
	if err := f.c.Draft().Delete(t.Context(), &api.DraftDeleteParams{ID: d.ID}); err != nil {
		t.Fatal(err)
	}
	f.win.RequestClose()
	f.until(t, "the question", func() bool { return f.tt.HasText("Close without saving?") })
	if f.closed.Load() || f.kept.Load() != 1 || !f.shown.Load() {
		t.Fatalf("closed %v, kept %d, shown %v", f.closed.Load(), f.kept.Load(), f.shown.Load())
	}
	if !slices.ContainsFunc(f.tt.Texts(), func(s string) bool { return strings.HasPrefix(s, "The draft was not saved: ") }) {
		t.Errorf("the window shows %q", f.tt.Texts())
	}
	if err := f.tt.Click("Close Anyway"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the window closing", f.closed.Load)
	if !f.win.RequestClose() {
		t.Error("the window may not go after Close Anyway")
	}
}

// Nothing is said of a save on closing that worked.
func TestClosingSaysNothingOfASave(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, true, false)
	f.focus(t, "Subject")
	f.tt.Type("Kept")
	f.tt.Frame()
	f.win.RequestClose()
	f.until(t, "the window closing", f.closed.Load)
	if got := f.subject(t, d); got != "Kept" {
		t.Errorf("the draft's subject is %q", got)
	}
	if f.kept.Load() != 0 || f.shown.Load() {
		t.Errorf("kept %d, shown %v", f.kept.Load(), f.shown.Load())
	}
}

func TestDeleteAsksWhenTheDraftHasContent(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, true, false)
	f.focus(t, "Subject")
	f.tt.Type("Something")
	f.tt.Frame()
	// From the body: in a one-line input Ctrl+Backspace deletes a word.
	f.focus(t, "Message body")
	f.tt.Key(ui.Cmd, ui.KeyBackspace)
	f.until(t, "the question", func() bool { return f.tt.HasText("Delete Draft?") })
}

func TestBccToggleAndFormatBar(t *testing.T) {
	f := start(t)
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew}), true, false)
	if f.tt.HasText("Bcc:") {
		t.Error("Bcc shows at first")
	}
	f.tt.Key(ui.Cmd|ui.Shift, ui.KeyB)
	f.until(t, "Bcc", func() bool { return f.tt.HasText("Bcc:") })
	if err := f.tt.Click("Show Format Bar"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the format bar", func() bool { _, ok := f.tt.Access("Strikethrough"); return ok })
}

func TestParseAddresses(t *testing.T) {
	good, bad := compose.ParseAddresses(`"Smith, Bob" <bob@x.test>; ann@x.test, ANN@x.test, nope`)
	if len(good) != 2 || good[0].Name != "Smith, Bob" || good[1].Address != "ann@x.test" {
		t.Errorf("good %+v", good)
	}
	if len(bad) != 1 || bad[0] != "nope" {
		t.Errorf("bad %q", bad)
	}
}

func TestSendChoices(t *testing.T) {
	morning := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	if got := compose.SendChoices(morning); len(got) != 2 || got[0].Label != "Send 9:00 PM Tonight" || got[1].Label != "Send 8:00 AM Tomorrow" {
		t.Errorf("morning: %+v", got)
	}
	late := time.Date(2026, 11, 9, 22, 0, 0, 0, time.UTC)
	if got := compose.SendChoices(late); len(got) != 1 {
		t.Errorf("late: %+v", got)
	}
}

// The window controls take room from the top bar, which a Tester has none
// of unless told: the actions must still show in a window of the smallest
// size, 480 wide, with the room GTK reserves.
func TestActionsShowBesideTheWindowControls(t *testing.T) {
	f := start(t)
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew}), true, false)
	for _, width := range []int{720, 480} {
		f.tt.SetSize(width, 560)
		f.tt.SetTitleBar(ui.TitleBar{Height: 52, Right: 224})
		f.tt.Frame()
		f.tt.Frame()
		for _, name := range []string{"Send (Ctrl+Enter)", "Send Later", "Insert Snippet", "Attach Files… (Ctrl+Shift+A)", "Show Format Bar", "Delete Draft (Ctrl+Backspace)"} {
			info, ok := f.tt.Access(name)
			if !ok || info.Bounds.W < 20 || info.Bounds.X+info.Bounds.W > float32(width) {
				t.Errorf("at %d wide, %q is at %+v, %v", width, name, info.Bounds, ok)
			}
		}
	}
}

// P-506: a Send Later choice queues the message for its time and sends
// nothing now.
func TestSendLaterQueuesForItsTime(t *testing.T) {
	f := start(t)
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew}), true, false)
	f.addRecipient(t, "bo@x.test")
	later, ok := f.tt.Access("Send Later")
	if !ok {
		t.Fatalf("no Send Later in %q", f.tt.Texts())
	}
	f.tt.ClickAt(later.Bounds.X+later.Bounds.W/2, later.Bounds.Y+later.Bounds.H/2)
	f.tt.Frame()
	if err := f.tt.ChooseMenuItem("Send 8:00 AM Tomorrow"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.until(t, "the window closing", f.closed.Load)
	item := f.sent.Load()
	if item == nil || !item.Scheduled {
		t.Fatalf("outbox item %+v", item)
	}
	if got := f.sink.Messages(); len(got) != 0 {
		t.Errorf("%d messages sent now", len(got))
	}
}

// P-503: a snippet goes in at the cursor.
func TestInsertSnippet(t *testing.T) {
	f := start(t)
	if _, err := f.c.Snippet().Create(t.Context(), &api.SnippetCreateParams{Name: "Thanks", HTML: "<p>Many thanks</p>"}); err != nil {
		t.Fatal(err)
	}
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, true, false)
	menu, ok := f.tt.Access("Insert Snippet")
	if !ok {
		t.Fatalf("no Insert Snippet in %q", f.tt.Texts())
	}
	f.tt.ClickAt(menu.Bounds.X+menu.Bounds.W/2, menu.Bounds.Y+menu.Bounds.H/2)
	f.tt.Frame()
	if err := f.tt.ChooseMenuItem("Thanks"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.until(t, "the saved snippet", func() bool {
		got, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
		return err == nil && strings.Contains(got.Content.HTML, "Many thanks")
	})
}

// P-504: an address seen in mail is offered as the user types.
func TestRecipientsOfferKnownAddresses(t *testing.T) {
	f := start(t)
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew}), true, false)
	f.focus(t, "To")
	f.tt.Type("ann")
	f.until(t, "the suggestion", func() bool {
		return slices.ContainsFunc(f.tt.Texts(), func(s string) bool { return strings.Contains(s, "ann@x.test") })
	})
}

// M33: a save that takes its time never stops the window, and what is
// typed during it is saved after it.
func TestTypingGoesOnDuringASlowSave(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	h := f.holdCalls(t, "draft.update")
	f.open(t, d, true, false)
	f.focus(t, "Subject")
	f.tt.Type("A")
	f.until(t, "the save reaching maild", h.reached)
	done := make(chan struct{})
	go func() {
		defer close(done)
		f.tt.Type("B")
		f.tt.Frame()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		h.free()
		t.Fatal("the window waited for the save")
	}
	h.free()
	f.until(t, "the later edit saved", func() bool { return f.subject(t, d) == "AB" })
}

// M33: a save that ran while the text changed leaves it to save: closing
// writes what the save in flight did not have.
func TestAnEditDuringASaveStaysToSave(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	h := f.holdCalls(t, "draft.update")
	f.open(t, d, true, false)
	f.focus(t, "Subject")
	f.tt.Type("A")
	f.until(t, "the save reaching maild", h.reached)
	f.tt.Type("B")
	f.tt.Frame()
	f.win.RequestClose()
	h.free()
	f.until(t, "the window closing", f.closed.Load)
	if got := f.subject(t, d); got != "AB" {
		t.Errorf("the draft's subject is %q", got)
	}
}

// M6: the window stays, hidden, until its save ends, so the app does not
// quit under it.
func TestClosingWaitsForTheSave(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, true, false)
	h := f.holdCalls(t, "draft.update")
	f.focus(t, "Subject")
	f.tt.Type("Kept")
	f.tt.Frame()
	if f.win.RequestClose() {
		t.Fatal("the window may go before its save")
	}
	if !f.hidden.Load() {
		t.Error("the window still shows while it saves")
	}
	f.until(t, "the save reaching maild", h.reached)
	if f.win.RequestClose() || f.closed.Load() {
		t.Fatal("the window went during its save")
	}
	h.free()
	f.until(t, "the window closing", f.closed.Load)
	if !f.win.RequestClose() {
		t.Error("the window may not go after its save")
	}
	if got := f.subject(t, d); got != "Kept" {
		t.Errorf("the draft's subject is %q", got)
	}
}

// M6: so does the deletion of a draft never touched.
func TestClosingWaitsForTheDeletion(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, true, false)
	h := f.holdCalls(t, "draft.delete")
	if f.win.RequestClose() {
		t.Fatal("the window may go before the deletion")
	}
	f.until(t, "the deletion reaching maild", h.reached)
	if f.closed.Load() {
		t.Fatal("the window went during the deletion")
	}
	h.free()
	f.until(t, "the window closing", f.closed.Load)
}

// M7: a window saves through the client of the moment, not the one it
// opened with, which a reconnect closed.
func TestSavesGoThroughTheCurrentClient(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	first := f.srv.Dial(t)
	f.current = first
	f.open(t, d, true, false)
	_ = first.Close()
	f.current = f.c
	f.focus(t, "Subject")
	f.tt.Type("After")
	f.until(t, "the save", func() bool { return f.subject(t, d) == "After" })
}

// M7: an edit made while disconnected is saved once the connection is back.
func TestASaveWaitsForTheConnection(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, true, false)
	f.current = nil
	f.focus(t, "Subject")
	f.tt.Type("Later")
	f.tt.Frame()
	if got := f.subject(t, d); got != "" {
		t.Fatalf("saved %q with no connection", got)
	}
	f.current = f.c
	f.until(t, "the save", func() bool { return f.subject(t, d) == "Later" })
}

// M7: past a change of maild the window has no client: closing writes
// nothing anywhere and keeps the window, saying the draft was not saved.
// Closing again, with a client back, saves.
func TestClosingWithNoClientKeepsTheWindow(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, false, false)
	f.current = nil
	f.focus(t, "Subject")
	f.tt.Type("Old")
	f.tt.Frame()
	f.win.RequestClose()
	f.until(t, "the question", func() bool { return f.tt.HasText("Close without saving?") })
	if f.closed.Load() || f.kept.Load() != 1 {
		t.Fatalf("closed %v, kept %d", f.closed.Load(), f.kept.Load())
	}
	if got := f.subject(t, d); got != "" {
		t.Errorf("wrote %q", got)
	}
	if err := f.tt.Click("Keep Open"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	f.current = f.c
	f.win.RequestClose()
	f.until(t, "the window closing", f.closed.Load)
	if got := f.subject(t, d); got != "Old" {
		t.Errorf("the draft's subject is %q", got)
	}
}

func (f *fixture) attached(t *testing.T, name string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("%PDF-1.4 test"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.picked = []string{path}
	f.focus(t, "Message body")
	f.tt.Key(ui.Cmd|ui.Shift, ui.KeyA)
	f.until(t, "the chip", func() bool { _, ok := f.tt.Access("Remove " + name); return ok })
}

// M30: nothing goes while a file is still being removed.
func TestSendWaitsForARemoval(t *testing.T) {
	f := start(t)
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew}), true, false)
	h := f.holdCalls(t, "draft.detach")
	f.addRecipient(t, "bo@x.test")
	f.attached(t, "notes.pdf")
	f.focus(t, "Remove notes.pdf")
	f.until(t, "the removal reaching maild", h.reached)
	f.tt.Frame()
	if info, ok := f.tt.Access("Send (Ctrl+Enter)"); !ok || !info.Disabled {
		t.Errorf("Send during a removal: %+v, %v", info, ok)
	}
	f.focus(t, "Message body")
	f.tt.Key(ui.Cmd, ui.KeyEnter)
	f.tt.Frame()
	h.free()
	f.until(t, "the chip gone", func() bool { return !f.tt.HasText("notes.pdf") })
	if f.closed.Load() {
		t.Fatal("sent during a removal")
	}
	m := f.sendAndGet(t)
	body, err := io.ReadAll(m.Body)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("notes.pdf")) {
		t.Errorf("the removed file went: %s", body)
	}
}

// M31: while a send runs the window takes no edits, which would be lost.
func TestSendingFreezesTheWindow(t *testing.T) {
	f := start(t)
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew}), true, false)
	h := f.holdCalls(t, "draft.send")
	f.addRecipient(t, "bo@x.test")
	f.focus(t, "Subject")
	f.tt.Type("Sent")
	f.tt.Frame()
	f.tt.Key(ui.Cmd, ui.KeyEnter)
	f.until(t, "the send reaching maild", h.reached)
	f.tt.Frame()
	for _, name := range []string{"Subject", "To", "Message body"} {
		if info, ok := f.tt.Access(name); !ok || !info.Disabled {
			t.Errorf("%s during the send: %+v, %v", name, info, ok)
		}
	}
	h.free()
	f.until(t, "the window closing", f.closed.Load)
}

// M32: a half-typed recipient survives the save, and still holds the send
// back when the draft reopens.
func TestInvalidRecipientsAreSaved(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, false, false)
	f.addRecipient(t, "bo@x.test")
	f.addRecipient(t, "half-typed")
	f.until(t, "the save", func() bool {
		got, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
		return err == nil && len(got.Content.To) == 2 && got.Content.To[1].Address == "half-typed"
	})
	again, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.open(t, again, false, false)
	f.until(t, "the token", func() bool { return f.tt.HasText("half-typed") })
	if info, ok := f.tt.Access("Send (Ctrl+Enter)"); !ok || !info.Disabled {
		t.Errorf("Send with an invalid recipient: %+v, %v", info, ok)
	}
}

func pictureBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func pictureURL(t *testing.T) string {
	t.Helper()
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(pictureBytes(t))
}

// bigPictureURL is a 1×1 PNG made size bytes larger by a text chunk of
// fill: big to send, cheap to decode.
func bigPictureURL(t *testing.T, size int, fill byte) string {
	t.Helper()
	raw := pictureBytes(t)
	end := len(raw) - 12 // IEND
	data := append([]byte("Comment\x00"), bytes.Repeat([]byte{fill}, size)...)
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	chunk = append(append(chunk, "tEXt"...), data...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(chunk[4:]))
	out := append(append(append([]byte(nil), raw[:end]...), chunk...), raw[end:]...)
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(out)
}

// M28: a pasted picture, which the editor holds only as a bitmap, is
// saved, and comes back when the draft reopens.
func TestPastedPictureIsSaved(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, false, false)
	url := pictureURL(t)
	f.focus(t, "Message body")
	f.tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte(`<p><img src="`+url+`" alt="pic"></p>`)))))
	f.tt.Key(ui.Cmd, ui.KeyV)
	var saved string
	f.until(t, "the saved picture", func() bool {
		got, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
		if err != nil {
			return false
		}
		saved = got.Content.HTML
		return strings.Contains(saved, url)
	})
	if strings.Contains(saved, `src=""`) {
		t.Errorf("html %q", saved)
	}
	if out := compose.ToHTML(compose.ToText(saved, nil)); !strings.Contains(out, url) {
		t.Errorf("reopened: %q", out)
	}
}

// M34: an image alone is content, so Delete asks.
func TestDeleteAsksForADraftWithOnlyAnImage(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	content := d.Content
	content.HTML = `<p><img src="` + pictureURL(t) + `"></p>`
	if _, err := f.c.Draft().Update(t.Context(), &api.DraftUpdateParams{ID: d.ID, Content: content}); err != nil {
		t.Fatal(err)
	}
	f.open(t, d, false, false)
	f.focus(t, "Message body")
	f.tt.Key(ui.Cmd, ui.KeyBackspace)
	f.until(t, "the question", func() bool { return f.tt.HasText("Delete Draft?") })
}

// M26: what a sender's name or address says never makes another recipient.
func TestHostileAddressesStayOneRecipient(t *testing.T) {
	const real = "real@x.test"
	for _, name := range []string{
		"extra@evil.test\nVictim", "extra@evil.test\r\nVictim", "a, extra@evil.test", "a; extra@evil.test",
		`a" <extra@evil.test>, "b`, "<extra@evil.test>", "a <extra@evil.test> b", "\"", "extra@evil.test\u2028Victim", "tab\there", "\x00\x1f",
	} {
		a := api.Address{Name: name, Address: real}
		tok := compose.Token(a)
		good, bad := compose.ParseAddresses(tok)
		if len(good) != 1 || len(bad) != 0 || good[0].Address != real {
			t.Errorf("name %q: token %q reads as %+v, %q", name, tok, good, bad)
		}
		both, bad := compose.ParseAddresses(strings.Join([]string{tok, "bo@x.test"}, ","))
		if len(both) != 2 || len(bad) != 0 {
			t.Errorf("name %q beside another: %+v, %q", name, both, bad)
		}
	}
	// net/mail unquotes a local part, so an address can hold what splits.
	for _, address := range []string{"extra@evil.test,b@x.test", "extra@evil.test\nb@x.test", "a;b@x.test", `"a,b"@x.test`, "a b@x.test", "<extra@evil.test>"} {
		for _, name := range []string{"", "Name"} {
			tok := compose.Token(api.Address{Name: name, Address: address})
			if good, bad := compose.ParseAddresses(tok); len(good) != 0 || len(bad) != 1 {
				t.Errorf("address %q, name %q: token %q reads as %+v, %q", address, name, tok, good, bad)
			}
		}
	}
}

// Ordinary names read back as they were.
func TestTokensKeepOrdinaryNames(t *testing.T) {
	for _, name := range []string{"Ann Example", "Smith, Bob", "J. Smith", "Ann (work)"} {
		tok := compose.Token(api.Address{Name: name, Address: "a@x.test"})
		good, bad := compose.ParseAddresses(tok)
		if len(good) != 1 || len(bad) != 0 || good[0].Name != name {
			t.Errorf("name %q: token %q reads as %+v, %q", name, tok, good, bad)
		}
	}
}

// M28: pictures pasted one by one, each well under a message to maild, are
// kept only while the draft's HTML stays within one: the draft still
// saves, and reopens with the pictures it kept.
func TestPastedPicturesStayWithinOneMessage(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, false, false)
	f.focus(t, "Message body")
	var urls []string
	for _, fill := range []byte("abc") {
		url := bigPictureURL(t, 9<<19, fill) // 6 MiB as a data: URL
		urls = append(urls, url)
		f.tt.SetClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.HTML, []byte(`<p><img src="`+url+`"></p>`)))))
		f.tt.Key(ui.Cmd, ui.KeyV)
		f.tt.Frame()
	}
	f.until(t, "the note", func() bool {
		return f.tt.HasText("A pasted picture cannot be kept in the message. Attach it as a file.")
	})
	f.win.RequestClose()
	f.until(t, "the end of the save", func() bool { return f.closed.Load() || f.kept.Load() > 0 })
	if f.kept.Load() > 0 {
		t.Fatalf("the save on closing failed: %q", f.tt.Texts())
	}
	got, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	html := got.Content.HTML
	if !strings.Contains(html, urls[0]) || strings.Contains(html, urls[1]) || strings.Contains(html, urls[2]) {
		t.Errorf("the draft has %d bytes, the first picture %v", len(html), strings.Contains(html, urls[0]))
	}
	pictures := 0
	for _, p := range compose.ToText(html, nil).Paragraphs() {
		for _, r := range p.Runs {
			if a := r.Attributes.Attachment; a != nil && a.Image != nil {
				pictures++
			}
		}
	}
	if pictures != 1 {
		t.Errorf("reopened with %d pictures", pictures)
	}
}

// holdLoads makes the window's image reads wait until free, as a read from
// lodge that takes its time.
func (f *fixture) holdLoads(t *testing.T) *hold {
	t.Helper()
	h := &hold{release: make(chan struct{})}
	t.Cleanup(h.free)
	picture := pictureBytes(t)
	f.loadPart = func(*api.Client, string) ([]byte, error) {
		h.arrived.Store(true)
		<-h.release
		return picture, nil
	}
	return h
}

// snippetWithImage is a snippet whose image the window reads through
// LoadPart, as it does a part of a message.
func (f *fixture) snippetWithImage(t *testing.T) {
	t.Helper()
	if _, err := f.c.Snippet().Create(t.Context(), &api.SnippetCreateParams{Name: "Thanks",
		HTML: `<p>Many thanks</p><p><img src="mailpart://localhost/m/1/2.png"></p>`}); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) chooseSnippet(t *testing.T, name string) {
	t.Helper()
	menu, ok := f.tt.Access("Insert Snippet")
	if !ok {
		t.Fatalf("no Insert Snippet in %q", f.tt.Texts())
	}
	f.tt.ClickAt(menu.Bounds.X+menu.Bounds.W/2, menu.Bounds.Y+menu.Bounds.H/2)
	f.tt.Frame()
	if err := f.tt.ChooseMenuItem(name); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
}

// M31: a snippet still being read holds Send back, so that the message goes
// with it.
func TestSendWaitsForASnippet(t *testing.T) {
	f := start(t)
	f.snippetWithImage(t)
	h := f.holdLoads(t)
	f.open(t, f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew}), true, false)
	f.addRecipient(t, "bo@x.test")
	f.chooseSnippet(t, "Thanks")
	f.until(t, "the snippet's image being read", h.reached)
	f.tt.Frame()
	if info, ok := f.tt.Access("Send (Ctrl+Enter)"); !ok || !info.Disabled {
		t.Errorf("Send while a snippet is read: %+v, %v", info, ok)
	}
	f.focus(t, "Message body")
	f.tt.Key(ui.Cmd, ui.KeyEnter)
	f.tt.Frame()
	if f.closed.Load() {
		t.Fatal("sent while a snippet was read")
	}
	h.free()
	f.until(t, "Send again", func() bool { info, ok := f.tt.Access("Send (Ctrl+Enter)"); return ok && !info.Disabled })
	m := f.sendAndGet(t)
	body, err := io.ReadAll(m.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("Many thanks")) {
		t.Errorf("the message went without the snippet: %s", body)
	}
}

// M31: closing waits for a snippet still being read, and saves the draft
// with it: before it is in, a fresh draft looks untouched, and would be
// deleted.
func TestClosingWaitsForASnippet(t *testing.T) {
	f := start(t)
	f.snippetWithImage(t)
	h := f.holdLoads(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, true, false)
	f.chooseSnippet(t, "Thanks")
	f.until(t, "the snippet's image being read", h.reached)
	if f.win.RequestClose() {
		t.Fatal("the window may go before the snippet is in")
	}
	h.free()
	f.until(t, "the window closing", f.closed.Load)
	got, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
	if err != nil {
		t.Fatalf("the draft: %v", err)
	}
	if !strings.Contains(got.Content.HTML, "Many thanks") {
		t.Errorf("the draft has %q", got.Content.HTML)
	}
}

// When the OS ends the session the process can end as soon as the quit's
// listeners return: SaveNow writes the last edits before it returns, without
// waiting for the autosave's delay, and gives up after its limit.
func TestSaveNowWritesTheLastEdits(t *testing.T) {
	f := start(t)
	d := f.draft(t, &api.DraftCreateParams{Kind: api.DraftKindNew})
	f.open(t, d, true, false)
	f.focus(t, "Subject")
	f.tt.Type("Before logout")
	f.tt.Frame()
	if err := f.win.SaveNow(10 * time.Second); err != nil {
		t.Fatal(err)
	}
	if got := f.subject(t, d); got != "Before logout" {
		t.Errorf("the draft's subject is %q", got)
	}
	h := f.holdCalls(t, "draft.update")
	f.tt.Type("!")
	f.tt.Frame()
	start := time.Now()
	if err := f.win.SaveNow(200 * time.Millisecond); err == nil {
		t.Error("a held save counted as done")
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Errorf("SaveNow waited %v past its limit", waited)
	}
	h.free()
}
