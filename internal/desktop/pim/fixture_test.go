package pim

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("FROSTUI_ICON_THEME", "Adwaita")
	frostui.SetIcons(frostui.NewIcons(icons.Embedded()))
	os.Exit(m.Run())
}

func ics(lines ...string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nBEGIN:VEVENT\r\n" +
		strings.Join(lines, "\r\n") + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n")
}

// longAddress is an account's address long enough to cut a pane's name
// (frostyard/frostmail#282).
const longAddress = "ann.lee.calendars@northwind-traders.example"

const (
	adaCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:ada\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\nORG:Engines\r\nTITLE:Countess\r\n" +
		"EMAIL;TYPE=HOME:ada@a.test\r\nTEL;TYPE=CELL:+44 20 7946 0000\r\nBDAY:1815-12-10\r\nNOTE:Notes on engines\r\nEND:VCARD\r\n"
	alanCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:alan\r\nFN:Alan Turing\r\nN:Turing;Alan;;;\r\nEMAIL:alan@a.test\r\nEND:VCARD\r\n"
	gracCard = "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:grace\r\nFN:Grace Hopper\r\nN:Hopper;Grace;;;\r\nEMAIL:grace@a.test\r\nEND:VCARD\r\n"
)

// testNow is the fixture's clock: Thursday, October 8, 2026, 10:30 UTC.
var testNow = time.Date(2026, 10, 8, 10, 30, 0, 0, time.UTC)

// fixture runs the module in a window of its own over a real maild.
type fixture struct {
	t      *testing.T
	srv    *rpctest.Server
	c      *api.Client
	loop   *frostuitest.Loop
	m      *Module
	shell  frostui.PaneShell
	table  *frostui.CommandTable
	tt     *ui.Tester
	module commands.Module
	builds int
	now    time.Time

	windows map[string]*window
	closed  []string
	toasts  []string
	shown   []commands.Module
	compose []string
	opened  []int64

	email                                                   string
	acct, book, work, home, side, tasks, errands, readTasks int64
	ada, report, charts, done, milk                         int64
}

// window is a window the module opened, with a tester of its own.
type window struct {
	spec WindowSpec
	tt   *ui.Tester
}

type options struct {
	rpctest.Options
	empty bool
	// email is the account's address, for a long one's test; "me@a.test"
	// when empty.
	email   string
	onStart func(*rpctest.Server)
}

// newFixture runs the module against a real maild holding the fixture's
// accounts, calendars, contacts and tasks.
func newFixture(t *testing.T, module commands.Module, o ...options) *fixture {
	t.Helper()
	var opt options
	if len(o) > 0 {
		opt = o[0]
	}
	f := &fixture{t: t, loop: frostuitest.NewLoop(), module: module, now: testNow, table: frostui.NewCommandTable(), windows: map[string]*window{}}
	f.email = opt.email
	if f.email == "" {
		f.email = "me@a.test"
	}
	if opt.Now == nil {
		opt.Now = func() time.Time { return testNow }
	}
	f.srv = rpctest.StartWith(t, opt.Options)
	if opt.onStart != nil {
		opt.onStart(f.srv)
	}
	if !opt.empty {
		f.seed()
	}
	f.shell.Dialogs = &frostui.Dialogs{}
	f.table.Editing = func(*ui.Context) bool { return false }
	f.m = New(Options{Post: f.loop.Post, Wake: func() { f.loop.Post(func() {}) }, Now: func() time.Time { return f.now }, Dialogs: f.shell.Dialogs,
		Toast: func(title string, err error) {
			if err != nil {
				title += ": " + err.Error()
			}
			f.toasts = append(f.toasts, title)
		},
		Compose:        func(address, _ string) { f.compose = append(f.compose, address) },
		OpenMessage:    func(id int64) { f.opened = append(f.opened, id) },
		OpenWindow:     f.openWindow,
		CloseWindow:    f.closeWindow,
		SetWindowTitle: func(string, string) {},
		ShowModule:     func(m commands.Module) { f.shown = append(f.shown, m); f.module = m },
	})
	f.tt = ui.NewTester(f.view, 1200, 780)
	c := f.srv.Dial(t)
	f.c = c
	events := live.NewEvents()
	go events.Run(c)
	if _, err := c.Events().Subscribe(t.Context(), &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	if err := f.m.Connect(t.Context(), c, events); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.m.Disconnect)
	return f
}

