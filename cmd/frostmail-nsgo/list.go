package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/live"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The list pane (plan 0016, Phase 3b): the search field over what it
// filters, the filter segment and the sort menu, and the messages as the
// kit's summary rows in NSGo's list, whose selection is the app's.

// tabFilters are the filters that are tabs; the rest are a menu's items.
const tabFilters = 3

// pending is the key of a row whose message maild has not sent yet.
type pending int

// messageList is the list pane.
func (w *workspace) messageList() core.Node {
	search := func(s string) { w.search = strings.TrimSpace(s); w.openView() }
	field := core.Dynamic(func(*core.Scope) core.Node {
		return workspaceui.SearchField{Placeholder: "Search " + w.title(), Keys: "mod+alt+f", Ref: w.searchInput, Value: w.term.Get, OnSubmit: search,
			OnChange: func(s string) {
				w.term.Set(s)
				// Clearing the field ends a search; typing starts none until Enter.
				if s == "" && w.search != "" {
					search("")
				}
			}}
	})
	return core.View{Style: ui.FillRest.Merge(core.Style{Gap: 8, Padding: core.Edges{Top: 8, Left: 8, Right: 8}}), Children: []core.Node{
		field, w.listOptions(),
		ui.List{Label: "Messages", TestID: "messages", Style: ui.FillRest, Gap: 2, Selection: w.selection, TapSelects: true,
			EstimatedSize: 84, RowStyle: core.Style{Group: rowGroup}, RowStyleFunc: workspaceui.SoftRows(w.cx),
			Count: func() int {
				w.rows.Get()
				if w.view == nil {
					return 0
				}
				return int(w.view.Count())
			},
			Key: func(i int) any {
				if m, ok := w.row(i); ok {
					return m.ID
				}
				return pending(i)
			},
			RowLabel: func(i int) string {
				if m, ok := w.row(i); ok {
					return m.Subject
				}
				return "Loading"
			},
			Row:               w.messageRow,
			OnSelectionChange: func(*ui.Selection) { w.selectionChanged() },
			OnActivate: func(i int) {
				if m, ok := w.row(i); ok {
					w.openMessage(m.ID, false)
				}
			},
			Empty: core.Dynamic(w.emptyList)},
	}}
}

// row is the list's row i, when maild has sent it. It follows the rows as
// they arrive, so a row's key and label do too.
func (w *workspace) row(i int) (api.MessageSummary, bool) {
	w.rows.Get()
	if w.view == nil {
		return api.MessageSummary{}, false
	}
	return w.view.Row(int64(i))
}

// selectedIDs are the messages chosen in the list.
func (w *workspace) selectedIDs() []int64 {
	keys := w.selection.Keys()
	ids := make([]int64, 0, len(keys))
	for _, k := range keys {
		if id, ok := k.(int64); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// selectionChanged follows the list's selection: one chosen message is
// read, and none or several leave the reader with no message.
func (w *workspace) selectionChanged() {
	ids := w.selectedIDs()
	if len(ids) == 1 {
		if ids[0] != w.opened.GetUntracked() {
			w.openMessage(ids[0], false)
		}
		return
	}
	if w.opened.GetUntracked() != 0 {
		w.generation++
		w.opened.Set(0)
		w.reader.Set(nil)
	}
}

// messageRow is a row of the list: the kit's summary row, which can be
// dragged to a mailbox and has the row's quick actions.
func (w *workspace) messageRow(cx *core.Scope, row core.ListRow) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		w.rows.Get()
		w.state.Get()
		st := w.store.State()
		lines := mail.PreviewLines(st.Settings)
		idx := row.Index()
		msg, ok := w.row(idx)
		if !ok {
			if w.view != nil {
				w.view.Ensure(int64(idx), int64(idx)+1)
			}
			return ui.Skeleton{Avatar: true, Lines: 2 + min(lines, 1), Label: "Loading message", Style: core.Style{Width: core.Pct(100)}}
		}
		sender := msg.From.Name
		if sender == "" {
			sender = msg.From.Address
		}
		unread := !msg.Flags.Seen
		if msg.ThreadUnread != nil {
			unread = *msg.ThreadUnread > 0
		}
		summary := workspaceui.SummaryRow{Unread: unread, Name: sender, Title: sender, When: mail.ListDate(msg.Date, time.Now()),
			Subject: msg.Subject, Marks: w.marks(cx, msg, st), Preview: msg.Preview, PreviewLines: lines}
		if w.showPhotos.Get() && w.photos != nil {
			w.photos.Want([]string{msg.From.Address})
			if photo := w.photos.Get(msg.From.Address); len(photo) > 0 {
				summary.Image = core.ImageSource{Data: photo}
			}
		}
		return w.rowWithActions(cx, msg, w.dragRow(cx, msg, summary))
	})
}

