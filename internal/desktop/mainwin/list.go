package mainwin

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/commands"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostmail/internal/desktop/uikit"
)

// searchPause is how long the search field waits after the last key;
// sortWidth the room the filter row leaves for the sort menu.
const (
	searchPause = 250 * time.Millisecond
	sortWidth   = 48
)

// flagColors are the flags' colors, light and dark (docs/specs/ui.md,
// Tokens: Flags).
var flagColors = [7][2]string{
	{"#ff3b30", "#ff453a"}, {"#ff9500", "#ff9f0a"}, {"#ffcc00", "#ffd60a"}, {"#34c759", "#30d158"},
	{"#007aff", "#0a84ff"}, {"#af52de", "#bf5af2"}, {"#8e8e93", "#98989d"},
}

func flagColor(t *ui.Theme, color int64) ui.Color {
	if color < 1 || color > 7 {
		color = 1
	}
	shade := 0
	if t.Dark {
		shade = 1
	}
	return ui.Hex(flagColors[color-1][shade])
}

// DragMessages is the value dragged from the list to a mailbox.
type DragMessages struct {
	Account int64
	IDs     []int64
}

// messages draws the search field, the filter row and the list.
func (w *Window) messages(c *ui.Context) {
	ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
		w.searchRow(c)
		w.filterRow(c)
		if w.q.Search != "" {
			w.scopeRow(c)
		}
		w.listBody(c)
	})
}

func (w *Window) searchRow(c *ui.Context) {
	ui.Row(c).Padding(frostui.SpaceS, frostui.SpaceS, frostui.SpaceXS, frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
		f := ui.SearchField(c, &w.searchDraft).Grow(1).Label("Search").Placeholder("Search")
		if w.focusSearch {
			f.Focus()
			w.focusSearch = false
		}
		if f.Changed() {
			w.searchToken++
			token := w.searchToken
			time.AfterFunc(searchPause, func() {
				w.o.Post(func() {
					if token == w.searchToken {
						w.commitSearch()
					}
				})
			})
		}
		if f.Submitted() {
			w.searchToken++
			w.commitSearch()
		}
	})
}

func (w *Window) commitSearch() {
	text := strings.TrimSpace(w.searchDraft)
	if text == w.q.Search {
		return
	}
	w.q.Search = text
	w.clearSelection()
	w.syncView()
}

// filterRow draws the filter tabs that fit, the rest in More, and the sort
// menu.
func (w *Window) filterRow(c *ui.Context) {
	row := ui.Row(c).Height(28).Padding(0, frostui.SpaceS).Gap(frostui.SpaceXS).AlignItems(ui.Center)
	if width := row.LayoutBounds().W; width > 0 {
		w.listWidth = width
	}
	count := mail.FitFilters(w.listWidth-2*frostui.SpaceS-sortWidth, w.q.Filter)
	row.Children(func() {
		for _, f := range mail.Filters[:count] {
			if w.tab(c.Key(f.Filter), f.Label, w.q.Filter == f.Filter).Clicked() {
				w.setFilter(f.Filter)
			}
		}
		more := append(slices.Clone(mail.Filters[count:]), mail.MoreFilters...)
		label := "More"
		for _, f := range more {
			if f.Filter == w.q.Filter {
				label = f.Label
			}
		}
		ui.MenuButton(c, label, func(m *ui.Menu) {
			for _, f := range more {
				if m.Item(f.Label).Checked(w.q.Filter == f.Filter).Chosen() {
					w.setFilter(f.Filter)
				}
			}
		}).FontSize(12).Height(22).Label("More filters")
		ui.Spacer(c)
		sort := "Sort"
		for _, s := range mail.Sorts {
			if s.Field == w.q.Sort {
				sort = "Sort by " + s.Label
			}
		}
		ui.MenuButton(c, "", w.sortMenu).Height(22).Label(sort).Tooltip(sort).Children(func() {
			frostui.Icon(c, "view-sort-descending")
		})
	})
}

// tab draws a compact filter tab.
func (w *Window) tab(c *ui.Context, label string, active bool) ui.Element {
	t := c.Theme()
	b := ui.Row(c).Height(22).Padding(0, frostui.SpaceS).Radius(6).AlignItems(ui.Center).Role(ui.RoleTab).Selected(active).Focusable().
		Cursor(ui.CursorPointer).Label(label)
	color := t.TextMuted
	if active {
		b.Background(t.SurfacePressed)
		color = t.Text
	} else if b.Hovered() {
		color = t.Text
	}
	b.Children(func() { ui.Text(c, label).FontSize(12).TextColor(color).SingleLine() })
	return b
}