func (f *fixture) view(c *ui.Context) {
	f.builds++
	f.loop.Flush()
	frostui.Adapt(c)
	f.table.Add(commands.Table(f.module, func() int { return f.m.Selected(f.module) }, f.m.Handlers(f.module))...)
	var panes []frostui.Pane
	if f.module != commands.Mail {
		panes = f.m.Panes(c, f.module)
	} else {
		panes = []frostui.Pane{{ID: "reader", Label: "Mail", Content: func() {}},
			{ID: "todo", Label: "To-Do", Width: 280, Min: 280, Max: 280, Content: func() { f.m.ToDoBar(c) }}}
	}
	f.shell.View(c, frostui.PaneFrame{SidebarPane: paneSidebar, Panes: panes, Table: f.table})
}

func (f *fixture) openWindow(s WindowSpec) {
	if w, ok := f.windows[s.Key]; ok {
		w.spec = s
		return
	}
	w := &window{spec: s}
	f.windows[s.Key] = w
	w.tt = ui.NewTester(func(c *ui.Context) {
		f.loop.Flush()
		if w.spec.View != nil {
			w.spec.View(c)
		}
	}, int(s.Width), int(s.Height))
}

// closeWindow closes a window as the app's does (cmd/frostmail, MyGo's
// Window.Close): CloseRequested can keep it open.
func (f *fixture) closeWindow(key string) {
	w, ok := f.windows[key]
	if !ok {
		return
	}
	if w.spec.CloseRequested != nil && !w.spec.CloseRequested() {
		return
	}
	f.closed = append(f.closed, key)
	delete(f.windows, key)
	if w.spec.OnClose != nil {
		w.spec.OnClose()
	}
}

// window returns the only window with a key that has a prefix.
func (f *fixture) window(prefix string) *window {
	for k, w := range f.windows {
		if strings.HasPrefix(k, prefix) {
			return w
		}
	}
	return nil
}

