package store

import (
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
)

// overflow is a SQL expression that fails with SQLite's "integer overflow"
// once a row reaches it: a store error partway through a statement.
const overflow = `abs(-9223372036854775808)`

// breakRoleChoices makes a read of a role choice fail partway: mailbox_roles
// becomes a view whose trash row overflows. The table is WITHOUT ROWID with
// (account_id, role) as its key, so the rows come in role order and trash is
// last.
func breakRoleChoices(t *testing.T, d *DB) {
	t.Helper()
	ctx := t.Context()
	err := d.Tx(ctx, func(tx *Tx) error {
		if _, err := tx.ExecContext(ctx, `ALTER TABLE mailbox_roles RENAME TO mailbox_roles_v`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `CREATE VIEW mailbox_roles AS
			SELECT account_id,
			       CASE WHEN role = 'trash' THEN `+overflow+` ELSE role END AS role,
			       path
			FROM mailbox_roles_v`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A role choice read that fails partway stops the listing and says so: the
// server's roles must not stand in for the choices the store did not give.
func TestReplaceMailboxesReportsARoleChoiceReadError(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct := addAccount(t, d, "a@mailtest.test")
	replace(t, d, acct, serverList)
	err := d.Tx(ctx, func(tx *Tx) error {
		for _, c := range [][2]string{{"drafts", "Docs"}, {"sent", "Sent"}, {"trash", "Trash"}} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO mailbox_roles (account_id, role, path) VALUES (?, ?, ?)`,
				acct, c[0], c[1]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	breakRoleChoices(t, d)
	err = d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.ReplaceMailboxes(ctx, acct, serverList)
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "mailbox roles") || !strings.Contains(err.Error(), "integer overflow") {
		t.Fatalf("ReplaceMailboxes = %v; want the store's \"mailbox roles\" error", err)
	}
}

// SetMailboxRole reports a store error of its role reset as a mailbox error.
// SQLite runs a whole UPDATE ... RETURNING on the first step and keeps its
// rows, so the trigger's abort comes back from QueryContext, before the rows
// are read: no injection makes the loop itself fail.
func TestSetMailboxRoleReportsAnUpdateError(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct := addAccount(t, d, "a@mailtest.test")
	var archive int64
	for _, mb := range replace(t, d, acct, serverList) {
		if mb.Path == "Archive/2026" {
			archive = mb.ID
		}
	}
	if archive == 0 {
		t.Fatal("the archive mailbox is missing")
	}
	err := d.Tx(ctx, func(tx *Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE mailboxes SET role = 'archive'
			WHERE account_id = ? AND path IN ('Archive/2026', 'Projects')`, acct); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `CREATE TRIGGER inject BEFORE UPDATE OF role ON mailboxes
			WHEN NEW.role = 'none' AND OLD.path = 'Projects' BEGIN SELECT RAISE(ABORT, 'injected'); END`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = d.Tx(ctx, func(tx *Tx) error { return tx.SetMailboxRole(ctx, archive, api.MailboxRoleArchive) })
	if err == nil || !strings.Contains(err.Error(), "set mailbox role") || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("SetMailboxRole = %v; want the store's \"set mailbox role\" error", err)
	}
}
