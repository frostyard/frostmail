package pim

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// weekdayNames are the days in RRULE order, Monday first.
var weekdayNames = []string{"MO", "TU", "WE", "TH", "FR", "SA", "SU"}

// repeatPreset is one of the editor's repeat choices.
type repeatPreset string

const (
	presetNever    repeatPreset = "never"
	presetDaily    repeatPreset = "daily"
	presetWeekdays repeatPreset = "weekdays"
	presetWeekly   repeatPreset = "weekly"
	presetBiweekly repeatPreset = "biweekly"
	presetMonthly  repeatPreset = "monthly"
	presetYearly   repeatPreset = "yearly"
	presetCustom   repeatPreset = "custom"
)

// repeat is a rule as the repeat sheet shows it, without its end.
type repeat struct {
	frequency string
	interval  int
	weekdays  []string
	byWeekday bool
}

// repeatEnd is when a rule stops: "never", "date" or "count".
type repeatEnd struct {
	kind  string
	date  string
	count int
}

// ruleMap is an RRULE's parts, in their order.
type ruleMap struct {
	keys []string
	vals map[string]string
}

func (r *ruleMap) get(k string) (string, bool) { v, ok := r.vals[k]; return v, ok }
func (r *ruleMap) set(k, v string) {
	if _, ok := r.vals[k]; !ok {
		r.keys = append(r.keys, k)
	}
	r.vals[k] = v
}

func (r *ruleMap) del(k string) {
	delete(r.vals, k)
	r.keys = slices.DeleteFunc(r.keys, func(x string) bool { return x == k })
}

func (r *ruleMap) String() string {
	parts := make([]string, len(r.keys))
	for i, k := range r.keys {
		parts[i] = k + "=" + r.vals[k]
	}
	return strings.Join(parts, ";")
}

func parseRule(rule string) *ruleMap {
	r := &ruleMap{vals: map[string]string{}}
	for part := range strings.SplitSeq(strings.ToUpper(rule), ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok || k == "" || v == "" || strings.Contains(v, "=") {
			return nil
		}
		if _, seen := r.vals[k]; seen {
			return nil
		}
		r.set(k, v)
	}
	if _, ok := r.get("FREQ"); !ok {
		return nil
	}
	return r
}

func isWeekday(v string) bool { return slices.Contains(weekdayNames, v) }

func startDay(start string) string { return weekdayNames[(weekday(start)+6)%7] }

func isLastWeek(start string) bool { return addDays(start, 7)[:7] != start[:7] }

// nthWeekday is the start's weekday in its month, as BYDAY writes it.
func nthWeekday(start string) string {
	day, _ := strconv.Atoi(start[8:10])
	ordinal := (day + 6) / 7
	if ordinal == 5 {
		ordinal = -1
	}
	return strconv.Itoa(ordinal) + startDay(start)
}

func matchesWeekday(value, start string) bool {
	return value == nthWeekday(start) || (value == "-1"+startDay(start) && isLastWeek(start))
}

var numberForm = regexp.MustCompile(`^\+?\d+$`)

func matchesNumber(value string, has bool, expected int) bool {
	if !has || !numberForm.MatchString(value) {
		return false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(value, "+"))
	return err == nil && n == expected
}

