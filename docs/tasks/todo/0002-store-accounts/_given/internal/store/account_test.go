package store

// CONTRACT TEST for task card T-0002 (docs/tasks). Do not edit.

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

func sampleAccount(email string) Account {
	return Account{
		Kind:        api.AccountKindIMAP,
		Email:       email,
		DisplayName: "Test One",
		Auth:        api.AuthKindPassword,
		IMAP:        ServerConfig{Host: "imap.mailtest.test", Port: 993, TLS: api.TLSModeTLS, Username: email},
		SMTP:        ServerConfig{Host: "smtp.mailtest.test", Port: 587, TLS: api.TLSModeStartTLS, Username: email},
	}
}

func insertAccount(t *testing.T, d *DB, a Account) Account {
	t.Helper()
	ctx := context.Background()
	var out Account
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		out, err = tx.InsertAccount(ctx, a)
		return err
	})
	if err != nil {
		t.Fatalf("InsertAccount: %v", err)
	}
	return out
}

func TestAccountInsertAndGet(t *testing.T) {
	d, events := openTest(t)
	in := sampleAccount("test1@mailtest.test")
	in.ID = 99                     // ignored
	in.CreatedAt = time.Unix(0, 0) // ignored
	got := insertAccount(t, d, in)

	want := sampleAccount("test1@mailtest.test")
	want.ID = got.ID
	want.CreatedAt = testNow
	if got.ID <= 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("InsertAccount = %+v\nwant %+v", got, want)
	}
	read, err := d.GetAccount(context.Background(), got.ID)
	if err != nil || !reflect.DeepEqual(read, want) {
		t.Fatalf("GetAccount = %+v, %v\nwant %+v", read, err, want)
	}
	if len(*events) != 1 || (*events)[0].Event != "account.changed" || (*events)[0].Seq == 0 {
		t.Fatalf("events = %+v, want one durable account.changed", *events)
	}
	ev, err := api.DecodeEvent((*events)[0].Event, (*events)[0].Data)
	if err != nil || ev != (api.AccountChanged{ID: got.ID}) {
		t.Fatalf("event = %+v, %v", ev, err)
	}
}

func TestAccountCreatedAtIsUTCMillis(t *testing.T) {
	d, _ := openTest(t)
	d.Now = func() time.Time {
		return time.Date(2026, 10, 7, 8, 0, 0, 123456789, time.FixedZone("EDT", -4*3600))
	}
	got := insertAccount(t, d, sampleAccount("a@mailtest.test"))
	want := time.Date(2026, 10, 7, 12, 0, 0, 123000000, time.UTC)
	if !got.CreatedAt.Equal(want) || got.CreatedAt.Location() != time.UTC {
		t.Fatalf("CreatedAt = %v, want %v in UTC", got.CreatedAt, want)
	}
	read, _ := d.GetAccount(context.Background(), got.ID)
	if !read.CreatedAt.Equal(want) || read.CreatedAt.Location() != time.UTC {
		t.Fatalf("stored CreatedAt = %v, want %v in UTC", read.CreatedAt, want)
	}
}

func TestAccountDuplicateEmailConflicts(t *testing.T) {
	d, events := openTest(t)
	insertAccount(t, d, sampleAccount("dup@mailtest.test"))
	ctx := context.Background()
	err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.InsertAccount(ctx, sampleAccount("DUP@mailtest.test"))
		return err
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate email error = %v, want ErrConflict", err)
	}
	if len(*events) != 1 {
		t.Fatalf("events after a failed insert = %d, want 1", len(*events))
	}
}

func TestAccountGetMissing(t *testing.T) {
	d, _ := openTest(t)
	if _, err := d.GetAccount(context.Background(), 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAccount(42) = %v, want ErrNotFound", err)
	}
}

func TestAccountList(t *testing.T) {
	d, _ := openTest(t)
	list, err := d.ListAccounts(context.Background())
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty ListAccounts = %#v, %v; want a non-nil empty slice", list, err)
	}
	a := insertAccount(t, d, sampleAccount("a@mailtest.test"))
	b := insertAccount(t, d, sampleAccount("b@mailtest.test"))
	list, err = d.ListAccounts(context.Background())
	if err != nil || len(list) != 2 || list[0].ID != a.ID || list[1].ID != b.ID {
		t.Fatalf("ListAccounts = %+v, %v", list, err)
	}
	if !reflect.DeepEqual(list[1], b) {
		t.Fatalf("listed account = %+v\nwant %+v", list[1], b)
	}
}

func TestAccountUpdate(t *testing.T) {
	d, events := openTest(t)
	a := insertAccount(t, d, sampleAccount("a@mailtest.test"))
	ctx := context.Background()
	name := "Renamed"
	smtp := ServerConfig{Host: "smtp2.mailtest.test", Port: 465, TLS: api.TLSModeTLS, Username: "other"}
	var got Account
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		got, err = tx.UpdateAccount(ctx, a.ID, AccountUpdate{DisplayName: &name, SMTP: &smtp})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	want := a
	want.DisplayName = name
	want.SMTP = smtp
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UpdateAccount = %+v\nwant %+v", got, want)
	}
	if read, _ := d.GetAccount(ctx, a.ID); !reflect.DeepEqual(read, want) {
		t.Fatalf("after update GetAccount = %+v", read)
	}
	if n := len(*events); n != 2 || (*events)[1].Event != "account.changed" {
		t.Fatalf("events = %+v, want a second account.changed", *events)
	}

	err = d.Tx(ctx, func(tx *Tx) error {
		got, err = tx.UpdateAccount(ctx, a.ID, AccountUpdate{})
		return err
	})
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("empty update = %+v, %v; want the account unchanged", got, err)
	}

	err = d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.UpdateAccount(ctx, 4242, AccountUpdate{DisplayName: &name})
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("update of a missing account = %v, want ErrNotFound", err)
	}
}

func TestAccountDelete(t *testing.T) {
	d, events := openTest(t)
	a := insertAccount(t, d, sampleAccount("a@mailtest.test"))
	ctx := context.Background()
	if err := d.Tx(ctx, func(tx *Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO mailboxes (account_id, path, name) VALUES (?, 'INBOX', 'INBOX')`, a.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.DeleteAccount(ctx, a.ID) }); err != nil {
		t.Fatalf("DeleteAccount: %v", err)
	}
	if _, err := d.GetAccount(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetAccount after delete = %v", err)
	}
	if boxes, _ := d.ListMailboxes(ctx, 0); len(boxes) != 0 {
		t.Fatalf("mailboxes survived their account: %+v", boxes)
	}
	last := (*events)[len(*events)-1]
	ev, err := api.DecodeEvent(last.Event, last.Data)
	if err != nil || ev != (api.AccountChanged{ID: a.ID, Deleted: true}) {
		t.Fatalf("delete event = %+v, %v", ev, err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.DeleteAccount(ctx, a.ID) }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}
