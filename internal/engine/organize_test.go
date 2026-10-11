package engine_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// TestViewConditions: view.open narrows by conditions (ADR-0023), and
// refuses ones it cannot compile and smart mailboxes that do not exist.
func TestViewConditions(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ids := conversation(t, srv, c)
	ctx := t.Context()
	archive := api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{
		{Field: api.ConditionFieldRole, Op: api.ConditionOpIs, Value: "archive"}}}
	v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Conditions: &archive}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 10})
	if err != nil || !slices.Equal(summaryIDs(rows), []int64{ids[3]}) {
		t.Errorf("role is archive = %v, %v; want [%d]", summaryIDs(rows), err, ids[3])
	}
	bad := api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{
		{Field: api.ConditionFieldUnread, Op: api.ConditionOpIs, Value: "yes"}}}
	if _, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Conditions: &bad}}); code(err) != api.CodeInvalidParams {
		t.Errorf("unread is yes: %v, want invalidParams", err)
	}
	if _, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{SmartMailboxID: ptr(int64(1))}}); code(err) != api.CodeNotFound {
		t.Errorf("an unknown smartMailboxId: %v, want notFound", err)
	}
}

// TestViewCount: view.count counts each query's messages and unread ones,
// in order, without opening views.
func TestViewCount(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ids := conversation(t, srv, c)
	ctx := t.Context()
	archive := api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{
		{Field: api.ConditionFieldRole, Op: api.ConditionOpIs, Value: "archive"}}}
	got, err := c.View().Count(ctx, &api.ViewCountParams{Queries: []api.ViewQuery{{Conditions: &archive}, {}, {Threads: ptr(true)}}})
	n := int64(len(ids))
	want := []api.ViewCount{{Total: 1, Unread: 1}, {Total: n, Unread: n}, {Total: n, Unread: n}}
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("view.count = %+v, %v; want %+v", got, err, want)
	}
	many := make([]api.ViewQuery, 101)
	if _, err := c.View().Count(ctx, &api.ViewCountParams{Queries: many}); code(err) != api.CodeInvalidParams {
		t.Errorf("101 queries: %v, want invalidParams", err)
	}
}

// TestSettings: defaults, changes to the fields given, and refusals
// (docs/design/organize.md, Settings).
func TestSettings(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	s, err := c.Settings().Get(ctx, &api.SettingsGetParams{})
	if err != nil || s.UndoDelay != 10 || s.NotifyScope != api.NotifyScopeInbox || len(s.FlagNames) != 7 || s.NotifySmartID != nil {
		t.Fatalf("defaults = %+v, %v", s, err)
	}
	names := []string{" Urgent ", "", "", "", "", "", "Someday"}
	s, err = c.Settings().Set(ctx, &api.SettingsSetParams{UndoDelay: ptr(int64(30)), FlagNames: names})
	if err != nil || s.UndoDelay != 30 || s.FlagNames[0] != "Urgent" || s.FlagNames[6] != "Someday" || s.NotifyScope != api.NotifyScopeInbox {
		t.Fatalf("set = %+v, %v", s, err)
	}
	s, err = c.Settings().Set(ctx, &api.SettingsSetParams{NotifyScope: ptr(api.NotifyScopeVips)})
	if err != nil || s.NotifyScope != api.NotifyScopeVips || s.UndoDelay != 30 || s.FlagNames[0] != "Urgent" {
		t.Fatalf("scope = %+v, %v; want the other settings kept", s, err)
	}
	if got, _ := c.Settings().Get(ctx, &api.SettingsGetParams{}); got == nil || got.NotifyScope != api.NotifyScopeVips || got.UndoDelay != 30 {
		t.Errorf("get after set = %+v", got)
	}
	for name, p := range map[string]*api.SettingsSetParams{
		"undoDelay 15":        {UndoDelay: ptr(int64(15))},
		"unknown scope":       {NotifyScope: ptr(api.NotifyScope("nope"))},
		"six names":           {FlagNames: names[:6]},
		"a long name":         {FlagNames: append([]string{strings.Repeat("x", 41)}, names[1:]...)},
		"smart id, not smart": {NotifySmartID: ptr(int64(1))},
	} {
		if _, err := c.Settings().Set(ctx, p); code(err) != api.CodeInvalidParams {
			t.Errorf("%s: %v, want invalidParams", name, err)
		}
	}
	if _, err := c.Settings().Set(ctx, &api.SettingsSetParams{NotifyScope: ptr(api.NotifyScopeSmart), NotifySmartID: ptr(int64(1))}); code(err) != api.CodeNotFound {
		t.Errorf("the scope of an unknown smart mailbox: %v, want notFound", err)
	}
	if _, err := c.Settings().Set(ctx, &api.SettingsSetParams{NotifyScope: ptr(api.NotifyScopeSmart)}); code(err) != api.CodeInvalidParams {
		t.Errorf("smart scope without a smart mailbox: %v, want invalidParams", err)
	}
}

