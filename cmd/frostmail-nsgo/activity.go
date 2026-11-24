package main

import (
	"context"
	"fmt"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity/entry"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
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
			cards = append(cards, workspaceui.Row(core.Text{Content: a.Email + " · " + line}, button("Sync "+a.Email, func() {
				w.change(func(ctx context.Context, c *api.Client) error {
					return c.Sync().Now(ctx, &api.SyncNowParams{AccountID: a.ID})
				}, nil)
			}), button("Connection doctor", func() { w.accountMenu(a) })))
		}
		return workspaceui.Stack(cards...)
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
	return core.View{Style: ui.FillRest, Children: []core.Node{statuses, workspaceui.Row(ui.Select[string]{Label: "Activity level", Options: named("All", "info", "warn", "error"), Value: filter.Get, OnChange: func(v string) { filter.Set(v); load() }}, button("Refresh activity", load)), ui.List{Label: "Activity entries", Style: ui.FillRest, EstimatedSize: 44, Count: func() int { return len(rows.Get()) }, Row: func(_ *core.Scope, row core.ListRow) core.Node {
		return core.Dynamic(func(*core.Scope) core.Node {
			all := rows.Get()
			i := row.Index()
			if i < 0 || i >= len(all) {
				return nil
			}
			e := all[i]
			source, line := entry.Line(e, names.Get())
			return core.Text{Content: fmt.Sprintf("%s  %s · %s · %s", e.At.Local().Format("15:04:05"), e.Level, source, line), Selectable: core.SelectableOn}
		})
	}, Empty: core.Text{Content: "No activity"}}}}
}
