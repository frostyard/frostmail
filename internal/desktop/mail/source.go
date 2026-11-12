// Package mail holds the desktop app's mail logic, with no UI: the sidebar's
// sources and their view queries, the mailbox tree, the rules of the
// actions on messages, undo, and date formats (docs/specs/ui.md). The
// windows draw what it decides.
package mail

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// SourceKind says what the sidebar chose.
type SourceKind int

// The kinds of source.
const (
	AllInboxes SourceKind = iota
	Mailbox
	Smart
	Role
	Flagged
	FlagColor
	VIPs
	VIP
)

// Source is what the sidebar selected.
type Source struct {
	Kind SourceKind
	// ID is the mailbox's or the smart mailbox's.
	ID        int64
	Role      api.MailboxRole
	Color     int
	VIPKey    string
	Addresses []string
}

// Key is a source's sidebar key.
func (s Source) Key() string {
	switch s.Kind {
	case Smart:
		return "smart:" + strconv.FormatInt(s.ID, 10)
	case Role:
		return "role:" + string(s.Role)
	case Flagged:
		return "flagged"
	case FlagColor:
		return "flag:" + strconv.Itoa(s.Color)
	case VIPs:
		return "vips"
	case VIP:
		return s.VIPKey
	case Mailbox:
		return "mailbox:" + strconv.FormatInt(s.ID, 10)
	}
	return "all-inboxes"
}

var (
	unifiedRoles = []api.MailboxRole{api.MailboxRoleDrafts, api.MailboxRoleSent, api.MailboxRoleJunk, api.MailboxRoleTrash, api.MailboxRoleArchive}
	colorKey     = regexp.MustCompile(`^flag:([1-7])$`)
	smartKey     = regexp.MustCompile(`^smart:(\d+)$`)
	mailboxKey   = regexp.MustCompile(`^(?:mailbox|favorite):(\d+)$`)
)

// SourceFromKey parses a sidebar key; false for keys that are not sources.
func SourceFromKey(key string, vips []api.Vip) (Source, bool) {
	switch key {
	case "all-inboxes":
		return Source{Kind: AllInboxes}, true
	case "flagged":
		return Source{Kind: Flagged}, true
	case "vips":
		return Source{Kind: VIPs}, true
	}
	if m := colorKey.FindStringSubmatch(key); m != nil {
		color, _ := strconv.Atoi(m[1])
		return Source{Kind: FlagColor, Color: color}, true
	}
	for _, g := range VIPGroups(vips) {
		if g.Key == key {
			return Source{Kind: VIP, VIPKey: g.Key, Addresses: g.Addresses}, true
		}
	}
	for _, r := range unifiedRoles {
		if key == "role:"+string(r) {
			return Source{Kind: Role, Role: r}, true
		}
	}
	if m := smartKey.FindStringSubmatch(key); m != nil {
		id, _ := strconv.ParseInt(m[1], 10, 64)
		return Source{Kind: Smart, ID: id}, true
	}
	if m := mailboxKey.FindStringSubmatch(key); m != nil {
		id, _ := strconv.ParseInt(m[1], 10, 64)
		return Source{Kind: Mailbox, ID: id}, true
	}
	return Source{}, false
}

// Names is what SourceTitle reads.
type Names struct {
	Accounts  []api.Account
	Mailboxes []api.Mailbox
	VIPs      []api.Vip
	Smart     []api.SmartMailbox
	FlagNames []string
}

// SourceTitle names a source as the toolbar titles it; "" for one not
// loaded.
func SourceTitle(s Source, n Names) string {
	switch s.Kind {
	case AllInboxes:
		return "All Inboxes"
	case Flagged:
		return "Flagged"
	case FlagColor:
		return FlagLabel(s.Color, n.FlagNames)
	case VIPs:
		return "VIPs"
	case VIP:
		for _, g := range VIPGroups(n.VIPs) {
			if g.Key == s.VIPKey {
				return g.Label
			}
		}
	case Smart:
		for _, m := range n.Smart {
			if m.ID == s.ID {
				return m.Name
			}
		}
	case Role:
		for _, it := range BuildSidebar(n.Accounts, n.Mailboxes, Extras{})[0].Items {
			if it.Key == s.Key() {
				return it.Label
			}
		}
	case Mailbox:
		for _, sec := range BuildSidebar(n.Accounts, n.Mailboxes, Extras{}) {
			for _, it := range sec.Items {
				if it.MailboxID == s.ID {
					return it.Label
				}
			}
		}
		for _, m := range n.Mailboxes {
			if m.ID == s.ID {
				return m.Name
			}
		}
	}
	return ""
}

