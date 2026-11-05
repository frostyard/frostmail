//go:build linux

package mailsync_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/std/problems"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/problemstest"
	"github.com/frostyard/frostmail/internal/store"
)

// installRefusal adds, once, a trigger that makes the store refuse every new
// message in a mailbox, as a broken database would, while the row in
// test_refuse says so; refuseInserts and allowInserts flip that row.
//
// The switch is a row, not a CREATE and DROP of the trigger: SQLite checks a
// CREATE against the schema that the pooled connection cached, and BEGIN
// IMMEDIATE does not refresh it, so a trigger dropped on one connection can
// still "already exist" on another.
func installRefusal(t *testing.T, db *store.DB) {
	t.Helper()
	err := db.Tx(t.Context(), func(tx *store.Tx) error {
		if _, err := tx.ExecContext(t.Context(), `CREATE TABLE test_refuse (refuse INTEGER NOT NULL)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(t.Context(), `INSERT INTO test_refuse (refuse) VALUES (0)`); err != nil {
			return err
		}
		_, err := tx.ExecContext(t.Context(), `CREATE TRIGGER test_refuse_switch BEFORE INSERT ON message_mailbox WHEN (SELECT refuse FROM test_refuse) BEGIN SELECT RAISE(ABORT, 'test: the store refuses the insert'); END`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func refuseInserts(t *testing.T, db *store.DB) {
	t.Helper()
	execStore(t, db, `UPDATE test_refuse SET refuse = 1`)
}

func allowInserts(t *testing.T, db *store.DB) {
	t.Helper()
	execStore(t, db, `UPDATE test_refuse SET refuse = 0`)
}

func execStore(t *testing.T, db *store.DB, sql string) {
	t.Helper()
	err := db.Tx(t.Context(), func(tx *store.Tx) error {
		_, err := tx.ExecContext(t.Context(), sql)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The refusal switch works from any connection: two handles are two
// connections with separate schema caches, as the store's pool can give.
func TestRefusalSwitchAcrossConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frostmail.db")
	a, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	b, err := store.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	installRefusal(t, a)
	refuseInserts(t, a)
	allowInserts(t, b)
	refuseInserts(t, a)
	allowInserts(t, b)
	refuseInserts(t, a)
}

// A store write that fails to apply the server's state records
// sync.apply_failed once in a streak of failed passes, whose error text is
// the record's MESSAGE; a healthy pass starts a new streak.
func TestApplyFailureRecordsOncePerStreak(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	j := problemstest.Listen(t)
	cfg := testConfig
	cfg.Problems = j.Recorder(t, problems.ComponentDaemon)
	h := newHarnessWith(t, mem.DialOptions(), "wrong", cfg)
	// The first sessions fail to sign in: no store write runs, no record.
	h.waitPhase(api.SyncPhaseUnauthorized)
	j.None(t)

	installRefusal(t, h.srv.DB)
	refuseInserts(t, h.srv.DB)
	if err := h.c.Account().SetPassword(t.Context(), &api.AccountSetPasswordParams{ID: h.acct, Password: imapxtest.Password}); err != nil {
		t.Fatal(err)
	}
	// The first failed pass records; the next ones in the streak do not.
	st := h.waitPhase(api.SyncPhaseOffline)
	if st.Error == nil || !strings.Contains(*st.Error, "test: the store refuses the insert") {
		t.Fatalf("offline status = %+v", st)
	}
	rec := j.NextError(t, problems.ComponentDaemon, mailsync.CodeApplyFailed)
	if !strings.Contains(rec.Message(), "test: the store refuses the insert") {
		t.Errorf("MESSAGE = %q", rec.Message())
	}
	h.waitPhase(api.SyncPhaseOffline)
	h.waitPhase(api.SyncPhaseOffline)
	j.None(t)

	// A healthy pass ends the streak.
	allowInserts(t, h.srv.DB)
	h.waitPhase(api.SyncPhaseIdle)
	j.None(t)

	// A new failure records again.
	refuseInserts(t, h.srv.DB)
	msg := "From: a@mailtest.test\r\nSubject: late\r\nMessage-ID: <late@mailtest.test>\r\n\r\nlate\r\n"
	if _, err := mem.User.Append("INBOX", bytes.NewReader([]byte(msg)), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := h.c.Sync().Now(t.Context(), &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseOffline)
	j.NextError(t, problems.ComponentDaemon, mailsync.CodeApplyFailed)
	h.waitPhase(api.SyncPhaseOffline)
	j.None(t)
}