// repeatOf reads a rule as the repeat sheet shows it; nil when it cannot.
func repeatOf(rule, start string) *repeat {
	parts := parseRule(rule)
	if parts == nil {
		return nil
	}
	freq, _ := parts.get("FREQ")
	if !slices.Contains([]string{"DAILY", "WEEKLY", "MONTHLY", "YEARLY"}, freq) {
		return nil
	}
	interval, ok := parts.get("INTERVAL")
	if !ok {
		interval = "1"
	}
	if wk, has := parts.get("WKST"); !positive(interval) || (has && !isWeekday(wk)) {
		return nil
	}
	allowed := []string{"FREQ", "INTERVAL", "WKST", "COUNT", "UNTIL"}
	if freq == "WEEKLY" || freq == "MONTHLY" {
		allowed = append(allowed, "BYDAY")
	}
	if freq == "MONTHLY" || freq == "YEARLY" {
		allowed = append(allowed, "BYMONTHDAY")
	}
	if freq == "YEARLY" {
		allowed = append(allowed, "BYMONTH")
	}
	for _, k := range parts.keys {
		if !slices.Contains(allowed, k) {
			return nil
		}
	}
	n, _ := strconv.Atoi(interval)
	r := &repeat{frequency: freq, interval: n}
	byDay, hasDay := parts.get("BYDAY")
	monthDay, hasMonthDay := parts.get("BYMONTHDAY")
	month, hasMonth := parts.get("BYMONTH")
	if freq == "WEEKLY" {
		days := []string{startDay(start)}
		if hasDay {
			days = strings.Split(byDay, ",")
		}
		for _, d := range days {
			if !isWeekday(d) {
				return nil
			}
		}
		for _, d := range weekdayNames {
			if slices.Contains(days, d) {
				r.weekdays = append(r.weekdays, d)
			}
		}
	} else if hasDay {
		if hasMonthDay || !matchesWeekday(byDay, start) {
			return nil
		}
		r.byWeekday = true
	}
	day, _ := strconv.Atoi(start[8:10])
	if hasMonthDay && !matchesNumber(monthDay, true, day) {
		return nil
	}
	mon, _ := strconv.Atoi(start[5:7])
	if hasMonth && !matchesNumber(month, true, mon) {
		return nil
	}
	return r
}

// defaultRepeat is what the sheet starts from for an event with no rule.
func defaultRepeat(start string) repeat {
	return repeat{frequency: "WEEKLY", interval: 1, weekdays: []string{startDay(start)}}
}

// ruleOfRepeat writes the sheet's rule, without an end; the start's own
// day is left to DTSTART.
func ruleOfRepeat(r repeat, start string) string {
	rule := "FREQ=" + r.frequency
	if r.interval > 1 {
		rule += ";INTERVAL=" + strconv.Itoa(r.interval)
	}
	if r.frequency == "WEEKLY" {
		var days []string
		for _, d := range weekdayNames {
			if slices.Contains(r.weekdays, d) {
				days = append(days, d)
			}
		}
		if len(days) > 0 && (len(days) != 1 || days[0] != startDay(start)) {
			rule += ";BYDAY=" + strings.Join(days, ",")
		}
	} else if r.frequency == "MONTHLY" && r.byWeekday {
		rule += ";BYDAY=" + nthWeekday(start)
	}
	return rule
}

// presetOf is the choice a rule shows as; its end does not count.
func presetOf(rule, start string) repeatPreset {
	if rule == "" {
		return presetNever
	}
	r := repeatOf(rule, start)
	if r == nil {
		return presetCustom
	}
	switch {
	case r.frequency == "WEEKLY":
		if r.interval == 1 && strings.Join(r.weekdays, ",") == "MO,TU,WE,TH,FR" {
			return presetWeekdays
		}
		if len(r.weekdays) == 1 && r.weekdays[0] == startDay(start) {
			if r.interval == 1 {
				return presetWeekly
			}
			if r.interval == 2 {
				return presetBiweekly
			}
		}
	case r.interval == 1 && r.frequency == "DAILY":
		return presetDaily
	case r.interval == 1 && r.frequency == "MONTHLY" && !r.byWeekday:
		return presetMonthly
	case r.interval == 1 && r.frequency == "YEARLY":
		return presetYearly
	}
	return presetCustom
}

// presetRule is the rule a preset writes, without an end.
func presetRule(p repeatPreset) string {
	switch p {
	case presetDaily:
		return "FREQ=DAILY"
	case presetWeekdays:
		return "FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR"
	case presetWeekly:
		return "FREQ=WEEKLY"
	case presetBiweekly:
		return "FREQ=WEEKLY;INTERVAL=2"
	case presetMonthly:
		return "FREQ=MONTHLY"
	case presetYearly:
		return "FREQ=YEARLY"
	}
	return ""
}

