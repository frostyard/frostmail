package mail_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

func mb(id, account int64, path string, role api.MailboxRole, unread int64) api.Mailbox {
	name := path[strings.LastIndex(path, "/")+1:]
	return api.Mailbox{ID: id, AccountID: account, Path: path, Name: name, Delimiter: "/", Role: role, Unread: unread}
}

func labels(items []mail.Item) []string {
	var out []string
	for _, it := range items {
		out = append(out, strings.Repeat(" ", it.Depth)+it.Label)
	}
	return out
}

var accounts = []api.Account{{ID: 1, Email: "a@x.test"}, {ID: 2, Email: "b@x.test", ReadOnly: true}}

// P-101, P-112: Favorites unifies a role two accounts have and sums inboxes.
func TestSidebarUnifiesAndSums(t *testing.T) {
	mbs := []api.Mailbox{mb(1, 1, "INBOX", api.MailboxRoleInbox, 2), mb(2, 2, "INBOX", api.MailboxRoleInbox, 3),
		mb(3, 1, "Sent", api.MailboxRoleSent, 0), mb(4, 2, "Sent", api.MailboxRoleSent, 0), mb(5, 1, "Trash", api.MailboxRoleTrash, 0)}
	s := mail.BuildSidebar(accounts, mbs, mail.Extras{})
	if s[0].Items[0].Key != "all-inboxes" || s[0].Items[0].Unread != 5 {
		t.Errorf("All Inboxes = %+v", s[0].Items[0])
	}
	if got := labels(s[0].Items); !reflect.DeepEqual(got, []string{"All Inboxes", "All Sent", "Flagged"}) {
		t.Errorf("favorites = %q", got)
	}
	if s[1].AddLabel != "New Mailbox" || s[2].AddLabel != "" {
		t.Errorf("add labels: %q, %q; a read-only account has none", s[1].AddLabel, s[2].AddLabel)
	}
}

func TestSidebarNestsFoldersAndMakesUpParents(t *testing.T) {
	mbs := []api.Mailbox{mb(1, 1, "INBOX", api.MailboxRoleInbox, 0), mb(2, 1, "INBOX/Sub", api.MailboxRoleNone, 1),
		mb(3, 1, "Zeta", api.MailboxRoleNone, 0), mb(4, 1, "Alpha/Beta/Gamma", api.MailboxRoleNone, 0), mb(5, 1, "Sent", api.MailboxRoleSent, 0)}
	s := mail.BuildSidebar(accounts[:1], mbs, mail.Extras{})
	want := []string{"Inbox", " Sub", "Sent", "Alpha", " Beta", "  Gamma", "Zeta"}
	if got := labels(s[1].Items); !reflect.DeepEqual(got, want) {
		t.Errorf("tree = %q, want %q", got, want)
	}
	if s[1].Items[3].Selectable {
		t.Error("a made-up parent is selectable")
	}
}

// P-102, P-103, P-104: favorites, flag colors in use, VIPs and their groups.
func TestSidebarExtras(t *testing.T) {
	person := int64(9)
	vips := []api.Vip{{Address: "ann@x.test", Name: "Ann", PersonID: &person}, {Address: "ann2@x.test", Name: "Ann", PersonID: &person}, {Address: "bob@x.test"}}
	counts := &mail.Counts{Flagged: api.ViewCount{Total: 4}}
	counts.Colors[2].Total = 4
	s := mail.BuildSidebar(nil, nil, mail.Extras{VIPs: vips, Counts: counts, FlagNames: []string{"", "", "Later"},
		Favorites: []api.Mailbox{mb(7, 1, "Work", api.MailboxRoleNone, 1)}})
	want := []string{"All Inboxes", "VIPs", " Ann", " bob@x.test", "Flagged", " Later", "Work"}
	if got := labels(s[0].Items); !reflect.DeepEqual(got, want) {
		t.Errorf("favorites = %q, want %q", got, want)
	}
	if g := mail.VIPGroups(vips); len(g) != 2 || len(g[0].Addresses) != 2 {
		t.Errorf("groups = %+v", g)
	}
}

