package lodge_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/lodge"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// seedThread stores one account whose INBOX holds a conversation of 25
// messages, a day apart, and one message whose text is past 32 KiB.
func seedThread(t *testing.T) (srv *rpctest.Server, acct int64, thread []int64, long int64) {
	t.Helper()
	srv = rpctest.Start(t)
	ctx := t.Context()
	var hdrs []store.MessageHeader
	var raws [][]byte
	add := func(uid uint32, day int, msgid, subject, body string, refs []string) {
		date := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC).AddDate(0, 0, day)
		raw := fmt.Sprintf("From: Ann <ann@a.test>\r\nTo: me@a.test\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s>\r\n",
			subject, date.Format(time.RFC1123Z), msgid)
		if len(refs) > 0 {
			raw += "In-Reply-To: <" + refs[0] + ">\r\nReferences: <" + refs[0] + ">\r\n"
		}
		raw += "\r\n" + body + "\r\n"
		h := store.MessageHeader{UID: uid, Size: int64(len(raw)), Subject: subject, From: store.Address{Name: "Ann", Addr: "ann@a.test"},
			Date: date, InternalDate: date, MessageID: msgid, References: refs, Preview: string([]rune(body)[:min(len([]rune(body)), 40)])}
		if len(refs) > 0 {
			h.InReplyTo = refs[0]
		}
		hdrs, raws = append(hdrs, h), append(raws, []byte(raw))
	}
	for i := range 25 {
		var refs []string
		if i > 0 {
			refs = []string{"plan-0@a.test"}
		}
		add(uint32(i+1), i, fmt.Sprintf("plan-%d@a.test", i), "Plan", fmt.Sprintf("Plan message %d.", i), refs)
	}
	add(26, 30, "long@a.test", "Long", "a"+strings.Repeat("é", 20_000), nil)
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "me@a.test", DisplayName: "Me",
			Auth: api.AuthKindPassword, IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"},
			SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"}})
		if err != nil {
			return err
		}
		acct = a.ID
		mbs, err := tx.ReplaceMailboxes(ctx, acct, []store.ServerMailbox{{Path: "INBOX", Delimiter: "/",
			Role: api.MailboxRoleInbox, Selectable: true, Subscribed: true}})
		if err != nil {
			return err
		}
		ids, err := tx.InsertHeaders(ctx, acct, mbs[0].ID, hdrs)
		if err != nil {
			return err
		}
		for i, id := range ids {
			blobID, err := srv.Blobs.Put(ctx, bytes.NewReader(raws[i]))
			if err != nil {
				return err
			}
			if err := tx.SetBody(ctx, id, blobID); err != nil {
				return err
			}
			if err := tx.IndexMessage(ctx, id, store.SearchDocFor(hdrs[i])); err != nil {
				return err
			}
		}
		thread, long = ids[:25], ids[25]
		_, err = tx.AssignThreads(ctx, acct, ids)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, acct, thread, long
}

// TestTheReadToolsLimits: a search answers 20 messages by default and at
// most 50, a thread its newest 20, and a message's text at most 32 KiB
// (docs/specs/lodge-app.md, Limits).
func TestTheReadToolsLimits(t *testing.T) {
	srv, acct, thread, long := seedThread(t)
	l := startLodge(t, lodge.NewServer(lodge.Options{Ctx: t.Context(), Version: "test",
		RPC: srv.RPC, Router: srv.Router, Log: discard()}))
	cs, err := mcpClient(t, l, "claude", "account="+itoa(acct))
	if err != nil {
		t.Fatal(err)
	}
	var found struct {
		Total    int64 `json:"total"`
		Messages []struct {
			ID int64 `json:"id"`
		} `json:"messages"`
	}
	// limit 0 is the default, as an omitted one is.
	for limit, want := range map[int]int{0: 20, 1: 1, 50: 26} {
		args := map[string]any{}
		if limit != 0 {
			args["limit"] = limit
		}
		if call(t, cs, "search_messages", args, &found) || found.Total != 26 || len(found.Messages) != want {
			t.Errorf("search_messages limit %d = %d of %d, want %d of 26", limit, len(found.Messages), found.Total, want)
		}
	}
	for _, limit := range []int{-1, 51} {
		if !call(t, cs, "search_messages", map[string]any{"limit": limit}, &found) {
			t.Errorf("search_messages limit %d answered no error", limit)
		}
	}

	var read struct {
		Count    int `json:"count"`
		Messages []struct {
			ID        int64  `json:"id"`
			Text      string `json:"text"`
			Truncated bool   `json:"truncated"`
		} `json:"messages"`
	}
	if call(t, cs, "read_thread", map[string]any{"message_id": thread[0]}, &read) || read.Count != 25 || len(read.Messages) != 20 {
		t.Fatalf("read_thread = %d of %d messages, want the newest 20 of 25", len(read.Messages), read.Count)
	}
	if read.Messages[0].ID != thread[5] || read.Messages[19].ID != thread[24] || read.Messages[0].Truncated {
		t.Errorf("read_thread runs %d to %d, want %d to %d, oldest first", read.Messages[0].ID, read.Messages[19].ID, thread[5], thread[24])
	}
	if call(t, cs, "read_thread", map[string]any{"message_id": long}, &read) || len(read.Messages) != 1 {
		t.Fatalf("read_thread of the long message = %+v", read)
	}
	text := read.Messages[0].Text
	if !read.Messages[0].Truncated || len(text) > 32<<10 || len(text) < 32<<10-4 || !utf8.ValidString(text) {
		t.Errorf("the long text = %d bytes (truncated %v), want cut at 32 KiB on a character", len(text), read.Messages[0].Truncated)
	}
}
