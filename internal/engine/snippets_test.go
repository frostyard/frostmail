package engine_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// TestSnippets: bounds, list order, edits and durable snippet.changed events.
func TestSnippets(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ctx := t.Context()
	sub, err := c.Events().Subscribe(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	seq0 := sub.Seq

	// Inserted in neither order: the list sorts by name without case, so
	// "apple" comes before "Mango", which a byte order would put first.
	b, err := c.Snippet().Create(ctx, &api.SnippetCreateParams{Name: "Zed", HTML: "Bee"})
	if err != nil {
		t.Fatal(err)
	}
	waitSnippetChanged(t, c, b.ID, false)
	a, err := c.Snippet().Create(ctx, &api.SnippetCreateParams{Name: "  apple  ", HTML: "<p>Hello</p>"})
	if err != nil || a.Name != "apple" || a.HTML != "<p>Hello</p>" {
		t.Fatalf("create apple = %+v, %v", a, err)
	}
	waitSnippetChanged(t, c, a.ID, false)
	a2, err := c.Snippet().Create(ctx, &api.SnippetCreateParams{Name: "Mango", HTML: "A2"})
	if err != nil {
		t.Fatal(err)
	}
	waitSnippetChanged(t, c, a2.ID, false)

	list, err := c.Snippet().List(ctx, &api.SnippetListParams{})
	if err != nil || len(list) != 3 || list[0].Name != "apple" || list[1].Name != "Mango" || list[2].Name != "Zed" {
		t.Fatalf("list = %+v, %v", list, err)
	}

	name := "Second"
	html := "<p>Updated</p>"
	updated, err := c.Snippet().Update(ctx, &api.SnippetUpdateParams{ID: b.ID, Name: &name, HTML: &html})
	if err != nil || updated.Name != "Second" || updated.HTML != html {
		t.Fatalf("update = %+v, %v", updated, err)
	}
	waitSnippetChanged(t, c, b.ID, false)

	if err := c.Snippet().Delete(ctx, &api.SnippetDeleteParams{ID: b.ID}); err != nil {
		t.Fatal(err)
	}
	waitSnippetChanged(t, c, b.ID, true)

	// A second connection replays the five durable changes made above,
	// in sequence, after the first subscription's starting sequence.
	second := srv.Dial(t)
	if _, err := second.Events().Subscribe(ctx, &api.EventsSubscribeParams{SinceSeq: &seq0}); err != nil {
		t.Fatal(err)
	}
	replay := []struct {
		id      int64
		deleted bool
	}{
		{id: b.ID, deleted: false},
		{id: a.ID, deleted: false},
		{id: a2.ID, deleted: false},
		{id: b.ID, deleted: false},
		{id: b.ID, deleted: true},
	}
	var previous int64
	for i, want := range replay {
		env := waitSnippetChangedEnvelope(t, second)
		if env.Seq <= previous {
			t.Fatalf("replayed snippet sequence %d is not greater than %d", env.Seq, previous)
		}
		previous = env.Seq
		ev, err := api.DecodeEvent(env.Event, env.Data)
		changed, ok := ev.(api.SnippetChanged)
		if err != nil || !ok || changed.ID != want.id || changed.Deleted != want.deleted {
			t.Fatalf("replay %d = %v, %+v; want id %d deleted %v", i, err, ev, want.id, want.deleted)
		}
	}

	list, err = c.Snippet().List(ctx, &api.SnippetListParams{})
	if err != nil || len(list) != 2 {
		t.Fatalf("after delete = %+v, %v", list, err)
	}

	if _, err := c.Snippet().Create(ctx, &api.SnippetCreateParams{Name: "APPLE", HTML: "duplicate"}); code(err) != api.CodeConflict {
		t.Errorf("duplicate create: %v, want conflict", err)
	}
	if _, err := c.Snippet().Update(ctx, &api.SnippetUpdateParams{ID: a2.ID, Name: ptr("Apple")}); code(err) != api.CodeConflict {
		t.Errorf("duplicate update: %v, want conflict", err)
	}
	ownCase := "MANGO"
	if _, err := c.Snippet().Update(ctx, &api.SnippetUpdateParams{ID: a2.ID, Name: &ownCase}); err != nil {
		t.Errorf("rename to own name in another case: %v", err)
	}

	for name, p := range map[string]*api.SnippetCreateParams{
		"61-character name": {Name: strings.Repeat("x", 61), HTML: ""},
		"control name":      {Name: "bad\nname", HTML: ""},
		"blank name":        {Name: "  ", HTML: ""},
		"oversize HTML":     {Name: "Oversize", HTML: strings.Repeat("x", 65537)},
	} {
		if _, err := c.Snippet().Create(ctx, p); code(err) != api.CodeInvalidParams {
			t.Errorf("%s: %v, want invalidParams", name, err)
		}
	}

	exactName := strings.Repeat("x", 60)
	if _, err := c.Snippet().Create(ctx, &api.SnippetCreateParams{Name: exactName, HTML: strings.Repeat("x", 65536)}); err != nil {
		t.Errorf("exact bounds: %v", err)
	}

	if _, err := c.Snippet().Update(ctx, &api.SnippetUpdateParams{ID: 999999}); code(err) != api.CodeNotFound {
		t.Errorf("unknown update: %v, want notFound", err)
	}
	if err := c.Snippet().Delete(ctx, &api.SnippetDeleteParams{ID: 999999}); code(err) != api.CodeNotFound {
		t.Errorf("unknown delete: %v, want notFound", err)
	}

	list, err = c.Snippet().List(ctx, &api.SnippetListParams{})
	if err != nil {
		t.Fatal(err)
	}
	for i := len(list); i < 200; i++ {
		name := fmt.Sprintf("Limit%03d", i)
		if _, err := c.Snippet().Create(ctx, &api.SnippetCreateParams{Name: name, HTML: ""}); err != nil {
			t.Fatalf("fill snippet %d: %v", i+1, err)
		}
	}
	if _, err := c.Snippet().Create(ctx, &api.SnippetCreateParams{Name: "TooMany", HTML: ""}); code(err) != api.CodeInvalidParams {
		t.Errorf("201st snippet: %v, want invalidParams", err)
	}
}

func waitSnippetChangedEnvelope(t *testing.T, c *api.Client) api.EventEnvelope {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case env := <-c.Notifications():
			if env.Seq <= 0 {
				t.Fatalf("replayed snippet.changed has no durable sequence: %+v", env)
			}
			if env.Event == "snippet.changed" {
				return env
			}
		case <-timeout:
			t.Fatal("no replayed snippet.changed")
		}
	}
}

func waitSnippetChanged(t *testing.T, c *api.Client, id int64, deleted bool) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case env := <-c.Notifications():
			if env.Seq <= 0 {
				t.Fatalf("snippet.changed has no durable sequence: %+v", env)
			}
			ev, err := api.DecodeEvent(env.Event, env.Data)
			changed, ok := ev.(api.SnippetChanged)
			if err == nil && ok && changed.ID == id && changed.Deleted == deleted {
				return
			}
		case <-timeout:
			t.Fatalf("no snippet.changed id %d deleted %v", id, deleted)
		}
	}
}
