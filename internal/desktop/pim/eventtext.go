package pim

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
)

var (
	dayNames = map[string]string{"MO": "Monday", "TU": "Tuesday", "WE": "Wednesday", "TH": "Thursday", "FR": "Friday", "SA": "Saturday", "SU": "Sunday"}
	ordinals = map[string]string{"1": "first", "2": "second", "3": "third", "4": "fourth", "-1": "last"}
	units    = map[string]string{"DAILY": "day", "WEEKLY": "week", "MONTHLY": "month", "YEARLY": "year"}

	monthDayForm = regexp.MustCompile(`^-?\d+$`)
	nthDayForm   = regexp.MustCompile(`^(-1|[1-4])(MO|TU|WE|TH|FR|SA|SU)$`)
	untilForm    = regexp.MustCompile(`^\d{8}`)
)

func positive(value string) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && n > 0 && !strings.HasPrefix(value, "+")
}

// describeParts reads the RRULE parts describeRecurrence understands; nil
// for anything else.
func describeParts(line string) map[string]string {
	allowed := []string{"FREQ", "INTERVAL", "COUNT", "UNTIL", "WKST", "BYDAY", "BYMONTHDAY"}
	parts := map[string]string{}
	for part := range strings.SplitSeq(strings.TrimPrefix(line, "RRULE:"), ";") {
		key, value, ok := strings.Cut(part, "=")
		if !ok || value == "" || strings.Contains(value, "=") || !slices.Contains(allowed, key) {
			return nil
		}
		if _, seen := parts[key]; seen {
			return nil
		}
		parts[key] = value
	}
	if _, c := parts["COUNT"]; c {
		if _, u := parts["UNTIL"]; u {
			return nil
		}
	}
	return parts
}

// dayDescription is the words of a rule's days; ok false when it cannot
// say them.
func dayDescription(parts map[string]string) (string, bool) {
	freq := parts["FREQ"]
	byDay, hasDay := parts["BYDAY"]
	monthDay, hasMonthDay := parts["BYMONTHDAY"]
	if hasDay && hasMonthDay {
		return "", false
	}
	if hasMonthDay {
		n, err := strconv.Atoi(monthDay)
		if freq != "MONTHLY" || !monthDayForm.MatchString(monthDay) || err != nil || n == 0 || n > 31 || n < -31 {
			return "", false
		}
		return fmt.Sprintf(" on day %d", n), true
	}
	if !hasDay {
		return "", true
	}
	if freq == "WEEKLY" {
		var names []string
		for d := range strings.SplitSeq(byDay, ",") {
			name, ok := dayNames[d]
			if !ok {
				return "", false
			}
			names = append(names, name)
		}
		if len(names) == 1 {
			return " on " + names[0], true
		}
		return " on " + strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1], true
	}
	if freq != "MONTHLY" {
		return "", false
	}
	m := nthDayForm.FindStringSubmatch(byDay)
	if m == nil {
		return "", false
	}
	return " on the " + ordinals[m[1]] + " " + dayNames[m[2]], true
}

func untilText(value string) (string, bool) {
	if !untilForm.MatchString(value) {
		return "", false
	}
	t, err := time.Parse("20060102", value[:8])
	if err != nil {
		return "", false
	}
	return t.Format("January 2, 2006"), true
}

// describeRecurrence says a recurrence's RRULE in words; "Custom" for
// what it cannot.
func describeRecurrence(recurrence string) string {
	if recurrence == "" {
		return ""
	}
	line := ""
	for l := range strings.SplitSeq(recurrence, "\n") {
		if strings.HasPrefix(l, "RRULE:") {
			line = strings.TrimRight(l, " \r\t")
			break
		}
	}
	if line == "" {
		return "Custom"
	}
	parts := describeParts(line)
	if parts == nil {
		return "Custom"
	}
	unit, ok := units[parts["FREQ"]]
	interval := parts["INTERVAL"]
	if interval == "" {
		interval = "1"
	}
	days, known := dayDescription(parts)
	if !ok || !positive(interval) || !known {
		return "Custom"
	}
	count, hasCount := parts["COUNT"]
	if hasCount && !positive(count) {
		return "Custom"
	}
	n, _ := strconv.Atoi(interval)
	text := "Every " + unit
	if n != 1 {
		text = fmt.Sprintf("Every %d %ss", n, unit)
	}
	weekdays := strings.Split(parts["BYDAY"], ",")
	if parts["FREQ"] == "WEEKLY" && n == 1 && len(weekdays) == 5 && !slices.ContainsFunc([]string{"MO", "TU", "WE", "TH", "FR"}, func(d string) bool { return !slices.Contains(weekdays, d) }) {
		text = "Every weekday"
	} else {
		text += days
	}
	if hasCount {
		c, _ := strconv.Atoi(count)
		if c == 1 {
			text += ", once"
		} else {
			text += fmt.Sprintf(", %d times", c)
		}
	}
	if until, has := parts["UNTIL"]; has {
		end, ok := untilText(until)
		if !ok {
			return "Custom"
		}
		text += ", until " + end
	}
	return text
}

// alarmText says how long before or after the start an alarm is.
func alarmText(minutes int64) string {
	if minutes == 0 {
		return "At time of event"
	}
	abs := minutes
	if abs < 0 {
		abs = -abs
	}
	divisor, unit := int64(1), "minute"
	switch {
	case abs%1440 == 0:
		divisor, unit = 1440, "day"
	case abs%60 == 0:
		divisor, unit = 60, "hour"
	}
	amount := abs / divisor
	plural := "s"
	if amount == 1 {
		plural = ""
	}
	side := "before"
	if minutes < 0 {
		side = "after"
	}
	return fmt.Sprintf("%d %s%s %s", amount, unit, plural, side)
}

