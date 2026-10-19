package lodge_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// linkIDs are seedLinks' message IDs.
type linkIDs struct {
	odd, none, digits, sent int64
	task                    int64
}

// seedLinks stores, on a clock at pimNow, an account whose inbox holds mail
// to me@a.test with the Message-ID a/b%c+d@x.test, with none, and with
// 12345, and whose Sent holds the user's mail with s+1/2@x.test, six days
// old; and a task made from the first message.
func seedLinks(t *testing.T) (*rpctest.Server, linkIDs) {
	t.Helper()
	srv := rpctest.StartWith(t, rpctest.Options{Now: func() time.Time { return pimNow }})
	ctx := t.Context()
	var ids linkIDs
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		acct, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "me@a.test", DisplayName: "Me",
			Auth: api.AuthKindPassword, IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"},
			SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "me"}})
		if err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, acct.ID, []store.ServerMailbox{
			{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true},
			{Path: "Sent", Role: api.MailboxRoleSent, Selectable: true}})
		if err != nil {
			return err
		}
		var all []int64
		add := func(box int64, uid uint32, msgid, from, to string, days int) int64 {
			date := pimNow.Add(-time.Duration(days) * 24 * time.Hour)
			raw := []byte("From: " + from + "\r\nTo: " + to + "\r\nSubject: Link " + msgid + "\r\n\r\nHello.\r\n")
			blob, err := srv.Blobs.Put(ctx, bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			got, err := tx.InsertHeaders(ctx, acct.ID, box, []store.MessageHeader{{UID: uid, Size: int64(len(raw)),
				Subject: "Link " + msgid, From: store.Address{Addr: from}, To: []store.Address{{Addr: to}}, Date: date,
				InternalDate: date, MessageID: msgid, Preview: "Hello."}})
			if err != nil {
				t.Fatal(err)
			}
			if err := tx.SetBody(ctx, got[0], blob); err != nil {
				t.Fatal(err)
			}
			all = append(all, got[0])
			return got[0]
		}
		ids.odd = add(mbs[0].ID, 1, "a/b%c+d@x.test", "ann@x.test", "me@a.test", 2)
		ids.none = add(mbs[0].ID, 2, "", "bob@x.test", "me@a.test", 3)
		ids.digits = add(mbs[0].ID, 3, "12345", "carol@x.test", "me@a.test", 4)
		ids.sent = add(mbs[1].ID, 1, "s+1/2@x.test", "me@a.test", "dan@x.test", 6)
		if _, err := tx.AssignThreads(ctx, acct.ID, all); err != nil {
			return err
		}
		// A task made from mail in Gmail links to its thread's message.
		if _, err := tx.ExecContext(ctx, `UPDATE messages SET gm_thrid = 77 WHERE id = ?`, ids.odd); err != nil {
			return err
		}
		s := pimStore{t, tx}
		s.services(acct.ID)
		list := s.collection(acct.ID, api.CollectionKindTasklist, "L", "Tasks")
		s.task(list, store.TaskRow{UID: "follow", Title: "Follow up", Position: "1", GmThrID: 77})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := srv.Dial(t).Tasks().List(ctx, &api.TasksListParams{})
	if err != nil || len(tasks) != 1 || tasks[0].MessageID == nil || *tasks[0].MessageID != ids.odd {
		t.Fatalf("the task = %+v, %v; want it linked to message %d", tasks, err, ids.odd)
	}
	ids.task = tasks[0].ID
	return srv, ids
}

type linked struct {
	ID              int64  `json:"id"`
	MessageID       int64  `json:"message_id"`
	MessageIDHeader string `json:"message_id_header"`
	URL             string `json:"url"`
}

const oddURL = "/mail/message/a%2Fb%25c%2Bd%40x.test"

