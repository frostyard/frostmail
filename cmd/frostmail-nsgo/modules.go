package main

import (
	"context"
	"strconv"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// The modules beside Mail (plan 0016, Phase 4d). Each gives the window's
// navigation its own rows while it is shown, in place of mail's: a
// calendar's calendars, the address books, the task lists, Settings'
// categories and Activity's levels. What a module shows of them it draws
// with the kit's parts and NSGo's controls, as Mail does.

// moduleNav is a module's rows in the window's navigation.
type moduleNav struct {
	sections func() []ui.SidebarSection
	selected func() string
	navigate func(id string)
}

// useNav gives the navigation nav's rows while cx lives.
func (w *workspace) useNav(cx *core.Scope, nav moduleNav) {
	mine := &nav
	w.nav.Set(mine)
	cx.Cleanup(func() {
		if w.nav.GetUntracked() == mine {
			w.nav.Set(nil)
		}
	})
}

// navRow is a row of a module's navigation.
func navRow(id, text string, icon func() *core.SVG) ui.SidebarItem {
	return ui.SidebarItem{ID: id, TestID: "nav-" + id, Text: text, Content: workspaceui.NavLabel{Icon: icon, Text: text}}
}

// collections are the calendars, address books and task lists of every
// account, which a module's navigation lists.
func (w *workspace) collections(cx *core.Scope) func() []api.Collection {
	rows, _ := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.AccountChanged); return ok }, func() func(context.Context, *api.Client) ([]api.Collection, error) {
		return func(ctx context.Context, c *api.Client) ([]api.Collection, error) {
			return c.Account().Collections(ctx, &api.AccountCollectionsParams{})
		}
	})
	return rows.Get
}

// collectionNav is a navigation of one kind of collection: a row for all of
// them, then a row for each that is on. chosen is the collection shown, 0
// for all.
func (w *workspace) collectionNav(cx *core.Scope, title, all string, icon func() *core.SVG, include func(api.Collection) bool, chosen *core.Signal[int64], load func(), before ...ui.SidebarSection) moduleNav {
	list := w.collections(cx)
	return moduleNav{
		sections: func() []ui.SidebarSection {
			items := []ui.SidebarItem{navRow("0", all, icon)}
			for _, c := range list() {
				if include(c) && c.Enabled {
					items = append(items, navRow(strconv.FormatInt(c.ID, 10), c.Name, icon))
				}
			}
			return append(append([]ui.SidebarSection{}, before...), ui.SidebarSection{Title: title, Items: items})
		},
		selected: func() string { return strconv.FormatInt(chosen.Get(), 10) },
		navigate: func(id string) {
			if n, err := strconv.ParseInt(id, 10, 64); err == nil {
				chosen.Set(n)
				load()
			}
		},
	}
}

// moduleBar is the row of controls over a module's content, which never
// wraps.
func moduleBar(children ...core.Node) core.Node {
	return core.View{TestID: "module-bar", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Edges{Top: 8, Left: 8, Right: 8}}, Children: children}
}

// collectionList is a list of rows to press: a title over a line of
// detail, with what leading draws before them.
func collectionList[T any](rows *core.Signal[[]T], label string, title, subtitle func(T) string, show func(T), leading ...func(T) core.Node) core.Node {
	return ui.List{Label: label, Style: ui.FillRest, EstimatedSize: 52, Count: func() int { return len(rows.Get()) }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Dynamic(func(*core.Scope) core.Node {
			all := rows.Get()
			i := row.Index()
			if i < 0 || i >= len(all) {
				return nil
			}
			v := all[i]
			r := ui.FormRow{Title: title(v), Detail: subtitle(v), TitleMaxLines: 1, OnPress: func() { show(v) }}
			if len(leading) > 0 {
				r.Leading = leading[0](v)
			}
			return core.View{Style: core.Style{Width: core.Pct(100)}, Children: []core.Node{r}}
		})
	}, Empty: workspaceui.EmptyState{Icon: lucide.Inbox, Title: "No matching items"}}
}

// taskRow is a task: the box that marks it done, its title, which opens
// it, and when it is due.
func (w *workspace) taskRow(t api.Task, checkLabel string, detail string, check func(done bool), open func()) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4, Width: core.Pct(100)}, Children: []core.Node{
		ui.Checkbox{Label: checkLabel + t.Title, Checked: func() bool { return t.Completed }, Disabled: func() bool { return t.ReadOnly }, OnChange: check},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			ui.FormRow{Title: t.Title, Detail: strings.TrimSpace(detail), TitleMaxLines: 1, OnPress: open}}}}}
}
