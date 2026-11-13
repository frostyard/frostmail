package mainwin

import (
	"bufio"
	"encoding/json/v2"
	"fmt"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostui"
	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/organize"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// syncMaild is a maild syncing an in-memory IMAP server holding the
// subjects, and a client of it.
func syncMaild(t *testing.T, subjects ...string) (*rpctest.Server, *api.Client) {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	for _, subject := range subjects {
		raw := fmt.Sprintf("From: Ann <ann@x.test>\r\nTo: me@x.test\r\nSubject: %s\r\nMessage-ID: <%s@x.test>\r\nDate: Mon, 09 Nov 2026 09:41:00 +0000\r\n\r\nBody\r\n",
			subject, strings.ReplaceAll(subject, " ", "."))
		if _, err := mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
	}
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
	return srv, c
}

func poll(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// heldEvents reads c's events and keeps them until the test delivers them,
// so a view on them shows the list as it was.
type heldEvents struct {
	*live.Events
	mu   sync.Mutex
	held []api.Event
}

func holdEvents(c *api.Client) *heldEvents {
	h := &heldEvents{Events: live.NewEvents()}
	go func() {
		for env := range c.Notifications() {
			if ev, err := api.DecodeEvent(env.Event, env.Data); err == nil {
				h.mu.Lock()
				h.held = append(h.held, ev)
				h.mu.Unlock()
			}
		}
	}()
	return h
}

func (h *heldEvents) deliver() {
	h.mu.Lock()
	evs := h.held
	h.held = nil
	h.mu.Unlock()
	for _, ev := range evs {
		h.Deliver(ev)
	}
}

// replayed reports whether message id's move has reached the server: until
// then maild skips a delete of it.
func replayed(t *testing.T, srv *rpctest.Server, id int64) bool {
	t.Helper()
	ctx := t.Context()
	landed := false
	if err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mems, err := tx.Memberships(ctx, []int64{id})
		if err != nil {
			return err
		}
		landed = len(mems) > 0 && !slices.ContainsFunc(mems, func(m store.Membership) bool { return m.UID == 0 })
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return landed
}

func inTrash(t *testing.T, c *api.Client, id int64) bool {
	t.Helper()
	ctx := t.Context()
	got, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{id}})
	if err != nil {
		t.Fatal(err)
	}
	boxes, err := c.Mailbox().List(ctx, &api.MailboxListParams{})
	if err != nil {
		t.Fatal(err)
	}
	return len(got) == 1 && slices.ContainsFunc(boxes, func(m api.Mailbox) bool {
		return m.Role == api.MailboxRoleTrash && slices.Contains(got[0].MailboxIDs, m.ID)
	})
}

// A key repeat after the first delete has reached the server, with the
// list not yet told, however long it waits: the message is in Trash, and a
// second delete would erase it for good.
func TestASecondDeleteWaitsForTheList(t *testing.T) {
	srv, c := syncMaild(t, "Alpha report")
	ctx := t.Context()
	inbox := api.ViewQuery{Role: ptr(api.MailboxRoleInbox)}
	poll(t, "the synced mail", func() bool {
		info, err := c.View().Open(ctx, &api.ViewOpenParams{Query: inbox})
		return err == nil && info.Count == 1
	})
	events := holdEvents(c)
	loop := frostuitest.NewLoop()
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	w := New(Options{Target: conn.Target{Socket: srv.Socket}, Post: loop.Post, Wake: time.Millisecond, Now: func() time.Time { return now }})
	view := live.OpenView(c, events.Events, inbox, func() {})
	defer view.Close()
	poll(t, "the view", view.Ready)
	view.Ensure(0, 10)
	var row api.MessageSummary
	poll(t, "the row", func() bool {
		var ok bool
		row, ok = view.Row(0)
		return ok
	})
	w.client, w.view, w.pane = c, view, paneList
	w.chosen.Add(row.ID)
	mailDeleted := func() bool {
		loop.Flush()
		h := w.deleting[row.ID]
		return h != nil && h.done
	}
	w.deleteKey()
	poll(t, "the first delete", mailDeleted)
	poll(t, "the move's replay", func() bool { return inTrash(t, c, row.ID) && replayed(t, srv, row.ID) })

	now = now.Add(time.Hour) // the list is no wiser for the wait
	w.releaseDeletes()
	first := w.deleting[row.ID]
	w.deleteKey()
	if h := w.deleting[row.ID]; h == nil || h != first {
		t.Fatal("a second Delete went out for the row the list still shows")
	}
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if !inTrash(t, c, row.ID) {
			t.Fatal("the second Delete erased the message from Trash")
		}
		time.Sleep(5 * time.Millisecond)
	}

	poll(t, "the list's update", func() bool {
		events.deliver()
		return view.Count() == 0
	})
	w.repairSelection()
	if len(w.deleting) != 0 {
		t.Errorf("still held once the list dropped the row: %v", w.deleting)
	}
	if ids := w.selectedIDs(); len(ids) != 0 {
		t.Errorf("selected %v with the row gone", ids)
	}
}

