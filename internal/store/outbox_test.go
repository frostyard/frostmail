package store

// CONTRACT TEST for task card T-0039 (docs/tasks). Do not edit.

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
)

func outboxFixture(t *testing.T) (*DB, int64, int64) {
	t.Helper()
	d, acct, _ := draftFixture(t)
	var dr Draft
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		var err error
		dr, err = tx.CreateDraft(t.Context(), Draft{AccountID: acct, Kind: "new", MessageID: "m@x"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return d, acct, dr.ID
}

func queue(t *testing.T, d *DB, o OutboxItem) OutboxItem {
	t.Helper()
	var out OutboxItem
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		var err error
		out, err = tx.QueueOutbox(t.Context(), o)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func item(acct, draft int64, sendAt time.Time, msgid string) OutboxItem {
	return OutboxItem{
		AccountID: acct, DraftID: draft, SendAt: sendAt, BlobID: "blob-" + msgid, MessageID: msgid,
		From: "a@mailtest.test", Recipients: []string{"ann@x.test", "bcc@x.test"}, Subject: "Hello",
		To: []Address{{Name: "Ann", Addr: "ann@x.test"}},
		// Ignored on insert:
		State: "sent", Attempts: 7, LastError: "old",
	}
}

func outboxIDs(os []OutboxItem) []int64 {
	var out []int64
	for _, o := range os {
		out = append(out, o.ID)
	}
	return out
}

func TestQueueAndGetOutbox(t *testing.T) {
	d, acct, draft := outboxFixture(t)
	sendAt := testNow.Add(10 * time.Second)
	got := queue(t, d, item(acct, draft, sendAt, "m1@x"))
	want := item(acct, draft, sendAt, "m1@x")
	want.ID, want.State, want.Attempts, want.LastError = got.ID, "queued", 0, ""
	want.CreatedAt, want.UpdatedAt = testNow, testNow
	if got.ID == 0 || !reflect.DeepEqual(got, want) {
		t.Fatalf("queued =\n%+v\nwant\n%+v", got, want)
	}
	read, err := d.GetOutbox(t.Context(), got.ID)
	if err != nil || !reflect.DeepEqual(read, want) {
		t.Fatalf("read = %+v, %v", read, err)
	}
	if _, err := d.GetOutbox(t.Context(), 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetOutbox(999) = %v, want ErrNotFound", err)
	}
}

func TestDueAndListOutbox(t *testing.T) {
	d, acct, draft := outboxFixture(t)
	late := queue(t, d, item(acct, draft, testNow.Add(time.Minute), "late@x"))
	early := queue(t, d, item(acct, 0, testNow.Add(time.Second), "early@x"))
	now := queue(t, d, item(acct, 0, testNow, "now@x"))
	due, err := d.DueOutbox(t.Context(), acct, testNow.Add(2*time.Second))
	if err != nil || !slices.Equal(outboxIDs(due), []int64{now.ID, early.ID}) {
		t.Fatalf("due = %v, %v; want the due ones, earliest first", outboxIDs(due), err)
	}
	if err := d.Tx(t.Context(), func(tx *Tx) error { return tx.MoveOutbox(t.Context(), now.ID, "queued", "sending") }); err != nil {
		t.Fatal(err)
	}
	due, _ = d.DueOutbox(t.Context(), acct, testNow.Add(time.Hour))
	if !slices.Equal(outboxIDs(due), []int64{early.ID, late.ID}) {
		t.Fatalf("due after one is sending = %v; only queued messages are due", outboxIDs(due))
	}
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		if err := tx.MoveOutbox(t.Context(), now.ID, "sending", "accepted"); err != nil {
			return err
		}
		return tx.MoveOutbox(t.Context(), now.ID, "accepted", "sent")
	}); err != nil {
		t.Fatal(err)
	}
	list, err := d.ListOutbox(t.Context(), acct)
	if err != nil || !slices.Equal(outboxIDs(list), []int64{late.ID, early.ID}) {
		t.Fatalf("list = %v, %v; want unsent messages in queue order", outboxIDs(list), err)
	}
	if all, _ := d.ListOutbox(t.Context(), 0); len(all) != 2 {
		t.Fatalf("list of every account = %v", outboxIDs(all))
	}
	if other, _ := d.DueOutbox(t.Context(), 999, testNow.Add(time.Hour)); len(other) != 0 {
		t.Fatalf("another account's due messages = %v", outboxIDs(other))
	}
	sent, err := d.OutboxInState(t.Context(), acct, "sent")
	if err != nil || !slices.Equal(outboxIDs(sent), []int64{now.ID}) {
		t.Fatalf("sent = %v, %v", outboxIDs(sent), err)
	}
}

func TestMoveOutboxIsCompareAndSet(t *testing.T) {
	d, acct, _ := outboxFixture(t)
	o := queue(t, d, item(acct, 0, testNow, "m@x"))
	later := testNow.Add(time.Minute)
	d.Now = func() time.Time { return later }
	err := d.Tx(t.Context(), func(tx *Tx) error { return tx.MoveOutbox(t.Context(), o.ID, "sending", "accepted") })
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("move from the wrong state = %v, want ErrConflict", err)
	}
	if err := d.Tx(t.Context(), func(tx *Tx) error { return tx.MoveOutbox(t.Context(), 999, "queued", "sending") }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("move of a missing message = %v, want ErrNotFound", err)
	}
	if err := d.Tx(t.Context(), func(tx *Tx) error { return tx.MoveOutbox(t.Context(), o.ID, "queued", "sending") }); err != nil {
		t.Fatal(err)
	}
	read, _ := d.GetOutbox(t.Context(), o.ID)
	if read.State != "sending" || !read.UpdatedAt.Equal(later) || !read.CreatedAt.Equal(testNow) {
		t.Fatalf("after move = %+v", read)
	}
}

