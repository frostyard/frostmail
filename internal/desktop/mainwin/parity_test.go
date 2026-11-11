package mainwin_test

import (
	"encoding/base64"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// listRight is the list pane's right edge at the default widths.
const listRight = 580

// syncFixture is the window against a maild syncing an in-memory IMAP
// server with three messages in INBOX, so actions reach a server.
type syncFixture struct {
	*fixture
	c   *api.Client
	mem *imapxtest.Mem
}

func appendMail(t *testing.T, mem *imapxtest.Mem, subject, from string, flags ...imap.Flag) {
	t.Helper()
	raw := fmt.Sprintf("From: %s\r\nTo: me@x.test\r\nSubject: %s\r\nMessage-ID: <%s@x.test>\r\nDate: Mon, 09 Nov 2026 09:41:00 +0000\r\n\r\nBody of %s\r\n",
		from, subject, strings.ReplaceAll(subject, " ", "."), subject)
	if _, err := mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{Flags: flags}); err != nil {
		t.Fatal(err)
	}
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	appendMail(t, mem, "Alpha report", "Ann <ann@x.test>")
	appendMail(t, mem, "Beta plan", "Bob <bob@x.test>", imap.FlagSeen)
	appendMail(t, mem, "Gamma notes", "Cid <cid@x.test>", imap.FlagSeen)
	cfg := mailsync.Config{PollInterval: time.Hour, IdleMax: time.Minute, MinBackoff: 50 * time.Millisecond, Chunk: 50, NoPrefetch: true}
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg})
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
	f := &fixture{srv: srv, acct: a.ID, loop: frostuitest.NewLoop()}
	f.win = mainwin.New(mainwin.Options{Target: conn.Target{Socket: srv.Socket}, Client: "frostmail test",
		Post: f.loop.Post, Wake: time.Millisecond, Retry: 10 * time.Millisecond})
	f.tt = ui.NewTester(func(c *ui.Context) {
		f.loop.Flush()
		f.win.View(c)
	}, 1200, 780)
	f.win.Start(t.Context())
	f.until(t, "the synced mail", func() bool {
		return f.tt.HasText("Alpha report") && f.tt.HasText("Beta plan") && f.tt.HasText("Gamma notes")
	})
	return &syncFixture{fixture: f, c: c, mem: mem}
}

func (f *syncFixture) hasNot(t *testing.T, what, text string) {
	t.Helper()
	f.until(t, what, func() bool { return !f.tt.HasText(text) })
}

func (f *syncFixture) has(t *testing.T, what, text string) {
	t.Helper()
	f.until(t, what, func() bool { return f.tt.HasText(text) })
}

// P-204: the filter row narrows the list; P-204's empty state names it.
func TestFilterRowNarrowsTheList(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.tt.Click("Unread"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "only the unread message", func() bool { return f.tt.HasText("Alpha report") && !f.tt.HasText("Beta plan") })
	// The sidebar's Flagged comes first by name, so aim at the filter's:
	// the tab just right of Unread.
	prev, ok := f.tt.Find("Unread")
	if !ok {
		t.Fatalf("no Unread tab: %q", f.tt.Texts())
	}
	f.tt.ClickAt(prev.X+prev.W+20, prev.Y+prev.H/2)
	f.has(t, "the empty state", "No Flagged Messages in All Inboxes")
	if err := f.tt.Click("Show All"); err != nil {
		t.Fatal(err)
	}
	f.has(t, "all mail back", "Beta plan")
}

// P-203 and P-202: the sort menu.
func TestSortMenuSorts(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.tt.Click("Sort by Date"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Subject"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.until(t, "the sort's label", func() bool { _, ok := f.tt.Access("Sort by Subject"); return ok })
	f.until(t, "subjects in order", func() bool {
		texts := f.tt.Texts()
		a, b := slices.Index(texts, "Alpha report"), slices.Index(texts, "Gamma notes")
		return a >= 0 && b > a
	})
}

// P-601: search, its scope and clearing it.
func TestSearchNarrowsAndClears(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.tt.Click("Search"); err != nil {
		t.Fatal(err)
	}
	f.tt.Type("Gamma")
	f.tt.Key(0, ui.KeyEnter)
	f.until(t, "only the match", func() bool { return f.tt.HasText("Gamma notes") && !f.tt.HasText("Beta plan") })
	f.has(t, "the scope bar", "All Mailboxes")
	if err := f.tt.Click("Clear"); err != nil {
		t.Fatal(err)
	}
	f.tt.Key(0, ui.KeyEnter)
	f.has(t, "the whole list", "Beta plan")
}

// P-404 and P-406: Delete from the toolbar, and Ctrl+Z.
func TestDeleteAndUndo(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.tt.Click("Beta plan"); err != nil {
		t.Fatal(err)
	}
	f.tt.Frame()
	if err := f.tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	f.hasNot(t, "the deleted row", "Beta plan")
	f.tt.Key(ui.Cmd, ui.KeyZ)
	f.has(t, "the row after undo", "Beta plan")
}

// P-407, P-409 and the row's menu.
func TestRowMenuFlagsAndMarks(t *testing.T) {
	f := newSyncFixture(t)
	f.has(t, "the Junk mailbox", "Junk")
	if err := f.tt.RightClick("Beta plan"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Flag Color", "Blue"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.until(t, "the flag on the row", func() bool {
		_, ok := f.tt.Access("Flagged: Blue")
		return ok
	})
	if err := f.tt.RightClick("Beta plan"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Mark as Unread"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the unread count", func() bool {
		return strings.Contains(strings.Join(f.tt.Texts(), "|"), "All Inboxes|2")
	})
}

