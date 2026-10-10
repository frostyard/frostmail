package store

import (
	"slices"
	"testing"
)

// Keep holds an open view's rows through flag changes (api.ViewQuery), over
// viewFixture: in account A, 6, 4 and 1 are unread; 2 and 4 are flagged.
func TestViewIDsKeep(t *testing.T) {
	d, acctA, _, inboxA, archiveA := viewFixture(t)
	cases := []struct {
		name string
		f    ViewFilter
		want []int64
	}{
		{"read rows stay", ViewFilter{AccountID: acctA, Unread: viewBool(true), Keep: []int64{6, 2, 4, 1}}, []int64{6, 2, 4, 1}},
		{"unflagged rows stay", ViewFilter{Flagged: viewBool(true), Keep: []int64{6, 2, 4}}, []int64{6, 2, 4}},
		{"both flags at once", ViewFilter{AccountID: acctA, Unread: viewBool(true), Flagged: viewBool(true), Keep: []int64{2}}, []int64{2, 4}},
		{"kept rows still need the mailbox", ViewFilter{MailboxID: archiveA, Unread: viewBool(true), Keep: []int64{6, 3}}, []int64{4, 3}},
		{"kept rows still need the account", ViewFilter{AccountID: acctA, Unread: viewBool(true), Keep: []int64{7}}, []int64{6, 4, 1}},
		{"deleted rows leave", ViewFilter{MailboxID: inboxA, Unread: viewBool(true), Keep: []int64{5}}, []int64{6, 4, 1}},
		{"kept rows still need the search", ViewFilter{AccountID: acctA, Match: `"invoice"*`, Unread: viewBool(true), Keep: []int64{2, 3}}, []int64{3, 1}},
		{"kept rows still need attachments", ViewFilter{AccountID: acctA, HasAttachment: viewBool(true), Unread: viewBool(true), Keep: []int64{2}}, nil},
		{"without a flag filter Keep adds nothing", ViewFilter{MailboxID: archiveA, Keep: []int64{1, 2}}, []int64{4, 3}},
		{"threads keep a read thread's row", ViewFilter{AccountID: acctA, Unread: viewBool(true), Threads: true, Keep: []int64{2}}, []int64{6, 2, 4, 1}},
	}
	for _, tc := range cases {
		got, err := d.ViewIDs(t.Context(), tc.f)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: ViewIDs = %v, want %v", tc.name, got, tc.want)
		}
	}
}