func (w *Window) setFilter(f mail.Filter) {
	w.q.Filter = f
	w.clearSelection()
	w.syncView()
}

func (w *Window) sortMenu(m *ui.Menu) {
	for _, s := range mail.Sorts {
		if m.Item(s.Label).Radio(w.q.Sort == s.Field).Chosen() {
			w.q.Sort, w.q.Ascending = s.Field, mail.DefaultAscending(s.Field)
			w.syncView()
		}
	}
	m.Separator()
	if m.Item("Ascending").Radio(w.q.Ascending).Chosen() {
		w.q.Ascending = true
		w.syncView()
	}
	if m.Item("Descending").Radio(!w.q.Ascending).Chosen() {
		w.q.Ascending = false
		w.syncView()
	}
	m.Separator()
	if m.Item("Conversations").Checked(w.q.Conversations).Chosen() {
		w.q.Conversations = !w.q.Conversations
		w.clearSelection()
		w.syncView()
	}
	if m.Item("Contact Photos").Checked(w.contactPhotos).Chosen() {
		w.contactPhotos = !w.contactPhotos
	}
}

// photo is a row's contact photo, asked for once.
func (w *Window) photo(address string) *ui.Bitmap {
	if !w.contactPhotos || w.photos == nil {
		return nil
	}
	if b, ok := w.bitmaps[address]; ok {
		return b
	}
	w.photos.Want([]string{address})
	data := w.photos.Get(address)
	if data == nil {
		return nil
	}
	b, err := ui.DecodeBitmap(data)
	if err != nil {
		b = nil
	}
	w.bitmaps[address] = b
	return b
}

func (w *Window) scopeRow(c *ui.Context) {
	title := mail.SourceTitle(w.q.Source, w.mailNames())
	ui.Column(c).Padding(0, frostui.SpaceS, frostui.SpaceXS, frostui.SpaceS).Gap(2).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Gap(frostui.SpaceXS).AlignItems(ui.Center).Children(func() {
			if ui.Button(c.Key("all"), "All Mailboxes").Selected(w.q.SearchAll).Clicked() {
				w.setSearchAll(true)
			}
			if title != "" && ui.Button(c.Key("source"), title).Selected(!w.q.SearchAll).Clicked() {
				w.setSearchAll(false)
			}
		})
		if line := w.coverage(); line != "" {
			frostui.Caption(c, line)
		}
	})
}

func (w *Window) setSearchAll(all bool) {
	w.q.SearchAll = all
	w.clearSelection()
	w.syncView()
}

// coverage is the line under a search that says how far back it looked.
func (w *Window) coverage() string {
	st := w.mail.State()
	accounts := st.Accounts
	if !w.q.SearchAll && w.q.Source.Kind == mail.Mailbox {
		for _, m := range st.Mailboxes {
			if m.ID == w.q.Source.ID {
				for _, a := range st.Accounts {
					if a.ID == m.AccountID {
						accounts = []api.Account{a}
					}
				}
			}
		}
	}
	return mail.Coverage(accounts, w.o.Now())
}

func (w *Window) listBody(c *ui.Context) {
	if w.view == nil {
		frostui.Loading(c, "Reconnecting to maild…")
		return
	}
	v := w.view
	if err := v.Err(); err != nil {
		frostui.Empty(c, "dialog-warning", "Messages could not be loaded", err.Error())
		return
	}
	// One state for the whole frame: the view changes on other goroutines.
	snap := v.Snapshot()
	if v.Ready() && snap.Count() == 0 {
		w.emptyList(c)
		return
	}
	w.repairSelection()
	w.list.Selected = &w.selected
	w.list.Selection = &w.chosen
	w.list.Key = func(i int) any {
		if r, ok := snap.Row(int64(i)); ok {
			return r.ID
		}
		return -int64(i) - 1
	}
	w.list.Label = func(i int) string {
		if r, ok := snap.Row(int64(i)); ok {
			return mail.DisplayName(r.From)
		}
		return ""
	}
	w.list.DragRows = func(rows []int) any {
		d := DragMessages{}
		for _, i := range rows {
			if r, ok := snap.Row(int64(i)); ok {
				d.Account = r.AccountID
				d.IDs = append(d.IDs, r.ID)
			}
		}
		return d
	}
	low, high := int64(-1), int64(-1)
	list := ui.List(c, &w.list, int(snap.Count()), func(i int) {
		n := int64(i)
		if low < 0 || n < low {
			low = n
		}
		high = max(high, n)
		w.row(c, snap, n)
	}).Grow(1).Label("Messages")
	if w.focusPane == paneList {
		w.focusPane = ""
		list.Focus()
	}
	if list.FocusWithin() {
		w.pane = paneList
	}
	if list.Submitted() {
		w.openSelected()
	}
	if list.Changed() {
		w.revealing = nil
		if w.selected >= 0 {
			w.lastIndex = int64(w.selected)
		}
	}
	if low >= 0 {
		v.Ensure(max(0, low-50), high+50)
	}
}

