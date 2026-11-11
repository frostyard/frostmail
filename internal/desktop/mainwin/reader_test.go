package mainwin_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/rpctest"
)

var tags = regexp.MustCompile(`<style>.*?</style>|<div class="print-only">.*?</div></div>|<[^>]*>`)

// fakeFrame stands in for a web view: it draws the page's text.
type fakeFrame struct {
	mu        sync.Mutex
	doc       string
	closed    bool
	printed   int
	selection string
	matches   int
	finds     []string
}

func (f *fakeFrame) Load(document string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.doc = document
}

func (f *fakeFrame) text() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.TrimSpace(tags.ReplaceAllString(f.doc, ""))
}

func (f *fakeFrame) View(c *ui.Context) ui.Element {
	return ui.Column(c).Children(func() { ui.Text(c, f.text()) })
}
func (f *fakeFrame) Height() float32 { return 60 }
func (f *fakeFrame) Selection() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.selection
}
func (f *fakeFrame) Find(text string, next, back bool) (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finds = append(f.finds, fmt.Sprintf("%s next=%v back=%v", text, next, back))
	return 1, f.matches
}
func (f *fakeFrame) StopFind() {}
func (f *fakeFrame) Print() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.printed++
}
func (f *fakeFrame) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
}

// readerFixture is the window with fake pages, over a syncing maild.
type readerFixture struct {
	*syncFixture
	mu       sync.Mutex
	frames   []*fakeFrame
	launched []string
	links    []string
	drafts   []int64
	saved    string
}

func (f *readerFixture) live() []*fakeFrame {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*fakeFrame
	for _, fr := range f.frames {
		fr.mu.Lock()
		if !fr.closed {
			out = append(out, fr)
		}
		fr.mu.Unlock()
	}
	return out
}

func newReaderFixture(t *testing.T, seed func(mem *imapxtest.Mem)) *readerFixture {
	t.Helper()
	return newReaderFixtureUndo(t, seed, 0)
}

// newReaderFixtureUndo is newReaderFixture with maild's undo delay.
func newReaderFixtureUndo(t *testing.T, seed func(mem *imapxtest.Mem), undo time.Duration) *readerFixture {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	seed(mem)
	data := t.TempDir()
	cfg := mailsync.Config{PollInterval: time.Hour, IdleMax: time.Minute, MinBackoff: 50 * time.Millisecond, Chunk: 50, NoPrefetch: true}
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, DataDir: data, UndoDelay: undo})
	ctx := t.Context()
	c := srv.Dial(t)
	o := mem.DialOptions()
	a, err := c.Account().Create(ctx, &api.AccountCreateParams{Kind: api.AccountKindIMAP, Email: "me@x.test", DisplayName: "Me", Auth: api.AuthKindPassword,
		IMAP: &api.ServerConfig{Host: o.Host, Port: int64(o.Port), TLS: o.TLS, Username: o.Username},
		SMTP: &api.ServerConfig{Host: o.Host, Port: 1, TLS: o.TLS, Username: o.Username}})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: imapxtest.Password}); err != nil {
		t.Fatal(err)
	}
	r := &readerFixture{}
	fx := &fixture{srv: srv, acct: a.ID, loop: frostuitest.NewLoop()}
	fx.win = mainwin.New(mainwin.Options{Target: conn.Target{Socket: srv.Socket}, Client: "frostmail test",
		Post: fx.loop.Post, Wake: time.Millisecond, Retry: 10 * time.Millisecond, CacheDir: data, DownloadDir: t.TempDir(),
		NewFrame: func() (uikit.Frame, error) {
			f := &fakeFrame{}
			r.mu.Lock()
			defer r.mu.Unlock()
			r.frames = append(r.frames, f)
			return f, nil
		},
		Launch: func(url string) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.links = append(r.links, url)
			return nil
		},
		LaunchFile: func(path string) error {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.launched = append(r.launched, path)
			return nil
		},
		SaveDialog: func(_, _, _, _ string) (string, error) {
			r.mu.Lock()
			defer r.mu.Unlock()
			return r.saved, nil
		},
		OpenCompose: func(_ *api.Client, d int64, _ bool) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.drafts = append(r.drafts, d)
		},
	})
	fx.tt = ui.NewTester(func(c *ui.Context) {
		fx.loop.Flush()
		fx.win.View(c)
	}, 1200, 780)
	fx.win.Start(t.Context())
	r.syncFixture = &syncFixture{fixture: fx, c: c, mem: mem}
	return r
}

