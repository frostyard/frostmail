package organize

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/frostyard/frostui"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/mail"
)

// choice is a value and how a menu shows it.
type choice struct{ value, label string }

// picker draws a drop-down of choices over a value, and reports a change.
// MyGo's Select holds labels, so this maps them; a value that fits no
// choice (a condition made elsewhere) shows as it can and changes only
// when edited.
func picker(c *ui.Context, name string, choices []choice, value *string) bool {
	labels := make([]string, 0, len(choices))
	selected := ""
	for _, ch := range choices {
		labels = append(labels, ch.label)
		if ch.value == *value {
			selected = ch.label
		}
	}
	before := selected
	if !ui.Select(c, &selected, labels).Label(name).Changed() || selected == before {
		return false
	}
	i := slices.Index(labels, selected)
	if i < 0 {
		return false
	}
	*value = choices[i].value
	return true
}

func mailboxChoices(ctx Context) []choice {
	var out []choice
	for _, a := range ctx.Accounts {
		for _, m := range ctx.Mailboxes {
			if m.AccountID == a.ID {
				out = append(out, choice{strconv.FormatInt(m.ID, 10), a.Email + " — " + m.Path})
			}
		}
	}
	return out
}

func valueChoices(f api.ConditionField, ctx Context) []choice {
	var out []choice
	switch f {
	case api.ConditionFieldAccount:
		for _, a := range ctx.Accounts {
			out = append(out, choice{strconv.FormatInt(a.ID, 10), a.Email})
		}
	case api.ConditionFieldMailbox:
		return mailboxChoices(ctx)
	case api.ConditionFieldRole:
		for _, r := range Roles {
			out = append(out, choice{r[0], r[1]})
		}
	case api.ConditionFieldColor:
		out = colorChoices(ctx)
	}
	return out
}

func colorChoices(ctx Context) []choice {
	out := make([]choice, 0, 7)
	for i := 1; i <= 7; i++ {
		out = append(out, choice{strconv.Itoa(i), mail.FlagLabel(i, ctx.FlagNames)})
	}
	return out
}

// ConditionEditor is a controlled editor of conditions; it reports whether
// the user changed them.
func ConditionEditor(c *ui.Context, conds *api.Conditions, ctx Context, lead, trail string) bool {
	changed := false
	ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
			frostui.Body(c, lead)
			match := string(conds.Match)
			if picker(c, "Match", []choice{{string(api.ConditionMatchAll), "all"}, {string(api.ConditionMatchAny), "any"}}, &match) {
				conds.Match = api.ConditionMatch(match)
				changed = true
			}
			frostui.Body(c, trail)
		})
		for i := range conds.Conditions {
			if conditionRow(c.Key(i), conds, i, ctx) {
				changed = true
				break // the rows may have changed under the loop
			}
		}
	})
	return changed
}

func conditionRow(c *ui.Context, conds *api.Conditions, i int, ctx Context) bool {
	row := &conds.Conditions[i]
	n := i + 1
	changed := false
	ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
		fields := make([]choice, 0, len(Fields))
		for _, f := range Fields {
			fields = append(fields, choice{string(f.Field), f.Label})
		}
		field := string(row.Field)
		if picker(c, fmt.Sprintf("Condition %d field", n), fields, &field) {
			*row = WithField(api.ConditionField(field), ctx)
			changed = true
		}
		if KindOf(row.Field, row.Op) != KindYesNo {
			var ops []choice
			for _, op := range OpsFor(row.Field) {
				ops = append(ops, choice{string(op), OpLabel[op]})
			}
			op := string(row.Op)
			if picker(c, fmt.Sprintf("Condition %d operator", n), ops, &op) {
				*row = WithOp(*row, api.ConditionOp(op), ctx)
				changed = true
			}
		}
		if valueControl(c, row, n, ctx) {
			changed = true
		}
		ui.Spacer(c)
		if frostui.IconButton(c.Key("remove"), "list-remove", fmt.Sprintf("Remove condition %d", n)).Disabled(len(conds.Conditions) == 1).Clicked() {
			conds.Conditions = slices.Delete(slices.Clone(conds.Conditions), i, i+1)
			changed = true
		}
		if frostui.IconButton(c.Key("add"), "list-add", fmt.Sprintf("Add condition after %d", n)).Clicked() {
			conds.Conditions = slices.Insert(slices.Clone(conds.Conditions), i+1, NewCondition())
			changed = true
		}
	})
	return changed
}

func valueControl(c *ui.Context, row *api.Condition, n int, ctx Context) bool {
	name := fmt.Sprintf("Condition %d value", n)
	switch KindOf(row.Field, row.Op) {
	case KindText:
		return ui.TextInput(c, &row.Value).Label(name).Grow(1).Changed()
	case KindYesNo:
		return picker(c, name, []choice{{"true", "Yes"}, {"false", "No"}}, &row.Value)
	case KindChoice:
		return picker(c, name, valueChoices(row.Field, ctx), &row.Value)
	case KindChoices:
		return multiPicker(c, name, valueChoices(row.Field, ctx), &row.Value)
	case KindDuration:
		return durationControl(c, row, n)
	case KindDate:
		return dateControl(c, row, name)
	}
	return false
}