// repairSelection keeps the selection on its rows: if they all went, it
// selects the row that took their place.
func (w *Window) repairSelection() {
	ids := w.selectedIDs()
	if len(ids) == 0 || w.view == nil || w.revealing != nil {
		return
	}
	if len(w.view.Rows(ids)) > 0 {
		return
	}
	if w.view.Count() == 0 {
		w.clearSelection()
		return
	}
	idx := min(w.lastIndex, w.view.Count()-1)
	r, ok := w.view.Row(idx)
	if !ok {
		return
	}
	w.chosen.Clear()
	w.chosen.Add(r.ID)
	w.selected = int(idx)
}

func (w *Window) emptyList(c *ui.Context) {
	scope := ""
	if w.q.Search == "" && (w.q.Filter != mail.FilterFlagged || w.q.Source.Kind != mail.Flagged) {
		scope = mail.SourceTitle(w.q.Source, w.mailNames())
	}
	ui.Column(c).Grow(1).Gap(frostui.SpaceS).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		frostui.Muted(c, mail.EmptyText(w.q.Filter, scope))
		if w.q.Filter != mail.FilterAll {
			ui.Row(c).Gap(frostui.SpaceM).Children(func() {
				if ui.Button(c.Key("all"), "Show All").Clicked() {
					w.setFilter(mail.FilterAll)
				}
				if w.q.Filter == mail.FilterFlagged && w.q.Source.Kind != mail.Flagged && ui.Button(c.Key("flagged"), "Show All Flagged").Clicked() {
					w.q.Filter = mail.FilterAll
					w.setSource(mail.Source{Kind: mail.Flagged})
				}
			})
		}
	})
}

func (w *Window) row(c *ui.Context, snap live.Snapshot, n int64) {
	r, ok := snap.Row(n)
	if !ok {
		uikit.SummaryRow(c, uikit.Summary{})
		return
	}
	st := w.mail.State()
	s := uikit.Summary{Title: mail.DisplayName(r.From), Date: mail.ListDate(r.Date, w.o.Now()), Subtitle: r.Subject, Preview: r.Preview,
		Unread: !r.Flags.Seen, Photo: w.photo(r.From.Address)}
	if r.Flags.Flagged {
		s.Mark = flagColor(c.Theme(), r.Flags.FlagColor)
		names := []string(nil)
		if st.Settings != nil {
			names = st.Settings.FlagNames
		}
		s.MarkLabel = "Flagged: " + mail.FlagLabel(int(max(r.Flags.FlagColor, 1)), names)
	}
	if mail.IsVIP(r.From.Address, st.VIPs) {
		s.Icons = append(s.Icons, uikit.SummaryIcon{Name: "starred", Label: "VIP"})
	}
	if r.Muted != nil && *r.Muted {
		s.Icons = append(s.Icons, uikit.SummaryIcon{Name: "notifications-disabled", Label: "Muted"})
	}
	if r.HasAttachments {
		s.Icons = append(s.Icons, uikit.SummaryIcon{Name: "mail-attachment", Label: "Has attachments"})
	}
	if r.RemindAt != nil {
		s.Icons = append(s.Icons, uikit.SummaryIcon{Name: "alarm", Label: "Reminder set"})
	}
	if w.q.Conversations && r.ThreadCount > 1 {
		s.Count = strconv.FormatInt(r.ThreadCount, 10)
	}
	if !w.readOnly(r.AccountID) {
		s.Actions = func() { w.rowActions(c, r) }
	}
	uikit.SummaryRow(c, s).ContextMenu(w.rowMenu)
}

