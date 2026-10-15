package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// schemaAt returns a new database at schema version n, opened without
// Open, so a test can fill it before the later migrations run.
func schemaAt(t *testing.T, n int) (string, *sql.DB) {
	t.Helper()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "frostmail.db")
	raw, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL) STRICT`); err != nil {
		t.Fatal(err)
	}
	for _, m := range ms[:n] {
		if _, err := raw.ExecContext(ctx, m.sql); err != nil {
			t.Fatalf("%s: %v", m.name, err)
		}
		if _, err := raw.ExecContext(ctx, `INSERT INTO schema_migrations VALUES (?, ?, 'x')`, m.version, m.name); err != nil {
			t.Fatal(err)
		}
	}
	return path, raw
}

// TestMigration12KeepsQueuedOps: rebuilding pending_ops for the mailbox op
// kinds keeps the queued actions of a database at schema 11, and the
// messages they cover, and takes the new kinds.
func TestMigration12KeepsQueuedOps(t *testing.T) {
	ctx := t.Context()
	path, raw := schemaAt(t, 11)
	for _, q := range []string{
		`INSERT INTO accounts (id, kind, email, display_name, auth, imap_host, imap_port, imap_tls, imap_username,
			smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
			VALUES (1, 'imap', 'a@x.test', '', 'password', 'h', 993, 'tls', 'u', 'h', 465, 'tls', 'u', 'x')`,
		`INSERT INTO mailboxes (id, account_id, path, delimiter, name, role, attrs_json, selectable, subscribed)
			VALUES (1, 1, 'INBOX', '/', 'INBOX', 'inbox', '[]', 1, 1)`,
		`INSERT INTO messages (id, account_id, subject, from_name, from_addr, internal_date, preview, size)
			VALUES (7, 1, 's', '', 'b@x.test', '2026-10-01T00:00:00Z', '', 1)`,
		`INSERT INTO pending_ops (id, account_id, kind, payload_json, next_try_at, created_at)
			VALUES (3, 1, 'flags', '{}', 'x', 'x')`,
		`INSERT INTO pending_op_messages (op_id, message_id) VALUES (3, 7)`,
	} {
		if _, err := raw.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	_ = raw.Close()

	d, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	var ops, covered int
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pending_ops WHERE id = 3 AND kind = 'flags'`).Scan(&ops); err != nil || ops != 1 {
		t.Fatalf("queued ops after the migration = %d, %v", ops, err)
	}
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pending_op_messages WHERE op_id = 3 AND message_id = 7`).Scan(&covered); err != nil || covered != 1 {
		t.Fatalf("covered messages after the migration = %d, %v", covered, err)
	}
	err = d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.QueueOp(ctx, 1, "mbrename", map[string]string{"path": "A", "to": "B"}, nil)
		return err
	})
	if err != nil {
		t.Fatalf("a mailbox op after the migration: %v", err)
	}
	// Deleting an op still drops the messages it covered.
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.DeleteOp(ctx, 3) }); err != nil {
		t.Fatal(err)
	}
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pending_op_messages`).Scan(&covered); err != nil || covered != 0 {
		t.Fatalf("covered messages after the delete = %d, %v", covered, err)
	}
}

// TestMigration16ListsGmailCalendarsAgain: Gmail accounts' calendars lose
// their sync state, so the next pass lists them again; their objects, and
// other collections and accounts, are left as they are.
func TestMigration16ListsGmailCalendarsAgain(t *testing.T) {
	ctx := t.Context()
	path, raw := schemaAt(t, 15)
	for _, q := range []string{
		`INSERT INTO accounts (id, kind, email, display_name, auth, imap_host, imap_port, imap_tls, imap_username,
			smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
			VALUES (1, 'gmail', 'a@gmail.test', '', 'oauth2', 'h', 993, 'tls', 'u', 'h', 465, 'tls', 'u', 'x'),
			(2, 'icloud', 'b@icloud.test', '', 'password', 'h', 993, 'tls', 'u', 'h', 465, 'tls', 'u', 'x')`,
		`INSERT INTO collections (id, account_id, kind, href, sync_token, ctag, synced_at) VALUES
			(1, 1, 'calendar', '/events/', 'token-1', 'ctag-1', 'x'),
			(2, 1, 'addressbook', '/book/', 'token-2', 'ctag-2', 'x'),
			(3, 2, 'calendar', '/home/', 'token-3', 'ctag-3', 'x')`,
		`INSERT INTO objects (collection_id, href, etag, kind, raw, updated_at) VALUES (1, '/events/a.ics', '"1"', 'vevent', x'00', 'x')`,
	} {
		if _, err := raw.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	_ = raw.Close()

	d, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	want := map[int64]string{1: "||x", 2: "token-2|ctag-2|x", 3: "token-3|ctag-3|x"}
	for id, w := range want {
		var got string
		err := d.db.QueryRowContext(ctx, `SELECT sync_token || '|' || ctag || '|' || synced_at FROM collections WHERE id = ?`, id).Scan(&got)
		if err != nil || got != w {
			t.Errorf("collection %d's sync state = %q, %v; want %q", id, got, err, w)
		}
	}
	var objects int
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM objects`).Scan(&objects); err != nil || objects != 1 {
		t.Errorf("objects after the migration = %d, %v", objects, err)
	}
}
