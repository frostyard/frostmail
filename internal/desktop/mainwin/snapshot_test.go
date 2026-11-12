package mainwin_test

import (
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/mainwin"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

func writePNG(path string, tt *ui.Tester) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, tt.Image())
}

// snapshotNow is the snapshot window's clock: Wednesday, November 11,
// 2026, 10:30 UTC.
var snapshotNow = time.Date(2026, 11, 11, 10, 30, 0, 0, time.UTC)

// snapshotMail is the inbox the snapshots show, oldest first: a thread of
// two, a flagged and an unread message, and a preview of one long token
// (frostyard/frostmail#267).
var snapshotMail = []struct {
	text  string
	flags []imap.Flag
}{
	{"From: Release notes <notes@updex.test>\nSubject: updex 1.4: sysext rollback\nMessage-ID: <r1@x.test>\nDate: Mon, 09 Nov 2026 08:00:00 +0000\n\nupdex can now roll a feature back to its previous image with one command.", []imap.Flag{imap.FlagSeen}},
	{"From: Ines Okafor <ines@x.test>\nSubject: Dinner on Saturday?\nMessage-ID: <d1@x.test>\nDate: Tue, 10 Nov 2026 18:20:00 +0000\n\nWe found a place near the river. Seven o'clock? Bring a coat; the terrace is open.", []imap.Flag{imap.FlagSeen}},
	{"From: Spacer Weekly <news@spacer.test>\nSubject: This week in spacing\nMessage-ID: <s1@x.test>\nDate: Tue, 10 Nov 2026 19:00:00 +0000\n\n" + strings.Repeat("spacer", 60), nil},
	{"From: Tomas Reyes <tomas@x.test>\nSubject: Frostmail 0.6 release checklist\nMessage-ID: <c1@x.test>\nDate: Tue, 10 Nov 2026 20:15:00 +0000\n\nHere is the checklist for 0.6. Item 1 is the candidate; the rest follow from it.", []imap.Flag{imap.FlagSeen}},
	{"From: Tomas Reyes <tomas@x.test>\nSubject: Sysext rollback test plan\nMessage-ID: <t1@x.test>\nDate: Wed, 11 Nov 2026 08:30:00 +0000\n\nThree hosts, two images, one rollback each. Results land in the shared notes by Friday.", []imap.Flag{imap.FlagFlagged}},
	{"From: Mara Lind <mara@x.test>\nSubject: Re: Frostmail 0.6 release checklist\nMessage-ID: <c2@x.test>\nIn-Reply-To: <c1@x.test>\nReferences: <c1@x.test>\nDate: Wed, 11 Nov 2026 09:12:00 +0000\n\nThe candidate passed the flatpak gate overnight. I'd call item 1 done; promote is yours when you're ready.", []imap.Flag{imap.FlagSeen}},
	{"From: snosi builds <builds@snosi.test>\nSubject: Image 2026.11.09 passed every gate\nMessage-ID: <b1@x.test>\nDate: Wed, 11 Nov 2026 09:41:00 +0000\n\nThe stable channel moves at 06:00. Staged hosts pick it up at their next restart.", nil},
}

// snapshotFixture is the window over a syncing maild that holds the
// snapshots' mail, contacts, calendars and tasks.
func snapshotFixture(t *testing.T) *fixture {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	for _, m := range snapshotMail {
		raw(mem, t, "To: Me <me@x.test>\n"+m.text+"\n", m.flags...)
	}
	sent := "From: Me <me@x.test>\r\nTo: Bo <bo@x.test>\r\nSubject: Signing key rotation\r\nMessage-ID: <k1@x.test>\r\nDate: Sun, 01 Nov 2026 09:41:00 +0000\r\n\r\nAny news on the new key?\r\n"
	if _, err := mem.User.Append("Sent", strings.NewReader(sent), &imap.AppendOptions{Flags: []imap.Flag{imap.FlagSeen}}); err != nil {
		t.Fatal(err)
	}
	cfg := mailsync.Config{PollInterval: time.Hour, IdleMax: time.Minute, MinBackoff: 50 * time.Millisecond, Chunk: 50, NoPrefetch: true}
	data := t.TempDir()
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, DataDir: data, Now: func() time.Time { return snapshotNow }})
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
	seedPIM(t, srv, a.ID)
	f := &fixture{srv: srv, acct: a.ID, loop: frostuitest.NewLoop()}
	f.win = mainwin.New(mainwin.Options{Target: conn.Target{Socket: srv.Socket}, Client: "frostmail test",
		Post: f.loop.Post, Wake: time.Millisecond, Retry: 10 * time.Millisecond, CacheDir: data, DownloadDir: t.TempDir(),
		Now:      func() time.Time { return snapshotNow },
		NewFrame: func() (uikit.Frame, error) { return &fakeFrame{}, nil }})
	f.tt = ui.NewTester(func(c *ui.Context) {
		f.loop.Flush()
		f.win.View(c)
	}, 1280, 800)
	f.win.Start(t.Context())
	return f
}