func TestSourceKeysRoundTrip(t *testing.T) {
	vips := []api.Vip{{Address: "ann@x.test", Name: "Ann"}}
	for _, key := range []string{"all-inboxes", "flagged", "vips", "flag:3", "vip:ann@x.test", "role:sent", "smart:4", "mailbox:5"} {
		s, ok := mail.SourceFromKey(key, vips)
		if !ok || s.Key() != key {
			t.Errorf("%s = %+v, %v (key %s)", key, s, ok, s.Key())
		}
	}
	if s, ok := mail.SourceFromKey("favorite:5", nil); !ok || s.Kind != mail.Mailbox || s.ID != 5 {
		t.Errorf("favorite:5 = %+v, %v", s, ok)
	}
	for _, key := range []string{"path:1:Alpha", "flag:9", "role:inbox", ""} {
		if _, ok := mail.SourceFromKey(key, nil); ok {
			t.Errorf("%q parsed", key)
		}
	}
}

// P-203, P-204, P-205: the list's query for a filter, a sort and a search.
func TestListQuery(t *testing.T) {
	inbox := mail.Source{Kind: mail.AllInboxes}
	base := mail.ListState{Source: inbox, Conversations: true, Filter: mail.FilterAll, Sort: api.ViewSortDate}
	q := mail.ListQuery(base)
	if q.Role == nil || *q.Role != api.MailboxRoleInbox || q.Threads == nil || q.Sort != nil {
		t.Errorf("base = %+v", q)
	}
	l := base
	l.Filter, l.Sort, l.Ascending = mail.FilterUnread, api.ViewSortFrom, true
	q = mail.ListQuery(l)
	if q.Unread == nil || !*q.Unread || q.Sort == nil || *q.Sort != api.ViewSortFrom || q.Ascending == nil {
		t.Errorf("unread, by from = %+v", q)
	}
	l = base
	l.Filter = mail.FilterToMe
	if q = mail.ListQuery(l); q.Filter == nil || q.Filter.Conditions[0].Field != api.ConditionFieldTome {
		t.Errorf("to me = %+v", q)
	}
	l = base
	l.Search, l.SearchAll = "hello", true
	if q = mail.ListQuery(l); q.Text == nil || *q.Text != "hello" || q.Role != nil {
		t.Errorf("search everywhere = %+v", q)
	}
	l.SearchAll = false
	if q = mail.ListQuery(l); q.Text == nil || q.Role == nil {
		t.Errorf("search the source = %+v", q)
	}
	if !mail.DefaultAscending(api.ViewSortSubject) || mail.DefaultAscending(api.ViewSortSize) {
		t.Error("default order")
	}
}

func TestFitFilters(t *testing.T) {
	if n := mail.FitFilters(1000, mail.FilterAll); n != len(mail.Filters) {
		t.Errorf("wide = %d", n)
	}
	if n := mail.FitFilters(10, mail.FilterAll); n != 1 {
		t.Errorf("narrow = %d: All always stays", n)
	}
}

func TestEmptyText(t *testing.T) {
	if got := mail.EmptyText(mail.FilterFlagged, "All Inboxes"); got != "No Flagged Messages in All Inboxes" {
		t.Error(got)
	}
	if got := mail.EmptyText(mail.FilterAll, "All Inboxes"); got != "No Messages" {
		t.Error(got)
	}
}

func TestArchiveAndSpamTargets(t *testing.T) {
	mbs := []api.Mailbox{mb(1, 1, "INBOX", api.MailboxRoleInbox, 0), mb(2, 1, "Junk", api.MailboxRoleJunk, 0), mb(3, 1, "Archive", api.MailboxRoleArchive, 0)}
	if a := mail.ArchiveOf(1, mbs); a == nil || a.ID != 3 {
		t.Errorf("archive = %+v", a)
	}
	gmail := []api.Mailbox{{ID: 9, AccountID: 2, Role: api.MailboxRoleAll, Label: true}}
	if a := mail.ArchiveOf(2, gmail); a == nil || a.ID != 9 {
		t.Errorf("Gmail archive = %+v", a)
	}
	inInbox := []api.MessageSummary{{AccountID: 1, MailboxIDs: []int64{1}}}
	if not, to := mail.SpamTarget(inInbox, mbs); not || to.ID != 2 {
		t.Errorf("spam of inbox mail = %v, %+v", not, to)
	}
	inJunk := []api.MessageSummary{{AccountID: 1, MailboxIDs: []int64{2}}}
	if not, to := mail.SpamTarget(inJunk, mbs); !not || to.ID != 1 {
		t.Errorf("spam of junk mail = %v, %+v", not, to)
	}
	targets := mail.MoveTargets(&inInbox[0], mbs)
	if len(targets) != 2 {
		t.Errorf("move targets = %+v", targets)
	}
	p := mail.MoveParams([]int64{5}, mbs[2], mail.Source{Kind: mail.Mailbox, ID: 1}, mbs)
	if p.FromMailboxID == nil || *p.FromMailboxID != 1 {
		t.Errorf("move params = %+v", p)
	}
}

