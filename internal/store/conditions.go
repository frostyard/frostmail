package store

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/search"
)

// The condition language of smart mailboxes, rules and filters (ADR-0023,
// docs/design/organize.md, Fields and operators), compiled to SQL over
// messages m.

// maxConditions bounds one Conditions; maxList bounds one anyof list.
const (
	maxConditions = 50
	maxList       = 100
)

// ErrCondition reports a condition maild cannot compile; the error names
// it, for invalidParams.
var ErrCondition = errors.New("invalid condition")

// Predicate is compiled conditions: an SQL expression over messages m and
// its arguments, in order.
type Predicate struct {
	SQL  string
	Args []any
}

// CheckConditions reports the first condition CompileConditions would
// refuse, wrapping ErrCondition. Stored conditions are checked first, so
// they always compile.
func CheckConditions(c api.Conditions) error {
	_, err := CompileConditions(c, time.Now(), time.UTC)
	return err
}

// CompileConditions compiles c into one predicate. Relative dates (today,
// within) are days in loc as of now. An empty list matches every message.
func CompileConditions(c api.Conditions, now time.Time, loc *time.Location) (Predicate, error) {
	join := " AND "
	switch c.Match {
	case api.ConditionMatchAll:
	case api.ConditionMatchAny:
		join = " OR "
	default:
		return Predicate{}, fmt.Errorf("%w: match %q is not all or any", ErrCondition, c.Match)
	}
	if len(c.Conditions) > maxConditions {
		return Predicate{}, fmt.Errorf("%w: more than %d conditions", ErrCondition, maxConditions)
	}
	if len(c.Conditions) == 0 {
		return Predicate{SQL: "1"}, nil
	}
	var p Predicate
	parts := make([]string, 0, len(c.Conditions))
	for i, cond := range c.Conditions {
		sql, args, err := compileCondition(cond, now, loc)
		if err != nil {
			return Predicate{}, fmt.Errorf("%w %d (%s %s %q): %v", ErrCondition, i+1, cond.Field, cond.Op, cond.Value, err)
		}
		parts = append(parts, "("+sql+")")
		p.Args = append(p.Args, args...)
	}
	p.SQL = "(" + strings.Join(parts, join) + ")"
	return p, nil
}

// ftsColumn is the search index column of each full-text field.
var ftsColumn = map[api.ConditionField]string{
	api.ConditionFieldFrom:      "from_text",
	api.ConditionFieldRecipient: "to_text",
	api.ConditionFieldSubject:   "subject",
	api.ConditionFieldContent:   "",
	api.ConditionFieldFilename:  "attachment_names",
}

func compileCondition(c api.Condition, now time.Time, loc *time.Location) (string, []any, error) {
	if !c.Field.Valid() {
		return "", nil, errors.New("unknown field")
	}
	if !c.Op.Valid() {
		return "", nil, errors.New("unknown op")
	}
	f, op, v := c.Field, c.Op, strings.TrimSpace(c.Value)
	switch f {
	case api.ConditionFieldFrom, api.ConditionFieldSubject, api.ConditionFieldRecipient,
		api.ConditionFieldContent, api.ConditionFieldFilename:
		if op == api.ConditionOpContains || op == api.ConditionOpNotcontains {
			return containsWords(ftsColumn[f], op, v)
		}
		switch f {
		case api.ConditionFieldFrom:
			return textMatch(op, v, "m.from_addr", "m.from_name")
		case api.ConditionFieldSubject:
			return textMatch(op, v, "m.subject")
		}
	case api.ConditionFieldTo, api.ConditionFieldCc:
		col := "m.to_json"
		if f == api.ConditionFieldCc {
			col = "m.cc_json"
		}
		pattern, err := likePattern(op, v, true)
		if err != nil {
			return "", nil, err
		}
		return `EXISTS (SELECT 1 FROM json_each(` + col + `) j WHERE json_extract(j.value, '$.addr') LIKE ? ESCAPE '\'` +
			` OR json_extract(j.value, '$.name') LIKE ? ESCAPE '\')`, []any{pattern, pattern}, nil
	case api.ConditionFieldTome, api.ConditionFieldCcme:
		col := "m.to_json"
		if f == api.ConditionFieldCcme {
			col = "m.cc_json"
		}
		return yesNo(op, v, `EXISTS (SELECT 1 FROM json_each(`+col+`) j JOIN identities i ON i.account_id = m.account_id`+
			` AND lower(i.email) = lower(json_extract(j.value, '$.addr')))`)
	case api.ConditionFieldListid:
		if op != api.ConditionOpContains && op != api.ConditionOpIs {
			break
		}
		pattern, err := likePattern(op, v, true)
		if err != nil {
			return "", nil, err
		}
		return `m.list_id LIKE ? ESCAPE '\'`, []any{pattern}, nil
	case api.ConditionFieldAccount:
		ids, err := idList(op, v)
		if err != nil {
			return "", nil, err
		}
		return inList("m.account_id", op, ids)
	case api.ConditionFieldMailbox:
		ids, err := idList(op, v)
		if err != nil {
			return "", nil, err
		}
		in, args, err := inList("mm.mailbox_id", api.ConditionOpAnyof, ids)
		if err != nil {
			return "", nil, err
		}
		return existsOrNot(op, `SELECT 1 FROM message_mailbox mm WHERE mm.message_id = m.id AND `+in), args, nil
	case api.ConditionFieldRole:
		roles, err := roleList(op, v)
		if err != nil {
			return "", nil, err
		}
		in, args, err := inList("mb.role", api.ConditionOpAnyof, roles)
		if err != nil {
			return "", nil, err
		}
		return existsOrNot(op, `SELECT 1 FROM message_mailbox mm JOIN mailboxes mb ON mb.id = mm.mailbox_id`+
			` WHERE mm.message_id = m.id AND `+in), args, nil
	case api.ConditionFieldReceived:
		return dateMatch("m.internal_date", op, v, now, loc)
	case api.ConditionFieldSent:
		return dateMatch("m.date_hdr", op, v, now, loc)
	case api.ConditionFieldUnread:
		return yesNo(op, v, "m.seen = 0")
	case api.ConditionFieldFlagged:
		return yesNo(op, v, "m.flagged = 1")
	case api.ConditionFieldAttachments:
		return yesNo(op, v, "m.has_attachments = 1")
	case api.ConditionFieldColor:
		colors, err := colorList(op, v)
		if err != nil {
			return "", nil, err
		}
		in, args, err := inList("m.flag_color", api.ConditionOpAnyof, colors)
		if err != nil {
			return "", nil, err
		}
		sql := "m.flagged = 1 AND " + in
		if op == api.ConditionOpIsnot {
			sql = "NOT (" + sql + ")"
		}
		return sql, args, nil
	case api.ConditionFieldVip:
		return yesNo(op, v, `EXISTS (SELECT 1 FROM vips v WHERE v.address = lower(m.from_addr))`)
	case api.ConditionFieldContact:
		return yesNo(op, v, `EXISTS (SELECT 1 FROM contact_emails ce WHERE ce.email = lower(m.from_addr))`)
	case api.ConditionFieldReminder:
		return yesNo(op, v, `EXISTS (SELECT 1 FROM message_reminders r WHERE r.message_id = m.id)`)
	}
	return "", nil, fmt.Errorf("%s does not take %s", f, op)
}