func raw(mem *imapxtest.Mem, t *testing.T, text string, flags ...imap.Flag) {
	t.Helper()
	if _, err := mem.User.Append("INBOX", strings.NewReader(strings.ReplaceAll(text, "\n", "\r\n")), &imap.AppendOptions{Flags: flags}); err != nil {
		t.Fatal(err)
	}
}

func oneMail(t *testing.T, headers, body string) *readerFixture {
	t.Helper()
	f := newReaderFixture(t, func(mem *imapxtest.Mem) {
		raw(mem, t, "From: Ann <ann@x.test>\nTo: me@x.test\nSubject: Alpha report\nMessage-ID: <a@x.test>\nDate: Mon, 09 Nov 2026 09:41:00 +0000\n"+headers+"\n"+body+"\n", imap.FlagSeen)
	})
	f.has(t, "the row", "Alpha report")
	return f
}

// open selects a row and waits for its message to draw.
//
// A sync delta landing between the press and the release changes the row's
// key, and the click is lost, so it clicks again until the message shows.
func (f *readerFixture) open(t *testing.T, subject string) {
	t.Helper()
	if err := f.tt.Click(subject); err != nil {
		t.Fatal(err)
	}
	frames := 0
	f.until(t, "the message's page", func() bool {
		if _, ok := f.tt.Access("Message body"); ok {
			return true
		}
		if frames++; frames%40 == 0 {
			if err := f.tt.Click(subject); err != nil {
				t.Fatal(err)
			}
		}
		return false
	})
}

// P-301: selecting a message shows it, and what shows is read.
func TestReaderShowsTheMessageAndMarksItRead(t *testing.T) {
	f := newReaderFixture(t, func(mem *imapxtest.Mem) {
		raw(mem, t, "From: Ann <ann@x.test>\nTo: Me <me@x.test>, Bo <bo@x.test>, Cy <cy@x.test>, Di <di@x.test>\nCc: Ed <ed@x.test>\nSubject: Alpha report\nMessage-ID: <a@x.test>\nDate: Mon, 09 Nov 2026 09:41:00 +0000\n\nHello from the body\n")
	})
	f.has(t, "the row", "Alpha report")
	f.open(t, "Alpha report")
	for _, want := range []string{"To:", "Cc:", "& 1 more", "Ed"} {
		if !f.tt.HasText(want) {
			t.Errorf("no %q in %q", want, f.tt.Texts())
		}
	}
	f.until(t, "the message read", func() bool {
		s, err := f.c.Message().Summaries(t.Context(), &api.MessageSummariesParams{IDs: []int64{1}})
		return err == nil && len(s) == 1 && s[0].Flags.Seen
	})
	snapshot(t, f.tt, "reader")
}

func thread(t *testing.T, mem *imapxtest.Mem, n int) {
	t.Helper()
	refs := ""
	for i := 1; i <= n; i++ {
		raw(mem, t, fmt.Sprintf("From: Ann <ann@x.test>\nTo: me@x.test\nSubject: Long talk\nMessage-ID: <m%d@x.test>\n%sDate: Mon, 09 Nov 2026 09:%02d:00 +0000\n\nBody number %d\n", i, refs, i%60, i), imap.FlagSeen)
		refs = fmt.Sprintf("In-Reply-To: <m%d@x.test>\nReferences: <m%d@x.test>\n", i, i)
	}
}

// P-310: older read messages collapse; a click opens one, and a page
// exists only for a message that shows it.
func TestConversationCollapsesReadMessages(t *testing.T) {
	f := newReaderFixture(t, func(mem *imapxtest.Mem) { thread(t, mem, 3) })
	f.has(t, "the row", "Long talk")
	f.open(t, "Long talk")
	if n := len(f.live()); n != 1 {
		t.Errorf("%d pages for 1 expanded message", n)
	}
	if err := f.tt.Click("Ann: Long talk"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "a second page", func() bool { return len(f.live()) == 2 })
	// A click on an expanded message's header collapses it again.
	who, ok := f.tt.Access("Ann, ann@x.test")
	if !ok {
		t.Fatalf("no sender in %q", f.tt.Texts())
	}
	f.tt.ClickAt(who.Bounds.X+who.Bounds.W+150, who.Bounds.Y+who.Bounds.H/2)
	f.until(t, "the page closed", func() bool { return len(f.live()) == 1 })
}

