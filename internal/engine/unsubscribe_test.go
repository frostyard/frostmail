package engine_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// fakePoster records one-click requests, failing while fail is set.
type fakePoster struct {
	urls []string
	fail bool
}

func (f *fakePoster) Post(_ context.Context, url string) error {
	f.urls = append(f.urls, url)
	if f.fail {
		return errors.New("list.example answered 500 Internal Server Error")
	}
	return nil
}

// listEnv is an account with an inbox and a way to store messages with
// their bodies.
type listEnv struct {
	t     *testing.T
	srv   *rpctest.Server
	c     *api.Client
	acct  int64
	inbox int64
	uid   uint32
}

func newListEnv(t *testing.T, poster *fakePoster) *listEnv {
	t.Helper()
	srv := rpctest.StartWith(t, rpctest.Options{Unsubscriber: poster})
	c := srv.Dial(t)
	a, err := c.Account().Create(t.Context(), createParams("lists@mailtest.test"))
	if err != nil {
		t.Fatal(err)
	}
	e := &listEnv{t: t, srv: srv, c: c, acct: a.ID}
	err = srv.DB.Tx(t.Context(), func(tx *store.Tx) error {
		mbs, err := tx.ReplaceMailboxes(t.Context(), a.ID, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		e.inbox = mbs[0].ID
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// message stores a message with h's headers and raw as its body.
func (e *listEnv) message(h store.MessageHeader, raw string) int64 {
	e.t.Helper()
	ctx := e.t.Context()
	blobID, err := e.srv.Blobs.Put(ctx, strings.NewReader(raw))
	if err != nil {
		e.t.Fatal(err)
	}
	e.uid++
	h.UID, h.InternalDate = e.uid, time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC)
	var id int64
	err = e.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		ids, err := tx.InsertHeaders(ctx, e.acct, e.inbox, []store.MessageHeader{h})
		if err != nil {
			return err
		}
		id = ids[0]
		return tx.SetBody(ctx, id, blobID)
	})
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

const listRaw = "From: Weekly <news@list.example>\r\nSubject: Issue 7\r\nList-Id: \"Weekly\" <weekly.list.example>\r\n" +
	"List-Unsubscribe: <mailto:leave@list.example?subject=bye>, <https://list.example/u/1>\r\n" +
	"List-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n\r\nNews.\r\n"

var listHeader = store.MessageHeader{Subject: "Issue 7", From: store.Address{Name: "Weekly", Addr: "news@list.example"},
	ListID: `"Weekly" <weekly.list.example>`, ListUnsubscribe: "<mailto:leave@list.example?subject=bye>, <https://list.example/u/1>",
	AuthResults: "mx.example; dkim=pass header.d=list.example"}

// TestUnsubscribe: maild says how a list message can be left, sends the
// one-click request, queues the mail or gives the page, and remembers the
// list for its later mail.
func TestUnsubscribe(t *testing.T) {
	poster := &fakePoster{}
	e := newListEnv(t, poster)
	ctx := t.Context()
	id := e.message(listHeader, listRaw)
	info, err := e.c.Message().UnsubscribeInfo(ctx, &api.MessageUnsubscribeInfoParams{ID: id})
	if err != nil || !slices.Equal(info.Methods, []api.UnsubscribeMethod{"oneclick", "mail", "web"}) || info.List != "Weekly" ||
		info.Host == nil || *info.Host != "list.example" || info.Address == nil || *info.Address != "leave@list.example" ||
		info.URL == nil || *info.URL != "https://list.example/u/1" || info.Done {
		t.Fatalf("info = %+v, %v", info, err)
	}

	poster.fail = true
	if _, err := e.c.Message().Unsubscribe(ctx, &api.MessageUnsubscribeParams{ID: id, Method: "oneclick"}); code(err) != api.CodeUnavailable ||
		!strings.Contains(err.Error(), "500") {
		t.Errorf("a refused one-click: %v", err)
	}
	if info, _ := e.c.Message().UnsubscribeInfo(ctx, &api.MessageUnsubscribeInfoParams{ID: id}); info.Done {
		t.Error("a refused one-click was remembered")
	}
	poster.fail = false
	if _, err := e.c.Message().Unsubscribe(ctx, &api.MessageUnsubscribeParams{ID: id, Method: "oneclick"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(poster.urls, []string{"https://list.example/u/1", "https://list.example/u/1"}) {
		t.Errorf("posts = %v", poster.urls)
	}
	later := e.message(listHeader, listRaw)
	if info, _ := e.c.Message().UnsubscribeInfo(ctx, &api.MessageUnsubscribeInfoParams{ID: later}); !info.Done {
		t.Error("later mail from the list is not marked done")
	}

	// Mail and web.
	mailOnly := listHeader
	mailOnly.ListID, mailOnly.ListUnsubscribe = "other.list.example", "<mailto:leave@other.example>"
	mid := e.message(mailOnly, "Subject: x\r\n\r\nx\r\n")
	if _, err := e.c.Message().Unsubscribe(ctx, &api.MessageUnsubscribeParams{ID: mid, Method: "oneclick"}); code(err) != api.CodeInvalidParams {
		t.Errorf("a method not offered: %v", err)
	}
	if _, err := e.c.Message().Unsubscribe(ctx, &api.MessageUnsubscribeParams{ID: mid, Method: "mail"}); err != nil {
		t.Fatal(err)
	}
	queued, err := e.c.Outbox().List(ctx, &api.OutboxListParams{})
	if err != nil || len(queued) != 1 || queued[0].Subject != "unsubscribe" || queued[0].To[0].Address != "leave@other.example" {
		t.Errorf("outbox = %+v, %v", queued, err)
	}
	web := listHeader
	web.ListID, web.AuthResults = "web.list.example", "mx.example; dkim=fail"
	wid := e.message(web, listRaw)
	res, err := e.c.Message().Unsubscribe(ctx, &api.MessageUnsubscribeParams{ID: wid, Method: "web"})
	if err != nil || res.URL == nil || *res.URL != "https://list.example/u/1" {
		t.Errorf("web = %+v, %v", res, err)
	}

	plain := e.message(store.MessageHeader{Subject: "Hi", From: store.Address{Name: "Ann", Addr: "ann@x.test"}}, "Subject: Hi\r\n\r\nHi\r\n")
	if info, err := e.c.Message().UnsubscribeInfo(ctx, &api.MessageUnsubscribeInfoParams{ID: plain}); err != nil || len(info.Methods) != 0 || info.List != "Ann" {
		t.Errorf("a message from no list = %+v, %v", info, err)
	}
	if _, err := e.c.Account().Update(ctx, &api.AccountUpdateParams{ID: e.acct, ReadOnly: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.c.Message().Unsubscribe(ctx, &api.MessageUnsubscribeParams{ID: mid, Method: "mail"}); code(err) != api.CodeConflict {
		t.Errorf("mail from a read-only account: %v", err)
	}
}

// TestRedirect: a redirect sends the message as it is, with Resent-*
// fields above it, to the new recipients only.
func TestRedirect(t *testing.T) {
	e := newListEnv(t, &fakePoster{})
	ctx := t.Context()
	raw := "From: Ann <ann@x.test>\r\nTo: lists@mailtest.test\r\nSubject: Plans\r\nMessage-ID: <p@x.test>\r\n\r\nPlans.\r\n"
	id := e.message(store.MessageHeader{Subject: "Plans", From: store.Address{Name: "Ann", Addr: "ann@x.test"}}, raw)
	item, err := e.c.Message().Redirect(ctx, &api.MessageRedirectParams{ID: id, To: []api.Address{{Name: "Bob", Address: "bob@x.test"}}})
	if err != nil || item.Subject != "Plans" || item.State != api.OutboxStateQueued || len(item.To) != 1 || item.To[0].Address != "bob@x.test" {
		t.Fatalf("redirect = %+v, %v", item, err)
	}
	rows, err := e.srv.DB.ListOutbox(ctx, e.acct)
	if err != nil || len(rows) != 1 || !slices.Equal(rows[0].Recipients, []string{"bob@x.test"}) || rows[0].From != "lists@mailtest.test" {
		t.Fatalf("outbox rows = %+v, %v", rows, err)
	}
	rc, err := e.srv.Blobs.Open(rows[0].BlobID)
	if err != nil {
		t.Fatal(err)
	}
	sent, _ := io.ReadAll(rc)
	rc.Close()
	head, rest, _ := bytes.Cut(sent, []byte("From: Ann"))
	for _, want := range []string{"Resent-From: ", "<lists@mailtest.test>", "Resent-To: \"Bob\" <bob@x.test>", "Resent-Date: ", "Resent-Message-ID: <"} {
		if !bytes.Contains(head, []byte(want)) {
			t.Errorf("the Resent fields lack %q:\n%s", want, head)
		}
	}
	if !bytes.HasSuffix(sent, []byte(raw)) || len(rest) == 0 {
		t.Errorf("the original is not kept whole:\n%s", sent)
	}
	for name, to := range map[string][]api.Address{"none": {}, "bad": {{Address: "not an address"}}} {
		if _, err := e.c.Message().Redirect(ctx, &api.MessageRedirectParams{ID: id, To: to}); code(err) != api.CodeInvalidParams {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := e.c.Message().Redirect(ctx, &api.MessageRedirectParams{ID: 9999, To: []api.Address{{Address: "bob@x.test"}}}); code(err) != api.CodeNotFound {
		t.Errorf("an unknown message: %v", err)
	}
}

// TestForwardAsAttachment: the draft attaches the message whole.
func TestForwardAsAttachment(t *testing.T) {
	e := newListEnv(t, &fakePoster{})
	ctx := t.Context()
	raw := "From: Ann <ann@x.test>\r\nSubject: Plans/Q4\r\n\r\nPlans.\r\n"
	id := e.message(store.MessageHeader{Subject: "Plans/Q4", From: store.Address{Name: "Ann", Addr: "ann@x.test"}}, raw)
	d, err := e.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindAttached, SourceID: &id})
	if err != nil {
		t.Fatal(err)
	}
	if d.Content.Subject != "Fwd: Plans/Q4" || len(d.Content.To) != 0 || len(d.Attachments) != 1 {
		t.Fatalf("draft = %+v", d)
	}
	a := d.Attachments[0]
	if a.Filename != "Plans-Q4.eml" || a.ContentType != "message/rfc822" || a.Size != int64(len(raw)) {
		t.Errorf("attachment = %+v", a)
	}
	content := d.Content
	content.To = []api.Address{{Address: "bob@x.test"}}
	if _, err := e.c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: content}); err != nil {
		t.Fatal(err)
	}
	item, err := e.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	rows, _ := e.srv.DB.ListOutbox(ctx, e.acct)
	rc, err := e.srv.Blobs.Open(rows[0].BlobID)
	if err != nil {
		t.Fatal(err)
	}
	sent, _ := io.ReadAll(rc)
	rc.Close()
	if item.Subject != "Fwd: Plans/Q4" || !bytes.Contains(sent, []byte("Plans-Q4.eml")) {
		t.Errorf("sent:\n%s", sent)
	}
}