// multiPicker is a menu of choices with checks, joined with commas in
// option order.
func multiPicker(c *ui.Context, name string, choices []choice, value *string) bool {
	chosen := strings.Split(*value, ",")
	label := "None"
	if count := len(slices.DeleteFunc(slices.Clone(chosen), func(s string) bool { return s == "" })); count > 0 {
		label = fmt.Sprintf("%d chosen", count)
	}
	changed := false
	ui.MenuButton(c, label, func(m *ui.Menu) {
		for _, ch := range choices {
			if m.Item(ch.label).Checked(slices.Contains(chosen, ch.value)).Chosen() {
				var next []string
				for _, other := range choices {
					if (other.value == ch.value) != slices.Contains(chosen, other.value) {
						next = append(next, other.value)
					}
				}
				*value = strings.Join(next, ",")
				changed = true
			}
		}
	}).Label(name)
	return changed
}

var durationUnits = []choice{{"d", "Days"}, {"w", "Weeks"}, {"m", "Months"}, {"y", "Years"}}

func durationControl(c *ui.Context, row *api.Condition, n int) bool {
	number, unit := "7", "d"
	if len(row.Value) >= 2 {
		number, unit = row.Value[:len(row.Value)-1], row.Value[len(row.Value)-1:]
	}
	changed := ui.TextInput(c, &number).Label(fmt.Sprintf("Condition %d value", n)).Width(56).Changed()
	if picker(c, fmt.Sprintf("Condition %d unit", n), durationUnits, &unit) {
		changed = true
	}
	if changed {
		row.Value = number + unit
	}
	return changed
}

func dateControl(c *ui.Context, row *api.Condition, name string) bool {
	day, err := time.ParseInLocation("2006-01-02", row.Value, time.Local)
	if err != nil {
		day = time.Now()
	}
	if !ui.DateInput(c, &day).Label(name).Changed() {
		return false
	}
	row.Value = day.Format("2006-01-02")
	return true
}

// ActionEditor is a controlled editor of a rule's actions; it reports
// whether the user changed them.
func ActionEditor(c *ui.Context, actions *[]api.RuleAction, ctx Context) bool {
	changed := false
	ui.Column(c).Gap(frostui.SpaceS).AlignItems(ui.Stretch).Children(func() {
		frostui.Body(c, "Perform the following actions:")
		for i := range *actions {
			if actionRow(c.Key(i), actions, i, ctx) {
				changed = true
				break
			}
		}
	})
	return changed
}

func actionRow(c *ui.Context, actions *[]api.RuleAction, i int, ctx Context) bool {
	row := &(*actions)[i]
	n := i + 1
	changed := false
	ui.Row(c).Gap(frostui.SpaceS).AlignItems(ui.Center).Children(func() {
		kinds := make([]choice, 0, len(Actions))
		for _, a := range Actions {
			kinds = append(kinds, choice{string(a.Kind), a.Label})
		}
		kind := string(row.Kind)
		if picker(c, fmt.Sprintf("Action %d", n), kinds, &kind) {
			*row = WithKind(*row, api.RuleActionKind(kind))
			changed = true
		}
		if TakesMailbox(row.Kind) {
			frostui.Body(c, "to mailbox:")
			if mailboxControl(c, row, n, ctx) {
				changed = true
			}
		}
		if row.Kind == api.RuleActionKindFlag {
			color := "1"
			if row.Color != nil {
				color = strconv.FormatInt(*row.Color, 10)
			}
			if picker(c, fmt.Sprintf("Action %d color", n), colorChoices(ctx), &color) {
				v, _ := strconv.ParseInt(color, 10, 64)
				row.Color = &v
				changed = true
			}
		}
		ui.Spacer(c)
		if frostui.IconButton(c.Key("remove"), "list-remove", fmt.Sprintf("Remove action %d", n)).Disabled(len(*actions) == 1).Clicked() {
			*actions = slices.Delete(slices.Clone(*actions), i, i+1)
			changed = true
		}
		if frostui.IconButton(c.Key("add"), "list-add", fmt.Sprintf("Add action after %d", n)).Clicked() {
			*actions = slices.Insert(slices.Clone(*actions), i+1, api.RuleAction{Kind: api.RuleActionKindMove})
			changed = true
		}
	})
	return changed
}

func mailboxControl(c *ui.Context, row *api.RuleAction, n int, ctx Context) bool {
	choices := mailboxChoices(ctx)
	value := ""
	if row.MailboxID == nil {
		choices = append([]choice{{"", "No Mailbox Selected"}}, choices...)
	} else {
		value = strconv.FormatInt(*row.MailboxID, 10)
		if !slices.ContainsFunc(choices, func(ch choice) bool { return ch.value == value }) {
			choices = append([]choice{{value, "Missing Mailbox"}}, choices...)
		}
	}
	if !picker(c, fmt.Sprintf("Action %d mailbox", n), choices, &value) || value == "" {
		return false
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return false
	}
	row.MailboxID = &id
	return true
}