// checkLink finds message id among got and checks its header and URL.
func checkLink(t *testing.T, tool string, got []linked, id int64, header, url string) {
	t.Helper()
	for _, l := range got {
		if l.ID == id || l.MessageID == id {
			if l.MessageIDHeader != header || l.URL != url {
				t.Errorf("%s: message %d's header %q and url %q, want %q and %q", tool, id, l.MessageIDHeader, l.URL, header, url)
			}
			return
		}
	}
	t.Errorf("%s does not answer message %d: %+v", tool, id, got)
}

// TestToolsLinkMessagesByMessageID: every tool that answers a message
// answers its Message-ID and its permanent lodge URL, encoded as one
// segment, and neither for a message without one; a Message-ID of digits
// alone, which the route would read as maild's ID, gets no URL.
func TestToolsLinkMessagesByMessageID(t *testing.T) {
	srv, ids := seedLinks(t)
	cs, err := mcpClient(t, lodgeAt(t, srv), "claude", "account=*;write")
	if err != nil {
		t.Fatal(err)
	}
	var found struct {
		Messages []linked `json:"messages"`
	}
	if call(t, cs, "search_messages", nil, &found) {
		t.Fatal("search_messages answered an error")
	}
	checkLink(t, "search_messages", found.Messages, ids.odd, "a/b%c+d@x.test", oddURL)
	checkLink(t, "search_messages", found.Messages, ids.none, "", "")
	checkLink(t, "search_messages", found.Messages, ids.digits, "12345", "")
	checkLink(t, "search_messages", found.Messages, ids.sent, "s+1/2@x.test", "/mail/message/s%2B1%2F2%40x.test")

	var thread struct {
		Messages []linked `json:"messages"`
	}
	for id, want := range map[int64][2]string{ids.odd: {"a/b%c+d@x.test", oddURL}, ids.none: {"", ""}} {
		if call(t, cs, "read_thread", map[string]any{"message_id": id}, &thread) {
			t.Fatalf("read_thread %d answered an error", id)
		}
		checkLink(t, "read_thread", thread.Messages, id, want[0], want[1])
	}

	var waiting struct {
		Threads []linked `json:"threads"`
	}
	if call(t, cs, "threads_awaiting_reply", nil, &waiting) || len(waiting.Threads) != 3 {
		t.Fatalf("threads_awaiting_reply = %+v, want three", waiting)
	}
	checkLink(t, "threads_awaiting_reply", waiting.Threads, ids.odd, "a/b%c+d@x.test", oddURL)
	checkLink(t, "threads_awaiting_reply", waiting.Threads, ids.none, "", "")
	checkLink(t, "threads_awaiting_reply", waiting.Threads, ids.digits, "12345", "")
	if call(t, cs, "threads_awaiting_others", nil, &waiting) || len(waiting.Threads) != 1 {
		t.Fatalf("threads_awaiting_others = %+v, want one", waiting)
	}
	checkLink(t, "threads_awaiting_others", waiting.Threads, ids.sent, "s+1/2@x.test", "/mail/message/s%2B1%2F2%40x.test")

	at := pimNow.Add(time.Hour).Format(time.RFC3339)
	for id, want := range map[int64][2]string{ids.odd: {"a/b%c+d@x.test", oddURL}, ids.none: {"", ""}} {
		var reminded linked
		if call(t, cs, "remind_me", map[string]any{"message_id": id, "at": at}, &reminded) {
			t.Fatalf("remind_me %d answered an error", id)
		}
		checkLink(t, "remind_me", []linked{reminded}, id, want[0], want[1])
	}

	var task struct {
		MessageID       int64  `json:"message_id"`
		MessageIDHeader string `json:"message_id_header"`
		MessageURL      string `json:"message_url"`
	}
	if call(t, cs, "read_task", map[string]any{"task_id": ids.task}, &task) || task.MessageID != ids.odd ||
		task.MessageIDHeader != "a/b%c+d@x.test" || task.MessageURL != oddURL {
		t.Errorf("read_task = %+v, want message %d's link", task, ids.odd)
	}
}
