package mainwin

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/icons"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/organize"
)

// roleChoices are the roles a mailbox can be used for.
var roleChoices = []struct {
	role  api.MailboxRole
	label string
}{
	{api.MailboxRoleDrafts, "Drafts"}, {api.MailboxRoleSent, "Sent"}, {api.MailboxRoleJunk, "Junk"},
	{api.MailboxRoleTrash, "Trash"}, {api.MailboxRoleArchive, "Archive"},
}

// footerShare is how much of the sidebar's height the sections under the
// tree may take before they scroll; footerStart is their limit in the
// first frame, before the sidebar has a height.
const (
	footerShare = 0.4
	footerStart = 200
)

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
	column := ui.Column(c).Grow(1).AlignItems(ui.Stretch).Background(frostui.Chrome(c.Theme()))
	height := column.LayoutBounds().H
	column.Children(func() {
		tree := w.sources.View(c, sections).Grow(1)
		if w.focusPane == paneSidebar {
			w.focusPane = ""
			tree.Focus()
		}
		if tree.FocusWithin() {
			w.pane = paneSidebar
		}
		w.footer(c, st, height)
		w.moduleBar(c)
	})
}

// footer holds the sections under the tree, which scroll when they would
// take the tree's room.
func (w *Window) footer(c *ui.Context, st live.MailState, height float32) {
	limit := height * footerShare
	if limit <= 0 {
		limit = footerStart
	}
	ui.Scroll(c).MaxHeight(limit).Shrink(0).Children(func() {
		ui.Column(c).AlignItems(ui.Stretch).Children(func() {
			w.followUp(c, st)
			w.outbox(c, st)
		})
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
			return
		}
		if m.Item("New Smart Mailbox…").Chosen() {
			w.smartSheet.New()
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
	fixed := readOnly || mb.Role != api.MailboxRoleNone
	if m.Item("New Mailbox…").Disabled(readOnly).Chosen() {
		parent := mb.ID
		w.mailboxSheet.Show(organize.MailboxNew, mb.AccountID, nil, &parent, w.locations(mb.AccountID, 0))
	}
	if m.Item("Rename Mailbox…").Disabled(fixed).Chosen() {
		w.mailboxSheet.Show(organize.MailboxRename, mb.AccountID, &mb, nil, nil)
	}
	if m.Item("Move Mailbox…").Disabled(fixed).Chosen() {
		w.mailboxSheet.Show(organize.MailboxMove, mb.AccountID, &mb, nil, w.locations(mb.AccountID, mb.ID))
	}
	if m.Item("Delete Mailbox…").Disabled(fixed).Chosen() {
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

// locations are an account's mailboxes a mailbox can go under, without it.
func (w *Window) locations(account, except int64) []api.Mailbox {
	var out []api.Mailbox
	for _, m := range w.mail.State().Mailboxes {
		if m.AccountID == account && m.ID != except && !m.Label {
			out = append(out, m)
		}
	}
	return out
}

func (w *Window) organizeContext() organize.Context {
	st := w.mail.State()
	ctx := organize.Context{Today: w.o.Now().Format("2006-01-02"), Accounts: st.Accounts, Mailboxes: st.Mailboxes}
	if st.Settings != nil {
		ctx.FlagNames = st.Settings.FlagNames
	}
	return ctx
}

func (w *Window) smartMenu(m *ui.Menu, id int64) {
	if m.Item("New Smart Mailbox…").Chosen() {
		w.smartSheet.New()
	}
	if m.Item("Edit Smart Mailbox…").Chosen() {
		for _, s := range w.mail.State().Smart {
			if s.ID == id {
				w.smartSheet.Edit(s)
			}
		}
	}
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
			if ui.ButtonBase(c.Key(t.ThreadID)).Row().Label(t.Message.Subject).Cursor(ui.CursorPointer).Children(func() {
				frostui.Body(c, t.Message.Subject).Grow(1).Shrink(1).MinWidth(0).SingleLine()
			}).Clicked() {
				w.OpenMessage(t.Message.ID)
			}
		}
	})
}

// outbox draws the messages not yet sent, and those to send later.
func (w *Window) outbox(c *ui.Context, st live.MailState) {
	var pending, later []api.OutboxItem
	for _, it := range st.Outbox {
		switch {
		case it.Scheduled && it.State == api.OutboxStateQueued:
			later = append(later, it)
		case it.State != api.OutboxStateQueued || it.Attempts > 0:
			pending = append(pending, it)
		}
	}
	now := w.o.Now()
	if len(pending) > 0 {
		ui.Column(c.Key("Outbox")).Padding(frostui.SpaceS).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).Label("Outbox").Children(func() {
			frostui.Caption(c, "Outbox")
			for _, it := range pending {
				ui.Column(c.Key(it.ID)).AlignItems(ui.Stretch).Children(func() {
					frostui.Body(c, it.Subject).SingleLine()
					frostui.Muted(c, mail.OutboxStatus(it, now)).SingleLine()
				}).ContextMenu(func(m *ui.Menu) { w.outboxMenu(m, it) })
			}
		})
	}
	w.sendLater(c, later, now)
}

