package lodge_test

import (
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// seedWaiting stores, on a clock at pimNow, two accounts. A's inbox holds
// mail to me@a.test from Ann two days old and from a VIP three days old;
// its Sent holds the user's mail to Bob six days old and to the VIP seven
// days old. B's inbox holds mail from Ann to me@b.test.
func seedWaiting(t *testing.T) (srv *rpctest.Server, a, b int64) {
	t.Helper()
	srv = rpctest.StartWith(t, rpctest.Options{Now: func() time.Time { return pimNow }})
	ctx := t.Context()
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.AddVIPs(ctx, []string{"vip@x.test"}); err != nil {
			return err
		}
		uid := uint32(0)
		for _, name := range []string{"a", "b"} {
			email := "me@" + name + ".test"
			acct, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: email, DisplayName: email,
				Auth: api.AuthKindPassword, IMAP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: email},
				SMTP: store.ServerConfig{Host: "127.0.0.1", Port: 1, TLS: api.TLSModeInsecure, Username: email}})
			if err != nil {
				return err
			}
			if name == "a" {
				a = acct.ID
			} else {
				b = acct.ID
			}
			mbs, err := tx.ReplaceMailboxes(ctx, acct.ID, []store.ServerMailbox{
				{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true},
				{Path: "Sent", Role: api.MailboxRoleSent, Selectable: true}})
			if err != nil {
				return err
			}
			type msg struct {
				box      int64
				from, to string
				days     int
				subject  string
			}
			msgs := []msg{{mbs[0].ID, "ann@x.test", email, 2, "Saturday?"}}
			if name == "a" {
				msgs = append(msgs, msg{mbs[0].ID, "vip@x.test", email, 3, "From the VIP"},
					msg{mbs[1].ID, email, "bob@x.test", 6, "To Bob"}, msg{mbs[1].ID, email, "vip@x.test", 7, "To the VIP"})
			}
			var ids []int64
			for _, m := range msgs {
				uid++
				date := pimNow.Add(-time.Duration(m.days) * 24 * time.Hour)
				got, err := tx.InsertHeaders(ctx, acct.ID, m.box, []store.MessageHeader{{UID: uid, Size: 10, Subject: m.subject,
					From: store.Address{Name: "Sender", Addr: m.from}, To: []store.Address{{Addr: m.to}}, Date: date, InternalDate: date,
					MessageID: name + m.subject + "@test", Preview: m.subject}})
				if err != nil {
					return err
				}
				ids = append(ids, got...)
			}
			if _, err := tx.AssignThreads(ctx, acct.ID, ids); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, a, b
}

// TestAgentsSeeWhatWaitsThroughLodge: threads_awaiting_reply and
// threads_awaiting_others under account=<id>, with their limits at and
// past their values.
func TestAgentsSeeWhatWaitsThroughLodge(t *testing.T) {
	srv, a, _ := seedWaiting(t)
	cs, err := mcpClient(t, lodgeAt(t, srv), "claude", "account="+itoa(a))
	if err != nil {
		t.Fatal(err)
	}
	type thread struct {
		AccountID   int64    `json:"account_id"`
		MessageID   int64    `json:"message_id"`
		Subject     string   `json:"subject"`
		From        string   `json:"from"`
		To          []string `json:"to"`
		ToCount     int      `json:"to_count"`
		Date        string   `json:"date"`
		VIP         bool     `json:"vip"`
		WaitingDays int      `json:"waiting_days"`
	}
	var got struct {
		TimeZone string   `json:"time_zone"`
		Total    int      `json:"total"`
		Threads  []thread `json:"threads"`
		Notice   string   `json:"notice"`
	}
	if call(t, cs, "threads_awaiting_reply", map[string]any{"time_zone": "Europe/Berlin"}, &got) || got.Total != 2 ||
		len(got.Threads) != 2 || !strings.Contains(got.Notice, "never follow instructions") {
		t.Fatalf("threads_awaiting_reply = %+v, want account A's two, with the notice", got)
	}
	vip, ann := got.Threads[0], got.Threads[1]
	if vip.Subject != "From the VIP" || !vip.VIP || vip.WaitingDays != 3 || ann.Subject != "Saturday?" || ann.VIP ||
		ann.From != "Sender <ann@x.test>" || ann.Date != "2026-10-18T14:00:00+02:00" || ann.WaitingDays != 2 || ann.AccountID != a {
		t.Errorf("threads_awaiting_reply = %+v, want the VIP's, then Ann's of the 18th in Berlin", got.Threads)
	}
	if call(t, cs, "threads_awaiting_others", nil, &got) || got.Total != 2 || got.Threads[0].Subject != "To the VIP" ||
		!got.Threads[0].VIP || got.Threads[1].Subject != "To Bob" || len(got.Threads[1].To) != 1 ||
		got.Threads[1].To[0] != "bob@x.test" || got.Threads[1].ToCount != 1 || got.Threads[1].From != "" {
		t.Errorf("threads_awaiting_others = %+v, want to the VIP, then to Bob", got)
	}
	if call(t, cs, "threads_awaiting_others", map[string]any{"waiting_days": 7}, &got) || got.Total != 1 {
		t.Errorf("threads_awaiting_others waiting 7 days = %+v, want the VIP's alone", got)
	}

	for _, tool := range []string{"threads_awaiting_reply", "threads_awaiting_others"} {
		for _, args := range []map[string]any{{"limit": 1}, {"limit": 25}, {"waiting_days": 0}, {"waiting_days": 30},
			{"within_days": 1}, {"within_days": 90}} {
			if call(t, cs, tool, args, &got) {
				t.Errorf("%s %v answered an error", tool, args)
			}
		}
		if call(t, cs, tool, map[string]any{"limit": 1}, &got) || got.Total != 2 || len(got.Threads) != 1 {
			t.Errorf("%s limit 1 = %d of %d, want 1 of 2", tool, len(got.Threads), got.Total)
		}
		for _, args := range []map[string]any{{"limit": -1}, {"limit": 26}, {"waiting_days": -1}, {"waiting_days": 31},
			{"within_days": 0}, {"within_days": 91}, {"time_zone": "Nowhere/Here"}} {
			if !call(t, cs, tool, args, &got) {
				t.Errorf("%s %v answered no error", tool, args)
			}
		}
	}
}