// P-102: a mailbox's menu adds it to Favorites.
func TestAddToFavorites(t *testing.T) {
	f := newSyncFixture(t)
	f.has(t, "the Junk mailbox", "Junk")
	if err := f.tt.RightClick("Junk"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Add to Favorites"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.until(t, "the favorite", func() bool {
		texts := f.tt.Texts()
		favorites, junk, smart := slices.Index(texts, "Favorites"), slices.Index(texts, "Junk"), slices.Index(texts, "Smart Mailboxes")
		return favorites >= 0 && favorites < junk && junk < smart
	})
}

// P-110: Erase asks first.
func TestEraseAsksFirst(t *testing.T) {
	f := newSyncFixture(t)
	f.has(t, "the Trash mailbox", "Trash")
	if err := f.tt.RightClick("Trash"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Erase Deleted Items…"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.has(t, "the confirmation", "Erase Deleted Items?")
}

// P-108: dragging a row to a mailbox moves it.
func TestDragToAMailboxMoves(t *testing.T) {
	f := newSyncFixture(t)
	f.has(t, "the Junk mailbox", "Junk")
	row, ok := f.tt.Find("Beta plan")
	if !ok {
		t.Fatal("no row")
	}
	target, ok := f.tt.Find("Junk")
	if !ok {
		t.Fatal("no target")
	}
	f.tt.Press(row.X+row.W/2, row.Y+row.H/2)
	f.tt.Move(row.X+row.W/2-30, row.Y+row.H/2-5)
	f.tt.Move(target.X+target.W/2, target.Y+target.H/2)
	f.tt.Release(target.X+target.W/2, target.Y+target.H/2)
	f.hasNot(t, "the moved row", "Beta plan")
}

// P-201: a conversation's row shows its count. P-208: a VIP's star.
func TestRowShowsCountAndVIPStar(t *testing.T) {
	f := newSyncFixture(t)
	reply := "From: Ann <ann@x.test>\r\nTo: me@x.test\r\nSubject: Re: Alpha report\r\nMessage-ID: <r1@x.test>\r\nIn-Reply-To: <Alpha.report@x.test>\r\nReferences: <Alpha.report@x.test>\r\nDate: Mon, 09 Nov 2026 10:41:00 +0000\r\n\r\nReply\r\n"
	if _, err := f.mem.User.Append("INBOX", strings.NewReader(reply), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := f.c.Sync().Now(t.Context(), &api.SyncNowParams{AccountID: f.acct}); err != nil {
		t.Fatal(err)
	}
	f.has(t, "the conversation's count", "2")
	if _, err := f.c.Vip().Add(t.Context(), &api.VipAddParams{Addresses: []string{"ann@x.test"}}); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the VIP star", func() bool { _, ok := f.tt.Access("VIP"); return ok })
}

// P-207: a row's hover actions show under the pointer, hidden from
// assistive technology.
func TestRowHoverActions(t *testing.T) {
	f := newSyncFixture(t)
	row, ok := f.tt.Find("Beta plan")
	if !ok {
		t.Fatal("no row")
	}
	// The row's buttons stand at its bottom right; Delete is the last.
	x, y := float32(listRight-30), row.Y+34
	f.tt.Move(x, y)
	f.tt.Frame()
	snapshot(t, f.tt, "hover")
	f.tt.ClickAt(x, y)
	f.hasNot(t, "the row after its hover Delete", "Beta plan")
}

// P-109: Use This Mailbox For sets a role.
func TestUseThisMailboxFor(t *testing.T) {
	f := newSyncFixture(t)
	ctx := t.Context()
	if _, err := f.c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: f.acct, Name: "Papers"}); err != nil {
		t.Fatal(err)
	}
	f.has(t, "the new mailbox", "Papers")
	if err := f.tt.RightClick("Papers"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Use This Mailbox For", "Archive"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.until(t, "the role", func() bool {
		list, err := f.c.Mailbox().List(ctx, &api.MailboxListParams{AccountID: &f.acct})
		return err == nil && slices.ContainsFunc(list, func(m api.Mailbox) bool { return m.Name == "Papers" && m.Role == api.MailboxRoleArchive })
	})
}