// sendLater draws the messages waiting for their chosen time, soonest
// first (docs/specs/ui.md, Send Later).
func (w *Window) sendLater(c *ui.Context, items []api.OutboxItem, now time.Time) {
	if len(items) == 0 {
		return
	}
	slices.SortStableFunc(items, func(a, b api.OutboxItem) int {
		return cmp.Compare(sendTime(a, now).UnixNano(), sendTime(b, now).UnixNano())
	})
	ui.Column(c.Key("Send Later")).Padding(frostui.SpaceS).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).Label("Send Later").Children(func() {
		frostui.Caption(c, "Send Later")
		for _, it := range items {
			ui.Column(c.Key(it.ID)).AlignItems(ui.Stretch).Children(func() {
				frostui.Body(c, subjectShown(it.Subject)).SingleLine()
				frostui.Muted(c, "To: "+recipients(it.To)).SingleLine()
				frostui.Muted(c, "Sends "+mail.WhenText(sendTime(it, now), now)).SingleLine()
				ui.Row(c).Gap(frostui.SpaceS).Wrap().Children(func() {
					if textButton(c.Key("edit"), "Edit") {
						w.editScheduled(it.ID)
					}
					if textButton(c.Key("now"), "Send Now") {
						w.reschedule(it.ID, w.o.Now())
					}
					if textButton(c.Key("time"), "Change Time…") {
						w.later.Show("Send Later", sendTime(it, now), func(at time.Time) { w.reschedule(it.ID, at) })
					}
				})
			}).ContextMenu(func(m *ui.Menu) { w.outboxMenu(m, it) })
		}
	})
}

func sendTime(it api.OutboxItem, now time.Time) time.Time {
	if it.SendAt != nil {
		return *it.SendAt
	}
	return now
}

func subjectShown(subject string) string {
	if strings.TrimSpace(subject) == "" {
		return "(no subject)"
	}
	return subject
}

// recipients names at most two recipients and counts the rest.
func recipients(to []api.Address) string {
	const shown = 2
	names := make([]string, 0, len(to))
	for _, a := range to {
		names = append(names, mail.DisplayName(a))
	}
	if len(names) > shown {
		return fmt.Sprintf("%s & %d more", strings.Join(names[:shown], ", "), len(names)-shown)
	}
	return strings.Join(names, ", ")
}

// textButton draws a small action in the accent color; it reports a click.
func textButton(c *ui.Context, label string) bool {
	accent := c.Theme().Accent
	return ui.ButtonBase(c).Row().Label(label).Cursor(ui.CursorPointer).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(accent).SingleLine()
	}).Clicked()
}

// editScheduled stops a scheduled message and opens its draft.
func (w *Window) editScheduled(id int64) {
	w.run("edit the message", func(ctx context.Context, c *api.Client) error {
		d, err := c.Outbox().Cancel(ctx, &api.OutboxCancelParams{ID: id})
		if err != nil {
			return err
		}
		if d.ID != 0 {
			w.showDraft(c, d.ID, false)
		}
		return nil
	})
}

// reschedule gives a scheduled message a new time.
func (w *Window) reschedule(id int64, at time.Time) {
	w.run("change the send time", func(ctx context.Context, c *api.Client) error {
		_, err := c.Outbox().Reschedule(ctx, &api.OutboxRescheduleParams{ID: id, SendAt: at})
		return err
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