// flagTones are the tones a flag's 7 colors draw in. The kit has no flag
// colors: Yellow draws as Orange and Purple as the accent until it has.
var flagTones = [7]workspaceui.Tone{workspaceui.Danger, workspaceui.Warning, workspaceui.Warning, workspaceui.Success, workspaceui.Accent, workspaceui.Accent, workspaceui.Neutral}

// marks end a row's subject line: the VIP's star, the flag in its color,
// the paperclip, the reminder's clock, and a conversation's count.
func (w *workspace) marks(cx *core.Scope, m api.MessageSummary, st live.MailState) []core.Node {
	colors := ui.ColorsOf(cx)
	tones := [...]func() core.Color{workspaceui.Neutral: colors.Muted, workspaceui.Accent: colors.Accent, workspaceui.Success: colors.Up, workspaceui.Warning: colors.Warn, workspaceui.Danger: colors.Danger}
	mark := func(label string, icon func() *core.SVG, tint func() core.Color) core.Node {
		return core.View{Label: label, Children: []core.Node{ui.Icon(icon, 14, tint)}}
	}
	var out []core.Node
	if mail.IsVIP(m.From.Address, st.VIPs) {
		out = append(out, mark("VIP", lucide.Star, colors.Warn))
	}
	if m.Flags.Flagged {
		tone, label := workspaceui.Warning, "Flagged"
		if c := int(m.Flags.FlagColor); c >= 1 && c <= len(flagTones) {
			var names []string
			if st.Settings != nil {
				names = st.Settings.FlagNames
			}
			tone, label = flagTones[c-1], "Flagged "+mail.FlagLabel(c, names)
		}
		out = append(out, mark(label, lucide.Flag, tones[tone]))
	}
	if m.HasAttachments {
		out = append(out, mark("Has attachments", lucide.Paperclip, colors.Muted))
	}
	if m.RemindAt != nil {
		out = append(out, mark("Has a reminder", lucide.Clock, colors.Muted))
	}
	if w.listState.Conversations && m.ThreadCount > 1 {
		out = append(out, core.View{Label: fmt.Sprintf("%d messages", m.ThreadCount), Children: []core.Node{ui.Badge{Text: strconv.FormatInt(m.ThreadCount, 10)}}})
	}
	return out
}

// emptyList is what the list shows with no row: rows loading, an error, or
// that nothing is there or nothing matches.
func (w *workspace) emptyList(*core.Scope) core.Node {
	w.rows.Get()
	v := w.view
	switch {
	case v == nil:
		return nil
	case v.Err() != nil:
		return workspaceui.EmptyState{Icon: lucide.CircleAlert, Title: "The messages did not load", Hint: v.Err().Error()}
	case !v.Ready():
		var rows []core.Node
		for range 6 {
			rows = append(rows, ui.Skeleton{Avatar: true, Lines: 2, Label: "Loading messages"})
		}
		return core.View{Style: core.Style{Gap: 16, Padding: core.All(8), Width: core.Pct(100)}, Children: rows}
	case w.search != "" || w.listState.Filter != mail.FilterAll:
		return workspaceui.EmptyState{Icon: lucide.SearchX, Title: "No matching messages", Hint: "Try another search or filter."}
	}
	return workspaceui.EmptyState{Icon: lucide.Inbox, Title: "No messages"}
}