// A delete never sent lets go at once, and so does one that left the
// message where its row shows it (maild skips a message still moving):
// another Delete acts on what the list shows. One that moved it holds
// until the list shows that, and one whose end no read has shown holds
// unread, whatever the list shows.
func TestAHeldDeleteLetsGo(t *testing.T) {
	w := New(Options{Target: conn.Target{Socket: "none"}, Post: func(fn func()) { fn() }})
	inbox := []int64{1}
	for id := range int64(6) {
		w.deleting[id+1] = &heldDelete{from: inbox}
	}
	w.deleteEnded([]int64{1}, nil, false, false, nil)
	if w.deleting[1] != nil {
		t.Error("a delete never sent stays held")
	}
	w.deleteEnded([]int64{2}, nil, true, true, []api.MessageSummary{{ID: 2, MailboxIDs: inbox}})
	if w.deleting[2] != nil {
		t.Error("a delete that left the message where the list shows it stays held")
	}
	w.deleteEnded([]int64{3}, nil, true, true, []api.MessageSummary{{ID: 3, MailboxIDs: []int64{9}}})
	if h := w.deleting[3]; h == nil || !h.done {
		t.Fatalf("a finished delete: %+v, want held and done", h)
	}
	w.deleteEnded([]int64{4}, nil, true, true, nil)                                               // answered, not read back
	w.deleteEnded([]int64{5}, nil, true, false, []api.MessageSummary{{ID: 5, MailboxIDs: inbox}}) // not answered
	for _, id := range []int64{4, 5} {
		if h := w.deleting[id]; h == nil || h.done || !h.unread {
			t.Fatalf("delete %d: %+v, want held and unread", id, h)
		}
	}
	w.releaseDeletes() // no view: nothing lists them
	if w.deleting[3] != nil {
		t.Error("a finished delete of a row no list has stays held")
	}
	for _, id := range []int64{4, 5, 6} {
		if w.deleting[id] == nil {
			t.Errorf("delete %d, still running in maild or here, let go", id)
		}
	}
}

// fakeView opens a view on s that lists row, as maild's list would.
func fakeView(t *testing.T, c *api.Client, events *live.Events) *live.View {
	t.Helper()
	view := live.OpenView(c, events, api.ViewQuery{}, func() {})
	t.Cleanup(view.Close)
	poll(t, "the view", view.Ready)
	view.Ensure(0, 10)
	poll(t, "the row", func() bool { _, ok := view.Row(0); return ok })
	return view
}

// heldDeleteOn is a window whose list shows row from s, with row selected
// and its Delete sent; maild answers the delete once release closes.
func heldDeleteOn(t *testing.T, s *fakeMaild, row api.MessageSummary, release chan struct{}) (*Window, *api.Client, *frostuitest.Loop) {
	t.Helper()
	s.result = map[string]any{"view.open": api.ViewInfo{ID: 1, Count: 1},
		"view.range": []api.MessageSummary{row}, "message.summaries": []api.MessageSummary{row}}
	s.hold = map[string]chan struct{}{"message.delete": release}
	loop := frostuitest.NewLoop()
	w := New(Options{Target: conn.Target{Socket: "none"}, Post: loop.Post, Wake: time.Millisecond})
	w.readBackEvery = time.Millisecond
	c := s.client(t)
	w.client, w.view, w.pane = c, fakeView(t, c, live.NewEvents()), paneList
	w.chosen.Add(row.ID)
	w.deleteKey()
	poll(t, "the delete", func() bool { return s.count("message.delete") == 1 })
	return w, c, loop
}

// moveTo has s answer message.summaries and view.range with row in boxes.
func (s *fakeMaild) moveTo(row api.MessageSummary, boxes ...int64) {
	row.MailboxIDs = boxes
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result["message.summaries"] = []api.MessageSummary{row}
	s.result["view.range"] = []api.MessageSummary{row}
}

// A delete maild answered, whose read after failed: it may have moved the
// message to Trash. It holds, the read is tried again until one gets
// through, and a second Delete meanwhile sends nothing. It lets go once
// the list shows the row moved.
func TestADeleteNotReadBackStaysHeld(t *testing.T) {
	row := api.MessageSummary{ID: 7, AccountID: 1, MailboxIDs: []int64{1}, Subject: "Plans"}
	release := make(chan struct{})
	s := &fakeMaild{fail: map[string][]int{"message.summaries": {2, 3}}} // the read after, and the first read back
	w, _, loop := heldDeleteOn(t, s, row, release)
	s.moveTo(row, 2)
	close(release)
	loop.RunUntil(t, func() bool { return len(w.toasts) > 0 })
	if got := w.toasts[0].Title; got != "Undo cannot take this back" {
		t.Errorf("toast %q", got)
	}
	held := w.deleting[row.ID]
	if held == nil {
		t.Fatal("a delete done but not read back let go")
	}
	w.repairSelection()
	w.deleteKey()
	if w.deleting[row.ID] != held {
		t.Fatal("a second Delete went out for the row the list still shows")
	}
	poll(t, "the move read back", func() bool {
		loop.Flush()
		return held.done || w.deleting[row.ID] != held
	})
	if n := s.count("message.summaries"); n != 4 {
		t.Errorf("%d reads, want the one before, the one after and 2 reads back", n)
	}

	w.repairSelection()
	w.deleteKey()
	if w.deleting[row.ID] != held {
		t.Fatal("a second Delete went out for the row the list still shows")
	}
	w.view = fakeView(t, w.client, live.NewEvents())
	poll(t, "the row moved", func() bool {
		rows := w.view.Rows([]int64{row.ID})
		return len(rows) == 1 && !slices.Equal(rows[0].MailboxIDs, row.MailboxIDs)
	})
	w.repairSelection()
	if len(w.deleting) != 0 {
		t.Errorf("still held with the row shown moved: %v", w.deleting)
	}
}