// P-202: conversations can be turned off.
func TestConversationsToggle(t *testing.T) {
	f := newSyncFixture(t)
	reply := "From: Ann <ann@x.test>\r\nTo: me@x.test\r\nSubject: Re: Alpha report\r\nMessage-ID: <r1@x.test>\r\nIn-Reply-To: <Alpha.report@x.test>\r\nReferences: <Alpha.report@x.test>\r\nDate: Mon, 09 Nov 2026 10:41:00 +0000\r\n\r\nReply\r\n"
	if _, err := f.mem.User.Append("INBOX", strings.NewReader(reply), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := f.c.Sync().Now(t.Context(), &api.SyncNowParams{AccountID: f.acct}); err != nil {
		t.Fatal(err)
	}
	f.has(t, "3 conversations", "3 messages")
	f.has(t, "the newest of the thread", "Re: Alpha report")
	if err := f.tt.Click("Sort by Date"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Conversations"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.has(t, "4 messages", "4 messages")
}

// P-205: More's filters.
func TestMoreFilterNarrows(t *testing.T) {
	f := newSyncFixture(t)
	if err := f.tt.Click("More filters"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("From VIPs"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.has(t, "the empty state", "No Messages from VIPs in All Inboxes")
}

// P-405: Mark as Spam moves to Junk.
func TestMarkAsSpam(t *testing.T) {
	f := newSyncFixture(t)
	f.has(t, "the Junk mailbox", "Junk")
	if err := f.tt.RightClick("Beta plan"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Mark as Spam"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.hasNot(t, "the row gone to Junk", "Beta plan")
}

// P-105: a smart mailbox shows in the sidebar and lists its messages.
func TestSmartMailboxShowsAndLists(t *testing.T) {
	f := newSyncFixture(t)
	cond := api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{{Field: api.ConditionFieldSubject, Op: api.ConditionOpContains, Value: "Gamma"}}}
	if _, err := f.c.Smart().Create(t.Context(), &api.SmartCreateParams{Name: "Gammas", Conditions: cond}); err != nil {
		t.Fatal(err)
	}
	f.has(t, "the smart mailbox", "Gammas")
	if err := f.tt.Click("Gammas"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "its messages", func() bool { return f.tt.HasText("Gamma notes") && !f.tt.HasText("Beta plan") })
}

// P-209: with Contact Photos on, a sender who is a contact with a photo has
// it in the list, from maild; off, the row has none.
func TestContactPhotosInTheList(t *testing.T) {
	f := newFixture(t, 2)
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	err = f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.SetService(ctx, f.acct, api.ServiceKindContacts, true, "https://dav.test/"); err != nil {
			return err
		}
		cols, err := tx.ReplaceCollections(ctx, f.acct, api.CollectionKindAddressbook, []store.RemoteCollection{{Href: "/ab/", Name: "Contacts"}})
		if err != nil {
			return err
		}
		id, err := tx.PutObject(ctx, store.Object{CollectionID: cols[0].ID, Href: "/ab/ann.vcf", ETag: `"1"`, Kind: store.ObjectVCard,
			Raw: []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:ann\r\nFN:Ann\r\nEMAIL:ann@x.test\r\nEND:VCARD\r\n")})
		if err != nil {
			return err
		}
		if err := tx.IndexContact(ctx, id, store.ContactIndex{DisplayName: "Ann", SortKey: "ann",
			Emails: []store.ContactEmail{{Email: "ann@x.test"}}, Photo: png, PhotoType: "image/png"}); err != nil {
			return err
		}
		return tx.RelinkPeople(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	f.until(t, "the list", func() bool { return slices.Contains(f.tt.Texts(), "Message 2") })
	if _, ok := f.tt.AccessByRole(ui.RoleImage, "Ann"); ok {
		t.Fatal("a photo with Contact Photos off")
	}
	if err := f.tt.Click("Sort by Date"); err != nil {
		t.Fatal(err)
	}
	if err := f.tt.ChooseMenuItem("Contact Photos"); err != nil {
		t.Fatalf("%v; menu %q", err, f.tt.Menu())
	}
	f.until(t, "the sender's photo", func() bool { _, ok := f.tt.AccessByRole(ui.RoleImage, "Ann"); return ok })
}