// TestVIPs: addresses and a person's addresses become VIPs, named from
// People, and stop being VIPs.
func TestVIPs(t *testing.T) {
	ps := newPeopleServer(t)
	ctx := t.Context()
	people, err := ps.c.People().List(ctx, &api.PeopleListParams{})
	if err != nil || len(people) != 1 {
		t.Fatalf("people = %+v, %v", people, err)
	}
	ada := people[0].ID
	got, err := ps.c.Vip().Add(ctx, &api.VipAddParams{Addresses: []string{" Stranger@Example.com "}, PersonID: &ada})
	if err != nil || len(got) != 2 {
		t.Fatalf("vip.add = %+v, %v", got, err)
	}
	if got[0].Address != "ada@example.com" || got[0].Name != "Ada Lovelace" || got[0].PersonID == nil || *got[0].PersonID != ada {
		t.Errorf("ada = %+v", got[0])
	}
	if got[1].Address != "stranger@example.com" || got[1].Name != "Stranger Danger" || got[1].PersonID != nil {
		t.Errorf("stranger = %+v", got[1])
	}
	if again, _ := ps.c.Vip().Add(ctx, &api.VipAddParams{Addresses: []string{"ada@example.com"}}); len(again) != 2 {
		t.Errorf("adding a VIP again = %+v", again)
	}
	if err := ps.c.Vip().Remove(ctx, &api.VipRemoveParams{PersonID: &ada}); err != nil {
		t.Fatal(err)
	}
	if left, _ := ps.c.Vip().List(ctx, &api.VipListParams{}); len(left) != 1 || left[0].Address != "stranger@example.com" {
		t.Errorf("after removing Ada = %+v", left)
	}
	if _, err := ps.c.Vip().Add(ctx, &api.VipAddParams{}); code(err) != api.CodeInvalidParams {
		t.Errorf("nothing to add: %v", err)
	}
	if _, err := ps.c.Vip().Add(ctx, &api.VipAddParams{Addresses: []string{"not an address"}}); code(err) != api.CodeInvalidParams {
		t.Errorf("not an address: %v", err)
	}
	if _, err := ps.c.Vip().Add(ctx, &api.VipAddParams{PersonID: ptr(int64(9999))}); code(err) != api.CodeNotFound {
		t.Errorf("unknown person: %v", err)
	}
}

