package mailsync_test

import (
	"slices"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
)

// serverMailboxes lists the memory server's mailbox names, sorted.
func serverMailboxes(t *testing.T, mem *imapxtest.Mem) []string {
	t.Helper()
	s, err := imapx.Open(t.Context(), mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list, err := s.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, mb := range list {
		out = append(out, mb.Path)
	}
	slices.Sort(out)
	return out
}

func serverCount(t *testing.T, mem *imapxtest.Mem, name string) uint32 {
	t.Helper()
	st, err := mem.User.Status(name, &imap.StatusOptions{NumMessages: true})
	if err != nil {
		return 0
	}
	return *st.NumMessages
}

// TestMailboxOperations: mailboxes made, renamed, moved and deleted here
// change at once and on the server; a mailbox with a role stays put.
func TestMailboxOperations(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	seed(t, mem)
	h := newHarnessWith(t, mem.DialOptions(), imapxtest.Password, testConfig)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()

	projects, err := h.c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: h.acct, Name: " Projects "})
	if err != nil || projects.Path != "Projects" || projects.Role != api.MailboxRoleNone {
		t.Fatalf("create = %+v, %v", projects, err)
	}
	waitUntil(t, 5*time.Second, "Projects on the server", func() bool {
		return slices.Contains(serverMailboxes(t, mem), "Projects")
	})
	q4, err := h.c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: h.acct, Name: "Q4", ParentID: &projects.ID})
	if err != nil || q4.Path != "Projects/Q4" || q4.Name != "Q4" {
		t.Fatalf("create inside = %+v, %v", q4, err)
	}
	waitUntil(t, 5*time.Second, "Projects/Q4 on the server", func() bool {
		return slices.Contains(serverMailboxes(t, mem), "Projects/Q4")
	})

	// Messages go into the new mailbox as into any other.
	inbox := h.inbox()
	ids := h.viewIDs(inbox.ID)
	if err := h.c.Message().Move(ctx, &api.MessageMoveParams{IDs: ids[:1], MailboxID: q4.ID}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "the message in Projects/Q4", func() bool { return serverCount(t, mem, "Projects/Q4") == 1 })
	h.waitReplayed()

	renamed, err := h.c.Mailbox().Rename(ctx, &api.MailboxRenameParams{ID: q4.ID, Name: "Q1"})
	if err != nil || renamed.Path != "Projects/Q1" || renamed.ID != q4.ID {
		t.Fatalf("rename = %+v, %v", renamed, err)
	}
	waitUntil(t, 5*time.Second, "Projects/Q1 on the server", func() bool {
		got := serverMailboxes(t, mem)
		return slices.Contains(got, "Projects/Q1") && !slices.Contains(got, "Projects/Q4")
	})
	h.waitReplayed()
	moved, err := h.c.Mailbox().Move(ctx, &api.MailboxMoveParams{ID: q4.ID})
	if err != nil || moved.Path != "Q1" {
		t.Fatalf("move to the top = %+v, %v", moved, err)
	}
	waitUntil(t, 5*time.Second, "Q1 at the top on the server", func() bool {
		return slices.Contains(serverMailboxes(t, mem), "Q1") && serverCount(t, mem, "Q1") == 1
	})
	// A full pass keeps the message and the mailbox's ID.
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	h.waitReplayed()
	if mb := h.mailboxByPath("Q1"); mb.ID != q4.ID || mb.Total != 1 {
		t.Errorf("Q1 after a pass = %+v", mb)
	}

	if err := h.c.Mailbox().Delete(ctx, &api.MailboxDeleteParams{ID: q4.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.Message().Get(ctx, &api.MessageGetParams{ID: ids[0]}); !isCode(err, api.CodeNotFound) {
		t.Errorf("the deleted mailbox's message: %v, want notFound", err)
	}
	waitUntil(t, 5*time.Second, "Q1 gone from the server", func() bool { return !slices.Contains(serverMailboxes(t, mem), "Q1") })

	for name, err := range map[string]error{
		"no name":       ignoreMailbox(h.c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: h.acct, Name: "  "})),
		"a delimiter":   ignoreMailbox(h.c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: h.acct, Name: "a/b"})),
		"unknown":       ignoreMailbox(h.c.Mailbox().Rename(ctx, &api.MailboxRenameParams{ID: 9999, Name: "X"})),
		"taken":         ignoreMailbox(h.c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: h.acct, Name: "Projects"})),
		"inbox":         ignoreMailbox(h.c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: h.acct, Name: "inbox"})),
		"rename Trash":  ignoreMailbox(h.c.Mailbox().Rename(ctx, &api.MailboxRenameParams{ID: h.mailboxByPath("Trash").ID, Name: "Bin"})),
		"delete Inbox":  h.c.Mailbox().Delete(ctx, &api.MailboxDeleteParams{ID: inbox.ID}),
		"inside itself": ignoreMailbox(h.c.Mailbox().Move(ctx, &api.MailboxMoveParams{ID: projects.ID, ParentID: &projects.ID})),
	} {
		want := map[string]api.ErrorCode{"no name": api.CodeInvalidParams, "a delimiter": api.CodeInvalidParams,
			"unknown": api.CodeNotFound}[name]
		if want == 0 {
			want = api.CodeConflict
		}
		if !isCode(err, want) {
			t.Errorf("%s: %v, want %d", name, err, want)
		}
	}
}