// SourceUnread is how many unread messages a source holds: the
// mailboxes' counts for a mailbox, All Inboxes and a unified role, and
// maild's counts of the built-in sources and smart mailboxes for the rest
// (counts and smart may be nil while they have not answered).
func SourceUnread(s Source, mailboxes []api.Mailbox, counts *Counts, smart map[int64]api.ViewCount) int64 {
	if counts == nil {
		counts = &Counts{}
	}
	var n int64
	switch s.Kind {
	case AllInboxes, Role, Mailbox:
		for _, m := range mailboxes {
			switch {
			case s.Kind == AllInboxes && m.Role == api.MailboxRoleInbox,
				s.Kind == Role && m.Role == s.Role,
				s.Kind == Mailbox && m.ID == s.ID:
				n += m.Unread
			}
		}
	case Flagged:
		n = counts.Flagged.Unread
	case FlagColor:
		if s.Color >= 1 && s.Color <= len(counts.Colors) {
			n = counts.Colors[s.Color-1].Unread
		}
	case VIPs:
		n = counts.VIPs.Unread
	case VIP:
		n = counts.VIP[s.VIPKey].Unread
	case Smart:
		n = smart[s.ID].Unread
	}
	return n
}

func condition(field api.ConditionField, value string) api.Condition {
	return api.Condition{Field: field, Op: api.ConditionOpIs, Value: value}
}

// SourceQuery is a source's view query.
func SourceQuery(s Source, conversations bool) api.ViewQuery {
	var q api.ViewQuery
	switch s.Kind {
	case Smart:
		q.SmartMailboxID = &s.ID
	case AllInboxes:
		role := api.MailboxRoleInbox
		q.Role = &role
	case Role:
		q.Role = &s.Role
	case Flagged:
		flagged := true
		q.Flagged = &flagged
	case FlagColor:
		q.Conditions = &api.Conditions{Match: api.ConditionMatchAll,
			Conditions: []api.Condition{condition(api.ConditionFieldColor, strconv.Itoa(s.Color))}}
	case VIPs:
		q.Conditions = &api.Conditions{Match: api.ConditionMatchAll,
			Conditions: []api.Condition{condition(api.ConditionFieldVip, "true")}}
	case VIP:
		cs := make([]api.Condition, 0, len(s.Addresses))
		for _, a := range s.Addresses {
			cs = append(cs, condition(api.ConditionFieldFrom, a))
		}
		q.Conditions = &api.Conditions{Match: api.ConditionMatchAny, Conditions: cs}
	case Mailbox:
		q.MailboxID = &s.ID
	}
	if conversations {
		t := true
		q.Threads = &t
	}
	return q
}

// Filter is the choice that narrows the message list.
type Filter string

// The filters.
const (
	FilterAll         Filter = "all"
	FilterUnread      Filter = "unread"
	FilterFlagged     Filter = "flagged"
	FilterAttachments Filter = "attachments"
	FilterToMe        Filter = "toMe"
	FilterCcMe        Filter = "ccMe"
	FilterVIPs        Filter = "vips"
)

// FilterChoice is a filter and its label.
type FilterChoice struct {
	Filter Filter
	Label  string
}

// Filters are the filter row's tabs; MoreFilters its More menu's.
var (
	Filters = []FilterChoice{{FilterAll, "All"}, {FilterUnread, "Unread"}, {FilterFlagged, "Flagged"}, {FilterAttachments, "Attachments"}}

	MoreFilters = []FilterChoice{{FilterToMe, "To Me"}, {FilterCcMe, "Cc Me"}, {FilterVIPs, "From VIPs"}}
)

