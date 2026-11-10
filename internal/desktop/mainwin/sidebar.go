package mainwin

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
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

func (w *Window) sidebar(c *ui.Context) {
	st := w.mail.State()
	var favorites []api.Mailbox
	var favoriteIDs []int64
	var flagNames []string
	if st.Settings != nil {
		favoriteIDs, flagNames = st.Settings.Favorites, st.Settings.FlagNames
		for _, id := range favoriteIDs {
			for _, m := range st.Mailboxes {
				if m.ID == id {
					favorites = append(favorites, m)
				}
			}
		}
	}
	sections := mail.BuildSidebar(st.Accounts, st.Mailboxes, mail.Extras{Favorites: favorites, VIPs: st.VIPs, FlagNames: flagNames,
		Counts: st.Counts, Smart: st.Smart, HasSmart: true, SmartCounts: st.SmartCounts})
	key := w.q.Source.Key()
	if w.q.Search != "" && w.q.SearchAll {
		key = ""
	}
	before := key
	ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
		ui.Sidebar(c, &key, func() {
			for _, sec := range sections {
				ui.SidebarSection(c.Key(sec.Key), w.sectionTitle(sec, st), nil, func() {
					for _, it := range sec.Items {
						w.sidebarItem(c.Key(it.Key), it, st, favoriteIDs)
					}
				})
			}
		}).Grow(1)
		w.followUp(c, st)
		w.outbox(c, st)
	})
	if key != before {
		if s, ok := mail.SourceFromKey(key, st.VIPs); ok {
			w.setSource(s)
		}
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

func (w *Window) sidebarItem(c *ui.Context, it mail.Item, st live.MailState, favorites []int64) {
	icon := frostui.IconSVG(it.Icon)
	item := ui.SidebarItem(c, it.Key, icon, it.Label).Margin(0, 0, 0, float32(it.Depth)*14)
	if it.FlagColor > 0 {
		item.TextColor(flagColor(c.Theme(), int64(it.FlagColor)))
	}
	if it.Unread > 0 {
		item.Children(func() { ui.Badge(c, strconv.FormatInt(it.Unread, 10)) })
	}
	if it.MailboxID == 0 {
		if s, ok := mail.SourceFromKey(it.Key, st.VIPs); ok && s.Kind == mail.Smart {
			item.ContextMenu(func(m *ui.Menu) { w.smartMenu(m, s.ID) })
		}
		return
	}
	var mb *api.Mailbox
	for i := range st.Mailboxes {
		if st.Mailboxes[i].ID == it.MailboxID {
			mb = &st.Mailboxes[i]
		}
	}
	if mb == nil {
		return
	}
	if len(it.Key) > 9 && it.Key[:9] == "favorite:" {
		item.ContextMenu(func(m *ui.Menu) { w.favoriteMenu(m, mb.ID, favorites) })
	} else {
		item.ContextMenu(func(m *ui.Menu) { w.mailboxMenu(m, *mb, favorites) })
	}
	if _, over := ui.DragOver[DragMessages](item); over {
		item.Border(1, c.Theme().Accent)
	}
	if d, ok := ui.Drop[DragMessages](item); ok {
		w.dropped(c, d, *mb)
	}
}

// dropped moves messages dropped on a mailbox of their account; Alt or
// Ctrl copies.
func (w *Window) dropped(c *ui.Context, d DragMessages, to api.Mailbox) {
	if to.AccountID != d.Account || w.readOnly(d.Account) {
		return
	}
	if mods := c.Modifiers(); mods&(ui.Alt|ui.Ctrl) != 0 {
		w.copy(d.IDs, &to)
		return
	}
	w.move(d.IDs, &to)
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