// maild takes its time over a delete, past any deadline: only its answer
// settles the hold. A view that loads meanwhile, and keeps the row as it
// was after the move, is no proof of it, and Delete pressed before the
// answer or after it never reaches maild again.
func TestASlowDeleteSettlesFromItsAnswer(t *testing.T) {
	row := api.MessageSummary{ID: 7, AccountID: 1, MailboxIDs: []int64{1}, Subject: "Plans"}
	release := make(chan struct{})
	s := &fakeMaild{}
	w, c, loop := heldDeleteOn(t, s, row, release)
	held := w.deleting[row.ID]
	w.view = fakeView(t, c, live.NewEvents()) // loads while maild still deletes
	// However long maild takes, the hold stays, and so does what it is.
	for start := time.Now(); time.Since(start) < 300*time.Millisecond; {
		loop.Flush()
		w.repairSelection()
		w.deleteKey()
		if w.deleting[row.ID] != held || held.done || held.unread {
			t.Fatalf("while maild deletes: %+v, want the hold sent", w.deleting[row.ID])
		}
		time.Sleep(5 * time.Millisecond)
	}

	s.moveTo(row, 2)
	close(release) // maild answers: the message is in Trash
	poll(t, "the answer", func() bool {
		loop.Flush()
		return held.done || held.unread || w.deleting[row.ID] != held
	})
	if !held.done || held.unread {
		t.Fatalf("after the answer: %+v, want done from it", held)
	}
	w.repairSelection()
	w.deleteKey()
	if w.deleting[row.ID] != held {
		t.Fatal("a second Delete went out for the row the view still shows as before the move")
	}
	if n := s.count("message.delete"); n != 1 {
		t.Errorf("%d deletes: the second would erase the message from Trash", n)
	}
}

// The connection ends before maild answers a delete. A read on the next
// connection settles a move, but an unchanged row must stay held: the old
// transaction can still commit after that read.
func TestADeleteCutOffSettlesOnTheNextConnection(t *testing.T) {
	row := api.MessageSummary{ID: 7, AccountID: 1, MailboxIDs: []int64{1}, Subject: "Plans"}
	for _, tc := range []struct {
		name  string
		boxes []int64
	}{{"moved", []int64{2}}, {"not moved", row.MailboxIDs}} {
		t.Run(tc.name, func(t *testing.T) {
			release := make(chan struct{})
			t.Cleanup(func() { close(release) })
			s := &fakeMaild{}
			w, c, loop := heldDeleteOn(t, s, row, release)
			held := w.deleting[row.ID]
			_ = c.Close()
			poll(t, "the delete to end unanswered", func() bool {
				loop.Flush()
				return held.unread
			})
			w.repairSelection()
			w.deleteKey()
			if w.deleting[row.ID] != held || held.done {
				t.Fatalf("before the next connection: %+v, want the same hold, unread", w.deleting[row.ID])
			}
			if n := s.count("message.summaries"); n != 1 {
				t.Errorf("%d reads on the connection that ended", n)
			}

			next := &fakeMaild{result: map[string]any{"view.open": api.ViewInfo{ID: 1, Count: 1}}}
			next.moveTo(row, tc.boxes...)
			w.takeClient(next.client(t), live.NewEvents())
			poll(t, "the read on the next connection", func() bool {
				loop.Flush()
				return !held.unread || next.count("message.summaries") >= 2
			})
			switch h := w.deleting[row.ID]; tc.name {
			case "moved":
				if h != held || !held.done {
					t.Errorf("a delete maild carried out: %+v, want done", h)
				}
			default:
				if h != held || !h.unread || h.done {
					t.Errorf("an unanswered delete with an unchanged read: %+v, want held and unread", h)
				}
			}
		})
	}
}

