package main

import (
	"context"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity/entry"
	"github.com/frostyard/frostui/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type activityNames struct{ accounts, mailboxes map[int64]string }

func (n activityNames) Account(id int64) string { return n.accounts[id] }
func (n activityNames) Mailbox(id int64) string { return n.mailboxes[id] }
func (w *workspace) activity(cx *core.Scope) core.Node {
	filter := core.NewSignal(cx, "All")
	rows, load := collection(cx, w, func(ev api.Event) bool { _, ok := ev.(api.ActivityAdded); return ok }, func() func(context.Context, *api.Client) ([]api.ActivityEntry, error) {
		level := filter.GetUntracked()
		return func(ctx context.Context, c *api.Client) ([]api.ActivityEntry, error) {
			limit := int64(entry.MaxRecent)
			recent, err := c.Activity().Recent(ctx, &api.ActivityRecentParams{Limit: &limit})
			if err != nil {
				return nil, err
			}
			values := make([]api.ActivityEntry, 0, len(recent))
			for i := len(recent) - 1; i >= 0; i-- {
				e := recent[i]
				if level == "All" || level == string(e.Level) {
					values = append(values, e)
				}
			}
			return values, nil
		}
	})
	statuses := core.Dynamic(func(*core.Scope) core.Node {
		w.state.Get()
		st := w.store.State()
		var cards []core.Node
		for _, a := range st.Accounts {
			s := st.Sync[a.ID]
			line := string(s.Phase)
			if s.Mailbox != nil {
				line += " · " + *s.Mailbox
			}
			if s.Error != nil {
				line += " · " + *s.Error
			}
			cards = append(cards, ui.FormRow{Title: a.Email, Detail: line, Control: workspaceui.Row(small(button("Sync "+a.Email, func() {
				w.change(func(ctx context.Context, c *api.Client) error {
					return c.Sync().Now(ctx, &api.SyncNowParams{AccountID: a.ID})
				}, nil)
			})), small(button("Connection doctor", func() { w.accountMenu(a) })))})
		}
		if len(cards) == 0 {
			return nil
		}
		return core.View{Style: core.Style{Padding: core.Edges{Left: 8, Right: 8}}, Children: []core.Node{ui.FormSection{Children: cards}}}
	})
	names := core.NewMemo(cx, func() *activityNames {
		w.state.Get()
		st := w.store.State()
		n := activityNames{map[int64]string{}, map[int64]string{}}
		for _, a := range st.Accounts {
			n.accounts[a.ID] = a.Email
		}
		for _, b := range st.Mailboxes {
			n.mailboxes[b.ID] = b.Path
		}
		return &n
	})
	levels := []struct{ id, name string }{{"All", "All activity"}, {"info", "Information"}, {"warn", "Warnings"}, {"error", "Errors"}}
	w.useNav(cx, moduleNav{
		sections: func() []ui.SidebarSection {
			var items []ui.SidebarItem
			for _, l := range levels {
				items = append(items, navRow(l.id, l.name, lucide.Activity))
			}
			return []ui.SidebarSection{{Title: "Show", Items: items}}
		},
		selected: filter.Get,
		navigate: func(id string) { filter.Set(id); load() },
	})
	return core.View{Style: ui.FillRest.Merge(core.Style{Gap: 8}), Children: []core.Node{moduleBar(ui.Spacer{}, ui.IconButton{Label: "Refresh activity", Icon: lucide.RefreshCw, OnPress: load}), statuses, ui.List{Label: "Activity entries", Style: ui.FillRest, EstimatedSize: 44, Count: func() int { return len(rows.Get()) }, Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Dynamic(func(*core.Scope) core.Node {
			all := rows.Get()
			i := row.Index()
			if i < 0 || i >= len(all) {
				return nil
			}
			e := all[i]
			source, line := entry.Line(e, names.Get())
			variant := ui.BadgeNeutral
			switch string(e.Level) {
			case "warn":
				variant = ui.BadgeWarn
			case "error":
				variant = ui.BadgeDanger
			}
			return core.View{TestID: "activity-entry", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Width: core.Pct(100)}, Children: []core.Node{
				figures(cx, e.At.Local().Format("15:04:05")),
				ui.Badge{Text: string(e.Level), Variant: variant},
				core.Text{Content: source + " · " + line, MaxLines: 2, Selectable: core.SelectableOn, Style: ui.TypeBody.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)})}}}
		})
	}, Empty: workspaceui.EmptyState{Icon: lucide.Activity, Title: "No activity", Hint: "What maild does in the background is listed here."}}}}
}
