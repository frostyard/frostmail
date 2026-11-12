package mailsync

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/store"
)

// mailboxesColumns are the columns of the mailboxes table, in order.
var mailboxesColumns = []string{"id", "account_id", "path", "delimiter", "name", "role", "attrs_json",
	"is_gmail_label", "subscribed", "selectable", "uidvalidity", "uidnext", "highestmodseq",
	"server_count", "synced_low_uid", "needs_reconcile", "last_sync_at"}

// breakMailboxRow makes a read of one mailbox row fail on column: mailboxes
// becomes a view whose value for that column, on that row, overflows SQLite's
// integers ("integer overflow"). A query that does not read the column on
// that row is left alone.
func breakMailboxRow(t *testing.T, e *gmailEnv, column string, id int64) {
	t.Helper()
	if !slices.Contains(mailboxesColumns, column) {
		t.Fatalf("%q is not a mailboxes column", column)
	}
	expr := fmt.Sprintf(`CASE WHEN id = %d THEN abs(-9223372036854775808) ELSE %s END AS %s`, id, column, column)
	cols := slices.Clone(mailboxesColumns)
	cols[slices.Index(cols, column)] = expr
	ctx := e.t.Context()
	e.query(func(tx *store.Tx) error {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE mailboxes RENAME TO mailboxes_v`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `CREATE VIEW mailboxes AS SELECT `+strings.Join(cols, ", ")+` FROM mailboxes_v`)
		return err
	})
}

// A store error while Move checks its destination is reported as it is, not
// as a mailbox that is not in the account.
func TestMoveReportsAStoreErrorOfItsMailboxCheck(t *testing.T) {
	e := newGmailEnv(t)
	e.g.add(gAll, "A", 0, `\Inbox`)
	e.pass()
	to := e.mbs["[Gmail]/Important"]
	breakMailboxRow(t, e, "account_id", to)
	err := e.a.m.Move(t.Context(), []int64{e.id("A")}, 0, to)
	if err == nil || errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "integer overflow") {
		t.Fatalf("Move = %v; want the store's error, not ErrInvalid", err)
	}
}

// A store error while isGmail decides whether an account is Gmail is
// reported: the answer "not Gmail" must not come from a failed read, which
// would store a Gmail label as a folder.
func TestIsGmailReportsAStoreError(t *testing.T) {
	e := newGmailEnv(t)
	ctx := t.Context()
	breakMailboxRow(t, e, "is_gmail_label", e.mbs["INBOX"])
	err := e.db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.GmailAccount(ctx, e.a.acct.ID)
		return err
	})
	if err == nil {
		t.Fatal("GmailAccount read a broken row without an error: the fault is not injected")
	}
	gmail, err := e.a.m.isGmail(ctx, e.a.acct)
	if err == nil {
		t.Fatalf("isGmail = %v, nil; want the store's error", gmail)
	}
}