// An edit and a click that come before one frame: Redirect sends what the
// field shows once it has taken the edit, not what it held before.
func TestRedirectSendsTheEditOfItsFrame(t *testing.T) {
	s := &fakeMaild{result: map[string]any{"message.redirect": api.OutboxItem{ID: 1}}}
	loop := frostuitest.NewLoop()
	w := New(Options{Target: conn.Target{Socket: "none"}, Post: loop.Post, Wake: time.Millisecond})
	w.client = s.client(t)
	w.redirect = &redirectSheet{summary: api.MessageSummary{ID: 7, Subject: "Plans"}, to: "a@x.test"}
	var tt *ui.Tester
	var input func() // input for the next frame, all of it before that frame
	tt = ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		if fn := input; fn != nil {
			input = nil
			fn() // the Tester builds no frame while one builds: this one takes it all
		}
		w.redirectView(c)
	}, 800, 600)
	tt.Frame()
	info, ok := tt.AccessByRole(ui.RoleButton, "Redirect")
	if !ok || info.Disabled {
		t.Fatalf("Redirect with an address: %+v, %v", info, ok)
	}
	tt.Command("selectAll")
	input = func() {
		tt.Type("b@x.test")
		tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
	}
	tt.Frame()
	draw(t, tt, "the sheet to close", func() bool { return w.redirect == nil })
	req, ok := s.call("message.redirect")
	if !ok {
		t.Fatal("no message.redirect")
	}
	var p api.MessageRedirectParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.To) != 1 || p.To[0].Address != "b@x.test" {
		t.Errorf("redirected to %+v, want b@x.test", p.To)
	}
}

// paletteOn draws the window's palette alone, over w's own commands none,
// and runs after after the palette in each frame.
func paletteOn(w *Window, loop *frostuitest.Loop, after *func()) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		w.shell.Palette.View(c, nil)
		if fn := *after; fn != nil {
			*after = nil
			fn()
		}
	}, 900, 700)
}

// The last maild's answer to the palette comes after the window moved to
// another maild: its messages never show there.
func TestPaletteDropsAnAnswerOfTheLastMaild(t *testing.T) {
	old := api.MessageSummary{ID: 42, Subject: "Old plans", From: api.Address{Address: "ann@x.test"}}
	release := make(chan struct{})
	a := &fakeMaild{result: map[string]any{"view.open": api.ViewInfo{ID: 1, Count: 1}, "view.range": []api.MessageSummary{old}},
		hold: map[string]chan struct{}{"view.range": release}}
	b := &fakeMaild{}
	loop := frostuitest.NewLoop()
	var posted sync.Mutex
	posts := 0
	w := New(Options{Target: conn.Target{Socket: "none"}, Wake: time.Millisecond, Post: func(fn func()) {
		posted.Lock()
		posts++
		posted.Unlock()
		loop.Post(fn)
	}})
	w.client = a.client(t)
	var after func()
	tt := paletteOn(w, loop, &after)
	w.shell.Palette.Show()
	w.shell.Palette.Query = "plans"
	tt.Frame()
	poll(t, "the search", func() bool { _, ok := a.call("view.range"); return ok })

	w.leaveDaemon()
	w.client = b.client(t)
	posted.Lock()
	before := posts
	posted.Unlock()
	close(release)
	poll(t, "the last maild's answer", func() bool {
		posted.Lock()
		defer posted.Unlock()
		return posts > before
	})
	tt.Frame()
	if tt.HasText("Old plans") {
		t.Errorf("the palette shows the last maild's message: %q", tt.Texts())
	}
	if w.shell.Palette.Open {
		t.Error("the palette stayed open on the last maild's query")
	}
}

// An item chosen just before the window moved to another maild runs after
// the move: it names the last one's message or mailbox, and does nothing.
func TestPaletteChoiceStaysWithItsMaild(t *testing.T) {
	old := api.MessageSummary{ID: 42, Subject: "Old plans", From: api.Address{Address: "ann@x.test"}}
	for _, tc := range []struct {
		name, shown string
		tree        map[string]mail.Item
	}{
		{name: "message", shown: "Old plans"},
		{name: "mailbox", shown: "Plans folder", tree: map[string]mail.Item{"mailbox:5": {Key: "mailbox:5", Label: "Plans folder", Selectable: true, MailboxID: 5}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &fakeMaild{result: map[string]any{"view.open": api.ViewInfo{ID: 1, Count: 1}, "view.range": []api.MessageSummary{old}}}
			b := &fakeMaild{}
			loop := frostuitest.NewLoop()
			w := New(Options{Target: conn.Target{Socket: "none"}, Post: loop.Post, Wake: time.Millisecond})
			w.client, w.tree = a.client(t), tc.tree
			var next *api.Client
			if tc.tree == nil {
				next = b.client(t)
			}
			var after func()
			tt := paletteOn(w, loop, &after)
			w.shell.Palette.Show()
			w.shell.Palette.Query = "plans"
			draw(t, tt, "the results", func() bool { return tt.HasText(tc.shown) && tt.HasText("Old plans") })

			after = func() {
				if w.shell.Palette.Open {
					t.Error("Enter did not choose the item")
				}
				w.leaveDaemon() // between the choice and its run
				w.client = next
			}
			tt.Key(0, ui.KeyEnter)
			tt.Frame()
			if w.pendingOpen != 0 {
				t.Errorf("the item went on to open message %d on the new maild", w.pendingOpen)
			}
			if n := b.count("message.summaries"); n != 0 {
				t.Errorf("the new maild was asked for the last one's message %d times", n)
			}
			if w.q.Source.Kind != mail.AllInboxes {
				t.Errorf("the item went on to show %+v on the new maild", w.q.Source)
			}
		})
	}
}