var untilDateForm = regexp.MustCompile(`^\d{8}(T\d{6}Z?)?$`)

func untilDate(value string, loc *time.Location) (string, bool) {
	if !untilDateForm.MatchString(value) {
		return "", false
	}
	day, err := time.Parse("20060102", value[:8])
	if err != nil {
		return "", false
	}
	date := day.Format(time.DateOnly)
	if len(value) == 8 || !strings.HasSuffix(value, "Z") {
		return date, true
	}
	t, err := time.Parse("20060102T150405Z", value)
	if err != nil {
		return "", false
	}
	d, _ := zoned(t, loc)
	return d, true
}

// endOf reads when a rule stops.
func endOf(rule string, loc *time.Location) repeatEnd {
	parts := parseRule(rule)
	if parts == nil {
		return repeatEnd{kind: "never"}
	}
	if count, ok := parts.get("COUNT"); ok && positive(count) {
		n, _ := strconv.Atoi(count)
		return repeatEnd{kind: "count", count: n}
	}
	if until, ok := parts.get("UNTIL"); ok {
		if date, ok := untilDate(until, loc); ok {
			return repeatEnd{kind: "date", date: date}
		}
	}
	return repeatEnd{kind: "never"}
}

// withEnd is rule ending as end says.
func withEnd(rule string, end repeatEnd, allDay bool, loc *time.Location) string {
	parts := parseRule(rule)
	if parts == nil {
		return rule
	}
	parts.del("COUNT")
	parts.del("UNTIL")
	switch end.kind {
	case "count":
		parts.set("COUNT", strconv.Itoa(max(1, end.count)))
	case "date":
		if allDay {
			parts.set("UNTIL", strings.ReplaceAll(end.date, "-", ""))
		} else {
			until := dayStart(addDays(end.date, 1), loc).Add(-time.Second).UTC()
			parts.set("UNTIL", until.Format("20060102T150405Z"))
		}
	}
	return parts.String()
}

// followStart moves the parts of a rule naming the start's day when the
// start moves.
func followStart(rule, from, to string) string {
	if from == to || rule == "" {
		return rule
	}
	parts := parseRule(rule)
	if parts == nil {
		return rule
	}
	freq, _ := parts.get("FREQ")
	byDay, hasDay := parts.get("BYDAY")
	if freq == "WEEKLY" && byDay == startDay(from) {
		parts.set("BYDAY", startDay(to))
	} else if freq == "MONTHLY" && hasDay && matchesWeekday(byDay, from) {
		if strings.HasPrefix(byDay, "-1") && isLastWeek(to) {
			parts.set("BYDAY", "-1"+startDay(to))
		} else {
			parts.set("BYDAY", nthWeekday(to))
		}
	}
	day, _ := strconv.Atoi(from[8:10])
	toDay, _ := strconv.Atoi(to[8:10])
	if v, ok := parts.get("BYMONTHDAY"); matchesNumber(v, ok, day) {
		parts.set("BYMONTHDAY", strconv.Itoa(toDay))
	}
	mon, _ := strconv.Atoi(from[5:7])
	toMon, _ := strconv.Atoi(to[5:7])
	if v, ok := parts.get("BYMONTH"); freq == "YEARLY" && matchesNumber(v, ok, mon) {
		parts.set("BYMONTH", strconv.Itoa(toMon))
	}
	return parts.String()
}

// ordinalWord says a BYDAY's ordinal for the sheet.
func ordinalWord(n string) string { return ordinals[strings.TrimRight(n, "MOTUWEHFRSA")] }

// monthlyByDayLabel is the sheet's "on the first Thursday".
func monthlyByDayLabel(start string) string {
	nth := nthWeekday(start)
	return fmt.Sprintf("on the %s %s", ordinalWord(nth), dayNames[startDay(start)])
}