// FitFilters is how many of Filters fit in available DIPs beside More,
// whose label is the chosen filter's when it is not a tab. Text widths are
// estimated from their lengths: All and More always stay.
func FitFilters(available float32, chosen Filter) int {
	width := func(label string) float32 { return float32(len(label))*6.5 + 18 }
	const gap = 4
	for count := len(Filters); count > 1; count-- {
		more := "More"
		for _, f := range append(slices.Clone(Filters[count:]), MoreFilters...) {
			if f.Filter == chosen {
				more = f.Label
			}
		}
		used := width(more) + 14 + gap*float32(count)
		for _, f := range Filters[:count] {
			used += width(f.Label)
		}
		if used <= available {
			return count
		}
	}
	return 1
}

// EmptyText names an empty message list for its filter, and the source it
// narrows: "No Flagged Messages in All Inboxes".
func EmptyText(f Filter, scope string) string {
	text := map[Filter]string{
		FilterAll: "No Messages", FilterUnread: "No Unread Messages", FilterFlagged: "No Flagged Messages",
		FilterAttachments: "No Messages with Attachments", FilterToMe: "No Messages to You",
		FilterCcMe: "No Messages Cc'd to You", FilterVIPs: "No Messages from VIPs",
	}[f]
	if f == FilterAll || scope == "" {
		return text
	}
	return text + " in " + scope
}

// Sort is a sort field and its label.
type Sort struct {
	Field api.ViewSort
	Label string
}

// Sorts are the sort menu's fields.
var Sorts = []Sort{
	{api.ViewSortDate, "Date"}, {api.ViewSortFrom, "From"}, {api.ViewSortTo, "To"}, {api.ViewSortSubject, "Subject"},
	{api.ViewSortSize, "Size"}, {api.ViewSortFlags, "Flags"}, {api.ViewSortUnread, "Unread"}, {api.ViewSortAttachments, "Attachments"},
}

// DefaultAscending is the order a sort field starts in.
func DefaultAscending(f api.ViewSort) bool {
	return f == api.ViewSortFrom || f == api.ViewSortTo || f == api.ViewSortSubject
}

// ListState is what decides the list's query.
type ListState struct {
	Source        Source
	Search        string
	SearchAll     bool // search every mailbox, rather than the source
	Conversations bool
	Filter        Filter
	Sort          api.ViewSort
	Ascending     bool
}

// ListQuery is the view query the list shows.
func ListQuery(l ListState) api.ViewQuery {
	var q api.ViewQuery
	if l.Search != "" && l.SearchAll {
		text := l.Search
		q.Text = &text
		if l.Conversations {
			t := true
			q.Threads = &t
		}
	} else {
		q = SourceQuery(l.Source, l.Conversations)
		if l.Search != "" {
			text := l.Search
			q.Text = &text
		}
	}
	t := true
	switch l.Filter {
	case FilterUnread:
		q.Unread = &t
	case FilterFlagged:
		q.Flagged = &t
	case FilterAttachments:
		q.HasAttachments = &t
	case FilterToMe, FilterCcMe, FilterVIPs:
		field := map[Filter]api.ConditionField{FilterToMe: api.ConditionFieldTome, FilterCcMe: api.ConditionFieldCcme, FilterVIPs: api.ConditionFieldVip}[l.Filter]
		q.Filter = &api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{condition(field, "true")}}
	}
	if l.Sort != "" && (l.Sort != api.ViewSortDate || l.Ascending) {
		sort := l.Sort
		q.Sort = &sort
		if l.Ascending {
			q.Ascending = &t
		}
	}
	return q
}

// IsVIP reports whether an address is one of the VIPs', without case.
func IsVIP(address string, vips []api.Vip) bool {
	for _, v := range vips {
		if strings.EqualFold(v.Address, address) {
			return true
		}
	}
	return false
}

// FlagNames are the seven flags' default names.
var FlagNames = []string{"Red", "Orange", "Yellow", "Green", "Blue", "Purple", "Gray"}

// FlagLabel names a flag color 1 to 7, as the user renamed it.
func FlagLabel(color int, names []string) string {
	if color < 1 || color > len(FlagNames) {
		return ""
	}
	if color <= len(names) {
		if name := strings.TrimSpace(names[color-1]); name != "" {
			return name
		}
	}
	return FlagNames[color-1]
}
