package mailsync_test

import (
	"bytes"
	"net/mail"
	"slices"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
)

// dateOf is a sent message's Date header.
func dateOf(t *testing.T, raw []byte) time.Time {
	t.Helper()
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	d, err := m.Header.Date()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestSendLater: a send at a time past the undo delay waits in the outbox,
// scheduled, until then, and goes dated then; reschedule moves it and
// rebuilds it with the new Date; a time already past sends it now.
func TestSendLater(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	d := h.draft("Later", []api.Address{bob}, nil, nil)
	first := time.Now().Add(time.Hour).Truncate(time.Second)
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID, SendAt: &first})
	if err != nil || !item.Scheduled || item.SendAt == nil || !item.SendAt.Equal(first) {
		t.Fatalf("send later = %+v, %v", item, err)
	}
	soon := time.Now().Add(2 * time.Second).Truncate(time.Second).Add(time.Second)
	moved, err := h.c.Outbox().Reschedule(ctx, &api.OutboxRescheduleParams{ID: item.ID, SendAt: soon})
	if err != nil || !moved.Scheduled || !moved.SendAt.Equal(soon) {
		t.Fatalf("reschedule = %+v, %v", moved, err)
	}
	if n := len(h.smtp.Messages()); n != 0 {
		t.Fatalf("SMTP server got %d messages before the time", n)
	}
	h.waitOutbox(item.ID, api.OutboxStateSending)
	if now := time.Now(); now.Before(soon) {
		t.Errorf("sending at %v, before %v", now, soon)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent)
	msgs := h.smtp.Messages()
	if len(msgs) != 1 || !dateOf(t, msgs[0].Data).Equal(soon) {
		t.Fatalf("sent %d messages; Date %v, want %v", len(msgs), dateOf(t, msgs[0].Data), soon)
	}
	if _, err := h.c.Outbox().Reschedule(ctx, &api.OutboxRescheduleParams{ID: item.ID, SendAt: first}); !isCode(err, api.CodeConflict) {
		t.Errorf("reschedule of a sent message = %v, want conflict", err)
	}

	// A time already past is no Send Later: the message goes now.
	d2 := h.draft("Now", []api.Address{bob}, nil, nil)
	past := time.Now().Add(-time.Minute)
	now, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d2.ID, SendAt: &past})
	if err != nil || now.Scheduled {
		t.Fatalf("send at a past time = %+v, %v", now, err)
	}
	h.waitOutbox(now.ID, api.OutboxStateSent)
	if _, err := h.c.Outbox().Reschedule(ctx, &api.OutboxRescheduleParams{ID: 999, SendAt: first}); !isCode(err, api.CodeNotFound) {
		t.Errorf("reschedule of nothing = %v, want notFound", err)
	}
}

// TestSendLaterCanBeEdited: cancel returns a scheduled message's draft,
// and nothing goes.
func TestSendLaterCanBeEdited(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	d := h.draft("Edit me", []api.Address{bob}, nil, nil)
	at := time.Now().Add(time.Hour)
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID, SendAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	back, err := h.c.Outbox().Cancel(ctx, &api.OutboxCancelParams{ID: item.ID})
	if err != nil || back.ID != d.ID {
		t.Fatalf("cancel = %+v, %v", back, err)
	}
	if list, _ := h.c.Outbox().List(ctx, &api.OutboxListParams{}); len(list) != 0 {
		t.Errorf("outbox after the edit = %+v", list)
	}
}

// TestRemindMe: a reminder brings a filed message back to the top of the
// inbox at its time, on the server too, and announces it; maild refuses
// times past and read-only accounts; deleting a message drops its
// reminder.
func TestRemindMe(t *testing.T) {
	r := newRulesHarness(t)
	ctx := t.Context()
	receipts := r.mailboxByPath("Receipts")
	r.deliver("Ann <ann@x.test>", "Remind me")
	r.settledWith("Remind me")
	r.deliver("Bob <bob@x.test>", "Newer")
	r.settledWith("Newer")
	msg := r.summary("Remind me")
	if err := r.c.Message().Move(ctx, &api.MessageMoveParams{IDs: []int64{msg.ID}, MailboxID: receipts.ID}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "the move on the server", func() bool {
		st, err := r.mem.User.Status("Receipts", &imap.StatusOptions{NumMessages: true})
		return err == nil && *st.NumMessages == 1
	})

	at := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := r.c.Message().Remind(ctx, &api.MessageRemindParams{IDs: []int64{msg.ID}, At: &at}); err != nil {
		t.Fatal(err)
	}
	if got := r.summary("Remind me").RemindAt; got == nil || !got.Equal(at) {
		t.Fatalf("remindAt = %v, want %v", got, at)
	}
	past := time.Now().Add(-time.Minute)
	if err := r.c.Message().Remind(ctx, &api.MessageRemindParams{IDs: []int64{msg.ID}, At: &past}); !isCode(err, api.CodeInvalidParams) {
		t.Errorf("a time past = %v, want invalidParams", err)
	}
	if err := r.c.Message().Remind(ctx, &api.MessageRemindParams{IDs: []int64{9999}, At: &at}); !isCode(err, api.CodeNotFound) {
		t.Errorf("an unknown message = %v, want notFound", err)
	}

	// Not yet due: nothing happens.
	if err := r.srv.Sync.FireReminders(ctx, at.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := r.summary("Remind me"); got.RemindAt == nil || !slices.Equal(got.MailboxIDs, []int64{receipts.ID}) {
		t.Fatalf("before its time = %+v", got)
	}
	// Due, as at the first check after maild was stopped past its time.
	if err := r.srv.Sync.FireReminders(ctx, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	inbox := r.inbox()
	back := r.summary("Remind me")
	if back.RemindAt != nil || !slices.Equal(back.MailboxIDs, []int64{inbox.ID}) {
		t.Errorf("after its time = %+v; want back in the inbox, no reminder", back)
	}
	select {
	case mail := <-r.announced:
		if len(mail) != 1 || mail[0].Subject != "Reminder: Remind me" || mail[0].ID != msg.ID {
			t.Errorf("announced %+v", mail)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the reminder was not announced")
	}
	v, err := r.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := r.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 1}); err != nil || len(rows) != 1 || rows[0].ID != msg.ID {
		t.Errorf("the inbox's first row = %+v, %v; want the reminded message above Newer", rows, err)
	}
	waitUntil(t, 5*time.Second, "the message back in INBOX on the server", func() bool {
		st, err := r.mem.User.Status("Receipts", &imap.StatusOptions{NumMessages: true})
		return err == nil && *st.NumMessages == 0
	})

	// Clearing, and deleting, drop a reminder.
	newer := r.summary("Newer")
	if err := r.c.Message().Remind(ctx, &api.MessageRemindParams{IDs: []int64{msg.ID, newer.ID}, At: &at}); err != nil {
		t.Fatal(err)
	}
	if err := r.c.Message().Remind(ctx, &api.MessageRemindParams{IDs: []int64{msg.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := r.c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: []int64{newer.ID}}); err != nil {
		t.Fatal(err)
	}
	if due, err := r.srv.DB.DueMessageReminders(ctx, at.Add(time.Hour)); err != nil || len(due) != 0 {
		t.Errorf("reminders left = %+v, %v", due, err)
	}

	if _, err := r.c.Account().Update(ctx, &api.AccountUpdateParams{ID: r.acct, ReadOnly: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if err := r.c.Message().Remind(ctx, &api.MessageRemindParams{IDs: []int64{msg.ID}, At: &at}); !isCode(err, api.CodeConflict) {
		t.Errorf("a read-only account's message = %v, want conflict", err)
	}
}
