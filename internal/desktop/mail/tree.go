package mail

import (
	"slices"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// Item is one row of a sidebar section.
type Item struct {
	// Key is "all-inboxes", "role:<role>", "flagged", "favorite:<id>",
	// "mailbox:<id>", or "path:<accountId>:<path>" for a parent not in the
	// list.
	Key   string
	Label string
	Icon  string // a freedesktop icon name
	Depth int
	// Unread is the row's count: unread, or the total for flagged rows.
	Unread     int64
	Selectable bool
	MailboxID  int64 // 0 for built-in sources and made-up parents
	FlagColor  int
}

// Section is Favorites, Smart Mailboxes or one account.
type Section struct {
	Key       string
	Title     string
	AddLabel  string
	AccountID int64
	Items     []Item
}

// VIPGroup is one person or standalone VIP address.
type VIPGroup struct {
	Key, Label string
	Addresses  []string
}

// VIPGroups groups a person's addresses, keeping the first VIP's label.
func VIPGroups(vips []api.Vip) []VIPGroup {
	var out []VIPGroup
	byIdentity := map[string]int{}
	for _, v := range vips {
		identity := "address:" + v.Address
		if v.PersonID != nil {
			identity = "person:" + itoa(*v.PersonID)
		}
		if i, ok := byIdentity[identity]; ok {
			out[i].Addresses = append(out[i].Addresses, v.Address)
			continue
		}
		byIdentity[identity] = len(out)
		label := v.Name
		if label == "" {
			label = v.Address
		}
		out = append(out, VIPGroup{Key: "vip:" + v.Address, Label: label, Addresses: []string{v.Address}})
	}
	return out
}

// Counts holds message counts for Favorites' built-in sources.
type Counts struct {
	VIPs    api.ViewCount
	VIP     map[string]api.ViewCount
	Flagged api.ViewCount
	Colors  [7]api.ViewCount
}

// Extras supplies VIPs, flag names, favorites, smart mailboxes and counts.
type Extras struct {
	Favorites   []api.Mailbox
	VIPs        []api.Vip
	FlagNames   []string
	Counts      *Counts
	Smart       []api.SmartMailbox
	HasSmart    bool
	SmartCounts map[int64]api.ViewCount
}

var roleOrder = []api.MailboxRole{api.MailboxRoleInbox, api.MailboxRoleDrafts, api.MailboxRoleSent, api.MailboxRoleJunk,
	api.MailboxRoleTrash, api.MailboxRoleArchive, api.MailboxRoleAll, api.MailboxRoleFlagged}

var roleLabel = map[api.MailboxRole]string{
	api.MailboxRoleInbox: "Inbox", api.MailboxRoleDrafts: "Drafts", api.MailboxRoleSent: "Sent", api.MailboxRoleJunk: "Junk",
	api.MailboxRoleTrash: "Trash", api.MailboxRoleArchive: "Archive", api.MailboxRoleAll: "All Mail", api.MailboxRoleFlagged: "Flagged",
}

var roleIcon = map[api.MailboxRole]string{
	api.MailboxRoleNone: "folder", api.MailboxRoleInbox: "mail-read", api.MailboxRoleDrafts: "document-edit", api.MailboxRoleSent: "mail-send",
	api.MailboxRoleJunk: "dialog-warning", api.MailboxRoleTrash: "user-trash", api.MailboxRoleArchive: "mail-archive",
	api.MailboxRoleAll: "mail-archive", api.MailboxRoleFlagged: "mail-mark-important",
}

type node struct {
	item     Item
	children []*node
}

func lastComponent(path, delimiter string) string {
	if delimiter == "" {
		return path
	}
	return path[strings.LastIndex(path, delimiter)+len(delimiter):]
}

func parentPath(path, delimiter string) (string, bool) {
	if delimiter == "" {
		return "", false
	}
	i := strings.LastIndex(path, delimiter)
	if i < 0 {
		return "", false
	}
	return path[:i], true
}

func byLabel(a, b *node) int {
	return strings.Compare(strings.ToLower(a.item.Label), strings.ToLower(b.item.Label))
}

func flatten(nodes []*node, depth int, out *[]Item) {
	for _, n := range nodes {
		n.item.Depth = depth
		*out = append(*out, n.item)
		slices.SortStableFunc(n.children, byLabel)
		flatten(n.children, depth+1, out)
	}
}

func accountRows(a api.Account, mailboxes []api.Mailbox) []Item {
	var own []api.Mailbox
	byPath := map[string]api.Mailbox{}
	for _, m := range mailboxes {
		if m.AccountID == a.ID {
			own = append(own, m)
			byPath[m.Path] = m
		}
	}
	roleNodes := map[string]*node{}
	var roots []*node
	for _, role := range roleOrder {
		for _, m := range own {
			if m.Role != role {
				continue
			}
			n := &node{item: Item{Key: "mailbox:" + itoa(m.ID), Label: roleLabel[role], Icon: roleIcon[role], Unread: m.Unread, Selectable: true, MailboxID: m.ID}}
			roleNodes[m.Path] = n
			roots = append(roots, n)
		}
	}
	nodes := map[string]*node{}
	var top []*node
	var folder func(path, delimiter string) *node
	folder = func(path, delimiter string) *node {
		if n, ok := nodes[path]; ok {
			return n
		}
		real, ok := byPath[path]
		n := &node{item: Item{Key: "path:" + itoa(a.ID) + ":" + path, Label: lastComponent(path, delimiter), Icon: "folder"}}
		if ok {
			n.item.Key, n.item.Label, n.item.Unread, n.item.Selectable, n.item.MailboxID = "mailbox:"+itoa(real.ID), real.Name, real.Unread, true, real.ID
		}
		nodes[path] = n
		parent, has := parentPath(path, delimiter)
		switch {
		case !has:
			top = append(top, n)
		case roleNodes[parent] != nil:
			roleNodes[parent].children = append(roleNodes[parent].children, n)
		default:
			p := folder(parent, delimiter)
			p.children = append(p.children, n)
		}
		return n
	}
	for _, m := range own {
		if m.Role == api.MailboxRoleNone {
			folder(m.Path, m.Delimiter)
		}
	}
	slices.SortStableFunc(top, byLabel)
	var out []Item
	flatten(roots, 0, &out)
	flatten(top, 0, &out)
	return out
}

var unified = []struct {
	role  api.MailboxRole
	label string
}{
	{api.MailboxRoleDrafts, "All Drafts"}, {api.MailboxRoleSent, "All Sent"}, {api.MailboxRoleJunk, "All Junk"},
	{api.MailboxRoleTrash, "All Trash"}, {api.MailboxRoleArchive, "All Archives"},
}

func unifiedRows(mailboxes []api.Mailbox) []Item {
	var out []Item
	for _, u := range unified {
		accounts := map[int64]bool{}
		var unread int64
		for _, m := range mailboxes {
			if m.Role == u.role {
				accounts[m.AccountID] = true
				unread += m.Unread
			}
		}
		if len(accounts) >= 2 {
			out = append(out, Item{Key: "role:" + string(u.role), Label: u.label, Icon: roleIcon[u.role], Unread: unread, Selectable: true})
		}
	}
	return out
}

func extraRows(e Extras) []Item {
	var rows []Item
	c := e.Counts
	if c == nil {
		c = &Counts{}
	}
	if len(e.VIPs) > 0 {
		rows = append(rows, Item{Key: "vips", Label: "VIPs", Icon: "starred", Unread: c.VIPs.Unread, Selectable: true})
		for _, g := range VIPGroups(e.VIPs) {
			rows = append(rows, Item{Key: g.Key, Label: g.Label, Icon: "avatar-default", Depth: 1, Unread: c.VIP[g.Key].Unread, Selectable: true})
		}
	}
	rows = append(rows, Item{Key: "flagged", Label: "Flagged", Icon: "mail-mark-important", Unread: c.Flagged.Total, Selectable: true})
	for color := 1; color <= 7; color++ {
		if total := c.Colors[color-1].Total; total > 0 {
			rows = append(rows, Item{Key: "flag:" + itoa(int64(color)), Label: FlagLabel(color, e.FlagNames), Icon: "mail-mark-important",
				Depth: 1, Unread: total, Selectable: true, FlagColor: color})
		}
	}
	return rows
}

// BuildSidebar builds the sidebar: Favorites, Smart Mailboxes, then one
// section per account.
func BuildSidebar(accounts []api.Account, mailboxes []api.Mailbox, e Extras) []Section {
	var allInboxes int64
	for _, m := range mailboxes {
		if m.Role == api.MailboxRoleInbox {
			allInboxes += m.Unread
		}
	}
	favorites := Section{Key: "favorites", Title: "Favorites",
		Items: []Item{{Key: "all-inboxes", Label: "All Inboxes", Icon: "mail-read", Unread: allInboxes, Selectable: true}}}
	favorites.Items = append(favorites.Items, unifiedRows(mailboxes)...)
	favorites.Items = append(favorites.Items, extraRows(e)...)
	for _, m := range e.Favorites {
		favorites.Items = append(favorites.Items, Item{Key: "favorite:" + itoa(m.ID), Label: m.Name, Icon: roleIcon[m.Role], Unread: m.Unread, Selectable: true, MailboxID: m.ID})
	}
	out := []Section{favorites}
	if e.HasSmart {
		smart := Section{Key: "smart", Title: "Smart Mailboxes", AddLabel: "New Smart Mailbox"}
		for _, m := range e.Smart {
			smart.Items = append(smart.Items, Item{Key: "smart:" + itoa(m.ID), Label: m.Name, Icon: "folder-saved-search", Unread: e.SmartCounts[m.ID].Unread, Selectable: true})
		}
		out = append(out, smart)
	}
	for _, a := range accounts {
		s := Section{Key: "account:" + itoa(a.ID), Title: a.Email, AccountID: a.ID, Items: accountRows(a, mailboxes)}
		if !a.ReadOnly {
			s.AddLabel = "New Mailbox"
		}
		out = append(out, s)
	}
	return out
}
