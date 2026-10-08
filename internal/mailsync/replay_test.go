package mailsync

import (
	"log/slog"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/replay"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/store"
)

// Replay tests (docs/design/testing.md): sessions recorded against Gmail
// and iCloud with MAILD_IMAP_TRACE, scrubbed by tools/imaprec into
// testdata/replay, are played back to the sync engine. A replay fails on
// any command the recording does not have and on any recorded command the
// engine no longer sends, so these hold the engine to the servers' real
// responses, which gmailModel and the memory server only imitate. Names,
// subjects and bodies in the scripts are fakes; counts, flags, labels,
// threads and mailbox state are as recorded.

// replayFirstSync plays a recorded first sync to a new account and
// returns the store as the pass left it.
func replayFirstSync(t *testing.T, script string, acct store.Account) *gmailEnv {
	t.Helper()
	ctx := t.Context()
	lines, err := replay.Load(filepath.Join("testdata", "replay", script))
	if err != nil {
		t.Fatal(err)
	}
	srv := replay.Start(t, lines)
	host, port, _ := net.SplitHostPort(srv.Addr())
	acct.IMAP.Host, acct.IMAP.TLS, acct.IMAP.Username = host, api.TLSModeInsecure, acct.Email
	if acct.IMAP.Port, err = strconv.Atoi(port); err != nil {
		t.Fatal(err)
	}
	acct.SMTP = acct.IMAP

	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	err = db.Tx(ctx, func(tx *store.Tx) error {
		acct, err = tx.InsertAccount(ctx, acct)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	m := New(db, nil, blob.New(filepath.Join(dir, "blobs")), slog.New(slog.DiscardHandler), Config{}, func([]api.EventEnvelope) {})
	a := &actor{m: m, acct: acct, unsaved: map[int64]time.Time{}}

	opts := a.dialOptions("app-password", false)
	s, err := imapx.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	a.gmail = providers.ForKind(acct.Kind).Quirks.Gmail && s.Caps.Gmail
	_, passErr := a.fullPass(ctx, s)
	_ = s.Close()
	if err := srv.Close(); err != nil || passErr != nil {
		t.Fatalf("pass: %v; replay: %v", passErr, err)
	}

	e := &gmailEnv{t: t, db: db, a: a, mbs: map[string]int64{}}
	e.query(func(tx *store.Tx) error {
		mbs, err := tx.ListMailboxes(ctx, acct.ID)
		for _, mb := range mbs {
			e.mbs[mb.Path] = mb.ID
		}
		return err
	})
	return e
}

// members counts a mailbox's messages.
func (e *gmailEnv) members(path string) int {
	e.t.Helper()
	id, ok := e.mbs[path]
	if !ok {
		e.t.Fatalf("no mailbox %q", path)
	}
	return e.count(`SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ?`, id)
}

// paths lists the account's mailboxes matching a SQL condition.
func (e *gmailEnv) paths(where string) []string {
	e.t.Helper()
	var out []string
	e.query(func(tx *store.Tx) error {
		rows, err := tx.QueryContext(e.t.Context(), `SELECT path FROM mailboxes WHERE `+where+` ORDER BY path`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p string
			if err := rows.Scan(&p); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out
}

// The throwaway Gmail account's first sync with an app password (LOGIN):
// nine messages in All Mail, Spam and Trash empty (Gmail's ESEARCH then
// returns no ALL), labels in X-GM-LABELS, threads in X-GM-THRID.
func TestReplayGmailFirstSync(t *testing.T) {
	e := replayFirstSync(t, "gmail-first-sync.txt",
		store.Account{Kind: api.AccountKindGmail, Email: "ann@gmail.com", Auth: api.AuthKindPassword})
	if !e.a.gmail {
		t.Fatal("X-GM-EXT-1 did not select the Gmail path")
	}
	e.invariants()
	// The one label of the user's own ("Work", scrubbed).
	own := e.paths(`path <> 'INBOX' AND path NOT LIKE '[Gmail]%'`)
	if len(own) != 1 {
		t.Fatalf("labels of the user's own = %q, want one", own)
	}
	for path, want := range map[string]int{
		gAll: 9, gSpam: 0, gTrash: 0,
		"INBOX": 6, own[0]: 4, "[Gmail]/Important": 5,
		"[Gmail]/Starred": 1, "[Gmail]/Drafts": 1, "[Gmail]/Sent Mail": 1,
	} {
		if got := e.members(path); got != want {
			t.Errorf("%s holds %d messages, want %d", path, got, want)
		}
	}
	for what, c := range map[string]struct {
		query string
		want  int
	}{
		"messages":                 {`SELECT COUNT(*) FROM messages`, 9},
		"messages with X-GM-MSGID": {`SELECT COUNT(*) FROM messages WHERE gm_msgid IS NOT NULL`, 9},
		"threads (X-GM-THRID)":     {`SELECT COUNT(DISTINCT thread_id) FROM messages`, 6},
		"unread":                   {`SELECT COUNT(*) FROM messages WHERE seen = 0`, 3},
		"flagged (\\Starred)":      {`SELECT COUNT(*) FROM messages WHERE flagged = 1`, 1},
		"dated":                    {`SELECT COUNT(*) FROM messages WHERE date_hdr IS NOT NULL`, 9},
		"with a sender":            {`SELECT COUNT(*) FROM messages WHERE from_addr <> ''`, 9},
		"with a preview":           {`SELECT COUNT(*) FROM messages WHERE preview <> ''`, 9},
	} {
		if got := e.count(c.query); got != c.want {
			t.Errorf("%s = %d, want %d", what, got, c.want)
		}
	}
	// The fast path is armed: the next pass compares against these.
	if got := e.count(`SELECT COUNT(*) FROM mailboxes WHERE path = ? AND uidvalidity = 12 AND uidnext = 15
		AND highestmodseq = 3150 AND server_count = 9`, gAll); got != 1 {
		t.Error("All Mail's UIDVALIDITY, UIDNEXT, HIGHESTMODSEQ and count were not stored as Gmail sent them")
	}
}

// The user's iCloud account read-only (EXAMINE), in the session that
// finished its first sync; imaprec kept the five newest messages each
// folder fetched. iCloud marks Sent Messages and Deleted Messages with
// \Sent and \Trash without offering SPECIAL-USE, and folders other
// clients made (Sent Items, Sent, Trash, Deleted Items) must not take
// those roles. iCloud answers EXAMINE with [READ-WRITE] and gives empty
// folders HIGHESTMODSEQ 0.
func TestReplayICloudFirstSync(t *testing.T) {
	e := replayFirstSync(t, "icloud-first-sync.txt",
		store.Account{Kind: api.AccountKindICloud, Email: "ann@icloud.com", Auth: api.AuthKindPassword, ReadOnly: true})
	if e.a.gmail {
		t.Fatal("an iCloud account took the Gmail path")
	}
	for path, want := range map[string]struct {
		role     string
		messages int
	}{
		"INBOX": {"inbox", 5}, "Drafts": {"drafts", 4}, "Sent Messages": {"sent", 5},
		"Archive": {"archive", 5}, "Junk": {"junk", 5}, "Deleted Messages": {"trash", 5},
		"Deleted Items": {"none", 1}, "Sent": {"none", 3}, "Sent Items": {"none", 0}, "Trash": {"none", 5},
		"Notes": {"none", 0}, "Notes/Recovered Items": {"none", 0},
	} {
		if got := e.members(path); got != want.messages {
			t.Errorf("%s holds %d messages, want %d", path, got, want.messages)
		}
		if got := e.paths(`role = '` + want.role + `' AND path = '` + path + `'`); len(got) != 1 {
			t.Errorf("%s does not have the role %s", path, want.role)
		}
	}
	for what, c := range map[string]struct {
		query string
		want  int
	}{
		"messages":       {`SELECT COUNT(*) FROM messages`, 38},
		"unread":         {`SELECT COUNT(*) FROM messages WHERE seen = 0`, 9},
		"drafts":         {`SELECT COUNT(*) FROM messages WHERE draft = 1`, 4},
		"dated":          {`SELECT COUNT(*) FROM messages WHERE date_hdr IS NOT NULL`, 38},
		"with a preview": {`SELECT COUNT(*) FROM messages WHERE preview <> ''`, 38},
		"threaded":       {`SELECT COUNT(*) FROM messages WHERE thread_id IS NOT NULL`, 38},
		"labels":         {`SELECT COUNT(*) FROM mailboxes WHERE is_gmail_label = 1`, 0},
	} {
		if got := e.count(c.query); got != c.want {
			t.Errorf("%s = %d, want %d", what, got, c.want)
		}
	}
	for path, state := range map[string][3]int64{ // UIDVALIDITY, UIDNEXT, HIGHESTMODSEQ
		"INBOX":                 {1309100931, 10451, 410545100049662},
		"Notes/Recovered Items": {7, 1, 0},
	} {
		if got := e.count(`SELECT COUNT(*) FROM mailboxes WHERE path = ? AND uidvalidity = ? AND uidnext = ?
			AND highestmodseq = ?`, path, state[0], state[1], state[2]); got != 1 {
			t.Errorf("%s's state was not stored as iCloud sent it (%v)", path, state)
		}
	}
}