// P-406: the inverse of a move and of a flag change.
func TestUndoCalls(t *testing.T) {
	before := []api.MessageSummary{{ID: 1, MailboxIDs: []int64{10}}, {ID: 2, MailboxIDs: []int64{10}}}
	after := []api.MessageSummary{{ID: 1, MailboxIDs: []int64{20}}, {ID: 2, MailboxIDs: []int64{20}}}
	calls := mail.MoveBack(before, after)
	if len(calls) != 1 || calls[0].Move == nil || calls[0].Move.MailboxID != 10 || *calls[0].Move.FromMailboxID != 20 || len(calls[0].Move.IDs) != 2 {
		t.Errorf("move back = %+v", calls)
	}
	if calls := mail.MoveBack(before, nil); len(calls) != 0 {
		t.Errorf("a message gone has no inverse: %+v", calls)
	}
	seen := true
	rows := []api.MessageSummary{{ID: 1}, {ID: 2, Flags: api.Flags{Seen: true}}}
	calls = mail.FlagsBack(rows, api.FlagChanges{Seen: &seen})
	if len(calls) != 1 || calls[0].Flags == nil || *calls[0].Flags.Changes.Seen || calls[0].Flags.IDs[0] != 1 {
		t.Errorf("flags back = %+v", calls)
	}
}

func TestListDate(t *testing.T) {
	loc := time.FixedZone("x", -5*3600)
	now := time.Date(2026, 11, 10, 15, 4, 0, 0, loc)
	for _, c := range []struct {
		d    time.Time
		want string
	}{
		{time.Date(2026, 11, 10, 9, 5, 0, 0, loc), "9:05 AM"},
		{time.Date(2026, 11, 9, 23, 0, 0, 0, loc), "Yesterday"},
		{time.Date(2026, 11, 7, 12, 0, 0, 0, loc), "Saturday"},
		{time.Date(2026, 10, 1, 12, 0, 0, 0, loc), "10/1/26"},
		{time.Date(2026, 11, 12, 12, 0, 0, 0, loc), "11/12/26"},
	} {
		if got := mail.ListDate(c.d, now); got != c.want {
			t.Errorf("%v = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestCoverage(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	a := api.Account{Email: "a@x.test", SyncDays: 30}
	b := api.Account{DisplayName: "Bee", Email: "b@x.test", SyncDays: 90}
	all := api.Account{Email: "c@x.test"}
	if got := mail.Coverage([]api.Account{all}, now); got != "" {
		t.Error(got)
	}
	if got := mail.Coverage([]api.Account{a}, now); got != "Searched mail since October 11, 2026 on this computer." {
		t.Error(got)
	}
	if got := mail.Coverage([]api.Account{a, b}, now); !strings.Contains(got, "a@x.test: since") || !strings.Contains(got, "Bee: since") {
		t.Error(got)
	}
}

func TestRemindChoices(t *testing.T) {
	loc := time.FixedZone("x", 0)
	morning := mail.RemindChoices(time.Date(2026, 11, 10, 9, 0, 0, 0, loc))
	if len(morning) != 3 || morning[1].Label != "Remind Me Tonight" || morning[1].At.Hour() != 21 || morning[2].At.Day() != 11 || morning[2].At.Hour() != 8 {
		t.Errorf("morning = %+v", morning)
	}
	if late := mail.RemindChoices(time.Date(2026, 11, 10, 22, 0, 0, 0, loc)); len(late) != 2 {
		t.Errorf("late = %+v", late)
	}
}

func TestOutboxStatus(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	errText := "busy"
	at := now.Add(90 * time.Second)
	for _, c := range []struct {
		it   api.OutboxItem
		want string
	}{
		{api.OutboxItem{State: api.OutboxStateSending}, "Sending…"},
		{api.OutboxItem{State: api.OutboxStateAccepted}, "Saving to Sent…"},
		{api.OutboxItem{State: api.OutboxStateFailed, Error: &errText}, "Not sent: busy"},
		{api.OutboxItem{State: api.OutboxStateQueued}, "Waiting to send"},
		{api.OutboxItem{State: api.OutboxStateQueued, Attempts: 1, SendAt: &at, Error: &errText}, "Retrying in 2 min: busy"},
	} {
		if got := mail.OutboxStatus(c.it, now); got != c.want {
			t.Errorf("%+v = %q, want %q", c.it, got, c.want)
		}
	}
}