// P-310: the 20 newest show, then the rest on asking.
func TestConversationShowsEarlierOnAsking(t *testing.T) {
	f := newReaderFixture(t, func(mem *imapxtest.Mem) { thread(t, mem, 23) })
	f.has(t, "the row", "Long talk")
	f.open(t, "Long talk")
	f.has(t, "the button", "Show 3 Earlier Messages")
	f.tt.Scroll(800, 400, 0, 5000)
	f.tt.Frame()
	if err := f.tt.Click("Show 3 Earlier Messages"); err != nil {
		t.Fatal(err)
	}
	f.hasNot(t, "the button gone", "Show 3 Earlier Messages")
}

// P-304: More Actions shows All Headers and Raw Source.
func TestMoreActionsShowsHeadersAndSource(t *testing.T) {
	f := oneMail(t, "X-Canary: seen-in-headers\n", "Body here")
	f.open(t, "Alpha report")
	more, ok := f.tt.Access("More Actions")
	if !ok {
		t.Fatalf("no More Actions in %q", f.tt.Texts())
	}
	f.tt.ClickAt(more.Bounds.X+more.Bounds.W/2, more.Bounds.Y+more.Bounds.H/2)
	if err := f.tt.ChooseMenuItem("Show All Headers"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.until(t, "the headers", func() bool {
		return slices.ContainsFunc(f.tt.Texts(), func(s string) bool { return strings.Contains(s, "X-Canary: seen-in-headers") })
	})
	f.tt.Key(ui.Cmd|ui.Alt, ui.KeyU)
	f.has(t, "the raw source", "Raw Source")
	if err := f.tt.Click("Close"); err != nil {
		t.Fatal(err)
	}
	f.hasNot(t, "the sheet closed", "Raw Source")
}

// P-302: the remote content banner.
func TestRemoteContentBanner(t *testing.T) {
	f := oneMail(t, "Content-Type: text/html\n", `<p>Hi</p><img src="https://remote.test/a.png" width="100" height="100">`)
	f.open(t, "Alpha report")
	f.has(t, "the banner", "This message contains remote content.")
	f.has(t, "the button", "Load Remote Content")
}

// ADR-0050: the sender check banner, with its details.
func TestSenderCheckBanner(t *testing.T) {
	f := oneMail(t, "Authentication-Results: mx.x.test; dmarc=fail header.from=x.test\n", "Careful")
	f.open(t, "Alpha report")
	f.until(t, "the banner", func() bool {
		return slices.ContainsFunc(f.tt.Texts(), func(s string) bool { return strings.HasPrefix(s, "Sender not verified.") })
	})
	if err := f.tt.Click("Show Details"); err != nil {
		t.Fatal(err)
	}
	f.has(t, "the details open", "Hide Details")
}

// A blocked sender's banner unblocks.
func TestBlockedBannerUnblocks(t *testing.T) {
	f := oneMail(t, "", "Spammy")
	if _, err := f.c.Block().Add(t.Context(), &api.BlockAddParams{Addresses: []string{"ann@x.test"}}); err != nil {
		t.Fatal(err)
	}
	f.open(t, "Alpha report")
	f.has(t, "the banner", "This message is from a blocked sender.")
	if err := f.tt.Click("Unblock"); err != nil {
		t.Fatal(err)
	}
	f.hasNot(t, "the banner gone", "This message is from a blocked sender.")
}

// P-305: Unsubscribe asks first, and a web method opens its page.
func TestUnsubscribeBanner(t *testing.T) {
	f := oneMail(t, "List-Id: The List <list.x.test>\nList-Unsubscribe: <https://x.test/leave>\n", "News")
	f.open(t, "Alpha report")
	f.has(t, "the banner", "This message is from the mailing list list.x.test.")
	if err := f.tt.Click("Unsubscribe"); err != nil {
		t.Fatal(err)
	}
	f.has(t, "the question", "Unsubscribe from list.x.test?")
	if !f.tt.HasText("The list's page on x.test will open in your browser.") {
		t.Errorf("no sentence in %q", f.tt.Texts())
	}
	f.tt.Key(0, ui.KeyEnter)
	f.until(t, "the list's page opened", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return slices.Contains(f.links, "https://x.test/leave")
	})
}