// Delete acts on what the list has a row for: a selected message it has no
// row for may have gone anywhere since, Trash included.
func TestDeleteLeavesAMessageTheListDoesNotHold(t *testing.T) {
	s := &fakeMaild{}
	w := New(Options{Target: conn.Target{Socket: "none"}, Post: func(fn func()) { fn() }})
	w.client = s.client(t)
	w.delete([]int64{99})
	if len(w.deleting) != 0 {
		t.Errorf("held %v", w.deleting)
	}
}

// fakeMaild records requests, and answers each with result[method] or {}.
type fakeMaild struct {
	mu     sync.Mutex
	result map[string]any
	fail   map[string][]int         // the calls of a method, counted from 1, answered with an error
	hold   map[string]chan struct{} // a method's answers wait for its channel to close
	calls  []api.Frame
}

func (s *fakeMaild) client(t *testing.T) *api.Client {
	t.Helper()
	near, far := net.Pipe()
	c := api.NewClient(near)
	t.Cleanup(func() {
		_ = c.Close()
		_ = far.Close()
	})
	var write sync.Mutex
	go func() {
		lines := bufio.NewScanner(far)
		for lines.Scan() {
			var req api.Frame
			if err := json.Unmarshal(lines.Bytes(), &req); err != nil {
				return
			}
			go s.answer(far, &write, req)
		}
	}()
	return c
}

func (s *fakeMaild) answer(c net.Conn, write *sync.Mutex, req api.Frame) {
	s.mu.Lock()
	s.calls = append(s.calls, req)
	n := 0
	for _, f := range s.calls {
		if f.Method == req.Method {
			n++
		}
	}
	result, ok := s.result[req.Method]
	failed, held := slices.Contains(s.fail[req.Method], n), s.hold[req.Method]
	s.mu.Unlock()
	if held != nil {
		<-held
	}
	if !ok {
		result = struct{}{}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return
	}
	frame := api.Frame{JSONRPC: "2.0", ID: req.ID, Result: data}
	if failed {
		frame.Result, frame.Error = nil, api.Errorf(api.CodeInternal, "failed")
	}
	line, err := json.Marshal(frame)
	if err != nil {
		return
	}
	write.Lock()
	defer write.Unlock()
	_, _ = c.Write(append(line, '\n'))
}

func (s *fakeMaild) call(method string) (api.Frame, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.calls, func(f api.Frame) bool { return f.Method == method })
	if i < 0 {
		return api.Frame{}, false
	}
	return s.calls[i], true
}

// count is how many calls of method came.
func (s *fakeMaild) count(method string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, f := range s.calls {
		if f.Method == method {
			n++
		}
	}
	return n
}

// windowOn runs a window on target, drawing in a Tester.
func windowOn(t *testing.T, target conn.Target) (*Window, *ui.Tester) {
	t.Helper()
	loop := frostuitest.NewLoop()
	w := New(Options{Target: target, Client: "frostmail test", Post: loop.Post, Wake: time.Millisecond, Retry: 10 * time.Millisecond})
	tt := ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		w.View(c)
	}, 1200, 780)
	w.Start(t.Context())
	return w, tt
}

