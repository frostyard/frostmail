package engine_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// sortFixture stores three messages in one inbox: Zed's beta (day 1, 300
// bytes), amy's flagged Re: alpha (day 2, 100 bytes, attachment), and
// Mike's read gamma (day 3, 200 bytes), and returns their IDs in that order.
func sortFixture(t *testing.T, srv *rpctest.Server, c *api.Client) []int64 {
	t.Helper()
	ctx := t.Context()
	a, err := c.Account().Create(ctx, createParams("sort@mailtest.test"))
	if err != nil {
		t.Fatal(err)
	}
	day := func(d int) time.Time { return time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC) }
	var ids []int64
	err = srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, a.ID, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		ids, err = tx.InsertHeaders(ctx, a.ID, mbs[0].ID, []store.MessageHeader{
			{UID: 1, InternalDate: day(1), Date: day(1), Subject: "beta", Size: 300, MessageID: "a@x",
				From: store.Address{Name: "Zed", Addr: "zed@x.test"}, To: []store.Address{{Addr: "carol@x.test"}}},
			{UID: 2, InternalDate: day(2), Date: day(2), Subject: "Re: alpha", Size: 100, MessageID: "b@x", HasAttachments: true,
				From: store.Address{Addr: "amy@x.test"}, To: []store.Address{{Name: "Bob", Addr: "bob@x.test"}},
				Flags: store.Flags{Flagged: true, Color: 1}},
			{UID: 3, InternalDate: day(3), Date: day(3), Subject: "gamma", Size: 200, MessageID: "c@x",
				From: store.Address{Name: "Mike", Addr: "mike@x.test"}, To: []store.Address{{Name: "Al", Addr: "al@x.test"}},
				Flags: store.Flags{Seen: true}},
		})
		return err
	})
	if err != nil || len(ids) != 3 {
		t.Fatalf("fixture: %v, %v", ids, err)
	}
	return ids
}

// TestViewSort: a view orders by each sort, either way, ties by date.
func TestViewSort(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ids := sortFixture(t, srv, c)
	ctx := t.Context()
	zed, amy, mike := ids[0], ids[1], ids[2]
	list := func(sort api.ViewSort, ascending bool) []int64 {
		t.Helper()
		q := api.ViewQuery{Sort: &sort}
		if ascending {
			q.Ascending = &ascending
		}
		v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: q})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c.View().Close(ctx, &api.ViewCloseParams{ID: v.ID}) }()
		rows, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 10})
		if err != nil {
			t.Fatal(err)
		}
		return summaryIDs(rows)
	}
	for _, tc := range []struct {
		sort      api.ViewSort
		ascending bool
		want      []int64
	}{
		{api.ViewSortDate, false, []int64{mike, amy, zed}},
		{api.ViewSortDate, true, []int64{zed, amy, mike}},
		{api.ViewSortFrom, true, []int64{amy, mike, zed}}, // amy@x.test, Mike, Zed
		{api.ViewSortFrom, false, []int64{zed, mike, amy}},
		{api.ViewSortTo, true, []int64{mike, amy, zed}}, // Al, Bob, carol@x.test
		{api.ViewSortSubject, true, []int64{amy, zed, mike}},
		{api.ViewSortSize, false, []int64{zed, mike, amy}},
		{api.ViewSortFlags, false, []int64{amy, mike, zed}},
		{api.ViewSortUnread, false, []int64{amy, zed, mike}}, // unread first, newest first among them
		{api.ViewSortAttachments, false, []int64{amy, mike, zed}},
	} {
		if got := list(tc.sort, tc.ascending); !slices.Equal(got, tc.want) {
			t.Errorf("sort %s ascending=%v: %v, want %v", tc.sort, tc.ascending, got, tc.want)
		}
	}
	bad := api.ViewSort("color")
	if _, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Sort: &bad}}); code(err) != api.CodeInvalidParams {
		t.Errorf("an unknown sort: %v", err)
	}
	// Threads go by their row's message.
	threads, sort := true, api.ViewSortSize
	v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Threads: &threads, Sort: &sort}})
	if err != nil {
		t.Fatal(err)
	}
	if rows, _ := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 10}); !slices.Equal(summaryIDs(rows), []int64{zed, mike, amy}) {
		t.Errorf("threads by size = %v", summaryIDs(rows))
	}
}

// TestMessageSourceAndSave: Raw Source, All Headers and Save As read the
// stored message; a message with no body stored is unavailable without sync.
func TestMessageSourceAndSave(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	ids := sortFixture(t, srv, c)
	ctx := t.Context()
	raw := "From: Zed <zed@x.test>\r\nSubject: beta\r\nX-Odd: caf\xe9\r\n\r\nHello.\r\n"
	blobID, err := srv.Blobs.Put(ctx, strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.DB.Tx(ctx, func(tx *store.Tx) error { return tx.SetBody(ctx, ids[0], blobID) }); err != nil {
		t.Fatal(err)
	}
	src, err := c.Message().Source(ctx, &api.MessageSourceParams{ID: ids[0]})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(src.Text, "From: Zed") || src.Truncated || !strings.Contains(src.Text, "Hello.") {
		t.Errorf("source = %+v", src)
	}
	if src.Headers != "From: Zed <zed@x.test>\r\nSubject: beta\r\nX-Odd: caf�" {
		t.Errorf("headers = %q", src.Headers)
	}
	path := filepath.Join(t.TempDir(), "beta.eml")
	if err := c.Message().Save(ctx, &api.MessageSaveParams{ID: ids[0], Path: path}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != raw {
		t.Errorf("saved %q, %v", got, err)
	}
	if err := c.Message().Save(ctx, &api.MessageSaveParams{ID: ids[0], Path: "beta.eml"}); code(err) != api.CodeInvalidParams {
		t.Errorf("a relative path: %v", err)
	}
	if _, err := c.Message().Source(ctx, &api.MessageSourceParams{ID: 9999}); code(err) != api.CodeNotFound {
		t.Errorf("an unknown message: %v", err)
	}
}
