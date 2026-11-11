package pim

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
)

// draftInvitee is an invitee's editable address and role, with their
// current answer.
type draftInvitee struct {
	email, name string
	optional    bool
	answer      api.PartStat
}

// eventDraft is an event as the editor shows it. Dates are YYYY-MM-DD and
// times HH:MM in timeZone; an all-day event's endDate is its last day.
type eventDraft struct {
	summary, location, description string
	allDay                         bool
	startDate, startTime           string
	endDate, endTime               string
	timeZone, rrule                string
	calendarID                     int64
	alarms                         []int64
	transparent                    bool
	invitees                       []draftInvitee

	// startRead and endRead are the instants the times were read from.
	startRead, endRead readTime
	// fixedRoles says an invitee has a role calendar.update cannot write
	// (chair or none), so the invitees are left as they are.
	fixedRoles bool
}

// readTime is a wall time with the instant it was read from: in a repeated
// hour the wall time alone does not say which pass it is.
type readTime struct {
	date, wall, zone string
	at               time.Time
}

// instant is a wall time's instant: the one it was read from while it
// stands, else what the zone says.
func (r readTime) instant(date, wall, zone string, loc *time.Location) time.Time {
	if r.date == date && r.wall == wall && r.zone == zone {
		return r.at
	}
	return instantAt(date, minutesOf(wall), loc)
}

// skippedTime is the problem of a wall time the clocks jump over.
const skippedTime = "The clocks change then, so this time doesn't exist"

// draftErrors names each invalid field's problem.
type draftErrors struct{ start, end, timeZone, invitees string }

func (e draftErrors) any() bool { return e != draftErrors{} }

var (
	dateForm    = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	timeForm    = regexp.MustCompile(`^(?:[01]\d|2[0-3]):[0-5]\d$`)
	addressForm = regexp.MustCompile(`^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$`)
)

// validAddress reports whether text is a plausible email address.
func validAddress(text string) bool { return addressForm.MatchString(text) }

// ruleOfRecurrence is an event's RRULE value from its recurrence lines.
func ruleOfRecurrence(recurrence string) string {
	for line := range strings.SplitSeq(recurrence, "\n") {
		if strings.HasPrefix(line, "RRULE:") {
			return strings.TrimSpace(line[6:])
		}
	}
	return ""
}

// newTimes are a new event's first times.
type newTimes struct {
	allDay             bool
	start, end         time.Time
	startDate, endDate string
}

// newDraft is a new event's draft.
func newDraft(t newTimes, calendarID int64, zone string, loc *time.Location) eventDraft {
	d := eventDraft{allDay: t.allDay, timeZone: zone, calendarID: calendarID}
	if t.allDay {
		d.startDate, d.startTime = t.startDate, wallTime(540)
		d.endDate, d.endTime = addDays(t.endDate, -1), wallTime(600)
		return d
	}
	date, m := zoned(t.start, loc)
	d.startDate, d.startTime = date, wallTime(m)
	d.startRead = readTime{date, d.startTime, zone, t.start}
	date, m = zoned(t.end, loc)
	d.endDate, d.endTime = date, wallTime(m)
	d.endRead = readTime{date, d.endTime, zone, t.end}
	return d
}

// draftOf is the editable copy of an event, its times read in its own
// zone, else the app's.
func draftOf(e *api.CalendarEvent, appZone string, appLoc *time.Location) eventDraft {
	zone, loc := appZone, appLoc
	if !e.AllDay && e.TimeZone != "" {
		if l, err := time.LoadLocation(e.TimeZone); err == nil {
			zone, loc = e.TimeZone, l
		}
	}
	d := newDraft(newTimes{e.AllDay, e.Start, e.End, e.StartDate, e.EndDate}, e.CalendarID, zone, loc)
	d.summary, d.location, d.description = e.Summary, e.Location, e.Description
	d.rrule = ruleOfRecurrence(e.Recurrence)
	d.alarms = slices.Clone(e.Alarms)
	d.transparent = e.Transparent
	for _, a := range e.Attendees {
		if !a.IsUser {
			d.invitees = append(d.invitees, draftInvitee{a.Email, a.Name, a.Role == api.AttendeeRoleOptional, a.Answer})
			d.fixedRoles = d.fixedRoles || a.Role == api.AttendeeRoleChair || a.Role == api.AttendeeRoleNone
		}
	}
	return d
}

func (d eventDraft) zoneLoc() *time.Location {
	if l, err := time.LoadLocation(d.timeZone); err == nil {
		return l
	}
	return time.UTC
}

// instants are a timed draft's start and end. An end typed in a repeated
// hour that would come before the start is the hour's second pass.
func (d eventDraft) instants() (start, end time.Time) {
	loc := d.zoneLoc()
	start = d.startRead.instant(d.startDate, d.startTime, d.timeZone, loc)
	end = d.endRead.instant(d.endDate, d.endTime, d.timeZone, loc)
	if !end.After(start) {
		end = secondPass(end)
	}
	return start, end
}

// secondPass is the later instant of a wall time the clocks going back
// repeat; any other instant as it is.
func secondPass(t time.Time) time.Time {
	_, end := t.ZoneBounds()
	if end.IsZero() {
		return t
	}
	_, before := t.Zone()
	_, after := end.Zone()
	if later := t.Add(time.Duration(before-after) * time.Second); before > after && !later.Before(end) {
		return later
	}
	return t
}

// wallExists reports whether a wall time happens in a zone: the clocks
// going forward skip some.
func wallExists(date, wall string, loc *time.Location) bool {
	d, m := zoned(instantAt(date, minutesOf(wall), loc), loc)
	return d == date && m == minutesOf(wall)
}

