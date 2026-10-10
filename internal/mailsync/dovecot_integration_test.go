//go:build integration

package mailsync_test

// Runs against the frostmail-mailtest container (make engine-it) as
// test2@mailtest.test, whose INBOX is empty in the clean snapshot; the imapx
// integration tests use test1, so the two packages can run in parallel.

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
)

func dovecotTest2(t *testing.T) imapx.DialOptions {
	host := os.Getenv("FROSTMAIL_IT_HOST")
	if host == "" {
		t.Skip("FROSTMAIL_IT_HOST is not set; run make engine-it")
	}
	return imapx.DialOptions{Host: host, Port: 993, TLS: api.TLSModeTLS, Username: "test2@mailtest.test",
		Password: "frostmail-test", InsecureSkipVerify: true}
}

func TestDovecotSyncAndLiveChanges(t *testing.T) {
	opts := dovecotTest2(t)
	ctx := t.Context()
	other, err := imapx.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	files, _ := filepath.Glob("../../dev/incus/seed/*.eml")
	slices.Sort(files)
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := other.Append(ctx, "INBOX", raw, nil); err != nil {
			t.Fatal(err)
		}
	}

	h := newHarness(t, opts, opts.Password)
	h.waitPhase(api.SyncPhaseIdle)
	inbox := h.inbox()
	if inbox.Total != 5 {
		t.Fatalf("INBOX total = %d, want 5", inbox.Total)
	}
	view, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	if _, err := other.Append(ctx, "INBOX", []byte("Subject: Live\r\nFrom: z@mailtest.test\r\nDate: Thu, 08 Oct 2026 09:00:00 +0000\r\n\r\nnew\r\n"), nil); err != nil {
		t.Fatal(err)
	}
	h.waitFor(5*time.Second, "new mail in the view", func(_ api.EventEnvelope, ev api.Event) bool {
		d, ok := ev.(api.ViewDelta)
		return ok && d.ID == view.ID && d.Count == 6
	})
	t.Logf("new mail: %v", time.Since(start))

	start = time.Now()
	uids, err := other.UIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.StoreFlags(ctx, uids[:1], []string{`\Seen`}, nil); err != nil {
		t.Fatal(err)
	}
	h.waitFor(5*time.Second, "flag change", func(_ api.EventEnvelope, ev api.Event) bool {
		c, ok := ev.(api.MessageChanged)
		return ok && len(c.IDs) == 1
	})
	t.Logf("flag change: %v", time.Since(start))

	start = time.Now()
	if err := other.StoreFlags(ctx, uids[1:2], []string{`\Deleted`}, nil); err != nil {
		t.Fatal(err)
	}
	if err := other.Expunge(ctx, uids[1:2]); err != nil {
		t.Fatal(err)
	}
	h.waitFor(5*time.Second, "expunge", func(_ api.EventEnvelope, ev api.Event) bool {
		r, ok := ev.(api.MessageRemoved)
		return ok && len(r.IDs) == 1
	})
	t.Logf("expunge: %v", time.Since(start))
	if got := h.inbox(); got.Total != 5 || got.Unread != 4 {
		t.Fatalf("INBOX after changes = %+v, want 5 messages, 4 unread", got)
	}
}

func TestDovecotMoveAndDelete(t *testing.T) {
	opts := dovecotTest2(t)
	opts.Username = "test3@mailtest.test"
	ctx := t.Context()
	other, err := imapx.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	for _, subj := range []string{"one", "two", "three"} {
		raw := []byte("Subject: " + subj + "\r\nFrom: z@mailtest.test\r\nMessage-ID: <" + subj + "@mailtest.test>\r\n\r\nbody\r\n")
		if _, err := other.Append(ctx, "INBOX", raw, nil); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t, opts, opts.Password)
	h.waitPhase(api.SyncPhaseIdle)
	mbs, err := h.c.Mailbox().List(ctx, &api.MailboxListParams{AccountID: &h.acct})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[api.MailboxRole]int64{}
	for _, mb := range mbs {
		ids[mb.Role] = mb.ID
	}
	inboxIDs := func() []int64 {
		v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: ptr(ids[api.MailboxRoleInbox])}})
		if err != nil {
			t.Fatal(err)
		}
		rows, _ := h.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: v.Count})
		_ = h.c.View().Close(ctx, &api.ViewCloseParams{ID: v.ID})
		var out []int64
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	msgs := inboxIDs() // three, newest first
	if len(msgs) != 3 {
		t.Fatalf("INBOX = %v", msgs)
	}

	// Move the newest to Archive: the local view changes at once, the
	// server follows, and the message keeps its ID (COPYUID).
	if err := h.c.Message().Move(ctx, &api.MessageMoveParams{IDs: msgs[:1], MailboxID: ids[api.MailboxRoleArchive]}); err != nil {
		t.Fatal(err)
	}
	if got := inboxIDs(); len(got) != 2 {
		t.Fatalf("INBOX right after move = %v", got)
	}
	waitUntil(t, 5*time.Second, "the move on the server", func() bool {
		st, err := other.Status(ctx, "Archive")
		return err == nil && st.Messages == 1
	})
	if m, err := h.c.Message().Get(ctx, &api.MessageGetParams{ID: msgs[0]}); err != nil || !slices.Equal(m.Summary.MailboxIDs, []int64{ids[api.MailboxRoleArchive]}) {
		t.Fatalf("moved message = %+v, %v", m, err)
	}

	// Delete the next one: it goes to Trash; deleting it there expunges it.
	if err := h.c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: msgs[1:2]}); err != nil {
		t.Fatal(err)
	}
	// message.changed twice: the local move, then the confirmed one.
	changes := 0
	h.waitFor(5*time.Second, "the confirmed move to Trash", func(_ api.EventEnvelope, ev api.Event) bool {
		if c, ok := ev.(api.MessageChanged); ok && slices.Contains(c.IDs, msgs[1]) {
			changes++
		}
		return changes == 2
	})
	if err := h.c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: msgs[1:2]}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "the expunge", func() bool {
		st, err := other.Status(ctx, "Trash")
		return err == nil && st.Messages == 0
	})
	if _, err := h.c.Message().Get(ctx, &api.MessageGetParams{ID: msgs[1]}); err == nil {
		t.Fatal("expunged message still exists locally")
	}

	// Copy the last one to Archive: it stays in INBOX, and the copy arrives
	// in Archive as a message of its own.
	if err := h.c.Message().Copy(ctx, &api.MessageCopyParams{IDs: msgs[2:], MailboxID: ids[api.MailboxRoleArchive]}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "the copy on the server", func() bool {
		st, err := other.Status(ctx, "Archive")
		return err == nil && st.Messages == 2
	})
	waitUntil(t, 5*time.Second, "the copy in Archive", func() bool {
		return len(h.subjects(ids[api.MailboxRoleArchive])) == 2
	})
	if got := inboxIDs(); !slices.Equal(got, msgs[2:]) {
		t.Fatalf("INBOX after the copy = %v, want %v", got, msgs[2:])
	}
}
