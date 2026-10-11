package engine_test

import (
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// TestMuteAndBlockAPI: message.mute mutes whole conversations and says so
// in summaries; block.* keeps the list; blockAction is a setting (ADR-0028).
func TestMuteAndBlockAPI(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ids := conversation(t, srv, c) // Plan, Re: Plan, Other; Re: Plan in Archive
	ctx := t.Context()
	if err := c.Message().Mute(ctx, &api.MessageMuteParams{IDs: []int64{ids[0]}, Muted: true}); err != nil {
		t.Fatal(err)
	}
	rows, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []bool{true, true, false, true} {
		if got := rows[i].Muted != nil && *rows[i].Muted; got != want || (rows[i].Muted != nil && !*rows[i].Muted) {
			t.Errorf("%s muted = %v, want %v (absent when not)", rows[i].Subject, rows[i].Muted, want)
		}
	}
	if err := c.Message().Mute(ctx, &api.MessageMuteParams{Muted: true}); code(err) != api.CodeInvalidParams {
		t.Errorf("mute nothing: %v, want invalidParams", err)
	}
	if err := c.Message().Mute(ctx, &api.MessageMuteParams{IDs: []int64{999}, Muted: true}); code(err) != api.CodeNotFound {
		t.Errorf("mute an unknown message: %v, want notFound", err)
	}

	list, err := c.Block().Add(ctx, &api.BlockAddParams{Addresses: []string{"Ann@Mailtest.test"}})
	if err != nil || len(list) != 1 || list[0].Address != "ann@mailtest.test" || list[0].Name != "Ann" {
		t.Fatalf("block.add = %+v, %v", list, err)
	}
	if _, err := c.Block().Add(ctx, &api.BlockAddParams{Addresses: []string{"not an address"}}); code(err) != api.CodeInvalidParams {
		t.Errorf("block a non-address: %v, want invalidParams", err)
	}
	if err := c.Block().Remove(ctx, &api.BlockRemoveParams{}); code(err) != api.CodeInvalidParams {
		t.Errorf("unblock nothing: %v, want invalidParams", err)
	}
	if err := c.Block().Remove(ctx, &api.BlockRemoveParams{Addresses: []string{"ann@mailtest.test"}}); err != nil {
		t.Fatal(err)
	}
	if list, err := c.Block().List(ctx, &api.BlockListParams{}); err != nil || len(list) != 0 {
		t.Errorf("block.list after remove = %+v, %v", list, err)
	}

	s, err := c.Settings().Get(ctx, &api.SettingsGetParams{})
	if err != nil || s.BlockAction == nil || *s.BlockAction != api.BlockActionMark {
		t.Fatalf("settings.get blockAction = %v, %v; want mark", s.BlockAction, err)
	}
	trash := api.BlockActionTrash
	if s, err := c.Settings().Set(ctx, &api.SettingsSetParams{BlockAction: &trash}); err != nil || *s.BlockAction != trash {
		t.Errorf("settings.set blockAction trash = %+v, %v", s, err)
	}
	bad := api.BlockAction("delete")
	if _, err := c.Settings().Set(ctx, &api.SettingsSetParams{BlockAction: &bad}); code(err) != api.CodeInvalidParams {
		t.Errorf("blockAction delete: %v, want invalidParams", err)
	}
}
