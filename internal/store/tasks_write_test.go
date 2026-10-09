package store

import (
	"errors"
	"testing"
)

func TestGmailThreadMessage(t *testing.T) {
	f := newGmail(t)
	ctx := t.Context()
	f.insert(t, f.all, gmailHeader(1, 101, 900), gmailHeader(2, 102, 900), gmailHeader(3, 103, 901))
	id, err := f.d.GmailThreadMessage(ctx, f.acct, 900)
	if err != nil {
		t.Fatal(err)
	}
	var uid uint32
	if err := f.d.db.QueryRowContext(ctx, `SELECT mm.uid FROM message_mailbox mm WHERE mm.message_id = ?`, id).Scan(&uid); err != nil || uid != 2 {
		t.Errorf("thread 900's newest message has uid %d, %v; want 2", uid, err)
	}
	if _, err := f.d.GmailThreadMessage(ctx, f.acct, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown thread: %v", err)
	}
}

func TestTaskWriteHelpers(t *testing.T) {
	f := newTaskFixture(t)
	ctx := t.Context()
	report := f.ids["Report"]
	err := f.d.Tx(ctx, func(tx *Tx) error {
		if err := tx.SetObjectHref(ctx, report, "server-report"); err != nil {
			return err
		}
		if err := tx.ReparentTasks(ctx, f.a.ID, "p1", "server-report"); err != nil {
			return err
		}
		if _, err := tx.QueuePIMOp(ctx, PIMOp{AccountID: f.one.ID, CollectionID: f.a.ID, ObjectID: &report, Kind: "tasks.patch", Href: "x"}); err != nil {
			return err
		}
		n, err := tx.DropPIMOps(ctx, report)
		if n != 1 {
			t.Errorf("dropped %d ops", n)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if obj, err := f.d.ObjectByHref(ctx, f.a.ID, "server-report"); err != nil || obj.ID != report {
		t.Errorf("moved object = %+v, %v", obj, err)
	}
	if err := f.d.Tx(ctx, func(tx *Tx) error {
		return tx.IndexTask(ctx, report, TaskRow{UID: "server-report", Title: "Report", Position: "00002", Due: "2026-10-10"})
	}); err != nil {
		t.Fatal(err)
	}
	if charts, _ := f.d.Task(ctx, f.ids["Charts"]); charts.ParentID != report || charts.ParentUID != "server-report" {
		t.Errorf("Charts after the parent got its ID = %+v", charts)
	}
	if err := f.d.Tx(ctx, func(tx *Tx) error { return tx.SetObjectHref(ctx, 999999, "x") }); !errors.Is(err, ErrNotFound) {
		t.Errorf("moving an unknown object: %v", err)
	}
}
