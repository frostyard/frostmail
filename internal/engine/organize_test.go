package engine_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// TestViewConditions: view.open narrows by conditions (ADR-0023), refuses
// ones it cannot compile, and does not serve smart mailboxes before M5's
// Phase 3.
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
	if _, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{SmartMailboxID: ptr(int64(1))}}); code(err) != api.CodeUnavailable {
		t.Errorf("smartMailboxId: %v, want unavailable until Phase 3", err)
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
	if _, err := c.Settings().Set(ctx, &api.SettingsSetParams{NotifyScope: ptr(api.NotifyScopeSmart), NotifySmartID: ptr(int64(1))}); code(err) != api.CodeUnavailable {
		t.Errorf("smart scope: %v, want unavailable until Phase 3", err)
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