// until redraws until ok holds.
func (f *fixture) until(what string, ok func() bool) {
	f.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		f.tt.Frame()
		for _, w := range f.windows {
			w.tt.Frame()
		}
		if ok() {
			return
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("timed out waiting for %s; the window shows %q; toasts %q", what, f.tt.Texts(), f.toasts)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// text waits for a text.
func (f *fixture) text(want string) {
	f.t.Helper()
	f.until("text "+want, func() bool { return f.tt.HasText(want) })
}

// click clicks the element showing or named s.
func (f *fixture) click(s string) {
	f.t.Helper()
	f.until("element "+s, func() bool { _, ok := f.tt.Find(s); return ok })
	if err := f.tt.Click(s); err != nil {
		f.t.Fatalf("click %q: %v; shows %q", s, err, f.tt.Texts())
	}
	f.tt.Frame()
}

// clickIn clicks in a window.
func (f *fixture) clickIn(w *window, s string) {
	f.t.Helper()
	if err := w.tt.Click(s); err != nil {
		f.t.Fatalf("click %q: %v; shows %q", s, err, w.tt.Texts())
	}
	w.tt.Frame()
}

func (f *fixture) seed() {
	f.t.Helper()
	ctx := f.t.Context()
	err := f.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		server := store.ServerConfig{Host: "imap.a.test", Port: 993, TLS: api.TLSModeTLS, Username: "u"}
		a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindGmail, Email: f.email, Auth: api.AuthKindOAuth2, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		f.acct = a.ID
		for _, s := range []api.ServiceKind{api.ServiceKindContacts, api.ServiceKindCalendar, api.ServiceKindTasks} {
			if err := tx.SetService(ctx, a.ID, s, true, "https://dav.a.test/"); err != nil {
				return err
			}
		}
		books, err := tx.ReplaceCollections(ctx, a.ID, api.CollectionKindAddressbook, []store.RemoteCollection{{Href: "/ab/", Name: "Contacts"}, {Href: "/ro/", Name: "Card", ReadOnly: true}})
		if err != nil {
			return err
		}
		f.book = books[0].ID
		for _, c := range []struct {
			href, raw string
			idx       store.ContactIndex
		}{
			{"/ab/ada.vcf", adaCard, store.ContactIndex{DisplayName: "Ada Lovelace", SortKey: "lovelace ada", GivenName: "Ada", FamilyName: "Lovelace", Organization: "Engines", Emails: []store.ContactEmail{{Email: "ada@a.test", Label: "home"}}}},
			{"/ab/alan.vcf", alanCard, store.ContactIndex{DisplayName: "Alan Turing", SortKey: "turing alan", GivenName: "Alan", FamilyName: "Turing", Emails: []store.ContactEmail{{Email: "alan@a.test"}}}},
			{"/ab/grace.vcf", gracCard, store.ContactIndex{DisplayName: "Grace Hopper", SortKey: "hopper grace", GivenName: "Grace", FamilyName: "Hopper", Emails: []store.ContactEmail{{Email: "grace@a.test"}}}},
		} {
			id, err := tx.PutObject(ctx, store.Object{CollectionID: f.book, Href: c.href, ETag: `"1"`, Kind: store.ObjectVCard, Raw: []byte(c.raw)})
			if err != nil {
				return err
			}
			if err := tx.IndexContact(ctx, id, c.idx); err != nil {
				return err
			}
		}
		if err := tx.RelinkPeople(ctx); err != nil {
			return err
		}
		cals, err := tx.ReplaceCollections(ctx, a.ID, api.CollectionKindCalendar, []store.RemoteCollection{
			{Href: "/work/", Name: "Work", Color: "#3478f6"}, {Href: "/home/", Name: "Home", ReadOnly: true}, {Href: "/side/", Name: "Side"}})
		if err != nil {
			return err
		}
		f.work, f.home, f.side = cals[0].ID, cals[1].ID, cals[2].ID
		from, to := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
		for _, o := range []struct {
			col  int64
			href string
			raw  []byte
		}{
			{f.work, "/work/standup.ics", ics("UID:standup", "DTSTART:20261008T090000Z", "DTEND:20261008T091500Z", "SUMMARY:Standup", "LOCATION:Room 4")},
			{f.work, "/work/review.ics", ics("UID:review", "DTSTART:20261008T140000Z", "DTEND:20261008T153000Z", "SUMMARY:Review")},
			{f.work, "/work/holiday.ics", ics("UID:holiday", "DTSTART;VALUE=DATE:20261009", "DTEND;VALUE=DATE:20261010", "SUMMARY:Holiday")},
			{f.work, "/work/off.ics", ics("UID:off", "DTSTART:20261009T100000Z", "DTEND:20261009T110000Z", "SUMMARY:Offsite", "STATUS:CANCELLED")},
			{f.work, "/work/daily.ics", ics("UID:daily", "DTSTART:20261005T110000Z", "DTEND:20261005T113000Z", "RRULE:FREQ=DAILY;COUNT=5", "SUMMARY:Sync")},
			{f.home, "/home/gym.ics", ics("UID:gym", "DTSTART:20261008T180000Z", "DTEND:20261008T190000Z", "SUMMARY:Gym")},
			{f.work, "/work/lunch.ics", ics("UID:lunch", "DTSTART:20261012T120000Z", "DTEND:20261012T130000Z", "SUMMARY:Lunch with Ada",
				"ORGANIZER;CN=Bo:mailto:bo@a.test", "ATTENDEE;CN=Ada;PARTSTAT=ACCEPTED:mailto:ada@a.test", "ATTENDEE;CN=Me;PARTSTAT=NEEDS-ACTION:mailto:me@a.test")},
		} {
			if _, err := pimsync.StoreCalendarObject(ctx, tx, store.Object{CollectionID: o.col, Href: o.href, ETag: `"1"`, Kind: store.ObjectVEvent, Raw: o.raw},
				calendar.Options{Local: time.UTC, UserEmails: []string{f.email}}, from, to); err != nil {
				return err
			}
		}
		if err := tx.SetInstanceWindow(ctx, from, to); err != nil {
			return err
		}
		lists, err := tx.ReplaceCollections(ctx, a.ID, api.CollectionKindTasklist, []store.RemoteCollection{{Href: "L1", Name: "Work Tasks"}, {Href: "L2", Name: "Errands"}, {Href: "L3", Name: "Shared", ReadOnly: true}})
		if err != nil {
			return err
		}
		f.tasks, f.errands, f.readTasks = lists[0].ID, lists[1].ID, lists[2].ID
		for _, r := range []struct {
			col int64
			row store.TaskRow
			id  *int64
		}{
			{f.tasks, store.TaskRow{UID: "t1", Title: "Report", Position: "00001", Due: "2026-10-08", Notes: "Q3 numbers"}, &f.report},
			{f.tasks, store.TaskRow{UID: "t2", ParentUID: "t1", Title: "Charts", Position: "00001"}, &f.charts},
			{f.tasks, store.TaskRow{UID: "t3", Title: "Filed", Position: "00002", Due: "2026-10-01", Completed: true}, &f.done},
			{f.errands, store.TaskRow{UID: "t4", Title: "Buy oat milk", Due: "2026-10-07"}, &f.milk},
			{f.readTasks, store.TaskRow{UID: "t5", Title: "Read Only Item"}, new(int64)},
		} {
			id, err := tx.PutObject(ctx, store.Object{CollectionID: r.col, Href: r.row.UID, Kind: store.ObjectGTask, UID: r.row.UID, Raw: []byte(`{"id":"` + r.row.UID + `","title":"` + r.row.Title + `"}`)})
			if err != nil {
				return err
			}
			if err := tx.IndexTask(ctx, id, r.row); err != nil {
				return err
			}
			*r.id = id
		}
		return nil
	})
	if err != nil {
		f.t.Fatal(err)
	}
	people, err := f.srv.DB.People(ctx, "")
	if err != nil || len(people) == 0 {
		f.t.Fatalf("people = %+v, %v", people, err)
	}
	f.ada = people[0].ID
	for _, p := range people {
		if p.DisplayName == "Ada Lovelace" {
			f.ada = p.ID
		}
	}
}

func dump(tt *ui.Tester) string { return fmt.Sprintf("%q", tt.Texts()) }

// textWidth is the width a text takes drawn alone, with no limit, in a
// window of its own (frostyard/frostmail#282).
func textWidth(t *testing.T, text string) float32 {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		frostui.Adapt(c)
		ui.Row(c).Children(func() { frostui.Body(c, text) })
	}, 1200, 100)
	tt.Frame()
	r, ok := tt.Find(text)
	if !ok {
		t.Fatalf("a window of its own shows no %q", text)
	}
	return r.W
}

// detailText is the box of text drawn in the pane named pane, as
// assistive technology sees it: Find gives the first element showing a
// text, and the sidebar shows a name as the pane does
// (frostyard/frostmail#282).
func (f *fixture) detailText(pane, text string) (ui.Rect, bool) {
	p, ok := f.tt.AccessByRole(ui.RoleGroup, pane)
	if !ok {
		return ui.Rect{}, false
	}
	for _, a := range f.tt.AccessAll() {
		if a.Role == ui.RoleText && a.Name == text && a.Bounds.X >= p.Bounds.X && a.Bounds.X < p.Bounds.X+p.Bounds.W {
			return a.Bounds, true
		}
	}
	return ui.Rect{}, false
}
