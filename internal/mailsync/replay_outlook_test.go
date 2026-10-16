package mailsync

import (
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/store"
)

// The user's Outlook.com test account (plan 0010, Phase 2), signed in
// with OAuth and keeping 30 days, over two sessions recorded 2026-10-16.
// Exchange's IMAP has no CONDSTORE, so every pass reads the flags of what
// it holds; it has no SPECIAL-USE but marks Sent, Drafts, Deleted and
// Junk, and Archive goes by its name; it keeps no keywords, so a color is
// \Flagged alone on the server and stays local (Quirks.NoKeywords).
func TestReplayOutlook(t *testing.T) {
	ctx := t.Context()
	recorded := time.Date(2026, 10, 16, 2, 0, 0, 0, time.UTC)
	e := newReplayEnvWith(t, store.Account{Kind: api.AccountKindMicrosoft, Email: "ann@outlook.com",
		Auth: api.AuthKindOAuth2, SyncDays: 30}, Config{Now: func() time.Time { return recorded }})
	var s *imapx.Session
	full := func() {
		t.Helper()
		if _, err := e.a.fullPass(ctx, s); err != nil {
			t.Fatal(err)
		}
		e.loadMailboxes()
	}
	refreshInbox := func() {
		t.Helper()
		mb, err := e.a.mailbox(ctx, e.mbs["INBOX"])
		if err != nil {
			t.Fatal(err)
		}
		if err := e.a.refresh(ctx, s, mb); err != nil {
			t.Fatal(err)
		}
	}
	replayed := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if err := e.a.replay(ctx, s); err != nil {
			t.Fatal(err)
		}
	}

	// Session 1: the first sync, two refreshes IDLE asked for, a pass.
	s, done := e.session("outlook-1.txt")
	full()
	for path, role := range map[string]api.MailboxRole{
		"INBOX": api.MailboxRoleInbox, "Drafts": api.MailboxRoleDrafts, "Sent": api.MailboxRoleSent,
		"Archive": api.MailboxRoleArchive, "Junk": api.MailboxRoleJunk, "Deleted": api.MailboxRoleTrash,
		"Notes": api.MailboxRoleNone, "Outbox": api.MailboxRoleNone, "Sync Issues/Conflicts": api.MailboxRoleNone,
	} {
		if got := e.paths(`role = '` + string(role) + `' AND path = '` + path + `'`); len(got) != 1 {
			t.Errorf("%s does not have the role %s", path, role)
		}
	}
	// Exchange answered the first fetch of Deleted's three messages (its
	// stand-ins for calendar items) with nothing.
	for path, want := range map[string]int{"INBOX": 33, "Sent": 1, "Deleted": 0} {
		if got := e.members(path); got != want {
			t.Errorf("after the first pass %s holds %d messages, want %d", path, got, want)
		}
	}
	if got := e.count(`SELECT COUNT(*) FROM mailboxes WHERE highestmodseq <> 0`); got != 0 {
		t.Errorf("%d mailboxes store a modseq; Exchange has no CONDSTORE", got)
	}
	refreshInbox()
	refreshInbox()
	replayed(nil)
	full()
	if got := e.members("Deleted"); got != 3 {
		t.Errorf("the next pass left Deleted with %d messages, want the 3 the first fetch missed", got)
	}
	done()

	// Session 2: mailctl flag, mv and sync on the message in Sent.
	s, done = e.session("outlook-2.txt")
	replayed(nil)
	full()
	refreshInbox()
	probe := e.uid("Sent", 78)
	flags := func() (seen, flagged bool, color int) {
		t.Helper()
		e.query(func(tx *store.Tx) error {
			return tx.QueryRowContext(ctx, `SELECT seen, flagged, flag_color FROM messages WHERE id = ?`, probe).
				Scan(&seen, &flagged, &color)
		})
		return seen, flagged, color
	}
	// A color: STORE +FLAGS (\Flagged), no $MailFlagBit; the pass that
	// reads \Flagged back keeps the color.
	replayed(e.a.m.SetFlags(ctx, []int64{probe}, store.FlagChange{Color: new(4)}))
	full()
	if seen, flagged, color := flags(); !seen || !flagged || color != 4 {
		t.Errorf("after the color: seen %v flagged %v color %d", seen, flagged, color)
	}
	replayed(e.a.m.SetFlags(ctx, []int64{probe}, store.FlagChange{Seen: new(false)}))
	full()
	if seen, _, color := flags(); seen || color != 4 {
		t.Errorf("after unread: seen %v color %d", seen, color)
	}
	// MOVE with UIDPLUS's COPYUID: the same row, under the new mailbox's UID.
	replayed(e.a.m.Move(ctx, []int64{probe}, 0, e.mbs["Archive"]))
	full()
	if got := e.uid("Archive", 7); got != probe {
		t.Fatalf("Archive's UID 7 is message %d, want %d", got, probe)
	}
	replayed(e.a.m.Move(ctx, []int64{probe}, 0, e.mbs["Sent"]))
	full()
	if got := e.uid("Sent", 84); got != probe || !slices.Equal(e.in(probe), []string{"Sent"}) {
		t.Fatalf("Sent's UID 84 is message %d in %v, want %d in Sent alone", got, e.in(probe), probe)
	}
	replayed(e.a.m.SetFlags(ctx, []int64{probe}, store.FlagChange{Seen: new(true), Flagged: new(false)}))
	full()
	done()
	if seen, flagged, color := flags(); !seen || flagged || color != 0 {
		t.Errorf("at the end: seen %v flagged %v color %d", seen, flagged, color)
	}
	if got := e.count(`SELECT COUNT(*) FROM pending_ops`); got != 0 {
		t.Errorf("%d operations left", got)
	}
}
