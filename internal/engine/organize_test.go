package engine_test

import (
	"slices"
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
