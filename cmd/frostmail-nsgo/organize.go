package main

import (
	"context"
	"fmt"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/organize"
	"github.com/frostyard/pocketgo/desktop/workspaceui"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"slices"
	"strconv"
	"strings"
	"time"
)

func check(cx *core.Scope, label string, v *bool) core.Node {
	s := core.NewSignal(cx, *v)
	return ui.Checkbox{Text: label, Checked: s.Get, OnChange: func(on bool) { *v = on; s.Set(on) }}
}
func (w *workspace) filters(cx *core.Scope) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		w.state.Get()
		st := w.store.State()
		nodes := []core.Node{workspaceui.Row(button("New rule", func() { w.filterForm(api.Rule{Enabled: true}, nil) }), button("New smart mailbox", func() { w.filterForm(api.Rule{}, &api.SmartMailbox{}) })), core.Text{Content: "Rules", Style: ui.TypeTitle}}
		for i, r := range st.Rules {
			detail := "Enabled"
			if !r.Enabled {
				detail = "Disabled"
			}
			if r.Problem != nil {
				detail += " · " + *r.Problem
			}
			nodes = append(nodes, workspaceui.Card{Children: []core.Node{core.Text{Content: r.Name, Style: ui.TypeLabel}, core.Text{Content: detail}, workspaceui.Row(button("Edit "+r.Name, func() { w.filterForm(r, nil) }), button("Duplicate", func() { r.ID = 0; r.Name += " Copy"; w.filterForm(r, nil) }), ui.Button{Text: "Move up", Disabled: func() bool { return i == 0 }, OnPress: func() { w.moveRule(r, -1) }}, ui.Button{Text: "Move down", Disabled: func() bool { return i+1 == len(st.Rules) }, OnPress: func() { w.moveRule(r, 1) }}, button("Delete rule", func() {
				w.confirm("Delete rule?", r.Name, "Delete", func() {
					w.change(func(ctx context.Context, c *api.Client) error {
						return c.Rule().Delete(ctx, &api.RuleDeleteParams{ID: r.ID})
					}, nil)
				})
			}))}})
		}
		nodes = append(nodes, core.Text{Content: "Smart mailboxes", Style: ui.TypeTitle})
		for _, s := range st.Smart {
			nodes = append(nodes, workspaceui.Row(button("Edit "+s.Name, func() { w.filterForm(api.Rule{}, &s) }), button("Delete smart mailbox", func() {
				w.confirm("Delete smart mailbox?", s.Name, "Delete", func() {
					w.change(func(ctx context.Context, c *api.Client) error {
						return c.Smart().Delete(ctx, &api.SmartDeleteParams{ID: s.ID})
					}, nil)
				})
			})))
		}
		return workspaceui.Page(nodes...)
	})
}
func (w *workspace) moveRule(r api.Rule, delta int64) {
	w.change(func(ctx context.Context, c *api.Client) error {
		return c.Rule().Move(ctx, &api.RuleMoveParams{ID: r.ID, Position: r.Position + delta})
	}, nil)
}
func (w *workspace) filterForm(rule api.Rule, smart *api.SmartMailbox) {
	w.show(func(cx *core.Scope) core.Node {
		title := "Mail rule"
		name, cond := rule.Name, rule.Conditions
		if smart != nil {
			title = "Smart mailbox"
			name, cond = smart.Name, smart.Conditions
		}
		cond.Conditions = slices.Clone(cond.Conditions)
		if len(cond.Conditions) == 0 {
			cond = api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{organize.NewCondition()}}
		}
		actions := slices.Clone(rule.Actions)
		if len(actions) == 0 {
			actions = []api.RuleAction{{Kind: api.RuleActionKindRead}}
		}
		st := w.store.State()
		env := organize.Context{Today: time.Now().Format(time.DateOnly), Mailboxes: st.Mailboxes, Accounts: st.Accounts}
		if st.Settings != nil {
			env.FlagNames = st.Settings.FlagNames
		}
		var fields []ui.Option[api.ConditionField]
		for _, f := range organize.Fields {
			fields = append(fields, ui.Option[api.ConditionField]{Value: f.Field, Label: f.Label})
		}
		rev := core.NewSignal(cx, 0)
		rows := core.Dynamic(func(cx *core.Scope) core.Node {
			rev.Get()
			var nodes []core.Node
			for i, row := range cond.Conditions {
				ops := organize.OpsFor(row.Field)
				var options []ui.Option[api.ConditionOp]
				for _, op := range ops {
					options = append(options, ui.Option[api.ConditionOp]{Value: op, Label: organize.OpLabel[op]})
				}
				part := []core.Node{workspaceui.Row(choice("Condition field", row.Field, fields, func(v api.ConditionField) { cond.Conditions[i] = organize.WithField(v, env); invalidate(rev) }, cx), choice("Operator", row.Op, options, func(v api.ConditionOp) { cond.Conditions[i] = organize.WithOp(row, v, env); invalidate(rev) }, cx))}
				switch organize.KindOf(row.Field, row.Op) {
				case organize.KindNone:
				case organize.KindYesNo:
					part = append(part, choice("Value", row.Value, named("true", "false"), func(v string) { cond.Conditions[i].Value = v }, cx))
				case organize.KindChoice:
					var choices []ui.Option[string]
					switch row.Field {
					case api.ConditionFieldAccount:
						for _, a := range st.Accounts {
							choices = append(choices, ui.Option[string]{Value: strconv.FormatInt(a.ID, 10), Label: a.Email})
						}
					case api.ConditionFieldMailbox:
						for _, b := range st.Mailboxes {
							choices = append(choices, ui.Option[string]{Value: strconv.FormatInt(b.ID, 10), Label: b.Path})
						}
					case api.ConditionFieldRole:
						for _, r := range organize.Roles {
							choices = append(choices, ui.Option[string]{Value: r[0], Label: r[1]})
						}
					case api.ConditionFieldColor:
						for j := 1; j <= 7; j++ {
							choices = append(choices, ui.Option[string]{Value: strconv.Itoa(j), Label: fmt.Sprint(j)})
						}
					}
					part = append(part, choice("Value", row.Value, choices, func(v string) { cond.Conditions[i].Value = v }, cx))
				default:
					part = append(part, field("Condition value", ui.Input{Label: "Condition value", DefaultValue: row.Value, OnChange: func(v string) { cond.Conditions[i].Value = v }}))
				}
				if len(cond.Conditions) > 1 {
					part = append(part, button("Remove condition", func() { cond.Conditions = slices.Delete(cond.Conditions, i, i+1); invalidate(rev) }))
				}
				nodes = append(nodes, workspaceui.Card{Children: part})
			}
			nodes = append(nodes, button("Add condition", func() { cond.Conditions = append(cond.Conditions, organize.NewCondition()); invalidate(rev) }))
			if smart == nil {
				var kinds []ui.Option[api.RuleActionKind]
				for _, a := range organize.Actions {
					kinds = append(kinds, ui.Option[api.RuleActionKind]{Value: a.Kind, Label: a.Label})
				}
				for i, a := range actions {
					part := []core.Node{choice("Action", a.Kind, kinds, func(k api.RuleActionKind) { actions[i] = organize.WithKind(a, k); invalidate(rev) }, cx)}
					if organize.TakesMailbox(a.Kind) {
						var boxes []ui.Option[int64]
						for _, b := range st.Mailboxes {
							boxes = append(boxes, ui.Option[int64]{Value: b.ID, Label: b.Path})
						}
						part = append(part, choice("Destination", settingValue(a.MailboxID, int64(0)), boxes, func(id int64) { actions[i].MailboxID = &id }, cx))
					}
					if a.Kind == api.RuleActionKindFlag {
						var colors []ui.Option[int64]
						for j := int64(1); j <= 7; j++ {
							colors = append(colors, ui.Option[int64]{Value: j, Label: strconv.FormatInt(j, 10)})
						}
						part = append(part, choice("Flag color", settingValue(a.Color, int64(1)), colors, func(v int64) { actions[i].Color = &v }, cx))
					}
					if len(actions) > 1 {
						part = append(part, button("Remove action", func() { actions = slices.Delete(actions, i, i+1); invalidate(rev) }))
					}
					nodes = append(nodes, workspaceui.Card{Children: part})
				}
				nodes = append(nodes, button("Add action", func() { actions = append(actions, api.RuleAction{Kind: api.RuleActionKindRead}); invalidate(rev) }))
			}
			return workspaceui.Stack(nodes...)
		})
		children := []core.Node{stringField("Name", &name, false), choice("Match conditions", cond.Match, named(api.ConditionMatchAll, api.ConditionMatchAny), func(v api.ConditionMatch) { cond.Match = v }, cx), rows}
		enabled := rule.Enabled
		var trash, sent bool
		if smart != nil {
			trash, sent = smart.IncludeTrash, smart.IncludeSent
			children = append(children, check(cx, "Include trash", &trash), check(cx, "Include sent", &sent))
		} else {
			children = append(children, check(cx, "Enable rule", &enabled))
		}
		return w.dialog(title, []core.Node{core.Scroll{Style: core.Style{MaxHeight: core.Pt(490)}, Children: children}}, button("Cancel", w.closePanel), button("Save", func() {
			if strings.TrimSpace(name) == "" {
				w.problem.Set("Enter a name.")
				return
			}
			conditions := cond
			conditions.Conditions = slices.Clone(cond.Conditions)
			acts := slices.Clone(actions)
			n, on, tr, se := name, enabled, trash, sent
			if smart == nil && !organize.ActionsComplete(acts, st.Mailboxes) {
				w.problem.Set("Choose a destination mailbox.")
				return
			}
			w.change(func(ctx context.Context, c *api.Client) error {
				if smart != nil {
					if smart.ID == 0 {
						_, err := c.Smart().Create(ctx, &api.SmartCreateParams{Name: n, Conditions: conditions, IncludeTrash: &tr, IncludeSent: &se})
						return err
					}
					_, err := c.Smart().Update(ctx, &api.SmartUpdateParams{ID: smart.ID, Name: &n, Conditions: &conditions, IncludeTrash: &tr, IncludeSent: &se})
					return err
				}
				if rule.ID == 0 {
					_, err := c.Rule().Create(ctx, &api.RuleCreateParams{Name: n, Conditions: conditions, Actions: acts, Enabled: &on})
					return err
				}
				_, err := c.Rule().Update(ctx, &api.RuleUpdateParams{ID: rule.ID, Name: &n, Conditions: &conditions, Actions: acts, Enabled: &on})
				return err
			}, w.closePanel)
		}))
	})
}
