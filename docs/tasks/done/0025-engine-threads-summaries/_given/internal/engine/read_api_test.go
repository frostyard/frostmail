package engine_test

// CONTRACT TEST for task card T-0025 (docs/tasks): message.summaries,
// thread.messages and the view query's role and threads fields, over the
// socket. Do not edit.

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// conversation stores one account with an INBOX and an Archive:
//
//	ids[0] INBOX   Oct 1  "Plan"      <a@x>
//	ids[1] INBOX   Oct 2  "Re: Plan"  replies to a
//	ids[2] INBOX   Oct 3  "Other"
//	ids[3] Archive Oct 4  "Re: Plan"  replies to a
//
// so ids 0, 1 and 3 form one thread.
func conversation(t *testing.T, srv *rpctest.Server, c *api.Client) []int64 {
	t.Helper()
	ctx := t.Context()
	a, err := c.Account().Create(ctx, createParams("test1@mailtest.test"))
	if err != nil {
		t.Fatal(err)
	}
	day := func(d int) time.Time { return time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC) }
	hdr := func(uid uint32, d int, msgid, irt, subject string) store.MessageHeader {
		h := store.MessageHeader{UID: uid, InternalDate: day(d), Date: day(d), MessageID: msgid, Subject: subject,
			From: store.Address{Name: "Ann", Addr: "ann@mailtest.test"}, Preview: subject}
		if irt != "" {
			h.InReplyTo, h.References = irt, []string{irt}
		}
		return h
	}
	var ids []int64
	err = srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{
			{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true},
			{Path: "Archive", Role: api.MailboxRoleArchive, Selectable: true},
		})
		if err != nil {
			return err
		}
		byPath := map[string]int64{}
		for _, mb := range mbs {
			byPath[mb.Path] = mb.ID
		}
		in, err := tx.InsertHeaders(ctx, a.ID, byPath["INBOX"], []store.MessageHeader{
			hdr(1, 1, "a@x", "", "Plan"), hdr(2, 2, "b@x", "a@x", "Re: Plan"), hdr(3, 3, "c@x", "", "Other"),
		})
		if err != nil {
			return err
		}
		arc, err := tx.InsertHeaders(ctx, a.ID, byPath["Archive"], []store.MessageHeader{hdr(1, 4, "d@x", "a@x", "Re: Plan")})
		if err != nil {
			return err
		}
		ids = append(in, arc...)
		_, err = tx.AssignThreads(ctx, a.ID, ids)
		return err
	})
	if err != nil || len(ids) != 4 {
		t.Fatalf("fixture: %v, ids %v", err, ids)
	}
	return ids
}

func summaryIDs(rows []api.MessageSummary) []int64 {
	out := make([]int64, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

func TestMessageSummaries(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ids := conversation(t, srv, c)
	rows, err := c.Message().Summaries(t.Context(), &api.MessageSummariesParams{IDs: []int64{ids[2], 999999, ids[0]}})
	if err != nil {
		t.Fatal(err)
	}
	if got := summaryIDs(rows); !slices.Equal(got, []int64{ids[2], ids[0]}) {
		t.Fatalf("summaries = %v, want %v (order given, unknown IDs skipped)", got, []int64{ids[2], ids[0]})
	}
	if rows[0].ThreadCount != 1 || rows[1].ThreadCount != 3 || rows[1].Subject != "Plan" || rows[1].From.Address != "ann@mailtest.test" {
		t.Fatalf("rows = %+v", rows)
	}
	empty, err := c.Message().Summaries(t.Context(), &api.MessageSummariesParams{IDs: nil})
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("no IDs = %#v, %v; want an empty list", empty, err)
	}
}

func TestThreadMessages(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ids := conversation(t, srv, c)
	first, err := c.Message().Summaries(t.Context(), &api.MessageSummariesParams{IDs: ids[:1]})
	if err != nil || len(first) != 1 || first[0].ThreadID == 0 {
		t.Fatalf("summary of the first message = %+v, %v", first, err)
	}
	rows, err := c.Thread().Messages(t.Context(), &api.ThreadMessagesParams{ID: first[0].ThreadID})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := summaryIDs(rows), []int64{ids[0], ids[1], ids[3]}; !slices.Equal(got, want) {
		t.Fatalf("thread = %v, want %v (oldest first, every mailbox)", got, want)
	}
	if _, err := c.Thread().Messages(t.Context(), &api.ThreadMessagesParams{ID: 999999}); !errors.Is(err, api.ErrNotFound) {
		t.Fatalf("unknown thread error = %v, want notFound", err)
	}
}

func TestViewRoleAndThreads(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ids := conversation(t, srv, c)
	ctx := t.Context()
	yes := true
	inbox := api.MailboxRoleInbox
	cases := []struct {
		name string
		q    api.ViewQuery
		want []int64
	}{
		{"every inbox", api.ViewQuery{Role: &inbox}, []int64{ids[2], ids[1], ids[0]}},
		{"threads in every inbox", api.ViewQuery{Role: &inbox, Threads: &yes}, []int64{ids[2], ids[1]}},
		{"threads everywhere", api.ViewQuery{Threads: &yes}, []int64{ids[3], ids[2]}},
	}
	for _, tc := range cases {
		v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: tc.q})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		rows, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 10})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := summaryIDs(rows); v.Count != int64(len(tc.want)) || !slices.Equal(got, tc.want) {
			t.Errorf("%s: count %d rows %v, want %v", tc.name, v.Count, got, tc.want)
		}
		if tc.name == "threads everywhere" && (len(rows) == 0 || rows[0].ThreadCount != 3) {
			t.Errorf("%s: first row %+v, want threadCount 3", tc.name, rows)
		}
	}
}