// listOptions is the row under the search field: the filter as a segment,
// the other filters, and the sort menu. Its nodes stay and read the list's
// state as they draw, so a menu stays open while rows arrive.
func (w *workspace) listOptions() core.Node {
	state := func() mail.ListState { w.rows.Get(); return w.listState }
	setFilter := func(f mail.Filter) { w.listState.Filter = f; w.openView() }
	var tabs []ui.Option[mail.Filter]
	var more, sorts []core.Node
	for i, f := range append(append([]mail.FilterChoice{}, mail.Filters...), mail.MoreFilters...) {
		if i < tabFilters {
			tabs = append(tabs, ui.Option[mail.Filter]{Value: f.Filter, Label: f.Label})
			continue
		}
		more = append(more, ui.MenuItem{Text: f.Label, Checked: func() bool { return state().Filter == f.Filter }, OnSelect: func() {
			if w.listState.Filter == f.Filter {
				setFilter(mail.FilterAll)
				return
			}
			setFilter(f.Filter)
		}})
	}
	for _, s := range mail.Sorts {
		sorts = append(sorts, ui.MenuItem{Text: s.Label, Checked: func() bool { return state().Sort == s.Field }, OnSelect: func() {
			w.listState.Sort = s.Field
			w.listState.Ascending = mail.DefaultAscending(s.Field)
			w.openView()
		}})
	}
	toggle := func(text string, on func() bool, flip func()) core.Node {
		return ui.MenuItem{Text: text, Checked: on, OnSelect: func() { flip(); w.openView() }}
	}
	sorts = append(sorts, ui.MenuSeparator{},
		toggle("Ascending", func() bool { return state().Ascending }, func() { w.listState.Ascending = !w.listState.Ascending }),
		ui.MenuSeparator{},
		toggle("Show conversations", func() bool { return state().Conversations }, func() { w.listState.Conversations = !w.listState.Conversations }),
		ui.MenuItem{Text: "Contact photos", Checked: w.showPhotos.Get, OnSelect: func() { w.showPhotos.Set(!w.showPhotos.GetUntracked()) }},
		toggle("Search all mailboxes", func() bool { return state().SearchAll }, func() { w.listState.SearchAll = !w.listState.SearchAll }),
		ui.MenuSeparator{},
		ui.MenuItem{Text: "Save search as a smart mailbox", Disabled: func() bool { return strings.TrimSpace(w.term.Get()) == "" }, OnSelect: func() {
			w.search = strings.TrimSpace(w.term.GetUntracked())
			w.openView()
			w.saveSearch()
		}})
	return core.Dynamic(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		// The other filters' button is in the accent while one of them is on.
		tint := func() core.Color {
			for _, t := range tabs {
				if t.Value == state().Filter {
					return colors.Text()
				}
			}
			return colors.Accent()
		}
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4}, Children: []core.Node{
			ui.Tabs[mail.Filter]{Variant: ui.TabsSegmented, Size: ui.SizeSmall, Label: "Filter", Value: func() mail.Filter { return state().Filter }, OnChange: setFilter, Options: tabs},
			ui.Spacer{},
			ui.Menu{Label: "More filters", Trigger: ui.Button{Label: "More filters", Icon: lucide.ListFilter, Variant: ui.ButtonGhost, Size: ui.SizeSmall, Tint: tint}, Children: more},
			ui.Menu{Label: "Sort", Trigger: ui.Button{Label: "Sort", Icon: lucide.ArrowDownUp, Variant: ui.ButtonGhost, Size: ui.SizeSmall}, Children: sorts}}}
	})
}

