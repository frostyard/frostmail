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
	l.Row = func(key string) frostui.SourceRow {
		if it, ok := w.waits[key]; ok {
			return it.row
		}
		return w.sourceRow(w.tree[key])
	}
	l.Menu = func(m *ui.Menu, key, _ string) {
		if it, ok := w.waits[key]; ok {
			if it.outbox != nil {
				w.outboxMenu(m, *it.outbox)
			}
			return
		}
		w.sourceMenu(m, key)
	}
	l.OnSelect = func(key string) {
		if _, ok := w.waits[key]; ok {
			w.waitFor = w.q.Source.Key()
			return
		}
		st := w.mail.State()
		if s, ok := mail.SourceFromKey(key, st.VIPs); ok && w.tree[key].Selectable {
			w.setSource(s)
		}
	}
	l.OnClick = func(key string, _ ui.Modifiers) {
		if id := w.waits[key].message; id != 0 {
			w.OpenMessage(id)
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
	sections := append(w.indexSidebar(built, st), w.waitingSections(st)...)
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

// footer holds Send Later under the tree, which scrolls when it would
// take the tree's room. Its rows hold their actions, which a source
// list's rows cannot.
func (w *Window) footer(c *ui.Context, st live.MailState, height float32) {
	_, later := outboxGroups(st.Outbox)
	if len(later) == 0 {
		return
	}
	limit := height * footerShare
	if limit <= 0 {
		limit = footerStart
	}
	ui.Scroll(c).MaxHeight(limit).Shrink(0).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
		ui.Column(c).AlignItems(ui.Stretch).Children(func() {
			w.sendLater(c, later, w.o.Now())
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
		if _, waiting := w.waits[key]; waiting && w.waitFor == want {
			return // chosen in Follow Up or the Outbox while this source shows
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
		w.markAllRead(m, *mb)
		m.Separator()
		w.favoriteMenu(m, mb.ID, favorites)
		return
	}
	w.mailboxMenu(m, *mb, favorites)
}

// markAllRead is the menu's Mark All Messages as Read, for a mailbox with
// unread mail in an account that takes writes.
func (w *Window) markAllRead(m *ui.Menu, mb api.Mailbox) {
	if m.Item("Mark All Messages as Read").Disabled(w.readOnly(mb.AccountID) || mb.Unread == 0).Chosen() {
		w.run("mark the mailbox's messages as read", func(ctx context.Context, c *api.Client) error {
			_, err := c.Mailbox().MarkRead(ctx, &api.MailboxMarkReadParams{ID: mb.ID})
			return err
		})
	}
}

func (w *Window) mailboxMenu(m *ui.Menu, mb api.Mailbox, favorites []int64) {
	readOnly := w.readOnly(mb.AccountID)
	fixed := readOnly || mb.Role != api.MailboxRoleNone
	w.markAllRead(m, mb)
	m.Separator()
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

// The sections of the sidebar's list after the mailboxes, Follow Up and
// the Outbox (docs/specs/ui.md, Sidebar; frostyard/frostmail#271), and the
// keys of their rows.
const (
	followUpSection = "follow-up"
	outboxSection   = "outbox"
	followUpMore    = "follow-up:more"
)

// waiting is a row of Follow Up or the Outbox: how it shows, the message a
// click opens, and the outbox item its menu acts on.
type waiting struct {
	row     frostui.SourceRow
	message int64
	outbox  *api.OutboxItem
}

// waitingSections are Follow Up, the conversations waiting on other
// people, and the Outbox, the messages on their way or refused, as
// sections of the sidebar's list, with the mailboxes' rows, hover,
// selection, keys and collapse. They show only while they hold a row.
func (w *Window) waitingSections(st live.MailState) []frostui.SourceSection[string] {
	w.waits = map[string]waiting{}
	var out []frostui.SourceSection[string]
	if fu := st.FollowUp; fu != nil && len(fu.Threads) > 0 {
		sec := frostui.SourceSection[string]{ID: followUpSection, Title: "Follow Up"}
		for _, t := range fu.Threads {
			key := "follow-up:" + strconv.FormatInt(t.ThreadID, 10)
			w.waits[key] = waiting{row: followUpRow(t), message: t.Message.ID}
			sec.Items = append(sec.Items, key)
		}
		if shown := int64(len(fu.Threads)); fu.Total > shown {
			w.waits[followUpMore] = waiting{row: frostui.SourceRow{Title: fmt.Sprintf("Showing %d of %d", shown, fu.Total), Dim: true}}
			sec.Items = append(sec.Items, followUpMore)
		}
		out = append(out, sec)
	}
	if pending, _ := outboxGroups(st.Outbox); len(pending) > 0 {
		sec := frostui.SourceSection[string]{ID: outboxSection, Title: "Outbox"}
		now := w.o.Now()
		for _, it := range pending {
			key := "outbox:" + strconv.FormatInt(it.ID, 10)
			w.waits[key] = waiting{row: outboxRow(it, now), outbox: &it}
			sec.Items = append(sec.Items, key)
		}
		out = append(out, sec)
	}
	return out
}

// followUpRow is a conversation of Follow Up: its subject after an alarm,
// or a star when a recipient is a VIP, and how many days it has waited.
func followUpRow(t api.AwaitingThread) frostui.SourceRow {
	subject := strings.TrimSpace(t.Message.Subject)
	if subject == "" {
		subject = "(No Subject)"
	}
	row := frostui.SourceRow{Title: subject, Icon: "alarm", Tag: plural(t.WaitingDays, "day", "days")}
	spoken := []string{}
	if t.Vip {
		row.Icon = "starred"
		spoken = append(spoken, "VIP")
	}
	if to := awaitingRecipients(t); to != "" {
		spoken = append(spoken, "To: "+to)
	}
	row.Spoken = strings.Join(append(spoken, "waiting "+row.Tag), ", ")
	return row
}

// awaitingRecipients names a conversation's first two recipients and
// counts the rest.
func awaitingRecipients(t api.AwaitingThread) string {
	const shown = 2
	names := make([]string, 0, shown)
	for _, a := range t.Recipients[:min(shown, len(t.Recipients))] {
		names = append(names, mail.DisplayName(a))
	}
	out := strings.Join(names, ", ")
	if rest := t.RecipientCount - int64(len(names)); rest > 0 && out != "" {
		out += fmt.Sprintf(" & %d more", rest)
	}
	return out
}

// outboxRow is a message of the Outbox: its subject, and a spinner while
// it goes or a warning that says why it did not.
func outboxRow(it api.OutboxItem, now time.Time) frostui.SourceRow {
	status := mail.OutboxStatus(it, now)
	row := frostui.SourceRow{Title: subjectShown(it.Subject), Icon: "mail-send", Spoken: status}
	switch it.State {
	case api.OutboxStateSending, api.OutboxStateAccepted:
		row.Busy = true
	default:
		row.Alert = status
	}
	return row
}

// outboxGroups splits the outbox: the messages on their way or refused,
// for the Outbox, and those waiting for their chosen time, for Send Later.
func outboxGroups(items []api.OutboxItem) (pending, later []api.OutboxItem) {
	for _, it := range items {
		switch {
		case it.Scheduled && it.State == api.OutboxStateQueued:
			later = append(later, it)
		case it.State != api.OutboxStateQueued || it.Attempts > 0:
			pending = append(pending, it)
		}
	}
	return pending, later
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
	ui.Column(c.Key("Send Later")).Padding(0, frostui.SpaceS, frostui.SpaceS, frostui.SpaceS).Gap(frostui.SpaceXS).AlignItems(ui.Stretch).
		Label("Send Later").Children(func() {
		ui.Row(c).Height(28).AlignItems(ui.Center).Children(func() { frostui.Label(c, "Send Later").Role(ui.RoleHeading) })
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
		ui.Text(c, label).FontSize(frostui.TextSize(c, 12)).TextColor(accent).SingleLine()
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