// containsWords matches every word of v (a quoted v is a phrase) in an FTS
// column, as the search language does; notcontains is its negation.
func containsWords(column string, op api.ConditionOp, v string) (string, []any, error) {
	terms := search.Words(v, column)
	if len(terms) == 0 {
		return "", nil, errors.New("no words to look for")
	}
	match := search.Query{Terms: terms}.Match()
	sql := "m.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)"
	if op == api.ConditionOpNotcontains {
		sql = "m.id NOT IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)"
	}
	return sql, []any{match}, nil
}

// textMatch compares columns with v, ignoring case (ASCII letters): is,
// begins or ends; any of the columns may match.
func textMatch(op api.ConditionOp, v string, columns ...string) (string, []any, error) {
	if op != api.ConditionOpIs && op != api.ConditionOpBegins && op != api.ConditionOpEnds {
		return "", nil, fmt.Errorf("text does not take %s", op)
	}
	pattern, err := likePattern(op, v, false)
	if err != nil {
		return "", nil, err
	}
	parts := make([]string, len(columns))
	args := make([]any, len(columns))
	for i, c := range columns {
		parts[i] = c + ` LIKE ? ESCAPE '\'`
		args[i] = pattern
	}
	return strings.Join(parts, " OR "), args, nil
}

// likePattern is v as a LIKE pattern for is, begins, ends, and, when
// allowed, contains.
func likePattern(op api.ConditionOp, v string, contains bool) (string, error) {
	if v == "" {
		return "", errors.New("empty value")
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(v)
	switch {
	case op == api.ConditionOpIs:
		return esc, nil
	case op == api.ConditionOpBegins:
		return esc + "%", nil
	case op == api.ConditionOpEnds:
		return "%" + esc, nil
	case op == api.ConditionOpContains && contains:
		return "%" + esc + "%", nil
	}
	return "", fmt.Errorf("text does not take %s", op)
}

// yesNo is a yes-or-no field: is true holds when cond does, is false
// when it does not.
func yesNo(op api.ConditionOp, v, cond string) (string, []any, error) {
	if op != api.ConditionOpIs {
		return "", nil, fmt.Errorf("a yes-or-no field takes is, not %s", op)
	}
	switch v {
	case "true":
		return cond, nil, nil
	case "false":
		return "NOT (" + cond + ")", nil, nil
	}
	return "", nil, errors.New(`the value must be "true" or "false"`)
}

// listValues splits an is, isnot or anyof value: one item, or for anyof a
// comma-separated list without repeats.
func listValues(op api.ConditionOp, v string) ([]string, error) {
	switch op {
	case api.ConditionOpIs, api.ConditionOpIsnot:
		if v == "" || strings.Contains(v, ",") {
			return nil, fmt.Errorf("%s takes one value", op)
		}
		return []string{v}, nil
	case api.ConditionOpAnyof:
		var out []string
		for item := range strings.SplitSeq(v, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				return nil, errors.New("an empty item in the list")
			}
			if !slices.Contains(out, item) {
				out = append(out, item)
			}
		}
		if len(out) == 0 || len(out) > maxList {
			return nil, fmt.Errorf("anyof takes 1 to %d items", maxList)
		}
		return out, nil
	}
	return nil, fmt.Errorf("a list field takes is, isnot or anyof, not %s", op)
}