// bulkTools are the toolbar's tools while several messages are chosen:
// Delete, Flag and Move act on all of them, and the others wait for one.
func (w *workspace) bulkTools(ids []int64) ([]toolGroup, []core.Node) {
	groups, _ := w.messageTools(nil)
	undo := w.undo
	finished := func() { w.selection.Clear() }
	action := func(title string, fn func(context.Context, *api.Client) error) ui.MenuItem {
		return ui.MenuItem{Text: title, Disabled: w.acting.Get, OnSelect: func() { w.change(fn, finished) }}
	}
	flag := func(title string, ch api.FlagChanges) ui.MenuItem {
		return action(title, func(ctx context.Context, c *api.Client) error { return undo.Flags(ctx, c, ids, ch) })
	}
	yes, no := true, false
	file, mark := groups[0].tools, groups[2].tools
	file[1].run = func() {
		w.confirm("Delete selected messages?", fmt.Sprintf("%d selected messages", len(ids)), "Delete", func() {
			w.change(func(ctx context.Context, c *api.Client) error {
				_, err := undo.Move(ctx, c, ids, func() error { return c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: ids}) })
				return err
			}, func() { finished(); w.moved(fmt.Sprintf("%d messages deleted", len(ids)))() })
		})
	}
	mark[0].run = func() {
		w.change(func(ctx context.Context, c *api.Client) error {
			return undo.Flags(ctx, c, ids, api.FlagChanges{Flagged: &yes})
		}, nil)
	}
	mark[1].run = func() { w.moveSelected(ids) }
	more := []core.Node{flag("Mark as read", api.FlagChanges{Seen: &yes}), flag("Mark as unread", api.FlagChanges{Seen: &no}), flag("Clear flags", api.FlagChanges{Flagged: &no}),
		ui.MenuSeparator{},
		action("Apply mail rules", func(ctx context.Context, c *api.Client) error {
			_, err := c.Rule().Apply(ctx, &api.RuleApplyParams{IDs: ids})
			return err
		}),
		ui.MenuSeparator{},
		ui.MenuItem{Text: "Undo", Icon: lucide.Undo2, Shortcut: "mod+z", Disabled: w.acting.Get, OnSelect: func() {
			w.change(func(ctx context.Context, c *api.Client) error { _, err := undo.Do(ctx, c); return err }, nil)
		}}}
	return groups, more
}

// moveSelected asks where the chosen messages go, which must be of one
// account.
func (w *workspace) moveSelected(ids []int64) {
	undo := w.undo
	perform(w, func(ctx context.Context, c *api.Client) ([]api.MessageSummary, error) {
		return c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: ids})
	}, func(rows []api.MessageSummary, err error) {
		if err != nil {
			w.problem.Set(err.Error())
			return
		}
		if len(rows) != len(ids) {
			w.problem.Set("The selection has changed. Select the messages again.")
			return
		}
		for _, r := range rows {
			if r.AccountID != rows[0].AccountID {
				w.problem.Set("Choose messages from one account to move them.")
				return
			}
		}
		src := w.source
		boxes := w.store.State().Mailboxes
		w.show(func(*core.Scope) core.Node {
			var targets []core.Node
			for _, b := range mail.MoveTargets(&rows[0], boxes) {
				targets = append(targets, button(b.Path, func() {
					w.change(func(ctx context.Context, c *api.Client) error {
						_, err := undo.Move(ctx, c, ids, func() error { return c.Message().Move(ctx, mail.MoveParams(ids, b, src, boxes)) })
						return err
					}, func() {
						w.selection.Clear()
						w.closePanel()
						w.moved(fmt.Sprintf("%d moved to %s", len(ids), b.Name))()
					})
				}))
			}
			return w.dialog("Move messages", []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(410)}, Children: targets}})
		})
	})
}
