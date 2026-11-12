package settings

import (
	"context"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
)

// rulesPane is the Rules pane; the rules are in the window's data.
type rulesPane struct {
	selected int64
	err      string
}

// rulesChanged forgets a selected rule that is gone.
func (w *Window) rulesChanged() {
	if p := w.rule; p != nil && p.selected != 0 && w.ruleIndex(p.selected) < 0 {
		p.selected = 0
	}
}

func (w *Window) ruleIndex(id int64) int {
	return slices.IndexFunc(w.data.rules, func(r api.Rule) bool { return r.ID == id })
}

// ruleCall runs a change; its error stays until the next change succeeds.
func (w *Window) ruleCall(p *rulesPane, f func(ctx context.Context, c *api.Client) error) {
	cn := w.cn
	if cn == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(cn.ctx, callTimeout)
		defer cancel()
		err := f(ctx, cn.client)
		w.o.Post(func() {
			p.err = ""
			if err != nil {
				p.err = message(err)
			}
		})
	}()
}

func (w *Window) toggleRule(p *rulesPane, r api.Rule, enabled bool) {
	w.ruleCall(p, func(ctx context.Context, c *api.Client) error {
		_, err := c.Rule().Update(ctx, &api.RuleUpdateParams{ID: r.ID, Enabled: &enabled})
		return err
	})
}

func (w *Window) moveRule(p *rulesPane, r api.Rule, position int64) {
	w.ruleCall(p, func(ctx context.Context, c *api.Client) error {
		return c.Rule().Move(ctx, &api.RuleMoveParams{ID: r.ID, Position: position})
	})
}

func (w *Window) duplicateRule(p *rulesPane, r api.Rule) {
	w.ruleCall(p, func(ctx context.Context, c *api.Client) error {
		enabled := r.Enabled
		copied, err := c.Rule().Create(ctx, &api.RuleCreateParams{Name: r.Name + " Copy", Conditions: r.Conditions, Actions: r.Actions, Enabled: &enabled})
		if err != nil {
			return err
		}
		return c.Rule().Move(ctx, &api.RuleMoveParams{ID: copied.ID, Position: r.Position + 1})
	})
}

func (w *Window) removeRule(p *rulesPane, r api.Rule) {
	w.ask(frostui.Confirm{Title: "Remove Rule?", Body: `Remove the rule "` + r.Name + `"?`, Action: "Remove", Destructive: true, OnConfirm: func() {
		w.ruleCall(p, func(ctx context.Context, c *api.Client) error {
			return c.Rule().Delete(ctx, &api.RuleDeleteParams{ID: r.ID})
		})
	}})
}

func (w *Window) editRule(r *api.Rule) {
	if w.o.EditRule != nil {
		w.o.EditRule(r)
	}
}

func (w *Window) rulesBody(c *ui.Context) {
	p := w.rule
	if p == nil {
		return
	}
	rules := w.data.rules
	index := w.ruleIndex(p.selected)
	var selected *api.Rule
	if index >= 0 {
		selected = &rules[index]
	}
	ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
		listBox(c, "Rules", func() {
			if len(rules) == 0 {
				ui.Row(c).Height(28).Justify(ui.Center).AlignItems(ui.Center).Children(func() { frostui.Muted(c, "No Rules") })
			}
			for _, r := range rules {
				w.ruleRow(c.Key(r.ID), p, r)
			}
		})
		ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Wrap().Children(func() {
			if ui.Button(c, "Add Rule").Clicked() {
				w.editRule(nil)
			}
			if ui.Button(c, "Edit").Disabled(selected == nil).Clicked() {
				w.editRule(selected)
			}
			if ui.Button(c, "Duplicate").Disabled(selected == nil).Clicked() {
				w.duplicateRule(p, *selected)
			}
			if ui.Button(c, "Remove").Disabled(selected == nil).Clicked() {
				w.removeRule(p, *selected)
			}
			ui.Spacer(c)
			if frostui.LabeledButton(c, "go-up", "Move Up", false).Disabled(selected == nil || index == 0).Clicked() {
				w.moveRule(p, *selected, int64(index-1))
			}
			if frostui.LabeledButton(c, "go-down", "Move Down", false).Disabled(selected == nil || index == len(rules)-1).Clicked() {
				w.moveRule(p, *selected, int64(index+1))
			}
		})
		alert(c, p.err)
	})
}

func (w *Window) ruleRow(c *ui.Context, p *rulesPane, r api.Rule) {
	t := c.Theme()
	row := ui.Row(c).Height(28).Gap(frostui.SpaceS).Padding(0, frostui.SpaceS).AlignItems(ui.Center)
	if r.ID == p.selected {
		row.Background(t.SurfaceHover)
	}
	row.Children(func() {
		on := r.Enabled
		if ui.CheckboxBase(c, &on).Label("Enable " + r.Name).Children(func() { checkMark(c, on) }).Changed() {
			w.toggleRule(p, r, on)
		}
		name := ui.ButtonBase(c).Grow(1).Shrink(1).MinWidth(0).Justify(ui.Start).Height(28).Cursor(ui.CursorPointer).Label(r.Name).Checked(r.ID == p.selected)
		name.Children(func() {
			text := frostui.Body(c, r.Name).SingleLine()
			if !r.Enabled {
				text.TextColor(t.TextMuted)
			}
		})
		if name.Clicked() {
			p.selected = r.ID
		}
		if name.DoubleClicked() {
			w.editRule(&r)
		}
		if r.Problem != nil && *r.Problem != "" {
			frostui.Icon(c, "dialog-warning").FontSize(frostui.TextSize(c, 14)).TextColor(t.TextMuted).Role(ui.RoleImage).Label(*r.Problem).Tooltip(*r.Problem)
		}
	})
}

// checkMark is a check box's face, for one with no text.
func checkMark(c *ui.Context, on bool) {
	t := c.Theme()
	box := ui.Box(c).Size(16, 16).Radius(frostui.RadiusChip).Border(1, t.Border).Justify(ui.Center).AlignItems(ui.Center)
	if on {
		box.Background(t.Accent).TextColor(t.AccentText).Children(func() { frostui.Icon(c, "object-select").FontSize(frostui.TextSize(c, 12)) })
	}
}