// Attachments open by an allow-list.
func TestAttachmentsOpenByAnAllowList(t *testing.T) {
	mime := func(name, ctype string) string {
		return "Content-Type: multipart/mixed; boundary=b\n\n--b\nContent-Type: text/plain\n\nSee attached\n--b\nContent-Type: " + ctype + "; name=\"" + name + "\"\nContent-Disposition: attachment; filename=\"" + name + "\"\nContent-Transfer-Encoding: base64\n\nSGVsbG8=\n--b--"
	}
	f := oneMail(t, "MIME-Version: 1.0\n", "")
	raw(f.mem, t, "From: Bo <bo@x.test>\nTo: me@x.test\nSubject: Paper\nMessage-ID: <p@x.test>\nDate: Mon, 09 Nov 2026 10:41:00 +0000\nMIME-Version: 1.0\n"+mime("paper.pdf", "application/pdf"), imap.FlagSeen)
	raw(f.mem, t, "From: Cy <cy@x.test>\nTo: me@x.test\nSubject: Program\nMessage-ID: <e@x.test>\nDate: Mon, 09 Nov 2026 11:41:00 +0000\nMIME-Version: 1.0\n"+mime("setup.exe", "application/octet-stream"), imap.FlagSeen)
	if err := f.c.Sync().Now(t.Context(), &api.SyncNowParams{AccountID: f.acct}); err != nil {
		t.Fatal(err)
	}
	f.has(t, "the paper", "Paper")
	f.open(t, "Paper")
	f.has(t, "the chip", "paper.pdf")
	if err := f.tt.Click("paper.pdf"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the file opened", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return len(f.launched) == 1 && strings.HasSuffix(f.launched[0], "paper.pdf")
	})
	f.open(t, "Program")
	f.has(t, "the chip", "setup.exe")
	if err := f.tt.Click("setup.exe"); err != nil {
		t.Fatal(err)
	}
	f.has(t, "the refusal", "Frostmail does not open this kind of file")
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.launched) != 1 {
		t.Errorf("launched %q", f.launched)
	}
}

// P-309: Ctrl+F finds across the pages, Enter moves.
func TestFindCountsAcrossThePages(t *testing.T) {
	f := oneMail(t, "", "needle needle")
	f.open(t, "Alpha report")
	f.until(t, "the page", func() bool { return len(f.live()) == 1 })
	f.live()[0].matches = 2
	f.tt.Key(ui.Cmd, ui.KeyF)
	f.tt.Frame()
	f.tt.Type("needle")
	f.has(t, "the count", "1 of 2")
	f.tt.Key(0, ui.KeyEnter)
	f.has(t, "the next", "2 of 2")
	f.until(t, "the page's next", func() bool {
		fr := f.live()[0]
		fr.mu.Lock()
		defer fr.mu.Unlock()
		return slices.Contains(fr.finds, "needle next=true back=false")
	})
}

// P-508: Reply quotes the text selected in the page.
func TestReplyQuotesTheSelection(t *testing.T) {
	f := oneMail(t, "", "Whole body\nsecond line")
	f.open(t, "Alpha report")
	f.until(t, "the page", func() bool { return len(f.live()) == 1 && strings.Contains(f.live()[0].text(), "Whole body") })
	fr := f.live()[0]
	fr.mu.Lock()
	fr.selection = "second line"
	fr.mu.Unlock()
	f.tt.Key(ui.Cmd, ui.KeyR)
	f.until(t, "the draft", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return len(f.drafts) == 1
	})
	d, err := f.c.Draft().Get(t.Context(), &api.DraftGetParams{ID: f.drafts[0]})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Content.HTML, "second line") || strings.Contains(d.Content.HTML, "Whole body") {
		t.Errorf("draft quotes %q", d.Content.HTML)
	}
}

