package store

import (
	"testing"
	"time"
)

// TestListDateFollowsArrival: a stored message's list date is its arrival
// date until a Remind Me reminder fires (migration 0011), however it was
// inserted.
func TestListDateFollowsArrival(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct, inbox, _ := mailboxFixture(t, d)
	ids := insertHeaders(t, d, acct, inbox, header(7, "Lunch"))
	var listDate, internal string
	if err := d.db.QueryRowContext(ctx, `SELECT list_date, internal_date FROM messages WHERE id = ?`, ids[0]).
		Scan(&listDate, &internal); err != nil {
		t.Fatal(err)
	}
	if listDate == "" || listDate != internal {
		t.Errorf("list_date = %q, want the internal date %q", listDate, internal)
	}
}

// TestQueueOutboxScheduled: a Send Later row keeps its scheduled mark; a
// plain send has none.
func TestQueueOutboxScheduled(t *testing.T) {
	d, acct, draft := outboxFixture(t)
	later := item(acct, draft, testNow.Add(24*time.Hour), "later@x")
	later.Scheduled = true
	got := queue(t, d, later)
	if !got.Scheduled {
		t.Errorf("QueueOutbox(scheduled) = %+v, want Scheduled", got)
	}
	read, err := d.GetOutbox(t.Context(), got.ID)
	if err != nil || !read.Scheduled {
		t.Errorf("GetOutbox = %+v, %v; want Scheduled", read, err)
	}
	now := queue(t, d, item(acct, draft, testNow.Add(10*time.Second), "now@x"))
	if now.Scheduled {
		t.Errorf("QueueOutbox = %+v, want not Scheduled", now)
	}
}
