package mailsync_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
)

// viewRows lists a view's rows.
func viewRows(t *testing.T, h *harness, v *api.ViewInfo, count int64) []api.MessageSummary {
	t.Helper()
	rows, err := h.c.View().Range(t.Context(), &api.ViewRangeParams{ID: v.ID, Start: 0, End: count})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func rowIDs(rows []api.MessageSummary) []int64 {
	out := make([]int64, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

// The list's filters: a message read in an Unread view stays until the view
// is reopened (deleted ones still leave: TestViewIDsKeep). Attachments
// filter by the flag the headers set.
func TestFilteredViews(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	inbox := h.inbox()

	attached, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID, HasAttachments: ptr(true)}})
	if err != nil || attached.Count != 1 {
		t.Fatalf("with attachments = %+v, %v", attached, err)
	}
	without, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID, HasAttachments: ptr(false)}})
	if err != nil || without.Count != 4 {
		t.Fatalf("without attachments = %+v, %v", without, err)
	}
	// A search's has:attachment gives way to the query's own field.
	both, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Text: ptr("has:attachment"), HasAttachments: ptr(false)}})
	if err != nil || both.Count != 4 {
		t.Fatalf("has:attachment with hasAttachments false = %+v, %v", both, err)
	}

	// One message read before the view opens; the view lists the other 4.
	all := rowIDs(viewRows(t, h, openMailbox(t, h, inbox.ID), 5))
	seenBefore := all[4]
	setSeen := func(id int64, seen bool) {
		t.Helper()
		if err := h.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{id}, Changes: api.FlagChanges{Seen: ptr(seen)}}); err != nil {
			t.Fatal(err)
		}
	}
	setSeen(seenBefore, true)
	unread, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID, Unread: ptr(true)}})
	if err != nil || unread.Count != 4 {
		t.Fatalf("unread = %+v, %v", unread, err)
	}
	read := viewRows(t, h, unread, 4)[0].ID
	// Read a row, then mark the other unread: it joins, and the read row
	// stays (without Keep the delta would bring the count to 3, then 4).
	setSeen(read, true)
	setSeen(seenBefore, false)
	h.waitFor(5*time.Second, "the unread view's delta", func(_ api.EventEnvelope, ev api.Event) bool {
		d, ok := ev.(api.ViewDelta)
		return ok && d.ID == unread.ID && d.Count == 5
	})
	rows := viewRows(t, h, unread, 5)
	if got := rowIDs(rows); !slices.Equal(got, all) {
		t.Fatalf("unread view = %v, want every row %v", got, all)
	}
	if !rows[0].Flags.Seen {
		t.Fatalf("the read row = %+v, want seen", rows[0])
	}
	reopened, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID, Unread: ptr(true)}})
	if err != nil || reopened.Count != 4 {
		t.Fatalf("reopened unread view = %+v, %v; want the read row gone", reopened, err)
	}
}

// openMailbox opens a mailbox's unfiltered view.
func openMailbox(t *testing.T, h *harness, mailboxID int64) *api.ViewInfo {
	t.Helper()
	v, err := h.c.View().Open(t.Context(), &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &mailboxID}})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// Copy on a folder account: the source keeps its message, the server makes
// the copy, and the destination's sync brings it in as a message of its own.
func TestCopyBetweenFolders(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	inbox := h.inbox()
	mbs, err := h.c.Mailbox().List(ctx, &api.MailboxListParams{AccountID: &h.acct})
	if err != nil {
		t.Fatal(err)
	}
	var archive api.Mailbox
	for _, mb := range mbs {
		if mb.Role == api.MailboxRoleArchive {
			archive = mb
		}
	}
	v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID}})
	if err != nil {
		t.Fatal(err)
	}
	ids := rowIDs(viewRows(t, h, v, 5))

	// Copying into the mailbox that holds it does nothing.
	if err := h.c.Message().Copy(ctx, &api.MessageCopyParams{IDs: ids[:1], MailboxID: inbox.ID}); err != nil {
		t.Fatal(err)
	}
	if err := h.c.Message().Copy(ctx, &api.MessageCopyParams{IDs: ids[:2], MailboxID: archive.ID}); err != nil {
		t.Fatal(err)
	}
	other, err := imapx.Open(ctx, mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	waitUntil(t, 5*time.Second, "the copies on the server", func() bool {
		st, err := other.Status(ctx, "Archive")
		return err == nil && st.Messages == 2
	})
	if st, err := other.Status(ctx, "INBOX"); err != nil || st.Messages != 5 {
		t.Fatalf("server INBOX = %+v, %v", st, err)
	}
	waitUntil(t, 5*time.Second, "the copies in Archive", func() bool {
		return len(h.subjects(archive.ID)) == 2
	})
	if got := h.subjects(inbox.ID); len(got) != 5 {
		t.Fatalf("INBOX after the copy = %q", got)
	}
	copies := h.subjects(archive.ID)
	originals := h.subjects(inbox.ID)[:2]
	if !slices.Equal(copies, originals) {
		t.Fatalf("Archive = %q, want %q", copies, originals)
	}
	av, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &archive.ID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range viewRows(t, h, av, 2) {
		if slices.Contains(ids, r.ID) || !slices.Equal(r.MailboxIDs, []int64{archive.ID}) {
			t.Fatalf("copy %+v shares a row with the originals %v", r, ids)
		}
	}

	var apiErr *api.Error
	if err := h.c.Message().Copy(ctx, &api.MessageCopyParams{IDs: ids[:1], MailboxID: 9999}); !errors.As(err, &apiErr) || apiErr.Code != api.CodeInvalidParams {
		t.Fatalf("copy into a missing mailbox = %v, want invalid params", err)
	}
}
