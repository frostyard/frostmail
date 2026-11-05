//go:build linux

package mailsync_test

import (
	"bytes"
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

// refuseInserts makes the store refuse every new message in a mailbox, as
// a broken database would; allowInserts takes that back.
func refuseInserts(t *testing.T, h *harness) {
	t.Helper()
	execStore(t, h, `CREATE TRIGGER test_refuse BEFORE INSERT ON message_mailbox
		BEGIN SELECT RAISE(ABORT, 'test: the store refuses the insert'); END`)
}

func allowInserts(t *testing.T, h *harness) {
	t.Helper()
	execStore(t, h, `DROP TRIGGER test_refuse`)
}

func execStore(t *testing.T, h *harness, sql string) {
	t.Helper()
	err := h.srv.DB.Tx(t.Context(), func(tx *store.Tx) error {
		_, err := tx.ExecContext(t.Context(), sql)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
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

	refuseInserts(t, h)
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
	allowInserts(t, h)
	h.waitPhase(api.SyncPhaseIdle)
	j.None(t)

	// A new failure records again.
	refuseInserts(t, h)
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
