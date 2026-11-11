// Package organize is the desktop app's editors of maild's conditions and
// rules, and the sheets built on them (docs/specs/organize-ui.md; plan
// 0015, Phase 6): smart mailboxes, rules, mailboxes and Redirect.
package organize

import (
	"slices"
	"strconv"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// FieldInfo names a condition's field.
type FieldInfo struct {
	Field api.ConditionField
	Label string
}

// Fields are the condition fields in menu order.
var Fields = []FieldInfo{
	{api.ConditionFieldFrom, "From"}, {api.ConditionFieldTo, "To"}, {api.ConditionFieldCc, "Cc"},
	{api.ConditionFieldRecipient, "Any Recipient"}, {api.ConditionFieldTome, "Sent to Me"}, {api.ConditionFieldCcme, "Cc'd to Me"},
	{api.ConditionFieldSubject, "Subject"}, {api.ConditionFieldContent, "Entire Message"}, {api.ConditionFieldFilename, "Attachment Name"},
	{api.ConditionFieldListid, "Mailing List"}, {api.ConditionFieldAccount, "Account"}, {api.ConditionFieldMailbox, "Mailbox"},
	{api.ConditionFieldRole, "Mailbox Type"}, {api.ConditionFieldReceived, "Date Received"}, {api.ConditionFieldSent, "Date Sent"},
	{api.ConditionFieldUnread, "Unread"}, {api.ConditionFieldFlagged, "Flagged"}, {api.ConditionFieldAttachments, "Has Attachments"},
	{api.ConditionFieldColor, "Flag Color"}, {api.ConditionFieldVip, "Sender Is a VIP"}, {api.ConditionFieldContact, "Sender Is in Contacts"},
	{api.ConditionFieldReminder, "Has a Reminder"},
}

// OpLabel names each operator.
var OpLabel = map[api.ConditionOp]string{
	api.ConditionOpContains: "contains", api.ConditionOpNotcontains: "does not contain", api.ConditionOpIs: "is", api.ConditionOpIsnot: "is not",
	api.ConditionOpBegins: "begins with", api.ConditionOpEnds: "ends with", api.ConditionOpAnyof: "is any of",
	api.ConditionOpToday: "is today", api.ConditionOpYesterday: "is yesterday", api.ConditionOpThisweek: "is this week",
	api.ConditionOpThismonth: "is this month", api.ConditionOpThisyear: "is this year", api.ConditionOpWithin: "is in the last",
	api.ConditionOpNotwithin: "is not in the last", api.ConditionOpOn: "is", api.ConditionOpSince: "is on or after", api.ConditionOpBefore: "is before",
}

// OpsFor lists a field's operators.
func OpsFor(f api.ConditionField) []api.ConditionOp {
	switch f {
	case api.ConditionFieldFrom, api.ConditionFieldSubject:
		return []api.ConditionOp{api.ConditionOpContains, api.ConditionOpNotcontains, api.ConditionOpIs, api.ConditionOpBegins, api.ConditionOpEnds}
	case api.ConditionFieldTo, api.ConditionFieldCc:
		return []api.ConditionOp{api.ConditionOpContains, api.ConditionOpIs, api.ConditionOpBegins, api.ConditionOpEnds}
	case api.ConditionFieldRecipient, api.ConditionFieldContent, api.ConditionFieldFilename:
		return []api.ConditionOp{api.ConditionOpContains, api.ConditionOpNotcontains}
	case api.ConditionFieldListid:
		return []api.ConditionOp{api.ConditionOpContains, api.ConditionOpIs}
	case api.ConditionFieldAccount, api.ConditionFieldMailbox, api.ConditionFieldRole, api.ConditionFieldColor:
		return []api.ConditionOp{api.ConditionOpIs, api.ConditionOpIsnot, api.ConditionOpAnyof}
	case api.ConditionFieldReceived, api.ConditionFieldSent:
		return []api.ConditionOp{api.ConditionOpToday, api.ConditionOpYesterday, api.ConditionOpThisweek, api.ConditionOpThismonth, api.ConditionOpThisyear,
			api.ConditionOpWithin, api.ConditionOpNotwithin, api.ConditionOpOn, api.ConditionOpSince, api.ConditionOpBefore}
	}
	return []api.ConditionOp{api.ConditionOpIs}
}

// ValueKind is what control a condition's value takes.
type ValueKind int

// The kinds of value.
const (
	KindText ValueKind = iota
	KindYesNo
	KindChoice
	KindChoices
	KindDuration
	KindDate
	KindNone
)

var yesNoFields = []api.ConditionField{api.ConditionFieldTome, api.ConditionFieldCcme, api.ConditionFieldUnread, api.ConditionFieldFlagged,
	api.ConditionFieldAttachments, api.ConditionFieldVip, api.ConditionFieldContact, api.ConditionFieldReminder}

var choiceFields = []api.ConditionField{api.ConditionFieldAccount, api.ConditionFieldMailbox, api.ConditionFieldRole, api.ConditionFieldColor}

// KindOf says what a field and operator take.
func KindOf(f api.ConditionField, op api.ConditionOp) ValueKind {
	if slices.Contains(yesNoFields, f) {
		return KindYesNo
	}
	if slices.Contains(choiceFields, f) {
		switch op {
		case api.ConditionOpAnyof:
			return KindChoices
		case api.ConditionOpIs, api.ConditionOpIsnot:
			return KindChoice
		}
	}
	switch op {
	case api.ConditionOpWithin, api.ConditionOpNotwithin:
		return KindDuration
	case api.ConditionOpOn, api.ConditionOpSince, api.ConditionOpBefore:
		return KindDate
	case api.ConditionOpToday, api.ConditionOpYesterday, api.ConditionOpThisweek, api.ConditionOpThismonth, api.ConditionOpThisyear:
		return KindNone
	}
	return KindText
}

// Context is what starting values need.
type Context struct {
	Today     string // YYYY-MM-DD
	Accounts  []api.Account
	Mailboxes []api.Mailbox
	FlagNames []string
}

// StartingValue is a field and operator's first value.
func StartingValue(f api.ConditionField, op api.ConditionOp, ctx Context) string {
	switch KindOf(f, op) {
	case KindYesNo:
		return "true"
	case KindDuration:
		return "7d"
	case KindDate:
		return ctx.Today
	case KindChoice, KindChoices:
		switch f {
		case api.ConditionFieldAccount:
			if len(ctx.Accounts) > 0 {
				return strconv.FormatInt(ctx.Accounts[0].ID, 10)
			}
		case api.ConditionFieldMailbox:
			if len(ctx.Mailboxes) > 0 {
				return strconv.FormatInt(ctx.Mailboxes[0].ID, 10)
			}
		case api.ConditionFieldRole:
			return "inbox"
		case api.ConditionFieldColor:
			return "1"
		}
	}
	return ""
}

// Roles are the Mailbox Type choices: value, label.
var Roles = [][2]string{{"inbox", "Inbox"}, {"drafts", "Drafts"}, {"sent", "Sent"}, {"junk", "Junk"}, {"trash", "Trash"}, {"archive", "Archive"}, {"all", "All Mail"}}

// NewCondition is a new row's condition.
func NewCondition() api.Condition {
	return api.Condition{Field: api.ConditionFieldFrom, Op: api.ConditionOpContains}
}

// WithField gives a row a field's first operator and starting value.
func WithField(f api.ConditionField, ctx Context) api.Condition {
	op := OpsFor(f)[0]
	return api.Condition{Field: f, Op: op, Value: StartingValue(f, op, ctx)}
}

// WithOp changes a row's operator, keeping the value when it is the same
// kind; a list going to a single choice keeps its first item.
func WithOp(c api.Condition, op api.ConditionOp, ctx Context) api.Condition {
	before, after := KindOf(c.Field, c.Op), KindOf(c.Field, op)
	choice := func(k ValueKind) bool { return k == KindChoice || k == KindChoices }
	value := StartingValue(c.Field, op, ctx)
	if before == after || (choice(before) && choice(after)) {
		value = c.Value
		if after == KindChoice {
			value, _, _ = strings.Cut(c.Value, ",")
		}
	}
	return api.Condition{Field: c.Field, Op: op, Value: value}
}

// ActionInfo names a rule's action.
type ActionInfo struct {
	Kind  api.RuleActionKind
	Label string
}

// Actions are the rule actions in menu order.
var Actions = []ActionInfo{
	{api.RuleActionKindMove, "Move Message"}, {api.RuleActionKindCopy, "Copy Message"}, {api.RuleActionKindRead, "Mark as Read"},
	{api.RuleActionKindFlag, "Mark as Flagged"}, {api.RuleActionKindDelete, "Delete Message"}, {api.RuleActionKindNotify, "Send Notification"},
	{api.RuleActionKindStop, "Stop Evaluating Rules"},
}

// TakesMailbox reports whether an action needs a destination.
func TakesMailbox(k api.RuleActionKind) bool {
	return k == api.RuleActionKindMove || k == api.RuleActionKindCopy
}

// WithKind keeps only what the new kind takes.
func WithKind(a api.RuleAction, k api.RuleActionKind) api.RuleAction {
	if TakesMailbox(k) && TakesMailbox(a.Kind) && a.MailboxID != nil {
		return api.RuleAction{Kind: k, MailboxID: a.MailboxID}
	}
	if k == api.RuleActionKindFlag {
		color := int64(1)
		if a.Kind == api.RuleActionKindFlag && a.Color != nil {
			color = *a.Color
		}
		return api.RuleAction{Kind: k, Color: &color}
	}
	return api.RuleAction{Kind: k}
}

// ActionsComplete reports at least one action, every Move and Copy naming
// a mailbox among those given.
func ActionsComplete(actions []api.RuleAction, mailboxes []api.Mailbox) bool {
	if len(actions) == 0 {
		return false
	}
	for _, a := range actions {
		if !TakesMailbox(a.Kind) {
			continue
		}
		if a.MailboxID == nil || !slices.ContainsFunc(mailboxes, func(m api.Mailbox) bool { return m.ID == *a.MailboxID }) {
			return false
		}
	}
	return true
}
