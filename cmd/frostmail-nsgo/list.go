package main

import (
	"context"
	"fmt"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"slices"
	"strconv"
)

func (w *workspace) listOptions() core.Node {
	var filters, sorts []core.Node
	for _, f := range slices.Concat(mail.Filters, mail.MoreFilters) {
		filters = append(filters, ui.MenuItem{Text: f.Label, OnSelect: func() { w.listState.Filter = f.Filter; w.openView() }})
	}
	for _, s := range mail.Sorts {
		sorts = append(sorts, ui.MenuItem{Text: s.Label, OnSelect: func() {
			w.listState.Sort = s.Field
			w.listState.Ascending = mail.DefaultAscending(s.Field)
			w.openView()
		}})
	}
	return core.Dynamic(func(*core.Scope) core.Node {
		w.rows.Get()
		scope := "Search all mailboxes"
		if w.listState.SearchAll {
			scope = "Search this mailbox"
		}
		conversation := "Show conversations"
		if w.listState.Conversations {
			conversation = "Show individual messages"
		}
		order := "Sort ascending"
		if w.listState.Ascending {
			order = "Sort descending"
		}
		filterLabel := "Filter"
		for _, f := range slices.Concat(mail.Filters, mail.MoreFilters) {
			if f.Filter == w.listState.Filter && f.Filter != mail.FilterAll {
				filterLabel = f.Label
			}
		}
		return workspaceui.Row(ui.Menu{Trigger: ui.Button{Text: filterLabel}, Children: filters}, ui.Menu{Trigger: ui.Button{Text: "List options"}, Children: []core.Node{ui.Submenu{Text: "Sort by", Children: sorts}, ui.MenuItem{Text: order, OnSelect: func() { w.listState.Ascending = !w.listState.Ascending; w.openView() }}, ui.MenuItem{Text: conversation, OnSelect: func() { w.listState.Conversations = !w.listState.Conversations; w.openView() }}, ui.MenuItem{Text: scope, OnSelect: func() { w.listState.SearchAll = !w.listState.SearchAll; w.openView() }}}}, ui.Checkbox{Text: "Select messages", Checked: w.selecting.Get, OnChange: func(v bool) { w.selecting.Set(v); w.selection.Clear() }})
	})
}
func (w *workspace) bulkActions() core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		if !w.selecting.Get() || w.selection.Len() == 0 {
			return nil
		}
		keys := w.selection.Keys()
		ids := make([]int64, 0, len(keys))
		for _, k := range keys {
			if id, ok := k.(int64); ok {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			return nil
		}
		finished := func() { w.selection.Clear() }
		action := func(title string, fn func(context.Context, *api.Client) error) ui.MenuItem {
			return ui.MenuItem{Text: title, Disabled: w.acting.Get, OnSelect: func() { w.change(fn, finished) }}
		}
		flag := func(title string, ch api.FlagChanges) core.Node {
			return action(title, func(ctx context.Context, c *api.Client) error { return w.undo.Flags(ctx, c, ids, ch) })
		}
		yes, no := true, false
		items := []core.Node{flag("Mark read", api.FlagChanges{Seen: &yes}), flag("Mark unread", api.FlagChanges{Seen: &no}), flag("Flag messages", api.FlagChanges{Flagged: &yes}), flag("Clear flags", api.FlagChanges{Flagged: &no})}
		items = append(items, action("Apply rules to selection", func(ctx context.Context, c *api.Client) error {
			_, err := c.Rule().Apply(ctx, &api.RuleApplyParams{IDs: ids})
			return err
		}))
		items = append(items, ui.MenuItem{Text: "Move messages", OnSelect: func() {
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
								_, err := w.undo.Move(ctx, c, ids, func() error { return c.Message().Move(ctx, mail.MoveParams(ids, b, src, boxes)) })
								return err
							}, func() { finished(); w.closePanel() })
						}))
					}
					return w.dialog("Move messages", []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(410)}, Children: targets}})
				})
			})
		}}, ui.MenuItem{Text: "Delete selected messages", OnSelect: func() {
			w.confirm("Delete selected messages?", fmt.Sprintf("%d selected messages", len(ids)), "Delete", func() {
				w.change(func(ctx context.Context, c *api.Client) error {
					_, err := w.undo.Move(ctx, c, ids, func() error { return c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: ids}) })
					return err
				}, finished)
			})
		}})
		return ui.Menu{Trigger: ui.Button{Text: strconv.Itoa(len(ids)) + " selected"}, Children: items}
	})
}
