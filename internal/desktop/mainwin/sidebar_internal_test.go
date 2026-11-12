package mainwin

import (
	"encoding/json/v2"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"
	"github.com/frostyard/frostui/frostuitest"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// frostyard/frostmail#271: Follow Up and the Outbox are sections of the
// sidebar's list after the mailboxes, with the mailboxes' rows: a row
// names its conversation's recipients and wait, a click opens its
// message and keeps the choice while the source stays, and an Outbox
// row's menu acts on its message.
func TestFollowUpAndOutboxAreSourceListSections(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.Local)
	failure := "refused"
	st := live.MailState{
		FollowUp: &api.Awaiting{Total: 5, Threads: []api.AwaitingThread{
			{ThreadID: 11, Message: api.MessageSummary{ID: 101, Subject: "Any news"}, Vip: true, WaitingDays: 4,
				Recipients: []api.Address{{Name: "Bo", Address: "bo@x.test"}, {Name: "Cy", Address: "cy@x.test"}}, RecipientCount: 3},
			{ThreadID: 12, Message: api.MessageSummary{ID: 102, Subject: " "}, WaitingDays: 1,
				Recipients: []api.Address{{Address: "di@x.test"}}, RecipientCount: 1},
		}},
		Outbox: []api.OutboxItem{
			{ID: 3, Subject: "Refused one", State: api.OutboxStateFailed, Error: &failure},
			{ID: 4, Subject: "Going one", State: api.OutboxStateSending},
			{ID: 5, Subject: "Later one", State: api.OutboxStateQueued, Scheduled: true},
		},
	}
	s := &fakeMaild{}
	loop := frostuitest.NewLoop()
	w := New(Options{Target: conn.Target{Socket: "none"}, Post: loop.Post, Wake: time.Millisecond, Now: func() time.Time { return now }})
	w.client = s.client(t)
	w.tree = map[string]mail.Item{
		"all-inboxes": {Key: "all-inboxes", Label: "All Inboxes", Icon: "mail-read", Selectable: true},
		"flagged":     {Key: "flagged", Label: "Flagged", Icon: "mail-mark-important", Selectable: true},
	}
	tt := ui.NewTester(func(c *ui.Context) {
		loop.Flush()
		frostui.Adapt(c)
		sections := append([]frostui.SourceSection[string]{{ID: "favorites", Title: "Favorites", Items: []string{"all-inboxes", "flagged"}}},
			w.waitingSections(st)...)
		w.syncSource(st)
		ui.Column(c).Fill().AlignItems(ui.Stretch).Children(func() { w.sources.View(c, sections).Grow(1) })
	}, 320, 600)
	tt.Frame()
	tt.Frame()

	texts := tt.Texts()
	favorites, followUp, outbox := slices.Index(texts, "Favorites"), slices.Index(texts, "Follow Up"), slices.Index(texts, "Outbox")
	if favorites < 0 || followUp < favorites || outbox < followUp {
		t.Fatalf("the sections are not Favorites, Follow Up, Outbox: %q", texts)
	}
	for _, name := range []string{"Any news, VIP, To: Bo, Cy & 1 more, waiting 4 days", "(No Subject), To: di@x.test, waiting 1 day",
		"Showing 2 of 5", "Refused one, Not sent: refused", "Going one, Sending…"} {
		if _, ok := tt.AccessByRole(ui.RoleTreeItem, name); !ok {
			t.Errorf("no row %q in %q", name, texts)
		}
	}
	if slices.Contains(texts, "Later one") {
		t.Errorf("a message waiting for its time shows in the Outbox: %q", texts)
	}
	if !slices.Contains(texts, "4 days") {
		t.Errorf("no wait on the row in %q", texts)
	}

	if err := tt.Click("Any news"); err != nil {
		t.Fatal(err)
	}
	draw(t, tt, "the message asked for", func() bool { _, ok := s.call("message.summaries"); return ok })
	req, _ := s.call("message.summaries")
	var p api.MessageSummariesParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.IDs, []int64{101}) {
		t.Errorf("opened %v, want the conversation's newest message", p.IDs)
	}
	tt.Frame()
	if key, _, _ := w.sources.Selected(); key != "follow-up:11" {
		t.Errorf("the choice is %q, not the row clicked", key)
	}
	w.q.Source = mail.Source{Kind: mail.Flagged}
	tt.Frame()
	tt.Frame()
	if key, _, _ := w.sources.Selected(); key != "flagged" {
		t.Errorf("the choice is %q after the source changed, want flagged", key)
	}

	if err := tt.RightClick("Refused one"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.ChooseMenuItem("Try Again"); err != nil {
		t.Fatalf("%v: the menu holds %q", err, tt.Menu())
	}
	draw(t, tt, "the retry", func() bool { _, ok := s.call("outbox.retry"); return ok })
}
