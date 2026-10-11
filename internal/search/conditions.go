package search

import (
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
)

// fieldOfColumn is the condition field each search column becomes.
var fieldOfColumn = map[string]api.ConditionField{
	"":                 api.ConditionFieldContent,
	"from_text":        api.ConditionFieldFrom,
	"to_text":          api.ConditionFieldRecipient,
	"subject":          api.ConditionFieldSubject,
	"attachment_names": api.ConditionFieldFilename,
}

// ToConditions turns a parsed search into conditions that list the same
// messages (docs/design/organize.md, From a search), for Save as Smart
// Mailbox. Dates are days in loc; newer_than: and older_than: stay
// relative.
func ToConditions(q Query, loc *time.Location) api.Conditions {
	out := api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{}}
	add := func(f api.ConditionField, op api.ConditionOp, v string) {
		out.Conditions = append(out.Conditions, api.Condition{Field: f, Op: op, Value: v})
	}
	for _, t := range q.Terms {
		op := api.ConditionOpContains
		if t.Not {
			op = api.ConditionOpNotcontains
		}
		v := t.Text
		if t.Phrase {
			v = `"` + v + `"`
		}
		add(fieldOfColumn[t.Column], op, v)
	}
	for _, b := range []struct {
		f api.ConditionField
		v *bool
	}{{api.ConditionFieldUnread, q.Unread}, {api.ConditionFieldFlagged, q.Flagged}, {api.ConditionFieldAttachments, q.HasAttachment}} {
		if b.v != nil {
			add(b.f, api.ConditionOpIs, boolText(*b.v))
		}
	}
	day := func(t time.Time) string { return t.In(loc).Format(time.DateOnly) }
	switch {
	case q.AfterRel == "" && q.BeforeRel == "" && !q.After.IsZero() && q.Before.Equal(q.After.AddDate(0, 0, 1)):
		add(api.ConditionFieldReceived, api.ConditionOpOn, day(q.After))
	default:
		if q.AfterRel != "" {
			add(api.ConditionFieldReceived, api.ConditionOpWithin, q.AfterRel)
		} else if !q.After.IsZero() {
			add(api.ConditionFieldReceived, api.ConditionOpSince, day(q.After))
		}
		if q.BeforeRel != "" {
			add(api.ConditionFieldReceived, api.ConditionOpNotwithin, q.BeforeRel)
		} else if !q.Before.IsZero() {
			add(api.ConditionFieldReceived, api.ConditionOpBefore, day(q.Before))
		}
	}
	if len(q.Roles) > 0 {
		roles := make([]string, len(q.Roles))
		for i, r := range q.Roles {
			roles[i] = string(r)
		}
		add(api.ConditionFieldRole, api.ConditionOpAnyof, strings.Join(roles, ","))
	}
	return out
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
