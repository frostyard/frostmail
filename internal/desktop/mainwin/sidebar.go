package mainwin

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// roleChoices are the roles a mailbox can be used for.
var roleChoices = []struct {
	role  api.MailboxRole
	label string
}{
	{api.MailboxRoleDrafts, "Drafts"}, {api.MailboxRoleSent, "Sent"}, {api.MailboxRoleJunk, "Junk"},
	{api.MailboxRoleTrash, "Trash"}, {api.MailboxRoleArchive, "Archive"},
}

// setUpSources says how the sidebar's tree shows and what it does.
func (w *Window) setUpSources() {
	l := &w.sources
	l.Label = "Mailboxes"
	l.Children = func(key string) []string { return w.kids[key] }
	l.Row = func(key string) frostui.SourceRow { return w.sourceRow(w.tree[key]) }
	l.Menu = func(m *ui.Menu, key, _ string) { w.sourceMenu(m, key) }
	l.OnSelect = func(key string) {
		st := w.mail.State()
		if s, ok := mail.SourceFromKey(key, st.VIPs); ok && w.tree[key].Selectable {
			w.setSource(s)
		}
	}
	frostui.DropOn(l, func(key string, d DragMessages) bool {
		mb := w.mailboxOf(key)
		return mb != nil && mb.AccountID == d.Account && !w.readOnly(d.Account)
	}, func(key string, d DragMessages, mods ui.Modifiers) {
		mb := w.mailboxOf(key)
		if mb == nil {
			return
		}
		if mods&(ui.Alt|ui.Ctrl) != 0 {
			w.copy(d.IDs, mb)
			return
		}
		w.move(d.IDs, mb)
	})
}

// mailboxOf is the mailbox a sidebar row shows, nil for other rows.
func (w *Window) mailboxOf(key string) *api.Mailbox {
	id := w.tree[key].MailboxID
	if id == 0 {
		return nil
	}
	st := w.mail.State()
	for i := range st.Mailboxes {
		if st.Mailboxes[i].ID == id {
			return &st.Mailboxes[i]
		}
	}
	return nil
}

func (w *Window) sidebar(c *ui.Context) {
	st := w.mail.State()
	var favorites []api.Mailbox
	var flagNames []string
	if st.Settings != nil {
		flagNames = st.Settings.FlagNames
		for _, id := range st.Settings.Favorites {
			for _, m := range st.Mailboxes {
				if m.ID == id {
					favorites = append(favorites, m)
				}
			}
		}
	}
	built := mail.BuildSidebar(st.Accounts, st.Mailboxes, mail.Extras{Favorites: favorites, VIPs: st.VIPs, FlagNames: flagNames,
		Counts: st.Counts, Smart: st.Smart, HasSmart: true, SmartCounts: st.SmartCounts})
	sections := w.indexSidebar(built, st)
	w.syncSource(st)
	ui.Column(c).Grow(1).AlignItems(ui.Stretch).Background(frostui.Chrome(c.Theme())).Children(func() {
		w.sources.View(c, sections).Grow(1)
		w.followUp(c, st)
		w.outbox(c, st)
	})
}

// indexSidebar turns the sections' rows, flat with depths, into the tree
// the list asks for, and opens each branch the first time it shows.
func (w *Window) indexSidebar(built []mail.Section, st live.MailState) []frostui.SourceSection[string] {
	w.tree, w.kids = map[string]mail.Item{}, map[string][]string{}
	sections := make([]frostui.SourceSection[string], 0, len(built))
	for _, sec := range built {
		top := []string{}
		var path []string // the keys above the current row, by depth
		for _, it := range sec.Items {
			w.tree[it.Key] = it
			path = append(path[:min(it.Depth, len(path))], it.Key)
			if len(path) == 1 {
				top = append(top, it.Key)
				continue
			}
			parent := path[len(path)-2]
			w.kids[parent] = append(w.kids[parent], it.Key)
			if !w.branches[parent] {
				w.branches[parent] = true
				w.sources.SetOpen(sec.Key, parent, true)
			}
		}
		sections = append(sections, frostui.SourceSection[string]{ID: sec.Key, Title: w.sectionTitle(sec, st), Items: top})
	}
	return sections
}

// syncSource keeps the list's choice on the source shown, as a search of
// every mailbox shows none.
func (w *Window) syncSource(st live.MailState) {
	if w.q.Search != "" && w.q.SearchAll {
		w.sources.ClearSelection()
		return
	}
	want := w.q.Source.Key()
	if key, _, ok := w.sources.Selected(); ok {
		if s, known := mail.SourceFromKey(key, st.VIPs); known && s.Key() == want {
			return
		}
	}
	if _, ok := w.tree[want]; ok {
		w.sources.Select("", want)
	}
}

