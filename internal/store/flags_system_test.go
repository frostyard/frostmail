package store

import (
	"slices"
	"testing"
)

// A server without keywords (Outlook.com) reports only system flags: the
// stored keywords stay, and so does the color while it stays flagged.
func TestUpdateSystemFlags(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct, inbox, _ := mailboxFixture(t, d)
	ids := insertHeaders(t, d, acct, inbox, header(1, "a"), header(2, "b"), header(3, "c"))
	if _, err := d.db.ExecContext(ctx, `UPDATE messages SET seen = 0, flagged = 1, flag_color = 4, keywords_json = '["Work"]' WHERE id IN (?, ?, ?)`,
		ids[0], ids[1], ids[2]); err != nil {
		t.Fatal(err)
	}
	var changed []int64
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		changed, err = tx.UpdateSystemFlags(ctx, inbox, []FlagUpdate{
			{UID: 1, Flags: Flags{Flagged: true, Color: 1}},             // still flagged: keeps its color
			{UID: 2, Flags: Flags{Seen: true}},                          // unflagged elsewhere: no color
			{UID: 3, Flags: Flags{Flagged: true, Color: 1, Seen: true}}, // read elsewhere
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed, []int64{ids[1], ids[2]}) {
		t.Fatalf("changed = %v, want %v", changed, ids[1:])
	}
	for i, want := range []struct {
		seen, flagged, color int64
	}{{0, 1, 4}, {1, 0, 0}, {1, 1, 4}} {
		var seen, flagged, color int64
		var keywords string
		if err := d.db.QueryRowContext(ctx, `SELECT seen, flagged, flag_color, keywords_json FROM messages WHERE id = ?`, ids[i]).
			Scan(&seen, &flagged, &color, &keywords); err != nil {
			t.Fatal(err)
		}
		if seen != want.seen || flagged != want.flagged || color != want.color || keywords != `["Work"]` {
			t.Errorf("message %d: seen %d flagged %d color %d keywords %s, want %+v and [\"Work\"]", i+1, seen, flagged, color, keywords, want)
		}
	}
}