func draw(t *testing.T, tt *ui.Tester, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		tt.Frame()
		if ok() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s; the window shows %q", what, tt.Texts())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// A move between local and lodge: every ID the window holds names other
// mail on the new maild, so the selection, the source, what waits to open,
// the undo stack and the sheets all go.
func TestAChangeOfMaildLeavesItsState(t *testing.T) {
	a, _ := syncMaild(t, "Alpha one", "Alpha two")
	b, _ := syncMaild(t, "Bravo one")
	w, tt := windowOn(t, conn.Target{Socket: a.Socket})
	draw(t, tt, "the first maild's mail", func() bool { return tt.HasText("Alpha one") && tt.HasText("Alpha two") })

	// An undo to take back, on the first maild.
	first, ok := w.view.Row(0)
	if !ok {
		t.Fatal("no first row")
	}
	w.setFlags([]int64{first.ID}, api.FlagChanges{FlagColor: ptr(int64(1))})
	draw(t, tt, "the undo entry", func() bool { return w.undo.Len() == 1 })
	undo := w.undo

	inbox := first.MailboxIDs[0]
	w.setSource(mail.Source{Kind: mail.Mailbox, ID: inbox})
	w.chosen.Add(first.ID)
	w.revealing, w.pendingOpen = &first, first.ID
	w.deleting[first.ID] = &heldDelete{}
	w.redirect = &redirectSheet{summary: first}
	w.rawSource = &rawSourceSheet{summary: first, source: &api.MessageSource{}}
	w.later.Show("Remind Me", time.Now().Add(time.Hour), func(time.Time) { t.Error("the time sheet ran on the new maild") })
	w.smartSheet.New()
	w.mailboxSheet.Show(organize.MailboxNew, first.AccountID, nil, &inbox, nil)
	w.shell.Dialogs.Ask(frostui.Confirm{Title: "Erase Deleted Items?", Action: "Erase", OnConfirm: func() { t.Error("the question ran on the new maild") }})
	smart, boxes := w.smartSheet, w.mailboxSheet
	late := w.bound(func() { t.Error("a late answer reached the new maild") })

	w.SetTarget(conn.Target{Socket: b.Socket})
	draw(t, tt, "the second maild's mail", func() bool { return tt.HasText("Bravo one") && !tt.HasText("Alpha one") })

	if w.undo == undo || w.undo.Len() != 0 {
		t.Errorf("the undo stack came along: %d entries", w.undo.Len())
	}
	if ids := w.selectedIDs(); len(ids) != 0 {
		t.Errorf("selected %v", ids)
	}
	if w.revealing != nil || w.pendingOpen != 0 {
		t.Errorf("revealing %v, pending open %d", w.revealing, w.pendingOpen)
	}
	if w.q.Source.Kind != mail.AllInboxes {
		t.Errorf("source = %+v", w.q.Source)
	}
	if len(w.deleting) != 0 {
		t.Errorf("held deletes: %v", w.deleting)
	}
	if w.redirect != nil || w.rawSource != nil {
		t.Errorf("sheets open: redirect %v, raw source %v", w.redirect, w.rawSource)
	}
	if w.smartSheet == smart || w.mailboxSheet == boxes {
		t.Error("an organize sheet came along")
	}
	if w.shell.Dialogs.Showing() {
		t.Error("the question stayed up")
	}
	for _, text := range []string{"Erase Deleted Items?", "New Smart Mailbox", "New Mailbox", "Remind Me"} {
		if tt.HasText(text) {
			t.Errorf("%q still shows in %q", text, tt.Texts())
		}
	}
	late()
}

func ptr[T any](v T) *T { return &v }

// The store goes at the change of maild, before the new one's answers: a
// load that fails there must not leave the last one's standing. A reconnect
// to the same maild keeps it all.
func TestFollowForgetsAnotherMailds(t *testing.T) {
	srv := rpctest.Start(t)
	ctx := t.Context()
	c := srv.Dial(t)
	if _, err := c.Vip().Add(ctx, &api.VipAddParams{Addresses: []string{"ann@x.test"}}); err != nil {
		t.Fatal(err)
	}
	loop := frostuitest.NewLoop()
	target := conn.Target{Socket: srv.Socket}
	w := New(Options{Target: target, Post: loop.Post, Wake: time.Millisecond})
	events := live.NewEvents()
	go events.Run(c)
	if err := w.mail.Connect(ctx, c, events); err != nil {
		t.Fatal(err)
	}
	poll(t, "the VIPs", func() bool { return len(w.mail.State().VIPs) == 1 })

	w.follow(target)
	loop.Flush()
	if len(w.mail.State().VIPs) != 1 || w.gen != 0 {
		t.Errorf("a reconnect to the same maild reset: vips %v, gen %d", w.mail.State().VIPs, w.gen)
	}
	w.mail.Disconnect()
	w.follow(conn.Target{Socket: srv.Socket + ".other"})
	if st := w.mail.State(); st.VIPs != nil || st.Accounts != nil || st.Loaded {
		t.Errorf("the store kept the last maild's: %+v", st)
	}
	loop.Flush()
	if w.gen != 1 {
		t.Errorf("gen = %d", w.gen)
	}
}

// Select All over rows not loaded: their stand-in keys are nothing an
// action can use, and a row that loads later is not selected.
func TestSelectAllChoosesTheRowsLoaded(t *testing.T) {
	srv := rpctest.Start(t)
	ctx := t.Context()
	base := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	if err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		acct, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "me@a.test", Auth: api.AuthKindPassword,
			IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"},
			SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"}})
		if err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, acct.ID, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		var hs []store.MessageHeader
		for uid := 1; uid <= 500; uid++ {
			d := base.Add(time.Duration(uid) * time.Minute)
			hs = append(hs, store.MessageHeader{UID: uint32(uid), Subject: fmt.Sprintf("Message %d", uid), MessageID: fmt.Sprintf("%d@x", uid),
				From: store.Address{Name: "Ann", Addr: "ann@x.test"}, Date: d, InternalDate: d})
		}
		_, err = tx.InsertHeaders(ctx, acct.ID, mbs[0].ID, hs)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	w, tt := windowOn(t, conn.Target{Socket: srv.Socket})
	draw(t, tt, "the newest row", func() bool { return tt.HasText("Message 500") })
	if err := tt.Click("Message 500"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyA)
	draw(t, tt, "the selection", func() bool { return w.chosen.Len() > 1 })
	tt.Frame()
	for id := range w.chosen.All() {
		if id <= 0 {
			t.Fatalf("the selection holds the stand-in %d", id)
		}
	}
	if n := w.chosen.Len(); n >= 500 {
		t.Errorf("selected %d with rows not loaded", n)
	}
	if got, want := len(w.rows(w.selectedIDs())), w.chosen.Len(); got != want {
		t.Errorf("%d of the selection's %d have rows", got, want)
	}
}

// What is typed counts, with no comma or Enter after it.
func TestRedirectSendsWhatIsTyped(t *testing.T) {
	s := &fakeMaild{result: map[string]any{"message.redirect": api.OutboxItem{ID: 1}}}
	loop := frostuitest.NewLoop()
	w := New(Options{Target: conn.Target{Socket: "none"}, Post: loop.Post, Wake: time.Millisecond})
	w.client = s.client(t)
	w.redirect = &redirectSheet{summary: api.MessageSummary{ID: 7, Subject: "Plans"}}
	tt := ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		w.redirectView(c)
	}, 800, 600)
	tt.Frame()
	tt.Type("bo@x.test")
	tt.Frame()
	info, ok := tt.AccessByRole(ui.RoleButton, "Redirect")
	if !ok || info.Disabled {
		t.Fatalf("Redirect with an address typed: %+v, %v", info, ok)
	}
	tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
	draw(t, tt, "the sheet to close", func() bool { return w.redirect == nil })
	req, ok := s.call("message.redirect")
	if !ok {
		t.Fatal("no message.redirect")
	}
	var p api.MessageRedirectParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		t.Fatal(err)
	}
	if p.ID != 7 || len(p.To) != 1 || p.To[0].Address != "bo@x.test" {
		t.Errorf("redirect = %+v", p)
	}
}