func ignoreMailbox(_ *api.Mailbox, err error) error { return err }

// TestMailboxRolesAndErase: Use This Mailbox For holds across listings, and
// Erase Deleted Items deletes a trash mailbox's messages for good.
func TestMailboxRolesAndErase(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	seed(t, mem)
	if err := mem.User.Create("Bin", nil); err != nil {
		t.Fatal(err)
	}
	h := newHarnessWith(t, mem.DialOptions(), imapxtest.Password, testConfig)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	bin, trash := h.mailboxByPath("Bin"), h.mailboxByPath("Trash")

	got, err := h.c.Mailbox().SetRole(ctx, &api.MailboxSetRoleParams{ID: bin.ID, Role: api.MailboxRoleTrash})
	if err != nil || got.Role != api.MailboxRoleTrash {
		t.Fatalf("setRole = %+v, %v", got, err)
	}
	if h.mailboxByPath("Trash").Role != api.MailboxRoleNone {
		t.Errorf("the server's Trash kept its role")
	}
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	if h.mailboxByPath("Bin").Role != api.MailboxRoleTrash || h.mailboxByPath("Trash").Role != api.MailboxRoleNone {
		t.Errorf("after a listing: Bin %s, Trash %s", h.mailboxByPath("Bin").Role, h.mailboxByPath("Trash").Role)
	}
	if _, err := h.c.Mailbox().SetRole(ctx, &api.MailboxSetRoleParams{ID: trash.ID, Role: api.MailboxRoleInbox}); !isCode(err, api.CodeInvalidParams) {
		t.Errorf("setRole inbox: %v, want invalidParams", err)
	}

	// Deleting now goes to Bin; erasing Bin deletes for good.
	ids := h.viewIDs(h.inbox().ID)
	if err := h.c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: ids[:2]}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "two messages in Bin on the server", func() bool { return serverCount(t, mem, "Bin") == 2 })
	n, err := h.c.Mailbox().Erase(ctx, &api.MailboxEraseParams{ID: bin.ID})
	if err != nil || n != 2 {
		t.Fatalf("erase = %d, %v", n, err)
	}
	if got := h.viewIDs(bin.ID); len(got) != 0 {
		t.Errorf("Bin after the erase lists %v", got)
	}
	waitUntil(t, 5*time.Second, "Bin empty on the server", func() bool { return serverCount(t, mem, "Bin") == 0 })
	if _, err := h.c.Mailbox().Erase(ctx, &api.MailboxEraseParams{ID: h.inbox().ID}); !isCode(err, api.CodeInvalidParams) {
		t.Errorf("erase the inbox: %v, want invalidParams", err)
	}

	// Favorites keep mailboxes in order and forget deleted ones.
	extra, err := h.c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: h.acct, Name: "Extra"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := h.c.Settings().Set(ctx, &api.SettingsSetParams{Favorites: []int64{extra.ID, bin.ID}})
	if err != nil || !slices.Equal(s.Favorites, []int64{extra.ID, bin.ID}) {
		t.Fatalf("favorites = %+v, %v", s, err)
	}
	waitUntil(t, 5*time.Second, "Extra on the server", func() bool { return slices.Contains(serverMailboxes(t, mem), "Extra") })
	h.waitReplayed()
	if err := h.c.Mailbox().Delete(ctx, &api.MailboxDeleteParams{ID: extra.ID}); err != nil {
		t.Fatal(err)
	}
	if s, err := h.c.Settings().Get(ctx, &api.SettingsGetParams{}); err != nil || !slices.Equal(s.Favorites, []int64{bin.ID}) {
		t.Errorf("favorites after a delete = %+v, %v", s, err)
	}
	for _, bad := range [][]int64{{bin.ID, bin.ID}, {9999}} {
		if _, err := h.c.Settings().Set(ctx, &api.SettingsSetParams{Favorites: bad}); err == nil {
			t.Errorf("favorites %v accepted", bad)
		}
	}

	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, ReadOnly: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.Mailbox().Create(ctx, &api.MailboxCreateParams{AccountID: h.acct, Name: "Nope"}); !isCode(err, api.CodeConflict) {
		t.Errorf("create on a read-only account: %v, want conflict", err)
	}
}

// viewIDs lists a mailbox's message IDs, newest first.
func (h *harness) viewIDs(mailboxID int64) []int64 {
	h.t.Helper()
	ctx := h.t.Context()
	v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &mailboxID}})
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = h.c.View().Close(ctx, &api.ViewCloseParams{ID: v.ID}) }()
	rows, err := h.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: v.Count})
	if err != nil {
		h.t.Fatal(err)
	}
	out := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// waitReplayed waits until the account has no op waiting for the server.
func (h *harness) waitReplayed() {
	h.t.Helper()
	waitUntil(h.t, 5*time.Second, "the ops replayed", func() bool {
		busy, err := h.srv.DB.HasQueuedOps(h.t.Context(), h.acct)
		return err == nil && !busy
	})
}