// sectionTitle adds an account's sync state to its title.
func (w *Window) sectionTitle(sec mail.Section, st live.MailState) string {
	if sec.AccountID == 0 {
		return sec.Title
	}
	switch s := st.Sync[sec.AccountID]; s.Phase {
	case api.SyncPhaseOffline, api.SyncPhaseFailed, api.SyncPhaseUnauthorized:
		return sec.Title + " ⚠"
	case api.SyncPhaseConnecting, api.SyncPhaseListing, api.SyncPhaseSyncing:
		return sec.Title + " ⟳"
	}
	return sec.Title
}

func (w *Window) sourceRow(it mail.Item) frostui.SourceRow {
	row := frostui.SourceRow{Title: it.Label, Icon: it.Icon, Dim: !it.Selectable && it.MailboxID == 0 && it.Key != ""}
	if it.FlagColor > 0 {
		row.Icon = fmt.Sprintf("%s%d", icons.FlagPrefix, it.FlagColor)
	}
	if it.Unread > 0 {
		row.Count = strconv.FormatInt(it.Unread, 10)
		row.Spoken = row.Count + " unread"
	}
	return row
}

// sourceMenu is a sidebar row's menu.
func (w *Window) sourceMenu(m *ui.Menu, key string) {
	st := w.mail.State()
	var favorites []int64
	if st.Settings != nil {
		favorites = st.Settings.Favorites
	}
	mb := w.mailboxOf(key)
	if mb == nil {
		if s, ok := mail.SourceFromKey(key, st.VIPs); ok && s.Kind == mail.Smart {
			w.smartMenu(m, s.ID)
		}
		return
	}
	if strings.HasPrefix(key, "favorite:") {
		w.favoriteMenu(m, mb.ID, favorites)
		return
	}
	w.mailboxMenu(m, *mb, favorites)
}

func (w *Window) mailboxMenu(m *ui.Menu, mb api.Mailbox, favorites []int64) {
	readOnly := w.readOnly(mb.AccountID)
	if m.Item("Delete Mailbox…").Disabled(readOnly || mb.Role != api.MailboxRoleNone).Chosen() {
		w.confirmMailbox(mb, false)
	}
	if !mb.Label {
		m.Submenu("Use This Mailbox For", func(m *ui.Menu) {
			for _, r := range roleChoices {
				if m.Item(r.label).Checked(mb.Role == r.role).Disabled(readOnly).Chosen() {
					role := r.role
					w.run("set the mailbox's use", func(ctx context.Context, c *api.Client) error {
						_, err := c.Mailbox().SetRole(ctx, &api.MailboxSetRoleParams{ID: mb.ID, Role: role})
						return err
					})
				}
			}
		}).Disabled(readOnly)
	}
	if mb.Role == api.MailboxRoleTrash || mb.Role == api.MailboxRoleJunk {
		label := "Erase Junk Mail…"
		if mb.Role == api.MailboxRoleTrash {
			label = "Erase Deleted Items…"
		}
		if m.Item(label).Disabled(readOnly).Chosen() {
			w.confirmMailbox(mb, true)
		}
	}
	m.Separator()
	isFavorite := slices.Contains(favorites, mb.ID)
	label := "Add to Favorites"
	if isFavorite {
		label = "Remove from Favorites"
	}
	if m.Item(label).Chosen() {
		next := slices.Clone(favorites)
		if isFavorite {
			next = slices.DeleteFunc(next, func(id int64) bool { return id == mb.ID })
		} else {
			next = append(next, mb.ID)
		}
		w.setFavorites(next)
	}
}

func (w *Window) favoriteMenu(m *ui.Menu, id int64, favorites []int64) {
	i := slices.Index(favorites, id)
	if m.Item("Remove from Favorites").Chosen() {
		w.setFavorites(slices.DeleteFunc(slices.Clone(favorites), func(f int64) bool { return f == id }))
	}
	if m.Item("Move Up").Disabled(i <= 0).Chosen() {
		next := slices.Clone(favorites)
		next[i], next[i-1] = next[i-1], next[i]
		w.setFavorites(next)
	}
	if m.Item("Move Down").Disabled(i < 0 || i == len(favorites)-1).Chosen() {
		next := slices.Clone(favorites)
		next[i], next[i+1] = next[i+1], next[i]
		w.setFavorites(next)
	}
}

func (w *Window) setFavorites(ids []int64) {
	w.run("change Favorites", func(ctx context.Context, c *api.Client) error {
		_, err := c.Settings().Set(ctx, &api.SettingsSetParams{Favorites: ids})
		return err
	})
}

