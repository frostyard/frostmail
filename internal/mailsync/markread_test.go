package mailsync_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
)

func rpcCode(err error) api.ErrorCode {
	var e *api.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 0
}

// Mark All Messages as Read marks every unread message of the mailbox, at
// once in the store and then on the server, and no message of another
// mailbox. The mailbox holds more unread mail than one flags op carries
// (500), so the marks go out as several.
func TestMarkMailboxRead(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem) // 5 in INBOX
	const more = 600
	for i := range more {
		raw := fmt.Sprintf("From: a@x.test\r\nTo: b@x.test\r\nSubject: Note %d\r\nMessage-ID: <n%d@x.test>\r\nDate: Mon, 09 Nov 2026 09:41:00 +0000\r\n\r\nNote\r\n", i, i)
		flags := []imap.Flag{}
		if i%100 == 0 {
			flags = append(flags, imap.FlagSeen) // 6 are read already
		}
		if _, err := mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{Flags: flags}); err != nil {
			t.Fatal(err)
		}
	}
	if err := mem.User.Create("Other", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.User.Append("Other", strings.NewReader("From: a@x.test\r\nSubject: Elsewhere\r\nMessage-ID: <o@x.test>\r\n\r\nUnread\r\n"), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	const unread = 5 + more - 6
	inbox := h.inbox()
	if inbox.Unread != unread {
		t.Fatalf("unread before = %d, want %d", inbox.Unread, unread)
	}
	otherBox := func() api.Mailbox {
		list, err := h.c.Mailbox().List(ctx, &api.MailboxListParams{})
		if err != nil {
			t.Fatal(err)
		}
		for _, mb := range list {
			if mb.Path == "Other" {
				return mb
			}
		}
		t.Fatal("no Other mailbox")
		return api.Mailbox{}
	}

	n, err := h.c.Mailbox().MarkRead(ctx, &api.MailboxMarkReadParams{ID: inbox.ID})
	if err != nil || n != unread {
		t.Fatalf("markRead = %d, %v; want %d", n, err, unread)
	}
	// The local change is immediate, and stays in its mailbox.
	if got := h.inbox(); got.Unread != 0 {
		t.Fatalf("unread right after = %d", got.Unread)
	}
	if got := otherBox(); got.Unread != 1 {
		t.Errorf("another mailbox's unread = %d, want 1", got.Unread)
	}
	if n, err := h.c.Mailbox().MarkRead(ctx, &api.MailboxMarkReadParams{ID: inbox.ID}); err != nil || n != 0 {
		t.Errorf("a second markRead = %d, %v; want 0", n, err)
	}

	server, err := imapx.Open(ctx, mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if _, err := server.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}
	uids := make([]uint32, 5+more)
	for i := range uids {
		uids[i] = uint32(i + 1)
	}
	waitUntil(t, 20*time.Second, "every message read on the server", func() bool {
		ups, err := server.FetchFlags(ctx, uids, 0)
		if err != nil || len(ups) != len(uids) {
			return false
		}
		for _, up := range ups {
			if !up.Flags.Seen {
				return false
			}
		}
		return true
	})
	if _, err := server.Select(ctx, "Other"); err != nil {
		t.Fatal(err)
	}
	if ups, err := server.FetchFlags(ctx, []uint32{1}, 0); err != nil || len(ups) != 1 || ups[0].Flags.Seen {
		t.Errorf("the other mailbox's message on the server = %+v, %v", ups, err)
	}
}

func TestMarkMailboxReadRefuses(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	if _, err := h.c.Mailbox().MarkRead(ctx, &api.MailboxMarkReadParams{ID: 99999}); rpcCode(err) != api.CodeNotFound {
		t.Errorf("an unknown mailbox: %v", err)
	}
	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, ReadOnly: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	inbox := h.inbox()
	if _, err := h.c.Mailbox().MarkRead(ctx, &api.MailboxMarkReadParams{ID: inbox.ID}); rpcCode(err) != api.CodeConflict {
		t.Errorf("a read-only account: %v", err)
	}
	if got := h.inbox(); got.Unread != inbox.Unread || h.hasQueuedOps() {
		t.Errorf("a refused markRead changed the unread count to %d or queued ops", got.Unread)
	}
}