// validate names what keeps a draft from being saved.
func (d eventDraft) validate() draftErrors {
	e := d.validateTimes()
	if slices.ContainsFunc(d.invitees, func(i draftInvitee) bool { return !validAddress(i.email) }) {
		e.invitees = "Check the invitees' addresses"
	}
	return e
}

func (d eventDraft) validateTimes() draftErrors {
	if !dateForm.MatchString(d.startDate) || (!d.allDay && !timeForm.MatchString(d.startTime)) {
		return draftErrors{start: "Enter a start"}
	}
	if !dateForm.MatchString(d.endDate) || (!d.allDay && !timeForm.MatchString(d.endTime)) {
		return draftErrors{end: "Enter an end"}
	}
	if !d.allDay && d.timeZone == "" {
		return draftErrors{timeZone: "Choose a time zone"}
	}
	if d.allDay {
		if d.endDate < d.startDate {
			return draftErrors{end: "The end is before the start"}
		}
		return draftErrors{}
	}
	loc := d.zoneLoc()
	if !wallExists(d.startDate, d.startTime, loc) {
		return draftErrors{start: skippedTime}
	}
	if !wallExists(d.endDate, d.endTime, loc) {
		return draftErrors{end: skippedTime}
	}
	if start, end := d.instants(); !end.After(start) {
		return draftErrors{end: "The end is before the start"}
	}
	return draftErrors{}
}

// moveStart moves a draft's start, keeping its length; a rule naming the
// old start's day follows.
func moveStart(d eventDraft, startDate, startTime string) eventDraft {
	out := d
	out.startDate, out.startTime = startDate, startTime
	out.rrule = followStart(d.rrule, d.startDate, startDate)
	if d.allDay {
		out.endDate = addDays(d.endDate, daysBetween(d.startDate, startDate))
		return out
	}
	start, end := d.instants()
	loc := d.zoneLoc()
	newStart := d.startRead.instant(startDate, startTime, d.timeZone, loc)
	newEnd := newStart.Add(end.Sub(start))
	date, m := zoned(newEnd, loc)
	out.endDate, out.endTime = date, wallTime(m)
	out.startRead = readTime{startDate, startTime, d.timeZone, newStart}
	out.endRead = readTime{date, out.endTime, d.timeZone, newEnd}
	return out
}

func inviteesChanged(original *eventDraft, d eventDraft) bool {
	keys := func(list []draftInvitee) []string {
		out := make([]string, len(list))
		for i, v := range list {
			role := "required"
			if v.optional {
				role = "optional"
			}
			out[i] = strings.ToLower(v.email) + ":" + role
		}
		slices.Sort(out)
		return out
	}
	var before []string
	if original != nil {
		before = keys(original.invitees)
	}
	return !slices.Equal(before, keys(d.invitees))
}

// sendsInvitees reports whether a save writes a nonempty invitee list.
func sendsInvitees(original *eventDraft, d eventDraft) bool {
	return len(d.invitees) > 0 && inviteesChanged(original, d)
}

// changedFields is what a save sends: every field for a new event (empty
// text left out), else only what changed; a change to any of the times
// sends all of them.
func changedFields(original *eventDraft, d eventDraft) api.EventFields {
	var f api.EventFields
	text := func(pick func(eventDraft) string, set func(*string)) {
		v := pick(d)
		if (original != nil && pick(*original) != v) || (original == nil && v != "") {
			set(&v)
		}
	}
	text(func(e eventDraft) string { return e.summary }, func(v *string) { f.Summary = v })
	text(func(e eventDraft) string { return e.location }, func(v *string) { f.Location = v })
	text(func(e eventDraft) string { return e.description }, func(v *string) { f.Description = v })
	text(func(e eventDraft) string { return e.rrule }, func(v *string) { f.Rrule = v })
	if (original != nil && original.transparent != d.transparent) || (original == nil && d.transparent) {
		f.Transparent = ptr(d.transparent)
	}
	if original != nil && original.calendarID != d.calendarID {
		f.CalendarID = ptr(d.calendarID)
	}
	if (original != nil && !slices.Equal(original.alarms, d.alarms)) || (original == nil && len(d.alarms) > 0) {
		f.Alarms = append([]int64{}, d.alarms...)
	}
	if !d.fixedRoles && inviteesChanged(original, d) {
		f.Attendees = make([]api.Invitee, 0, len(d.invitees))
		for _, i := range d.invitees {
			role := api.AttendeeRoleRequired
			if i.optional {
				role = api.AttendeeRoleOptional
			}
			v := api.Invitee{Email: i.email, Role: &role}
			if i.name != "" {
				v.Name = ptr(i.name)
			}
			f.Attendees = append(f.Attendees, v)
		}
	}
	changed := original == nil || original.allDay != d.allDay || original.startDate != d.startDate || original.endDate != d.endDate ||
		(!d.allDay && (original.startTime != d.startTime || original.endTime != d.endTime || original.timeZone != d.timeZone))
	if changed {
		f.AllDay = ptr(d.allDay)
		if d.allDay {
			f.StartDate, f.EndDate = ptr(d.startDate), ptr(addDays(d.endDate, 1))
		} else {
			start, end := d.instants()
			f.Start, f.End, f.TimeZone = ptr(start.UTC()), ptr(end.UTC()), ptr(wireZone(d.timeZone))
		}
	}
	return f
}

// emptyFields reports whether a save has nothing to send.
func emptyFields(f api.EventFields) bool {
	return f.Summary == nil && f.Location == nil && f.Description == nil && f.AllDay == nil && f.Start == nil && f.End == nil &&
		f.TimeZone == nil && f.StartDate == nil && f.EndDate == nil && f.Rrule == nil && f.Alarms == nil && f.Transparent == nil &&
		f.CalendarID == nil && f.Attendees == nil
}