func (w *Window) smartMenu(m *ui.Menu, id int64) {
	if m.Item("Delete Smart Mailbox").Chosen() {
		w.run("delete the smart mailbox", func(ctx context.Context, c *api.Client) error {
			return c.Smart().Delete(ctx, &api.SmartDeleteParams{ID: id})
		})
		if w.q.Source.Kind == mail.Smart && w.q.Source.ID == id {
			w.setSource(mail.Source{Kind: mail.AllInboxes})
		}
	}
}

func (w *Window) confirmMailbox(mb api.Mailbox, erase bool) {
	title, body, action := "Delete Mailbox?", fmt.Sprintf("Delete the mailbox %q and the messages in it?", mb.Name), "Delete"
	if erase {
		title = "Erase Junk Mail?"
		if mb.Role == api.MailboxRoleTrash {
			title = "Erase Deleted Items?"
		}
		body, action = fmt.Sprintf("Erase the %d messages in %q? They cannot be recovered.", mb.Total, mb.Name), "Erase"
	}
	w.shell.Dialogs.Ask(frostui.Confirm{Title: title, Body: body, Action: action, Destructive: true, OnConfirm: func() {
		if erase {
			w.run("erase the mailbox", func(ctx context.Context, c *api.Client) error {
				_, err := c.Mailbox().Erase(ctx, &api.MailboxEraseParams{ID: mb.ID})
				return err
			})
			return
		}
		w.run("delete the mailbox", func(ctx context.Context, c *api.Client) error {
			return c.Mailbox().Delete(ctx, &api.MailboxDeleteParams{ID: mb.ID})
		})
		if w.q.Source.Kind == mail.Mailbox && w.q.Source.ID == mb.ID {
			w.setSource(mail.Source{Kind: mail.AllInboxes})
		}
	}})
}

// followUp draws the conversations waiting on other people.
func (w *Window) followUp(c *ui.Context, st live.MailState) {
	if st.FollowUp == nil || len(st.FollowUp.Threads) == 0 {
		return
	}
	ui.Column(c).Padding(frostui.SpaceS).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).Label("Follow Up").Children(func() {
		frostui.Caption(c, fmt.Sprintf("Follow Up (%d)", st.FollowUp.Total)).Tooltip(fmt.Sprintf("Sent mail with no reply for %d days or more", live.FollowUpDays))
		for _, t := range st.FollowUp.Threads {
			frostui.Body(c.Key(t.ThreadID), t.Message.Subject).SingleLine()
		}
	})
}

// outbox draws the messages not yet sent, and those to send later.
func (w *Window) outbox(c *ui.Context, st live.MailState) {
	var pending, later []api.OutboxItem
	for _, it := range st.Outbox {
		switch {
		case it.Scheduled:
			later = append(later, it)
		case it.State != api.OutboxStateQueued || it.Attempts > 0:
			pending = append(pending, it)
		}
	}
	section := func(title string, items []api.OutboxItem, status func(api.OutboxItem) string) {
		if len(items) == 0 {
			return
		}
		ui.Column(c.Key(title)).Padding(frostui.SpaceS).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).Label(title).Children(func() {
			frostui.Caption(c, title)
			for _, it := range items {
				ui.Column(c.Key(it.ID)).AlignItems(ui.Stretch).Children(func() {
					frostui.Body(c, it.Subject).SingleLine()
					frostui.Muted(c, status(it)).SingleLine()
				}).ContextMenu(func(m *ui.Menu) { w.outboxMenu(m, it) })
			}
		})
	}
	now := w.o.Now()
	section("Outbox", pending, func(it api.OutboxItem) string { return mail.OutboxStatus(it, now) })
	section("Send Later", later, func(it api.OutboxItem) string {
		if it.SendAt == nil {
			return ""
		}
		return mail.ListDate(*it.SendAt, now)
	})
}

func (w *Window) outboxMenu(m *ui.Menu, it api.OutboxItem) {
	if it.State == api.OutboxStateFailed && m.Item("Try Again").Chosen() {
		w.run("send the message again", func(ctx context.Context, c *api.Client) error {
			return c.Outbox().Retry(ctx, &api.OutboxRetryParams{ID: it.ID})
		})
	}
	if m.Item("Cancel Sending").Disabled(it.State == api.OutboxStateSending || it.State == api.OutboxStateAccepted).Chosen() {
		w.run("cancel the message", func(ctx context.Context, c *api.Client) error {
			_, err := c.Outbox().Cancel(ctx, &api.OutboxCancelParams{ID: it.ID})
			return err
		})
	}
}