func outboxTester(w *Window, st live.MailState, loop *frostuitest.Loop, now time.Time, height float32) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		ui.Column(c).Height(height).AlignItems(ui.Stretch).Children(func() {
			ui.Box(c).Grow(1)
			w.footer(c, st, height)
			ui.Text(c, "After the footer")
		})
		w.later.View(c, now)
	}, 320, int(height)+40)
}

// P-section Send Later: a scheduled message that is on its way or failed
// belongs to the Outbox, a section of the sidebar's list; those waiting
// come under the tree, soonest first, with their actions.
func TestSendLaterSection(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.Local)
	soon, later := now.Add(time.Hour), now.Add(3*time.Hour)
	failure := "refused"
	st := live.MailState{Outbox: []api.OutboxItem{
		{ID: 1, Subject: "Later one", State: api.OutboxStateQueued, Scheduled: true, SendAt: &later, To: []api.Address{{Address: "a@x.test"}}},
		{ID: 2, Subject: "Soon one", State: api.OutboxStateQueued, Scheduled: true, SendAt: &soon,
			To: []api.Address{{Name: "Ann", Address: "a@x.test"}, {Name: "Bo", Address: "b@x.test"}, {Name: "Cy", Address: "c@x.test"}}},
		{ID: 3, Subject: "Refused one", State: api.OutboxStateFailed, Scheduled: true, SendAt: &soon, Error: &failure},
		{ID: 4, Subject: "Going one", State: api.OutboxStateSending, Scheduled: true, SendAt: &soon},
	}}
	s := &fakeMaild{result: map[string]any{"outbox.cancel": api.Draft{}, "outbox.reschedule": api.OutboxItem{ID: 2}}}
	loop := frostuitest.NewLoop()
	w := New(Options{Target: conn.Target{Socket: "none"}, Post: loop.Post, Wake: time.Millisecond, Now: func() time.Time { return now }})
	w.client = s.client(t)
	tt := outboxTester(w, st, loop, now, 900)
	tt.Frame()
	tt.Frame()
	texts := tt.Texts()
	sendLater := slices.Index(texts, "Send Later")
	if sendLater < 0 {
		t.Fatalf("no Send Later in %q", texts)
	}
	for _, subject := range []string{"Refused one", "Going one"} {
		if slices.Contains(texts, subject) {
			t.Errorf("%q, in the Outbox, shows in Send Later: %q", subject, texts)
		}
	}
	var outbox []string
	for _, sec := range w.waitingSections(st) {
		if sec.ID == outboxSection {
			for _, key := range sec.Items {
				outbox = append(outbox, w.waits[key].row.Title)
			}
		}
	}
	if !slices.Equal(outbox, []string{"Refused one", "Going one"}) {
		t.Errorf("the Outbox holds %q", outbox)
	}
	if a, b := slices.Index(texts, "Soon one"), slices.Index(texts, "Later one"); a < sendLater || b < a {
		t.Errorf("soonest first: %q", texts)
	}
	for _, want := range []string{"To: Ann, Bo & 1 more", "Sends " + mail.WhenText(soon, now)} {
		if !slices.Contains(texts, want) {
			t.Errorf("no %q in %q", want, texts)
		}
	}
	click := func(name string) {
		t.Helper()
		if err := tt.Click(name); err != nil { // the first is the soonest message's
			t.Fatalf("%v: %q", err, tt.Texts())
		}
		tt.Frame()
	}
	click("Send Now")
	draw(t, tt, "the reschedule", func() bool { _, ok := s.call("outbox.reschedule"); return ok })
	req, _ := s.call("outbox.reschedule")
	var p api.OutboxRescheduleParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		t.Fatal(err)
	}
	if p.ID != 2 || !p.SendAt.Equal(now) {
		t.Errorf("reschedule = %+v", p)
	}
	click("Edit")
	draw(t, tt, "the cancel", func() bool { _, ok := s.call("outbox.cancel"); return ok })
	click("Change Time…")
	draw(t, tt, "the time sheet", func() bool { return tt.HasText("OK") })
}