// TestSmartMailboxes: smart.* keeps smart mailboxes in order with their
// unread counts; a view of one follows its edits; fromSearch saves what a
// search lists; the notification scope can name one.
func TestSmartMailboxes(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ids := conversation(t, srv, c)
	ctx := t.Context()
	role := func(r string) api.Conditions {
		return api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{
			{Field: api.ConditionFieldRole, Op: api.ConditionOpIs, Value: r}}}
	}
	archive, err := c.Smart().Create(ctx, &api.SmartCreateParams{Name: " Archived ", Conditions: role("archive")})
	if err != nil || archive.Name != "Archived" || archive.Position != 0 || archive.Unread != 1 {
		t.Fatalf("create = %+v, %v", archive, err)
	}
	inbox, err := c.Smart().Create(ctx, &api.SmartCreateParams{Name: "In", Conditions: role("inbox"), IncludeTrash: ptr(true)})
	if err != nil || inbox.Position != 1 || !inbox.IncludeTrash || inbox.Unread != 3 {
		t.Fatalf("second = %+v, %v", inbox, err)
	}
	for name, p := range map[string]*api.SmartCreateParams{
		"no name":        {Name: "  ", Conditions: role("inbox")},
		"bad conditions": {Name: "X", Conditions: role("nowhere")},
	} {
		if _, err := c.Smart().Create(ctx, p); code(err) != api.CodeInvalidParams {
			t.Errorf("%s: %v, want invalidParams", name, err)
		}
	}

	v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{SmartMailboxID: &archive.ID}})
	if err != nil || v.Count != 1 {
		t.Fatalf("view of Archived = %+v, %v", v, err)
	}
	if _, err := c.Smart().Update(ctx, &api.SmartUpdateParams{ID: archive.ID, Conditions: ptr(role("inbox"))}); err != nil {
		t.Fatal(err)
	}
	timeout := time.After(5 * time.Second)
	for delta := false; !delta; {
		select {
		case env := <-c.Notifications():
			ev, err := api.DecodeEvent(env.Event, env.Data)
			d, ok := ev.(api.ViewDelta)
			delta = err == nil && ok && d.ID == v.ID
		case <-timeout:
			t.Fatal("no view.delta after the edit")
		}
	}
	rows, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 10})
	if err != nil || !slices.Equal(summaryIDs(rows), []int64{ids[2], ids[1], ids[0]}) {
		t.Fatalf("the view did not follow the edit: %v, %v", summaryIDs(rows), err)
	}
	counts, err := c.View().Count(ctx, &api.ViewCountParams{Queries: []api.ViewQuery{{SmartMailboxID: &inbox.ID}}})
	if err != nil || len(counts) != 1 || counts[0].Total != 3 {
		t.Errorf("count = %+v, %v", counts, err)
	}
	if _, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{SmartMailboxID: ptr(int64(9999))}}); code(err) != api.CodeNotFound {
		t.Errorf("unknown smart mailbox: %v", err)
	}

	if err := c.Smart().Move(ctx, &api.SmartMoveParams{ID: inbox.ID, Position: 0}); err != nil {
		t.Fatal(err)
	}
	list, err := c.Smart().List(ctx, &api.SmartListParams{})
	if err != nil || len(list) != 2 || list[0].ID != inbox.ID || list[1].ID != archive.ID {
		t.Errorf("after the move = %+v, %v", list, err)
	}

	cond, err := c.Smart().FromSearch(ctx, &api.SmartFromSearchParams{Text: "plan is:unread"})
	want := []api.Condition{
		{Field: api.ConditionFieldContent, Op: api.ConditionOpContains, Value: "plan"},
		{Field: api.ConditionFieldUnread, Op: api.ConditionOpIs, Value: "true"},
	}
	if err != nil || cond.Match != api.ConditionMatchAll || !slices.Equal(cond.Conditions, want) {
		t.Errorf("fromSearch = %+v, %v", cond, err)
	}
	if _, err := c.Smart().FromSearch(ctx, &api.SmartFromSearchParams{Text: "x", MailboxID: ptr(int64(9999))}); code(err) != api.CodeNotFound {
		t.Errorf("fromSearch in an unknown mailbox: %v", err)
	}

	s, err := c.Settings().Set(ctx, &api.SettingsSetParams{NotifyScope: ptr(api.NotifyScopeSmart), NotifySmartID: &archive.ID})
	if err != nil || s.NotifyScope != api.NotifyScopeSmart || s.NotifySmartID == nil || *s.NotifySmartID != archive.ID {
		t.Fatalf("smart scope = %+v, %v", s, err)
	}
	if _, err := c.Settings().Set(ctx, &api.SettingsSetParams{NotifySmartID: ptr(int64(9999))}); code(err) != api.CodeNotFound {
		t.Errorf("scope of an unknown smart mailbox: %v", err)
	}
	if err := c.Smart().Delete(ctx, &api.SmartDeleteParams{ID: archive.ID}); err != nil {
		t.Fatal(err)
	}
	if s, _ := c.Settings().Get(ctx, &api.SettingsGetParams{}); s == nil || s.NotifyScope != api.NotifyScopeInbox {
		t.Errorf("after deleting the scope's smart mailbox = %+v", s)
	}
}