// seedPIM gives the account an address book, two calendars with a week of
// events around snapshotNow, and two task lists.
func seedPIM(t *testing.T, srv *rpctest.Server, account int64) {
	t.Helper()
	ctx := t.Context()
	card := func(uid, name, family, given, email string) string {
		return "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:" + uid + "\r\nFN:" + name + "\r\nN:" + family + ";" + given + ";;;\r\nEMAIL:" + email + "\r\nEND:VCARD\r\n"
	}
	event := func(lines ...string) []byte {
		return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nBEGIN:VEVENT\r\n" + strings.Join(lines, "\r\n") + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
	}
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		for _, s := range []api.ServiceKind{api.ServiceKindContacts, api.ServiceKindCalendar, api.ServiceKindTasks} {
			if err := tx.SetService(ctx, account, s, true, "https://dav.x.test/"); err != nil {
				return err
			}
		}
		books, err := tx.ReplaceCollections(ctx, account, api.CollectionKindAddressbook, []store.RemoteCollection{{Href: "/ab/", Name: "Contacts"}})
		if err != nil {
			return err
		}
		for _, p := range []struct{ uid, name, family, given, email string }{
			{"ada", "Ada Lovelace", "Lovelace", "Ada", "ada@x.test"},
			{"alan", "Alan Turing", "Turing", "Alan", "alan@x.test"},
			{"grace", "Grace Hopper", "Hopper", "Grace", "grace@x.test"},
			{"mara", "Mara Lind", "Lind", "Mara", "mara@x.test"},
			{"tomas", "Tomas Reyes", "Reyes", "Tomas", "tomas@x.test"},
		} {
			id, err := tx.PutObject(ctx, store.Object{CollectionID: books[0].ID, Href: "/ab/" + p.uid + ".vcf", ETag: `"1"`, Kind: store.ObjectVCard,
				Raw: []byte(card(p.uid, p.name, p.family, p.given, p.email))})
			if err != nil {
				return err
			}
			if err := tx.IndexContact(ctx, id, store.ContactIndex{DisplayName: p.name, SortKey: strings.ToLower(p.family + " " + p.given),
				GivenName: p.given, FamilyName: p.family, Emails: []store.ContactEmail{{Email: p.email, Label: "work"}}}); err != nil {
				return err
			}
		}
		if err := tx.RelinkPeople(ctx); err != nil {
			return err
		}
		cals, err := tx.ReplaceCollections(ctx, account, api.CollectionKindCalendar, []store.RemoteCollection{
			{Href: "/work/", Name: "Work", Color: "#3478f6"}, {Href: "/home/", Name: "Home", Color: "#34c759"}})
		if err != nil {
			return err
		}
		from, to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
		for _, o := range []struct {
			col  int64
			href string
			raw  []byte
		}{
			{cals[0].ID, "/work/standup.ics", event("UID:standup", "DTSTART:20261111T090000Z", "DTEND:20261111T091500Z", "SUMMARY:Standup", "LOCATION:Room 4")},
			{cals[0].ID, "/work/review.ics", event("UID:review", "DTSTART:20261111T140000Z", "DTEND:20261111T153000Z", "SUMMARY:Release review")},
			{cals[0].ID, "/work/sync.ics", event("UID:sync", "DTSTART:20261109T110000Z", "DTEND:20261109T113000Z", "RRULE:FREQ=DAILY;COUNT=5", "SUMMARY:Sync")},
			{cals[0].ID, "/work/offsite.ics", event("UID:offsite", "DTSTART;VALUE=DATE:20261113", "DTEND;VALUE=DATE:20261114", "SUMMARY:Offsite")},
			{cals[1].ID, "/home/gym.ics", event("UID:gym", "DTSTART:20261110T170000Z", "DTEND:20261110T180000Z", "SUMMARY:Gym")},
			{cals[1].ID, "/home/dinner.ics", event("UID:dinner", "DTSTART:20261114T180000Z", "DTEND:20261114T200000Z", "SUMMARY:Dinner with Ines")},
		} {
			if _, err := pimsync.StoreCalendarObject(ctx, tx, store.Object{CollectionID: o.col, Href: o.href, ETag: `"1"`, Kind: store.ObjectVEvent, Raw: o.raw},
				calendar.Options{Local: time.UTC, UserEmails: []string{"me@x.test"}}, from, to); err != nil {
				return err
			}
		}
		if err := tx.SetInstanceWindow(ctx, from, to); err != nil {
			return err
		}
		lists, err := tx.ReplaceCollections(ctx, account, api.CollectionKindTasklist, []store.RemoteCollection{{Href: "L1", Name: "Work Tasks"}, {Href: "L2", Name: "Errands"}})
		if err != nil {
			return err
		}
		for _, r := range []struct {
			col int64
			row store.TaskRow
		}{
			{lists[0].ID, store.TaskRow{UID: "t1", Title: "Promote 0.6", Position: "00001", Due: "2026-11-11", Notes: "After the flatpak gate"}},
			{lists[0].ID, store.TaskRow{UID: "t2", ParentUID: "t1", Title: "Check the cask", Position: "00001"}},
			{lists[0].ID, store.TaskRow{UID: "t3", Title: "Rotate the signing key", Position: "00002", Due: "2026-11-13"}},
			{lists[0].ID, store.TaskRow{UID: "t4", Title: "File the expenses", Position: "00003", Due: "2026-11-02", Completed: true}},
			{lists[1].ID, store.TaskRow{UID: "t5", Title: "Buy oat milk", Due: "2026-11-10"}},
		} {
			id, err := tx.PutObject(ctx, store.Object{CollectionID: r.col, Href: r.row.UID, Kind: store.ObjectGTask, UID: r.row.UID,
				Raw: []byte(`{"id":"` + r.row.UID + `","title":"` + r.row.Title + `"}`)})
			if err != nil {
				return err
			}
			if err := tx.IndexTask(ctx, id, r.row); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The mail, calendar, people and tasks views draw in the Frostyard theme
// and in Frostyard light (frostui ADR-0011); with FROSTMAIL_SNAPSHOT_DIR
// set, each is written as <module>-<theme>.png.
func TestModulesInBothThemes(t *testing.T) {
	f := snapshotFixture(t)
	f.until(t, "the inbox", func() bool {
		return f.tt.HasText("Re: Frostmail 0.6 release checklist") && f.tt.HasText("Signing key rotation")
	})
	if err := f.tt.Click("Re: Frostmail 0.6 release checklist"); err != nil {
		t.Fatal(err)
	}
	f.until(t, "the message", func() bool { _, ok := f.tt.Access("Message body"); return ok })
	// The module bar's buttons, not Ctrl+1, which also shows All Inboxes
	// and forgets the message chosen.
	has := func(s string) func() bool { return func() bool { return f.tt.HasText(s) } }
	modules := []struct {
		name, button string
		ready        func() bool
		pick         string
		picked       func() bool
	}{
		{"mail", "Mail", func() bool { _, ok := f.tt.Access("Message body"); return ok }, "", nil},
		{"calendar", "Calendar", has("Release review"), "Release review", func() bool { _, ok := f.tt.AccessByRole(ui.RoleHeading, "Release review"); return ok }},
		{"people", "People", has("Ada Lovelace"), "Mara Lind", has("Recent Mail")},
		{"tasks", "Tasks", has("Promote 0.6"), "Promote 0.6", func() bool { _, ok := f.tt.Access("Notes"); return ok }},
	}
	for _, theme := range []string{"light", "dark"} {
		f.tt.SetDark(theme == "dark")
		for _, m := range modules {
			b, ok := f.tt.AccessByRole(ui.RoleToggleButton, m.button)
			if !ok {
				t.Fatalf("no %s button in %q", m.button, f.tt.Texts())
			}
			f.tt.ClickAt(b.Bounds.X+b.Bounds.W/2, b.Bounds.Y+b.Bounds.H/2)
			f.until(t, m.name+" in "+theme, m.ready)
			if m.pick != "" && theme == "light" {
				if err := f.tt.Click(m.pick); err != nil {
					t.Fatal(err)
				}
				f.until(t, m.pick+" shown", m.picked)
			}
			for range 3 {
				f.tt.Frame()
			}
			snapshot(t, f.tt, m.name+"-"+theme)
		}
	}
}