// However many messages wait, the sections under the tree leave it room.
func TestSidebarFooterScrolls(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.Local)
	at := now.Add(time.Hour)
	var st live.MailState
	for i := range 40 {
		st.Outbox = append(st.Outbox, api.OutboxItem{ID: int64(i + 1), Subject: fmt.Sprintf("Waiting %d", i), State: api.OutboxStateQueued, Scheduled: true, SendAt: &at})
	}
	loop := frostuitest.NewLoop()
	w := New(Options{Target: conn.Target{Socket: "none"}, Post: loop.Post, Wake: time.Millisecond, Now: func() time.Time { return now }})
	const height = 600
	tt := outboxTester(w, st, loop, now, height)
	tt.Frame()
	tt.Frame()
	after, ok := tt.Find("After the footer")
	if !ok {
		t.Fatalf("nothing after the footer in %q", tt.Texts())
	}
	if top := float32(height)*(1-footerShare) - after.H; after.Y < top-1 || after.Y+after.H > height+1 {
		t.Errorf("the line after the footer is at %v, want the footer within %v of %d", after, footerShare, height)
	}
}

func TestPaletteMailResultsSwitchToMail(t *testing.T) {
	for _, module := range []commands.Module{commands.Calendar, commands.People, commands.Tasks} {
		for _, mailbox := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/mailbox=%v", module, mailbox), func(t *testing.T) {
				row := api.MessageSummary{ID: 42, Subject: "Plans", From: api.Address{Address: "ann@x.test"}}
				s := &fakeMaild{result: map[string]any{"view.open": api.ViewInfo{ID: 1, Count: 1}, "view.range": []api.MessageSummary{row}, "message.summaries": []api.MessageSummary{row}}}
				loop := frostuitest.NewLoop()
				w := New(Options{Target: conn.Target{Socket: "none"}, Post: loop.Post, Wake: time.Millisecond})
				w.client, w.events, w.module = s.client(t), live.NewEvents(), module
				t.Cleanup(func() {
					if w.view != nil {
						w.view.Close()
					}
				})
				if mailbox {
					w.tree = map[string]mail.Item{"mailbox:5": {Key: "mailbox:5", Label: "Plans folder", Selectable: true, MailboxID: 5}}
				}
				var after func()
				tt := paletteOn(w, loop, &after)
				w.shell.Palette.Show()
				w.shell.Palette.Query = "plans"
				draw(t, tt, "the result", func() bool { return tt.HasText("Plans") && (!mailbox || tt.HasText("Plans folder")) })
				tt.Key(0, ui.KeyEnter)
				tt.Frame()
				if w.module != commands.Mail {
					t.Fatalf("mail result left module %s showing", w.module)
				}
			})
		}
	}
}

// A notification's Reply (P-805) sends one draft.create. The connection
// ends after maild has the request and before its answer: maild may have
// made the draft, so the next connection must not ask for another
// (frostyard/frostmail#315).
func TestALostReplyIsNotSentAgain(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	s := &fakeMaild{hold: map[string]chan struct{}{"draft.create": release}}
	loop := frostuitest.NewLoop()
	opened := 0
	w := New(Options{Target: conn.Target{Socket: "none"}, Post: loop.Post, Wake: time.Millisecond,
		OpenCompose: func(*api.Client, int64, bool) { opened++ }})
	first := s.client(t)
	w.client = first
	w.ReplyTo(42)
	poll(t, "the draft.create", func() bool { return s.count("draft.create") == 1 })
	_ = first.Close() // the answer is lost
	<-first.Done()
	// The next connection, as takeClient delivers what waits.
	w.client = s.client(t)
	for start := time.Now(); time.Since(start) < 300*time.Millisecond; {
		loop.Flush()
		w.deliverReply()
		time.Sleep(5 * time.Millisecond)
	}
	if n := s.count("draft.create"); n != 1 {
		t.Errorf("%d draft.create calls for one Reply: the second makes a duplicate draft", n)
	}
	if w.pendingReply != 0 || opened != 0 {
		t.Errorf("after the lost answer: pending reply %d, %d compose windows; want none", w.pendingReply, opened)
	}
}

// A Reply clicked before there is a connection waits for it, and is sent
// once.
func TestAReplyWaitsForAConnection(t *testing.T) {
	s := &fakeMaild{result: map[string]any{"draft.create": api.Draft{ID: 9}}}
	loop := frostuitest.NewLoop()
	var opened []int64
	w := New(Options{Target: conn.Target{Socket: "none"}, Post: loop.Post, Wake: time.Millisecond,
		OpenCompose: func(_ *api.Client, d int64, _ bool) { opened = append(opened, d) }})
	w.ReplyTo(42)
	if s.count("draft.create") != 0 || w.pendingReply != 42 {
		t.Fatalf("with no connection: %d calls, pending %d", s.count("draft.create"), w.pendingReply)
	}
	w.client = s.client(t)
	w.deliverReply()
	w.deliverReply()
	poll(t, "the compose window", func() bool { loop.Flush(); return len(opened) == 1 })
	if n := s.count("draft.create"); n != 1 || opened[0] != 9 {
		t.Errorf("%d draft.create calls, opened %v; want 1 and draft 9", n, opened)
	}
}