func TestRetryFailRequeueOutbox(t *testing.T) {
	d, acct, _ := outboxFixture(t)
	o := queue(t, d, item(acct, 0, testNow, "m@x"))
	in := func(f func(tx *Tx) error) error { return d.Tx(t.Context(), f) }
	if err := in(func(tx *Tx) error { return tx.RetryOutboxLater(t.Context(), o.ID, testNow.Add(time.Minute), "x") }); !errors.Is(err, ErrConflict) {
		t.Fatalf("retry while queued = %v, want ErrConflict", err)
	}
	if err := in(func(tx *Tx) error { return tx.MoveOutbox(t.Context(), o.ID, "queued", "sending") }); err != nil {
		t.Fatal(err)
	}
	if err := in(func(tx *Tx) error {
		return tx.RetryOutboxLater(t.Context(), o.ID, testNow.Add(time.Minute), "421 try later")
	}); err != nil {
		t.Fatal(err)
	}
	read, _ := d.GetOutbox(t.Context(), o.ID)
	if read.State != "queued" || read.Attempts != 1 || read.LastError != "421 try later" || !read.SendAt.Equal(testNow.Add(time.Minute)) {
		t.Fatalf("after retry = %+v", read)
	}
	if err := in(func(tx *Tx) error { return tx.MoveOutbox(t.Context(), o.ID, "queued", "sending") }); err != nil {
		t.Fatal(err)
	}
	if err := in(func(tx *Tx) error { return tx.FailOutbox(t.Context(), o.ID, "550 no such user") }); err != nil {
		t.Fatal(err)
	}
	read, _ = d.GetOutbox(t.Context(), o.ID)
	if read.State != "failed" || read.Attempts != 2 || read.LastError != "550 no such user" {
		t.Fatalf("after fail = %+v", read)
	}
	if err := in(func(tx *Tx) error { return tx.FailOutbox(t.Context(), o.ID, "again") }); !errors.Is(err, ErrConflict) {
		t.Fatalf("fail while failed = %v, want ErrConflict", err)
	}
	if err := in(func(tx *Tx) error { return tx.RequeueOutbox(t.Context(), o.ID, testNow.Add(time.Hour)) }); err != nil {
		t.Fatal(err)
	}
	read, _ = d.GetOutbox(t.Context(), o.ID)
	if read.State != "queued" || read.Attempts != 2 || !read.SendAt.Equal(testNow.Add(time.Hour)) {
		t.Fatalf("after requeue = %+v; attempts keep counting", read)
	}
	if err := in(func(tx *Tx) error { return tx.RequeueOutbox(t.Context(), o.ID, testNow) }); !errors.Is(err, ErrConflict) {
		t.Fatalf("requeue while queued = %v, want ErrConflict", err)
	}
	if err := in(func(tx *Tx) error { return tx.RequeueOutbox(t.Context(), 999, testNow) }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("requeue of a missing message = %v, want ErrNotFound", err)
	}
}

func TestCancelOutbox(t *testing.T) {
	d, acct, draft := outboxFixture(t)
	o := queue(t, d, item(acct, draft, testNow.Add(10*time.Second), "m@x"))
	var cancelled OutboxItem
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		var err error
		cancelled, err = tx.CancelOutbox(t.Context(), o.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if cancelled.ID != o.ID || cancelled.DraftID != draft {
		t.Fatalf("cancelled = %+v; want the message as it was", cancelled)
	}
	if _, err := d.GetOutbox(t.Context(), o.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetOutbox after cancel = %v", err)
	}
	sending := queue(t, d, item(acct, 0, testNow, "s@x"))
	err := d.Tx(t.Context(), func(tx *Tx) error {
		if err := tx.MoveOutbox(t.Context(), sending.ID, "queued", "sending"); err != nil {
			return err
		}
		_, err := tx.CancelOutbox(t.Context(), sending.ID)
		return err
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("cancel while sending = %v, want ErrConflict", err)
	}
}

func TestOutboxDraftLinkClearsWithTheDraft(t *testing.T) {
	d, acct, draft := outboxFixture(t)
	o := queue(t, d, item(acct, draft, testNow, "m@x"))
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		_, err := tx.DeleteDraft(t.Context(), draft)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	read, err := d.GetOutbox(t.Context(), o.ID)
	if err != nil || read.DraftID != 0 {
		t.Fatalf("after the draft is deleted: %+v, %v; want DraftID 0", read, err)
	}
}