// P-307: Print goes to the page of the selected message.
func TestPrintGoesToThePage(t *testing.T) {
	f := oneMail(t, "", "To paper")
	f.open(t, "Alpha report")
	f.until(t, "the page", func() bool { return len(f.live()) == 1 })
	f.tt.Key(ui.Cmd, ui.KeyP)
	f.tt.Frame()
	fr := f.live()[0]
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if fr.printed != 1 {
		t.Errorf("printed %d times", fr.printed)
	}
	if !strings.Contains(fr.doc, `<div class="print-only">`) || !strings.Contains(fr.doc, "Alpha report") {
		t.Errorf("no print header in %s", fr.doc)
	}
}

// P-312: the contact popover adds the sender to VIPs.
func TestContactPopover(t *testing.T) {
	f := oneMail(t, "", "Body")
	f.open(t, "Alpha report")
	f.tt.Frame()
	who, ok := f.tt.Access("Ann, ann@x.test")
	if !ok {
		t.Fatalf("no sender in %q", f.tt.Texts())
	}
	f.tt.ClickAt(who.Bounds.X+who.Bounds.W/2, who.Bounds.Y+who.Bounds.H/2)
	f.has(t, "the card", "Add to VIPs")
	if err := f.tt.Click("Add to VIPs"); err != nil {
		t.Fatal(err)
	}
	f.has(t, "the VIP", "Remove from VIPs")
}

// --open-message and palette results: the message shows and is selected,
// whatever the list holds.
func TestOpenMessageRevealsIt(t *testing.T) {
	f := oneMail(t, "", "Far away")
	f.tt.Frame()
	f.win.OpenMessage(1)
	f.until(t, "the message's page", func() bool {
		_, ok := f.tt.Access("Message body")
		return ok
	})
}

// Tab moves between the panes.
func TestTabMovesBetweenPanes(t *testing.T) {
	f := oneMail(t, "", "Body")
	f.open(t, "Alpha report")
	f.tt.Key(0, ui.KeyTab)
	f.tt.Frame()
	f.tt.Frame()
	info, ok := f.tt.Access("Message")
	if !ok || !info.Focused {
		t.Errorf("after Tab from the list, the reader's focus is %+v, %v", info, ok)
	}
}

// The palette finds mailboxes and messages as well as commands.
func TestPaletteFindsMailboxesAndMessages(t *testing.T) {
	f := oneMail(t, "", "Findable body")
	f.has(t, "Trash", "Trash")
	f.tt.Key(ui.Cmd, ui.KeyK)
	f.tt.Frame()
	f.tt.Type("Alpha")
	f.until(t, "the message result", func() bool {
		return strings.Contains(strings.Join(f.tt.Texts(), "|"), "Messages|Alpha report")
	})
	f.tt.Key(0, ui.KeyEscape)
	f.tt.Frame()
	f.tt.Key(ui.Cmd, ui.KeyK)
	f.tt.Frame()
	f.tt.Type("Tras")
	f.until(t, "the mailbox result", func() bool {
		return strings.Contains(strings.Join(f.tt.Texts(), "|"), "Mailboxes|Trash")
	})
}

// P-306: the Remind Me banner, and Clear.
func TestReminderBanner(t *testing.T) {
	f := oneMail(t, "", "Later")
	at := time.Now().Add(48 * time.Hour)
	if err := f.c.Message().Remind(t.Context(), &api.MessageRemindParams{IDs: []int64{1}, At: &at}); err != nil {
		t.Fatal(err)
	}
	f.open(t, "Alpha report")
	f.until(t, "the banner", func() bool {
		return slices.ContainsFunc(f.tt.Texts(), func(s string) bool { return strings.HasPrefix(s, "Remind Me: ") })
	})
	if err := f.tt.Click("Clear"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the banner gone", func() bool {
		return !slices.ContainsFunc(f.tt.Texts(), func(s string) bool { return strings.HasPrefix(s, "Remind Me: ") })
	})
}