// clockTime writes a time of day.
func (k clock) clockTime(t time.Time) string {
	if k.h24 {
		return t.In(k.loc).Format("15:04")
	}
	return t.In(k.loc).Format("3:04 PM")
}

// timeRange writes a start and end: "9:00 – 9:15 AM", "9:00 AM – 1:00 PM",
// and across days with their dates.
func (k clock) timeRange(start, end time.Time) string {
	s, e := start.In(k.loc), end.In(k.loc)
	if s.Format(time.DateOnly) != e.Format(time.DateOnly) {
		return s.Format("Jan 2, ") + k.clockTime(s) + " – " + e.Format("Jan 2, ") + k.clockTime(e)
	}
	if !k.h24 && s.Format("PM") == e.Format("PM") {
		return s.Format("3:04") + " – " + e.Format("3:04 PM")
	}
	return k.clockTime(s) + " – " + k.clockTime(e)
}

// dateText writes a timed event's start day or an all-day event's dates.
func (k clock) dateText(allDay bool, start time.Time, startDate, endDate string) string {
	if allDay && endDate > addDays(startDate, 1) {
		first, last := dateOf(startDate), dateOf(addDays(endDate, -1))
		switch {
		case first.Year() != last.Year():
			return first.Format("January 2, 2006") + " – " + last.Format("January 2, 2006")
		case first.Month() != last.Month():
			return first.Format("January 2") + " – " + last.Format("January 2, 2006")
		}
		return first.Format("January 2") + " – " + last.Format("2, 2006")
	}
	if allDay {
		return longDate(startDate)
	}
	return start.In(k.loc).Format("Monday, January 2, 2006")
}

// answerText names an invitation's answer.
func answerText(a api.PartStat) string {
	switch a {
	case api.PartStatAccepted:
		return "Accepted"
	case api.PartStatDeclined:
		return "Declined"
	case api.PartStatTentative:
		return "Maybe"
	case api.PartStatDelegated:
		return "Delegated"
	}
	return "Not answered"
}

// upcomingWhen says when an occurrence is, for an Upcoming row.
func (k clock) upcomingWhen(o *api.Occurrence, now time.Time) string {
	current := todayIn(now, k.loc)
	date := o.StartDate
	if o.AllDay {
		if o.StartDate <= current && current < o.EndDate {
			date = current
		}
	} else {
		date, _ = zoned(o.Start, k.loc)
	}
	if date == current {
		if o.AllDay {
			return "Today"
		}
		return k.clockTime(o.Start)
	}
	if date > current && date <= addDays(current, 6) {
		return dateOf(date).Format("Mon")
	}
	return dateOf(date).Format("Jan 2")
}

// nextOccurrences keeps those not yet ended and not cancelled, at most
// limit: the To-Do bar's Upcoming.
func nextOccurrences(occs []api.Occurrence, now time.Time, loc *time.Location, limit int) []api.Occurrence {
	var out []api.Occurrence
	for _, o := range occs {
		if len(out) == limit {
			break
		}
		end := o.End
		if o.AllDay {
			end = dayStart(o.EndDate, loc)
		}
		if o.Status != api.EventStatusCancelled && end.After(now) {
			out = append(out, o)
		}
	}
	return out
}

// reminderWhen says when a reminder's occurrence starts, relative to now.
func (k clock) reminderWhen(r api.Reminder, now time.Time) string {
	minutes := r.Start.Sub(now).Minutes()
	if !r.AllDay {
		switch {
		case minutes > 0 && minutes < 60:
			n := int(math.Ceil(minutes))
			if n == 1 {
				return "In 1 minute"
			}
			return fmt.Sprintf("In %d minutes", n)
		case minutes <= 0 && minutes > -5:
			return "Now"
		case minutes <= -5 && minutes > -60:
			return fmt.Sprintf("%d minutes ago", int(math.Floor(-minutes)))
		}
	}
	current := todayIn(now, k.loc)
	date := r.StartDate
	if !r.AllDay {
		date, _ = zoned(r.Start, k.loc)
	}
	var day string
	switch date {
	case current:
		day = "Today"
	case addDays(current, 1):
		day = "Tomorrow"
	case addDays(current, -1):
		day = "Yesterday"
	default:
		day = dateOf(date).Format("Mon, Jan 2")
	}
	if r.AllDay {
		return day
	}
	return day + ", " + k.clockTime(r.Start)
}

// snoozeChoice is one entry of the Snooze menu.
type snoozeChoice struct {
	label string
	until time.Time
}

// snoozeChoices are 5, 10 and 15 minutes, an hour, and 9:00 tomorrow.
func snoozeChoices(now time.Time, loc *time.Location) []snoozeChoice {
	var out []snoozeChoice
	for _, m := range []int{5, 10, 15} {
		out = append(out, snoozeChoice{fmt.Sprintf("%d minutes", m), now.Add(time.Duration(m) * time.Minute)})
	}
	out = append(out, snoozeChoice{"1 hour", now.Add(time.Hour)})
	return append(out, snoozeChoice{"Tomorrow", instantAt(addDays(todayIn(now, loc), 1), 9*60, loc)})
}
