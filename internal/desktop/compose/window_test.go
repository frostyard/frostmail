package compose_test

import (
	"bytes"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	c      *api.Client
	acct   int64
	sink   *smtpxtest.Server
	loop   *frostuitest.Loop
	win    *compose.Window
	tt     *ui.Tester
	closed atomic.Bool
	sent   atomic.Pointer[api.OutboxItem]
	picked []string
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
	f := &fixture{c: c, acct: a.ID, sink: sink, loop: frostuitest.NewLoop()}
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
	f.win = compose.New(compose.Options{Client: f.c, Draft: d.ID, Fresh: fresh, Post: f.loop.Post,
		Close:        func() { f.closed.Store(true) },
		OpenFiles:    func() ([]string, error) { return f.picked, nil },
		FilesAsBytes: bytesMode,
		Sent:         func(i *api.OutboxItem) { f.sent.Store(i) },
	})
	f.tt = ui.NewTester(func(c *ui.Context) {
		f.loop.Flush()
		f.win.View(c)
	}, 720, 560)
	f.win.Start(t.Context())
	f.until(t, "the draft", func() bool { _, ok := f.tt.Access("Message body"); return ok })
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
	f.win.Closing()
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
	f.win.Closing()
	f.until(t, "the saved subject", func() bool {
		got, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: d.ID})
		return err == nil && got.Content.Subject == "Kept"
	})
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
