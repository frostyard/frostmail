package engine_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// TestDraftTo: a new message can start with recipients, as the People
// module's Message button and contact cards do.
func TestDraftTo(t *testing.T) {
	c := rpctest.Start(t).Dial(t)
	ctx := t.Context()
	if _, err := c.Account().Create(ctx, createParams("me@mailtest.test")); err != nil {
		t.Fatal(err)
	}
	d, err := c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew,
		To: []api.Address{{Name: " Ada ", Address: "ada@example.com"}, {Address: "  "}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Content.To) != 1 || d.Content.To[0] != (api.Address{Name: "Ada", Address: "ada@example.com"}) {
		t.Errorf("to = %+v", d.Content.To)
	}
}
