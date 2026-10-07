package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestOpenMigratesAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frostmail.db")
	d, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if v, err := d.SchemaVersion(t.Context()); err != nil || v != len(ms) {
		t.Fatalf("SchemaVersion = %d, %v; want %d", v, err, len(ms))
	}
	_ = d.Close()
	d, err = Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	_ = d.Close()
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frostmail.db")
	d, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.db.Exec(`INSERT INTO schema_migrations VALUES (999, 'future.sql', 'x')`); err != nil {
		t.Fatal(err)
	}
	_ = d.Close()
	if _, err := Open(t.Context(), path); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("Open = %v, want a newer-schema error", err)
	}
}

func TestFTS5Available(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO messages_fts (rowid, subject, body_text) VALUES (7, 'Café receipts', 'naïve totals')`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var rowid int64
	if err := d.db.QueryRowContext(ctx, `SELECT rowid FROM messages_fts WHERE messages_fts MATCH 'cafe AND naive'`).Scan(&rowid); err != nil || rowid != 7 {
		t.Fatalf("diacritic-folded match = %d, %v; want 7", rowid, err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM messages_fts WHERE rowid = 7`)
		return err
	}); err != nil {
		t.Fatalf("contentless delete: %v", err)
	}
}

func TestTxEmitsDurableEventsAfterCommit(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	err := d.Tx(ctx, func(tx *Tx) error {
		if err := tx.Emit(ctx, api.AccountChanged{ID: 1}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.MailboxChanged{ID: 2, AccountID: 1})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(*events) != 2 || (*events)[0].Seq != 1 || (*events)[1].Seq != 2 || (*events)[0].Event != "account.changed" {
		t.Fatalf("events = %+v", *events)
	}
	latest, err := d.LatestSeq(ctx)
	if err != nil || latest != 2 {
		t.Fatalf("LatestSeq = %d, %v", latest, err)
	}
	replay, err := d.ChangesSince(ctx, 1)
	if err != nil || len(replay) != 1 || replay[0].Seq != 2 || string(replay[0].Data) != `{"id":2,"accountId":1,"deleted":false}` {
		t.Fatalf("ChangesSince(1) = %+v, %v", replay, err)
	}
}

func TestTxRollbackDropsEvents(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	boom := errors.New("boom")
	err := d.Tx(ctx, func(tx *Tx) error {
		if err := tx.Emit(ctx, api.AccountChanged{ID: 1}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Tx = %v", err)
	}
	if len(*events) != 0 {
		t.Fatalf("rolled-back events delivered: %+v", *events)
	}
	if latest, _ := d.LatestSeq(ctx); latest != 0 {
		t.Fatalf("rolled-back event kept seq %d", latest)
	}
}

func TestPruneKeepsLatestSeq(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	for i := range 5 {
		if err := d.Tx(ctx, func(tx *Tx) error { return tx.Emit(ctx, api.AccountChanged{ID: int64(i)}) }); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.PruneChanges(ctx, 2); err != nil {
		t.Fatal(err)
	}
	oldest, _ := d.OldestSeq(ctx)
	latest, _ := d.LatestSeq(ctx)
	if oldest != 4 || latest != 5 {
		t.Fatalf("after prune oldest=%d latest=%d, want 4 and 5", oldest, latest)
	}
}

func TestUniqueViolationDetection(t *testing.T) {
	d, _ := openTest(t)
	ctx := context.Background()
	insert := func(tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO accounts (kind, email, auth, imap_host, imap_port, imap_tls, imap_username,
			smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
			VALUES ('imap', 'A@x.test', 'password', 'h', 993, 'tls', 'u', 'h', 587, 'starttls', 'u', 'now')`)
		return err
	}
	if err := d.Tx(ctx, insert); err != nil {
		t.Fatal(err)
	}
	err := d.Tx(ctx, insert)
	if !IsUniqueViolation(err) {
		t.Fatalf("duplicate insert error %v is not a unique violation", err)
	}
}