func (w *Window) readOnly(account int64) bool {
	for _, a := range w.mail.State().Accounts {
		if a.ID == account {
			return a.ReadOnly
		}
	}
	return true
}

func (w *Window) rowActions(c *ui.Context, r api.MessageSummary) {
	ids := []int64{r.ID}
	if frostui.IconButton(c.Key("flag"), "mail-mark-important", "Flag").Clicked() {
		w.toggleFlag(ids)
	}
	if mail.ArchiveOf(r.AccountID, w.mail.State().Mailboxes) != nil && frostui.IconButton(c.Key("archive"), "mail-archive", "Archive").Clicked() {
		w.archive(ids)
	}
	if frostui.IconButton(c.Key("delete"), "user-trash", "Delete").Clicked() {
		w.delete(ids)
	}
}

// rowMenu is the list row's menu; it acts on the selection, which a
// right-click has made the row or kept.
func (w *Window) rowMenu(m *ui.Menu) {
	ids := w.selectedIDs()
	rows := w.rows(ids)
	if len(rows) == 0 {
		return
	}
	st := w.mail.State()
	first := rows[0]
	readOnly := w.readOnly(first.AccountID)
	if mail.ArchiveOf(first.AccountID, st.Mailboxes) != nil && m.Item("Archive").Disabled(readOnly).Chosen() {
		w.archive(ids)
	}
	if m.Item("Delete").Disabled(readOnly).Chosen() {
		w.delete(ids)
	}
	notSpam, to := mail.SpamTarget(rows, st.Mailboxes)
	label := "Mark as Spam"
	if notSpam {
		label = "Not Spam"
	}
	if m.Item(label).Disabled(readOnly || to == nil).Chosen() {
		w.toggleSpam(ids)
	}
	m.Separator()
	targets := mail.MoveTargets(&first, st.Mailboxes)
	m.Submenu("Move to", func(m *ui.Menu) {
		for i := range targets {
			if m.Item(targets[i].Path).Disabled(readOnly).Chosen() {
				w.move(ids, &targets[i])
			}
		}
	}).Disabled(readOnly || len(targets) == 0)
	copies := mail.CopyTargets(&first, st.Mailboxes)
	m.Submenu("Copy to", func(m *ui.Menu) {
		for i := range copies {
			if m.Item(copies[i].Path).Disabled(readOnly).Chosen() {
				w.copy(ids, &copies[i])
			}
		}
	}).Disabled(readOnly || len(copies) == 0)
	m.Separator()
	flag := "Flag"
	if first.Flags.Flagged {
		flag = "Unflag"
	}
	if m.Item(flag).Disabled(readOnly).Chosen() {
		w.toggleFlag(ids)
	}
	var flagNames []string
	if st.Settings != nil {
		flagNames = st.Settings.FlagNames
	}
	m.Submenu("Flag Color", func(m *ui.Menu) {
		for color := int64(1); color <= 7; color++ {
			if m.Item(mail.FlagLabel(int(color), flagNames)).Checked(first.Flags.FlagColor == color).Disabled(readOnly).Chosen() {
				w.setFlagColor(ids, color)
			}
		}
		m.Separator()
		anyFlagged := false
		for _, r := range rows {
			anyFlagged = anyFlagged || r.Flags.Flagged
		}
		if m.Item("Clear Flag").Disabled(readOnly || !anyFlagged).Chosen() {
			w.setFlagColor(ids, 0)
		}
	}).Disabled(readOnly)
	allSeen, allMuted, anyReminder := true, true, false
	for _, r := range rows {
		allSeen = allSeen && r.Flags.Seen
		allMuted = allMuted && r.Muted != nil && *r.Muted
		anyReminder = anyReminder || r.RemindAt != nil
	}
	read := "Mark as Read"
	if allSeen {
		read = "Mark as Unread"
	}
	if m.Item(read).Disabled(readOnly).Chosen() {
		w.toggleRead(ids)
	}
	mute := "Mute"
	if allMuted {
		mute = "Unmute"
	}
	if m.Item(mute).Disabled(readOnly).Chosen() {
		w.mute(ids, !allMuted)
	}
	m.Submenu("Remind Me", func(m *ui.Menu) {
		for _, ch := range mail.RemindChoices(w.o.Now()) {
			if m.Item(ch.Label).Disabled(readOnly).Chosen() {
				at := ch.At
				w.remind(ids, &at)
			}
		}
		if anyReminder {
			m.Separator()
			if m.Item("Clear Reminder").Disabled(readOnly).Chosen() {
				w.remind(ids, nil)
			}
		}
	}).Disabled(readOnly)
	for _, rule := range st.Rules {
		if rule.Enabled {
			m.Separator()
			if m.Item("Apply Rules").Disabled(readOnly).Chosen() {
				w.applyRules(ids)
			}
			break
		}
	}
}

