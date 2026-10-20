package lodge_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/lodge"
)

// TestAgentsDraftNewMessagesThroughLodge: draft_message under ;write
// drafts a new message, to and cc, a subject and text above the
// signature, from the scope's first account or the one named, and sends
// nothing; another account is an error, and so is a recipient past the
// limit or an address that does not parse.
func TestAgentsDraftNewMessagesThroughLodge(t *testing.T) {
	n := seedTwoAccounts(t)
	l := startLodge(t, lodge.NewServer(lodge.Options{Ctx: t.Context(), Version: "test",
		RPC: n.srv.RPC, Router: n.srv.Router, Log: discard()}))
	ctx := t.Context()
	local := n.srv.Dial(t)

	reader, err := mcpClient(t, l, "claude-read", "account="+itoa(n.a))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(toolNames(t, reader), "draft_message") {
		t.Error("draft_message offered without ;write")
	}

	cs, err := mcpClient(t, l, "claude", "account="+itoa(n.a)+";write")
	if err != nil {
		t.Fatal(err)
	}
	var draft struct {
		DraftID   int64    `json:"draft_id"`
		URL       string   `json:"url"`
		AccountID int64    `json:"account_id"`
		To        []string `json:"to"`
		Cc        []string `json:"cc"`
		Subject   string   `json:"subject"`
		Note      string   `json:"note"`
	}
	if call(t, cs, "draft_message", map[string]any{"to": []string{"Omar Haddad <omar@x.test>"}, "cc": []string{"bob@x.test"},
		"subject": "Lisbon", "text": "Here is my itinerary for Lisbon.\n\nSee you <there>."}, &draft) {
		t.Fatal("draft_message answered an error")
	}
	if draft.AccountID != n.a || draft.URL != "/mail/compose/"+itoa(draft.DraftID) || draft.Subject != "Lisbon" ||
		!slices.Equal(draft.To, []string{"Omar Haddad <omar@x.test>"}) || !slices.Equal(draft.Cc, []string{"bob@x.test"}) ||
		!strings.Contains(draft.Note, "nothing was sent") {
		t.Errorf("draft_message = %+v", draft)
	}
	dr, err := local.Draft().Get(ctx, &api.DraftGetParams{ID: draft.DraftID})
	if err != nil {
		t.Fatal(err)
	}
	if dr.Kind != api.DraftKindNew || dr.AccountID != n.a || dr.SourceID != nil || len(dr.Content.To) != 1 ||
		dr.Content.To[0] != (api.Address{Name: "Omar Haddad", Address: "omar@x.test"}) || len(dr.Content.Cc) != 1 ||
		dr.Content.Subject != "Lisbon" ||
		!strings.HasPrefix(dr.Content.HTML, "<p>Here is my itinerary for Lisbon.</p><p>See you &lt;there&gt;.</p>") {
		t.Errorf("the draft = %+v, want a new message to Omar with the text escaped as paragraphs", dr)
	}
	if out, err := local.Outbox().List(ctx, &api.OutboxListParams{}); err != nil || len(out) != 0 {
		t.Errorf("the outbox after draft_message = %+v, %v; want it empty", out, err)
	}

	// Another account, named or not, is out of reach.
	if !call(t, cs, "draft_message", map[string]any{"to": []string{"omar@x.test"}, "subject": "x", "text": "x", "account_id": n.b}, &draft) {
		t.Error("draft_message from the other account answered no error")
	}
	every, err := mcpClient(t, l, "claude-all", "account=*;write")
	if err != nil {
		t.Fatal(err)
	}
	if call(t, every, "draft_message", map[string]any{"to": []string{"omar@x.test"}, "subject": "B", "text": "From B.",
		"account_id": n.b}, &draft) || draft.AccountID != n.b {
		t.Errorf("draft_message from account %d under account=* = %+v", n.b, draft)
	}

	// The limits, at their values and one past.
	many := func(k int) []string {
		out := make([]string, k)
		for i := range out {
			out[i] = fmt.Sprintf("p%d@x.test", i)
		}
		return out
	}
	for _, args := range []map[string]any{
		{"to": many(100), "subject": "To 100", "text": "x"},
		{"to": many(1), "cc": many(100), "subject": "Cc 100", "text": "x"},
	} {
		if call(t, cs, "draft_message", args, &draft) {
			t.Errorf("draft_message %s answered an error", args["subject"])
		}
	}
	for what, args := range map[string]map[string]any{
		"no recipient":        {"to": []string{}, "subject": "x", "text": "x"},
		"101 recipients":      {"to": many(101), "subject": "x", "text": "x"},
		"101 copied":          {"to": many(1), "cc": many(101), "subject": "x", "text": "x"},
		"a bad address":       {"to": []string{"not an address"}, "subject": "x", "text": "x"},
		"a bad copied":        {"to": many(1), "cc": []string{"bob@"}, "subject": "x", "text": "x"},
		"empty text":          {"to": many(1), "subject": "x", "text": "  \n "},
		"a missing recipient": {"subject": "x", "text": "x"},
	} {
		if !call(t, cs, "draft_message", args, &draft) {
			t.Errorf("draft_message with %s answered no error", what)
		}
	}
}
