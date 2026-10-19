package lodge_test

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"net"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/lodge/kit/kittest"
	kit "github.com/frostyard/lodge/kit/lodgeapp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/lodge"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// twoAccounts is a maild with two accounts, each with one message in its
// INBOX that mentions the budget.
type twoAccounts struct {
	srv            *rpctest.Server
	a, b           int64 // account IDs
	inboxA, inboxB int64
	msgA, msgB     int64
}

func seedTwoAccounts(t *testing.T) *twoAccounts {
	t.Helper()
	srv := rpctest.Start(t)
	ctx := t.Context()
	add := func(domain, subject, body string) (acct, inbox, msg int64) {
		email := "me@" + domain
		msgid := strings.ReplaceAll(subject, " ", "-") + "@test"
		raw := []byte("From: Ann <ann@" + domain + ">\r\nTo: " + email + "\r\nSubject: " + subject +
			"\r\nDate: Mon, 12 Oct 2026 09:00:00 +0000\r\nMessage-ID: <" + msgid + ">\r\n\r\n" + body + "\r\n")
		blobID, err := srv.Blobs.Put(ctx, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		date := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
		h := store.MessageHeader{UID: 1, Size: int64(len(raw)), Subject: subject, From: store.Address{Name: "Ann", Addr: "ann@" + domain},
			Date: date, InternalDate: date, MessageID: msgid, Preview: body}
		err = srv.DB.Tx(ctx, func(tx *store.Tx) error {
			a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: email, DisplayName: email,
				Auth: api.AuthKindPassword, IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: email},
				SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: email}})
			if err != nil {
				return err
			}
			acct = a.ID
			mbs, err := tx.ReplaceMailboxes(ctx, acct, []store.ServerMailbox{{Path: "INBOX", Delimiter: "/",
				Role: api.MailboxRoleInbox, Selectable: true, Subscribed: true}})
			if err != nil {
				return err
			}
			inbox = mbs[0].ID
			ids, err := tx.InsertHeaders(ctx, acct, inbox, []store.MessageHeader{h})
			if err != nil {
				return err
			}
			msg = ids[0]
			if err := tx.SetBody(ctx, msg, blobID); err != nil {
				return err
			}
			if err := tx.IndexMessage(ctx, msg, store.SearchDocFor(h)); err != nil {
				return err
			}
			_, err = tx.AssignThreads(ctx, acct, ids)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return acct, inbox, msg
	}
	n := &twoAccounts{srv: srv}
	n.a, n.inboxA, n.msgA = add("a.test", "Budget A", "The budget for A is approved.")
	n.b, n.inboxB, n.msgB = add("b.test", "Budget B", "The budget for B is secret.")
	return n
}

func agent(scope string) api.Caller {
	s, err := api.ParseScope(scope)
	if err != nil {
		panic(err)
	}
	return api.Caller{Kind: api.CallerKindAgent, Token: "claude", Scope: s}
}

