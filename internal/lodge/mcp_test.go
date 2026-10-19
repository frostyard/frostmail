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
	add := func(email, subject, body string) (acct, inbox, msg int64) {
		raw := []byte("From: Ann <ann@" + email + ">\r\nTo: " + email + "\r\nSubject: " + subject +
			"\r\nDate: Mon, 12 Oct 2026 09:00:00 +0000\r\nMessage-ID: <" + subject + "@test>\r\n\r\n" + body + "\r\n")
		blobID, err := srv.Blobs.Put(ctx, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		date := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
		h := store.MessageHeader{UID: 1, Size: int64(len(raw)), Subject: subject, From: store.Address{Name: "Ann", Addr: "ann@" + email},
			Date: date, InternalDate: date, MessageID: subject + "@test", Preview: body}
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
	l := kittest.Start(t, lodge.ID, lodge.NewServer(lodge.Options{Ctx: t.Context(), Version: "test",
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
	if want := []string{"list_mailboxes", "read_thread", "search_messages"}; !slices.Equal(names, want) {
		t.Errorf("tools = %v, want %v", names, want)
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

	for _, scope := range []string{"account=*;write", "account=1;send", "vault=1"} {
		if _, err := mcpClient(t, l, "claude", scope); err == nil {
			t.Errorf("a session under scope %q opened, want 403", scope)
		}
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
