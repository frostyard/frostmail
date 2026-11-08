package engine_test

import (
	"sync"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// updateRounds is how many pairs of concurrent updates each test sends. A
// read before the write transaction loses one of a pair whenever both
// reads come before the first write; over many rounds that happens.
const updateRounds = 40

// TestSmartUpdatesKeepEachOther: 2 concurrent smart.update calls that set
// different fields both stay, because each reads the row inside the
// transaction that writes it (#193).
func TestSmartUpdatesKeepEachOther(t *testing.T) {
	srv := rpctest.Start(t)
	c1, c2 := srv.Dial(t), srv.Dial(t)
	ctx := t.Context()
	inbox := api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{
		{Field: api.ConditionFieldRole, Op: api.ConditionOpIs, Value: "inbox"}}}
	for range updateRounds {
		m, err := c1.Smart().Create(ctx, &api.SmartCreateParams{Name: "In", Conditions: inbox})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		var errTrash, errSent error
		wg.Go(func() {
			<-start
			_, errTrash = c1.Smart().Update(ctx, &api.SmartUpdateParams{ID: m.ID, IncludeTrash: ptr(true)})
		})
		wg.Go(func() {
			<-start
			_, errSent = c2.Smart().Update(ctx, &api.SmartUpdateParams{ID: m.ID, IncludeSent: ptr(true)})
		})
		close(start)
		wg.Wait()
		if errTrash != nil || errSent != nil {
			t.Fatalf("updates: %v, %v", errTrash, errSent)
		}
		got, err := srv.DB.SmartMailbox(ctx, m.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !got.IncludeTrash || !got.IncludeSent {
			t.Fatalf("after 2 concurrent updates = %+v; want both includes set", got)
		}
	}
}

// TestRuleUpdatesKeepEachOther: 2 concurrent rule.update calls that set
// different fields both stay (#193).
func TestRuleUpdatesKeepEachOther(t *testing.T) {
	srv := rpctest.Start(t)
	c1, c2 := srv.Dial(t), srv.Dial(t)
	ctx := t.Context()
	from := api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{
		{Field: api.ConditionFieldFrom, Op: api.ConditionOpContains, Value: "ann"}}}
	read := api.RuleAction{Kind: api.RuleActionKindRead}
	for range updateRounds {
		r, err := c1.Rule().Create(ctx, &api.RuleCreateParams{Name: "Ann", Conditions: from, Actions: []api.RuleAction{read}})
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		var errOff, errName error
		wg.Go(func() {
			<-start
			_, errOff = c1.Rule().Update(ctx, &api.RuleUpdateParams{ID: r.ID, Enabled: ptr(false)})
		})
		wg.Go(func() {
			<-start
			_, errName = c2.Rule().Update(ctx, &api.RuleUpdateParams{ID: r.ID, Name: ptr("Ann read")})
		})
		close(start)
		wg.Wait()
		if errOff != nil || errName != nil {
			t.Fatalf("updates: %v, %v", errOff, errName)
		}
		got, err := srv.DB.Rule(ctx, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Enabled || got.Name != "Ann read" {
			t.Fatalf("after 2 concurrent updates = %+v; want off and renamed", got)
		}
	}
}