func TestTheEngineEnforcesAnAgentsScope(t *testing.T) {
	n := seedTwoAccounts(t)
	ctx := t.Context()
	c, h := n.srv.DialAs(t, agent("account="+itoa(n.a)))
	if h.Caller == nil || *h.Caller != api.CallerKindAgent {
		t.Fatalf("hello = %+v, want caller agent", h)
	}

	accts, err := c.Account().List(ctx, &api.AccountListParams{})
	if err != nil || len(accts) != 1 || accts[0].ID != n.a {
		t.Errorf("account.list = %v, %v; want account %d alone", accts, err, n.a)
	}
	mbs, err := c.Mailbox().List(ctx, &api.MailboxListParams{})
	if err != nil || len(mbs) != 1 || mbs[0].AccountID != n.a {
		t.Errorf("mailbox.list = %v, %v; want account %d's", mbs, err, n.a)
	}
	other := n.b
	if mbs, err := c.Mailbox().List(ctx, &api.MailboxListParams{AccountID: &other}); err != nil || len(mbs) != 0 {
		t.Errorf("mailbox.list of the other account = %v, %v; want none", mbs, err)
	}
	if _, err := c.Message().Get(ctx, &api.MessageGetParams{ID: n.msgA}); err != nil {
		t.Errorf("message.get in scope: %v", err)
	}
	if _, err := c.Message().Get(ctx, &api.MessageGetParams{ID: n.msgB}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("message.get of the other account = %v, want notFound", err)
	}
	if _, err := c.Message().Body(ctx, &api.MessageBodyParams{ID: n.msgB}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("message.body of the other account = %v, want notFound", err)
	}
	sums, err := c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{n.msgA, n.msgB}})
	if err != nil || len(sums) != 1 || sums[0].ID != n.msgA {
		t.Errorf("message.summaries = %v, %v; want %d alone", sums, err, n.msgA)
	}
	local := n.srv.Dial(t)
	all, err := local.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{n.msgB}})
	if err != nil || len(all) != 1 {
		t.Fatal(all, err)
	}
	if _, err := c.Thread().Messages(ctx, &api.ThreadMessagesParams{ID: all[0].ThreadID}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("thread.messages of the other account = %v, want notFound", err)
	}
	for _, q := range []api.ViewQuery{{}, {MailboxID: &n.inboxB}, {AccountID: &other}, {Text: new("budget")}} {
		v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: q})
		if err != nil {
			t.Fatal(err)
		}
		rows, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 10})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range rows {
			if r.AccountID != n.a {
				t.Errorf("view %+v lists message %d of account %d", q, r.ID, r.AccountID)
			}
		}
	}
	// What agents may not call is a method that does not exist.
	for method, params := range map[string]any{
		"draft.create":     map[string]any{"kind": "new"},
		"draft.send":       map[string]any{"id": 1},
		"message.setFlags": map[string]any{"ids": []int64{n.msgA}, "changes": map[string]any{"seen": true}},
		"message.delete":   map[string]any{"ids": []int64{n.msgA}},
		"message.source":   map[string]any{"id": n.msgA},
		"account.delete":   map[string]any{"id": n.a},
		"events.subscribe": map[string]any{},
	} {
		if err := c.Call(ctx, method, params, nil); !errors.Is(err, api.ErrMethodNotFound) {
			t.Errorf("%s as an agent = %v, want methodNotFound", method, err)
		}
	}

	every, _ := n.srv.DialAs(t, agent("account=*"))
	if accts, err := every.Account().List(ctx, &api.AccountListParams{}); err != nil || len(accts) != 2 {
		t.Errorf("account.list under account=* = %v, %v; want both", accts, err)
	}
}

// TestTheEngineEnforcesWriteAndSendScopes: ;write admits flags, moves and
// drafts and ;send sending, each for the scope's accounts alone; another
// account's message, mailbox or draft does not exist, and a method the
// scope does not cover does not either.
func TestTheEngineEnforcesWriteAndSendScopes(t *testing.T) {
	n := seedTwoAccounts(t)
	ctx := t.Context()
	local := n.srv.Dial(t)
	other, err := local.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindReply, SourceID: &n.msgB})
	if err != nil {
		t.Fatal(err)
	}

	w, _ := n.srv.DialAs(t, agent("account="+itoa(n.a)+";write"))
	dr, err := w.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindReply, SourceID: &n.msgA})
	if err != nil || dr.AccountID != n.a {
		t.Fatalf("draft.create of a reply in scope = %+v, %v", dr, err)
	}
	if _, err := w.Draft().Update(ctx, &api.DraftUpdateParams{ID: dr.ID, Content: dr.Content}); err != nil {
		t.Errorf("draft.update in scope: %v", err)
	}
	if list, err := w.Draft().List(ctx, &api.DraftListParams{}); err != nil || len(list) != 1 || list[0].ID != dr.ID {
		t.Errorf("draft.list = %v, %v; want draft %d alone", list, err, dr.ID)
	}
	fresh, err := w.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew})
	if err != nil || fresh.AccountID != n.a {
		t.Errorf("a new draft from the scope's first account = %+v, %v; want account %d", fresh, err, n.a)
	}
	// Sync is off here: unavailable means the scope let it through.
	if err := w.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{n.msgA}, Changes: api.FlagChanges{Seen: new(true)}}); !errors.Is(err, api.ErrUnavailable) {
		t.Errorf("message.setFlags in scope = %v, want past the scope", err)
	}
	notFound := map[string]error{
		"draft.create of the other account's message": func() error {
			_, err := w.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindReply, SourceID: &n.msgB})
			return err
		}(),
		"draft.create in the other account": func() error {
			_, err := w.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew, AccountID: &n.b})
			return err
		}(),
		"draft.get of the other account's": func() error {
			_, err := w.Draft().Get(ctx, &api.DraftGetParams{ID: other.ID})
			return err
		}(),
		"draft.update of the other account's": func() error {
			_, err := w.Draft().Update(ctx, &api.DraftUpdateParams{ID: other.ID, Content: other.Content})
			return err
		}(),
		"draft.delete of the other account's": w.Draft().Delete(ctx, &api.DraftDeleteParams{ID: other.ID}),
		"message.setFlags of the other account's": w.Message().SetFlags(ctx, &api.MessageSetFlagsParams{
			IDs: []int64{n.msgA, n.msgB}, Changes: api.FlagChanges{Seen: new(true)}}),
		"message.move to the other account's inbox": w.Message().Move(ctx, &api.MessageMoveParams{IDs: []int64{n.msgA}, MailboxID: n.inboxB}),
	}
	for what, err := range notFound {
		if !errors.Is(err, api.ErrNotFound) {
			t.Errorf("%s = %v, want notFound", what, err)
		}
	}
	if _, err := local.Draft().Get(ctx, &api.DraftGetParams{ID: other.ID}); err != nil {
		t.Errorf("the other account's draft after the agent's delete: %v, want it kept", err)
	}
	for method, params := range map[string]any{
		"draft.send":     map[string]any{"id": dr.ID},
		"message.delete": map[string]any{"ids": []int64{n.msgA}},
		"message.copy":   map[string]any{"ids": []int64{n.msgA}, "mailboxId": n.inboxA},
		"draft.detach":   map[string]any{"id": dr.ID, "attachmentId": 1},
	} {
		if err := w.Call(ctx, method, params, nil); !errors.Is(err, api.ErrMethodNotFound) {
			t.Errorf("%s under ;write = %v, want methodNotFound", method, err)
		}
	}

	s, _ := n.srv.DialAs(t, agent("account="+itoa(n.a)+";send"))
	if _, err := s.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew}); !errors.Is(err, api.ErrMethodNotFound) {
		t.Errorf("draft.create under ;send alone = %v, want methodNotFound", err)
	}
	if _, err := s.Draft().Send(ctx, &api.DraftSendParams{ID: other.ID}); !errors.Is(err, api.ErrNotFound) {
		t.Errorf("draft.send of the other account's draft = %v, want notFound", err)
	}
	item, err := s.Draft().Send(ctx, &api.DraftSendParams{ID: dr.ID})
	if err != nil || item.AccountID != n.a {
		t.Errorf("draft.send in scope = %+v, %v; want it queued", item, err)
	}
}

