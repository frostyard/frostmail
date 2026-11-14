package mailsync

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
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

// Two headers of 1 batch that name 1 stored Gmail message at 2 new UIDs:
// the second move takes the first UID's row. The batch fails and stores
// nothing, as it did when each header's UID was looked up by itself; it
// creates no message, so the check must not depend on a created one.
func TestGmailBatchThatMovesOneMessageTwiceFails(t *testing.T) {
	e := newGmailEnv(t)
	ctx := t.Context()
	all := store.Mailbox{ID: e.mbs[gAll], AccountID: e.a.acct.ID, Path: gAll}
	at := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	header := func(uid uint32) store.MessageHeader {
		return store.MessageHeader{UID: uid, GmMsgID: 111, GmThrID: 900, Subject: "Twice", MessageID: "twice@mail.gmail.com",
			From: store.Address{Name: "Ann", Addr: "ann@gmail.com"}, Date: at, InternalDate: at}
	}
	added, err := e.a.insertGmail(ctx, all, true, []store.MessageHeader{header(10)}, false)
	if err != nil || len(added) != 1 {
		t.Fatalf("the first insert = %v, %v", added, err)
	}
	_, err = e.a.insertGmail(ctx, all, true, []store.MessageHeader{header(20), header(21)}, false)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("the batch = %v, want ErrNotFound for the UID left with no message", err)
	}
	if n := e.count(`SELECT COUNT(*) FROM message_mailbox WHERE mailbox_id = ? AND uid = 10`, all.ID); n != 1 {
		t.Errorf("%d memberships at UID 10 after the failed batch, want the stored 1", n)
	}
}

// Delete of messages of 2 accounts in 1 call: each account's answers (its
// Trash, whether it is Gmail) are asked once and kept by account, so one
// account's must not serve the other.
func TestDeleteAcrossTwoAccounts(t *testing.T) {
	e := newGmailEnv(t)
	ctx := t.Context()
	e.g.add(gAll, "Doomed", 0, `\Inbox`, "Work")
	e.pass()
	server := store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: "bob"}
	var other store.Account
	var inbox, trash int64
	e.query(func(tx *store.Tx) error {
		var err error
		other, err = tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: "bob@x.test", Auth: api.AuthKindPassword, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		mbs, err := tx.ReplaceMailboxes(ctx, other.ID, []store.ServerMailbox{
			{Path: "INBOX", Delimiter: "/", Role: api.MailboxRoleInbox, Selectable: true},
			{Path: "Trash", Delimiter: "/", Role: api.MailboxRoleTrash, Selectable: true},
		})
		if err != nil {
			return err
		}
		for _, mb := range mbs {
			switch mb.Role {
			case api.MailboxRoleInbox:
				inbox = mb.ID
			case api.MailboxRoleTrash:
				trash = mb.ID
			}
		}
		at := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
		_, err = tx.InsertHeaders(ctx, other.ID, inbox, []store.MessageHeader{{UID: 1, Subject: "Plain", MessageID: "plain@x.test",
			From: store.Address{Name: "Bob", Addr: "bob@x.test"}, Date: at, InternalDate: at}})
		return err
	})
	gmailID, plainID := e.id("Doomed"), e.id("Plain")

	if err := e.a.m.Delete(ctx, []int64{plainID, gmailID}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Gmail's Trash", e.subjects(gTrash), []string{"Doomed"})
	eq(t, "Gmail's Inbox label", e.subjects("INBOX"), nil)
	eq(t, "Gmail's Work label", e.subjects("Work"), nil)
	if n := e.count(`SELECT COUNT(*) FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?`, plainID, trash); n != 1 {
		t.Errorf("the plain message has %d memberships in its account's Trash, want 1", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM message_mailbox WHERE message_id = ? AND mailbox_id != ?`, plainID, trash); n != 0 {
		t.Errorf("the plain message has %d memberships outside its account's Trash", n)
	}
	for name, acct := range map[string]int64{"Gmail": e.a.acct.ID, "plain": other.ID} {
		if n := e.count(`SELECT COUNT(*) FROM pending_ops WHERE account_id = ? AND kind = ?`, acct, opMove); n != 1 {
			t.Errorf("the %s account has %d move ops queued, want 1", name, n)
		}
	}
}
