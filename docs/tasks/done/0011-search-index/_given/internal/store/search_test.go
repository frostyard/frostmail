package store

// CONTRACT TEST for task card T-0011 (docs/tasks). Do not edit.

import (
	"slices"
	"testing"
)

func TestSearchQuery(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"   \t ", ""},
		{"invoice", `"invoice"*`},
		{"Invoice  March", `"Invoice"* "March"*`},
		{`say "hi"`, `"say"* """hi"""*`},
		{"foo OR bar", `"foo"* "OR"* "bar"*`},
		{"NOT -x", `"NOT"* "-x"*`},
		{"!!! invoice ???", `"invoice"*`},
		{"!!!", ""},
		{"café", `"café"*`},
	}
	for _, tc := range cases {
		if got := SearchQuery(tc.in); got != tc.want {
			t.Errorf("SearchQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// matches runs a SearchQuery against the index and returns sorted rowids.
func matches(t *testing.T, d *DB, input string) []int64 {
	t.Helper()
	rows, err := d.db.QueryContext(t.Context(), `SELECT rowid FROM messages_fts WHERE messages_fts MATCH ? ORDER BY rowid`, SearchQuery(input))
	if err != nil {
		t.Fatalf("MATCH %q: %v", SearchQuery(input), err)
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func index(t *testing.T, d *DB, id int64, doc SearchDoc) {
	t.Helper()
	ctx := t.Context()
	if err := d.Tx(ctx, func(tx *Tx) error { return tx.IndexMessage(ctx, id, doc) }); err != nil {
		t.Fatal(err)
	}
}

func TestIndexAndSearch(t *testing.T) {
	d, _ := openTest(t)
	index(t, d, 1, SearchDoc{Subject: "Café receipts", From: "Alice alice@mailtest.test", Body: "Totals for March"})
	index(t, d, 2, SearchDoc{Subject: "Lunch", To: "Bob bob@mailtest.test", AttachmentNames: "menu.pdf"})
	index(t, d, 3, SearchDoc{Subject: "Release plan", Body: "OR and NOT are words here"})

	for input, want := range map[string][]int64{
		"cafe":          {1},
		"rec caf":       {1},
		"alice":         {1},
		"bob":           {2},
		"menu":          {2},
		"march totals":  {1},
		"or not":        {3},
		"lunch release": nil,
		"zebra":         nil,
	} {
		if got := matches(t, d, input); !slices.Equal(got, want) {
			t.Errorf("search %q = %v, want %v", input, got, want)
		}
	}
}

func TestReindexReplaces(t *testing.T) {
	d, _ := openTest(t)
	index(t, d, 7, SearchDoc{Subject: "alpha"})
	index(t, d, 7, SearchDoc{Subject: "beta"})
	if got := matches(t, d, "alpha"); len(got) != 0 {
		t.Fatalf("old entry still matches: %v", got)
	}
	if got := matches(t, d, "beta"); !slices.Equal(got, []int64{7}) {
		t.Fatalf("new entry = %v", got)
	}
}

func TestRemoveFromIndex(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	for id := int64(1); id <= 3; id++ {
		index(t, d, id, SearchDoc{Subject: "shared"})
	}
	err := d.Tx(ctx, func(tx *Tx) error {
		if err := tx.RemoveFromIndex(ctx, []int64{1, 3, 99}); err != nil {
			return err
		}
		return tx.RemoveFromIndex(ctx, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := matches(t, d, "shared"); !slices.Equal(got, []int64{2}) {
		t.Fatalf("after removing 1, 3 and 99: %v", got)
	}
}