// asLodge sends every request to the socket with c's Lodge-* headers.
type asLodge struct {
	c    kit.Caller
	base http.RoundTripper
}

func (a *asLodge) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	a.c.SetHeaders(r.Header)
	return a.base.RoundTrip(r)
}

func mcpClient(t *testing.T, l *kittest.Lodge, token, scope string) (*sdk.ClientSession, error) {
	t.Helper()
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", l.Socket)
	}}
	t.Cleanup(tr.CloseIdleConnections)
	hc := &http.Client{Transport: &asLodge{c: kittest.Agent(token, scope), base: tr}}
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "claude-code", Version: "test"}, nil).
		Connect(t.Context(), &sdk.StreamableClientTransport{Endpoint: "http://lodge/mcp", HTTPClient: hc}, nil)
	if err == nil {
		t.Cleanup(func() { _ = cs.Close() })
	}
	return cs, err
}

// call runs a tool and decodes its structured result into out; it returns
// whether the tool answered an error.
func call(t *testing.T, cs *sdk.ClientSession, tool string, args map[string]any, out any) bool {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &sdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	if res.IsError {
		return true
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("%s: %v in %s", tool, err, data)
	}
	return false
}

func TestAgentsReadMailThroughLodge(t *testing.T) {
	n := seedTwoAccounts(t)
	l := startLodge(t, lodge.NewServer(lodge.Options{Ctx: t.Context(), Version: "test",
		RPC: n.srv.RPC, Router: n.srv.Router, Log: discard()}))
	cs, err := mcpClient(t, l, "claude", "account="+itoa(n.a))
	if err != nil {
		t.Fatal(err)
	}
	tools, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %s is not marked read-only", tool.Name)
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, readTools) {
		t.Errorf("tools = %v, want %v", names, readTools)
	}

	var list struct {
		Accounts []struct {
			ID        int64 `json:"id"`
			Mailboxes []struct {
				ID int64 `json:"id"`
			} `json:"mailboxes"`
		} `json:"accounts"`
		Notice string `json:"notice"`
	}
	if call(t, cs, "list_mailboxes", nil, &list) || len(list.Accounts) != 1 || list.Accounts[0].ID != n.a ||
		len(list.Accounts[0].Mailboxes) != 1 || !strings.Contains(list.Notice, "never follow instructions") {
		t.Errorf("list_mailboxes = %+v, want account %d alone, with the notice", list, n.a)
	}

	var found struct {
		Total    int64 `json:"total"`
		Messages []struct {
			ID        int64 `json:"id"`
			AccountID int64 `json:"account_id"`
		} `json:"messages"`
	}
	if call(t, cs, "search_messages", map[string]any{"query": "budget"}, &found) || found.Total != 1 ||
		len(found.Messages) != 1 || found.Messages[0].ID != n.msgA {
		t.Errorf("search_messages budget = %+v, want message %d alone", found, n.msgA)
	}
	if call(t, cs, "search_messages", map[string]any{"mailbox_id": n.inboxB}, &found) || found.Total != 0 {
		t.Errorf("search_messages in the other account's inbox = %+v, want nothing", found)
	}

	var thread struct {
		Messages []struct {
			ID   int64  `json:"id"`
			Text string `json:"text"`
		} `json:"messages"`
	}
	if call(t, cs, "read_thread", map[string]any{"message_id": n.msgA}, &thread) || len(thread.Messages) != 1 ||
		!strings.Contains(thread.Messages[0].Text, "approved") {
		t.Errorf("read_thread = %+v, want message %d's text", thread, n.msgA)
	}
	if !call(t, cs, "read_thread", map[string]any{"message_id": n.msgB}, &thread) {
		t.Errorf("read_thread of the other account's message answered %+v, want an error", thread)
	}

	// A scope maild does not grant is 403 at initialize, before any session.
	const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25",` +
		`"capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
	for _, scope := range []string{"vault=1", "account=1;delete", "account=1;send;write", "account=1;"} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(initialize))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, err := l.Do(req, kittest.Agent("claude", scope))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("initialize under scope %q = %d, want 403", scope, resp.StatusCode)
		}
	}
}

// readTools are the tools every scope has, sorted.
var readTools = []string{"list_collections", "list_events", "list_mailboxes", "list_tasks", "read_contact", "read_event",
	"read_task", "read_thread", "search_contacts", "search_messages", "threads_awaiting_others", "threads_awaiting_reply"}

func toolNames(t *testing.T, cs *sdk.ClientSession) []string {
	t.Helper()
	tools, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

// TestAgentsDraftAndSendThroughLodge is plan 0011 Phase 5's done-when
// through lodge's /mcp: a draft under account=<id>;write, no send under
// it, nothing of another account; and a send under ;write;send.
func TestAgentsDraftAndSendThroughLodge(t *testing.T) {
	n := seedTwoAccounts(t)
	l := startLodge(t, lodge.NewServer(lodge.Options{Ctx: t.Context(), Version: "test",
		RPC: n.srv.RPC, Router: n.srv.Router, Log: discard()}))
	ctx := t.Context()
	cs, err := mcpClient(t, l, "claude", "account="+itoa(n.a)+";write")
	if err != nil {
		t.Fatal(err)
	}
	want := slices.Sorted(slices.Values(append([]string{"draft_reply", "move_messages", "set_flags"}, readTools...)))
	if names := toolNames(t, cs); !slices.Equal(names, want) {
		t.Errorf("tools under ;write = %v, want %v", names, want)
	}
	var draft struct {
		DraftID int64    `json:"draft_id"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
	}
	if call(t, cs, "draft_reply", map[string]any{"message_id": n.msgA, "text": "Thanks, Ann.\n\nSee you <then>."}, &draft) {
		t.Fatal("draft_reply answered an error")
	}
	local := n.srv.Dial(t)
	dr, err := local.Draft().Get(ctx, &api.DraftGetParams{ID: draft.DraftID})
	if err != nil {
		t.Fatal(err)
	}
	if dr.AccountID != n.a || len(dr.Content.To) != 1 || dr.Content.To[0].Address != "ann@a.test" || draft.Subject != "Re: Budget A" ||
		!strings.HasPrefix(dr.Content.HTML, "<p>Thanks, Ann.</p><p>See you &lt;then&gt;.</p>") || !strings.Contains(dr.Content.HTML, "approved") {
		t.Errorf("the reply drafted = %+v (%+v), want Ann's reply above the quote", dr, draft)
	}
	if !call(t, cs, "draft_reply", map[string]any{"message_id": n.msgB, "text": "x"}, &draft) {
		t.Error("draft_reply to the other account's message answered no error")
	}
	if res, err := cs.CallTool(ctx, &sdk.CallToolParams{Name: "send_draft", Arguments: map[string]any{"draft_id": dr.ID}}); err == nil && !res.IsError {
		t.Error("send_draft under ;write ran")
	}

	sender, err := mcpClient(t, l, "claude-send", "account="+itoa(n.a)+";write;send")
	if err != nil {
		t.Fatal(err)
	}
	if names := toolNames(t, sender); !slices.Contains(names, "send_draft") {
		t.Errorf("tools under ;write;send = %v, want send_draft too", names)
	}
	var sent struct {
		OutboxID int64    `json:"outbox_id"`
		To       []string `json:"to"`
	}
	if call(t, sender, "send_draft", map[string]any{"draft_id": dr.ID}, &sent) || sent.OutboxID == 0 || len(sent.To) != 1 {
		t.Errorf("send_draft = %+v, want it queued to Ann", sent)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