// P-308: Save As writes the message as the server holds it.
func TestSaveAsWritesTheMessage(t *testing.T) {
	f := oneMail(t, "X-Canary: saved-whole\n", "Keep me")
	dest := filepath.Join(t.TempDir(), "saved.eml")
	f.mu.Lock()
	f.saved = dest
	f.mu.Unlock()
	f.open(t, "Alpha report")
	more, ok := f.tt.Access("More Actions")
	if !ok {
		t.Fatalf("no More Actions in %q", f.tt.Texts())
	}
	f.tt.ClickAt(more.Bounds.X+more.Bounds.W/2, more.Bounds.Y+more.Bounds.H/2)
	if err := f.tt.ChooseMenuItem("Save As…"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.until(t, "the file", func() bool {
		data, err := os.ReadFile(dest)
		return err == nil && strings.Contains(string(data), "X-Canary: saved-whole")
	})
}

// P-311: an invitation shows as a card with its answers.
func TestInvitationCard(t *testing.T) {
	ics := "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//t//t//EN\nMETHOD:REQUEST\nBEGIN:VEVENT\nUID:inv1@x.test\nDTSTAMP:20261109T090000Z\nDTSTART:20261120T150000Z\nDTEND:20261120T160000Z\nSUMMARY:Planning\nORGANIZER;CN=Ann:mailto:ann@x.test\nATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:me@x.test\nEND:VEVENT\nEND:VCALENDAR"
	f := oneMail(t, "MIME-Version: 1.0\nContent-Type: text/calendar; method=REQUEST; charset=utf-8\n", ics)
	f.open(t, "Alpha report")
	f.has(t, "the card", "Planning")
	f.has(t, "the badge", "Invitation")
	f.has(t, "the link to the calendar", "Show in Calendar")
}

// P-505: a sent message shows the undo toast, and Undo reopens its draft.
func TestUndoSendReopensTheDraft(t *testing.T) {
	f := newReaderFixtureUndo(t, func(mem *imapxtest.Mem) {
		raw(mem, t, "From: Ann <ann@x.test>\nTo: me@x.test\nSubject: Alpha report\nMessage-ID: <a@x.test>\nDate: Mon, 09 Nov 2026 09:41:00 +0000\n\nBody\n", imap.FlagSeen)
	}, time.Hour)
	f.has(t, "the row", "Alpha report")
	ctx := t.Context()
	d, err := f.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew, To: []api.Address{{Address: "bo@x.test"}}})
	if err != nil {
		t.Fatal(err)
	}
	content := d.Content
	content.Subject = "Going out"
	if _, err := f.c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: content}); err != nil {
		t.Fatal(err)
	}
	item, err := f.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.loop.Post(func() { f.win.Sent(item) })
	f.has(t, "the toast", "Sending “Going out”…")
	if err := f.tt.Click("Undo"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the draft reopened", func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return slices.Contains(f.drafts, d.ID)
	})
}

// A compose window closed with a failed save tells the main window, whose
// toast says so (cmd/frostmail/compose.go wires ShowProblem).
func TestShowProblemShowsAToast(t *testing.T) {
	f := oneMail(t, "", "Body")
	f.win.ShowProblem("The draft was not saved", errors.New("maild is gone"))
	f.has(t, "the toast", "The draft was not saved")
}

// A message sent later shows no toast: it waits in its own section.
func TestSendLaterShowsNoToast(t *testing.T) {
	f := oneMail(t, "", "Body")
	at := time.Now().Add(48 * time.Hour)
	f.loop.Post(func() { f.win.Sent(&api.OutboxItem{ID: 9, Subject: "Later", Scheduled: true, SendAt: &at}) })
	f.tt.Frame()
	f.tt.Frame()
	if f.tt.HasText("Sending “Later”…") {
		t.Errorf("a toast for a scheduled message: %q", f.tt.Texts())
	}
}

// P-413: a Follow Up row opens the message.
func TestFollowUpRowOpensTheMessage(t *testing.T) {
	f := newReaderFixture(t, func(mem *imapxtest.Mem) {
		if _, err := mem.User.Append("Sent", strings.NewReader("From: Me <me@x.test>\r\nTo: Bo <bo@x.test>\r\nSubject: Any news\r\nMessage-ID: <s@x.test>\r\nDate: Sun, 01 Nov 2026 09:41:00 +0000\r\n\r\nStill waiting\r\n"), &imap.AppendOptions{Flags: []imap.Flag{imap.FlagSeen}}); err != nil {
			t.Fatal(err)
		}
	})
	f.until(t, "the Follow Up section", func() bool {
		return slices.ContainsFunc(f.tt.Texts(), func(s string) bool { return strings.HasPrefix(s, "Follow Up (") })
	})
	if err := f.tt.Click("Any news"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the message's page", func() bool {
		_, ok := f.tt.Access("Message body")
		return ok
	})
}