// sidebarBar is the top bar's segment over the sidebar.
func (w *Window) sidebarBar(c *ui.Context) {
	if frostui.IconButton(c, "mail-send-receive", w.table.Tooltip(string(commands.GetMail))).Clicked() {
		w.table.Run(string(commands.GetMail))
	}
}

// listBar is the segment over the list.
func (w *Window) listBar(c *ui.Context) {
	ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
		frostui.Strong(c, w.title()).SingleLine()
		frostui.Caption(c, w.subtitle()).SingleLine()
	})
	if frostui.IconButton(c, "mail-message-new", w.table.Tooltip(string(commands.Compose))).Clicked() {
		w.compose()
	}
}

// readerBar is the segment over the reader.
func (w *Window) readerBar(c *ui.Context) {
	ids := w.selectedIDs()
	rows := w.rows(ids)
	has := len(rows) > 0 && !w.readOnly(rows[0].AccountID)
	if frostui.IconButton(c, "user-trash", "Delete").Disabled(!has).Clicked() {
		w.delete(ids)
	}
	canArchive := has && mail.ArchiveOf(rows[0].AccountID, w.mail.State().Mailboxes) != nil
	if frostui.IconButton(c, "mail-archive", "Archive").Disabled(!canArchive).Clicked() {
		w.archive(ids)
	}
	one := len(rows) == 1
	if frostui.IconButton(c, "mail-reply-sender", w.table.Tooltip(string(commands.Reply))).Disabled(!one).Clicked() {
		w.reply(api.DraftKindReply)
	}
	if frostui.IconButton(c, "mail-reply-all", w.table.Tooltip(string(commands.ReplyAll))).Disabled(!one).Clicked() {
		w.reply(api.DraftKindReplyall)
	}
	if frostui.IconButton(c, "mail-forward", w.table.Tooltip(string(commands.Forward))).Disabled(!one).Clicked() {
		w.forward()
	}
	ui.MenuButton(c, "", func(m *ui.Menu) { w.flagMenu(m, ids, rows) }).Disabled(!has).Label("Flag").Tooltip("Flag").Children(func() {
		frostui.Icon(c, "mail-mark-important")
	})
	if frostui.IconButton(c, "mail-unread", "Mark as Read or Unread").Disabled(!has).Clicked() {
		w.toggleRead(ids)
	}
	ui.MenuButton(c, "", func(m *ui.Menu) { w.moveMenu(m, ids, rows) }).Disabled(!has).Label("Move").Tooltip("Move").Children(func() {
		frostui.Icon(c, "folder")
	})
}

func (w *Window) flagMenu(m *ui.Menu, ids []int64, rows []api.MessageSummary) {
	if len(rows) == 0 {
		return
	}
	names := w.mailNames().FlagNames
	for color := int64(1); color <= 7; color++ {
		if m.Item(mail.FlagLabel(int(color), names)).Checked(rows[0].Flags.FlagColor == color).Chosen() {
			w.setFlagColor(ids, color)
		}
	}
	m.Separator()
	if m.Item("Clear Flag").Chosen() {
		w.setFlagColor(ids, 0)
	}
}

func (w *Window) moveMenu(m *ui.Menu, ids []int64, rows []api.MessageSummary) {
	if len(rows) == 0 {
		return
	}
	targets := mail.MoveTargets(&rows[0], w.mail.State().Mailboxes)
	for i := range targets {
		if m.Item(targets[i].Path).Chosen() {
			w.move(ids, &targets[i])
		}
	}
}

func (w *Window) title() string {
	if w.q.Search != "" {
		return "Searching"
	}
	if t := mail.SourceTitle(w.q.Source, w.mailNames()); t != "" {
		return t
	}
	return "Mail"
}

func (w *Window) subtitle() string {
	if w.view == nil {
		return ""
	}
	return fmt.Sprintf("%d %s", w.view.Count(), map[bool]string{true: "message", false: "messages"}[w.view.Count() == 1])
}