func idList(op api.ConditionOp, v string) ([]any, error) {
	items, err := listValues(op, v)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(items))
	for i, s := range items {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil || id < 1 {
			return nil, fmt.Errorf("%q is not an ID", s)
		}
		out[i] = id
	}
	return out, nil
}

func roleList(op api.ConditionOp, v string) ([]any, error) {
	items, err := listValues(op, v)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(items))
	for i, s := range items {
		if r := api.MailboxRole(s); !r.Valid() || r == api.MailboxRoleNone {
			return nil, fmt.Errorf("%q is not a mailbox role", s)
		}
		out[i] = s
	}
	return out, nil
}

func colorList(op api.ConditionOp, v string) ([]any, error) {
	items, err := listValues(op, v)
	if err != nil {
		return nil, err
	}
	out := make([]any, len(items))
	for i, s := range items {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 7 {
			return nil, fmt.Errorf("%q is not a flag color 1-7", s)
		}
		out[i] = int64(n)
	}
	return out, nil
}

// inList is column = the one item for is, <> for isnot, IN for anyof.
func inList(column string, op api.ConditionOp, items []any) (string, []any, error) {
	switch op {
	case api.ConditionOpIs:
		return column + " = ?", items, nil
	case api.ConditionOpIsnot:
		return column + " <> ?", items, nil
	case api.ConditionOpAnyof:
		return column + " IN (" + inPlace(len(items)) + ")", items, nil
	}
	return "", nil, fmt.Errorf("a list field does not take %s", op)
}

// existsOrNot wraps a subquery in EXISTS, or NOT EXISTS for isnot.
func existsOrNot(op api.ConditionOp, subquery string) string {
	if op == api.ConditionOpIsnot {
		return "NOT EXISTS (" + subquery + ")"
	}
	return "EXISTS (" + subquery + ")"
}

// dateMatch bounds a UTC time column by days in loc. A message without the
// date (no Date header) matches no date condition.
func dateMatch(column string, op api.ConditionOp, v string, now time.Time, loc *time.Location) (string, []any, error) {
	n := now.In(loc)
	today := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, loc)
	between := func(lo, hi time.Time) (string, []any, error) {
		return column + " >= ? AND " + column + " < ?", []any{FormatTime(lo), FormatTime(hi)}, nil
	}
	noValue := func() error {
		if v != "" {
			return fmt.Errorf("%s takes no value", op)
		}
		return nil
	}
	switch op {
	case api.ConditionOpToday, api.ConditionOpYesterday, api.ConditionOpThisweek,
		api.ConditionOpThismonth, api.ConditionOpThisyear:
		if err := noValue(); err != nil {
			return "", nil, err
		}
		switch op {
		case api.ConditionOpToday:
			return between(today, today.AddDate(0, 0, 1))
		case api.ConditionOpYesterday:
			return between(today.AddDate(0, 0, -1), today)
		case api.ConditionOpThisweek:
			monday := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
			return between(monday, monday.AddDate(0, 0, 7))
		case api.ConditionOpThismonth:
			first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, loc)
			return between(first, first.AddDate(0, 1, 0))
		default:
			first := time.Date(today.Year(), 1, 1, 0, 0, 0, 0, loc)
			return between(first, first.AddDate(1, 0, 0))
		}
	case api.ConditionOpWithin, api.ConditionOpNotwithin:
		start, ok := search.ParseRelative(v, now, loc)
		if !ok {
			return "", nil, errors.New("the value must be N and d, w, m or y, such as 7d")
		}
		if op == api.ConditionOpWithin {
			return column + " >= ?", []any{FormatTime(start)}, nil
		}
		return column + " < ?", []any{FormatTime(start)}, nil
	case api.ConditionOpOn, api.ConditionOpSince, api.ConditionOpBefore:
		d, err := time.ParseInLocation(time.DateOnly, v, loc)
		if err != nil {
			return "", nil, errors.New("the value must be a day, YYYY-MM-DD")
		}
		switch op {
		case api.ConditionOpOn:
			return between(d, d.AddDate(0, 0, 1))
		case api.ConditionOpSince:
			return column + " >= ?", []any{FormatTime(d)}, nil
		default:
			return column + " < ?", []any{FormatTime(d)}, nil
		}
	}
	return "", nil, fmt.Errorf("a date does not take %s", op)
}
