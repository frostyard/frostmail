package engine

import (
	"cmp"
	"context"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/store"
	"slices"
	"strings"
	"time"
)

// Calendar implements the calendar domain.
func (e *Engine) Calendar() api.CalendarService { return calendarService{e.d} }

type calendarService struct{ Deps }

// Range returns visible occurrences overlapping the requested days.
func (c calendarService) Range(ctx context.Context, p *api.CalendarRangeParams) ([]api.Occurrence, error) {
	zone := time.Local
	if p.TimeZone != nil {
		var err error
		if *p.TimeZone == "" || *p.TimeZone == "Local" {
			return nil, api.InvalidParams("unknown time zone")
		}
		zone, err = time.LoadLocation(*p.TimeZone)
		if err != nil {
			return nil, api.InvalidParams("unknown time zone")
		}
	}
	from, err := time.Parse(time.DateOnly, p.From)
	if err != nil {
		return nil, api.InvalidParams("from must be YYYY-MM-DD")
	}
	to, err := time.Parse(time.DateOnly, p.To)
	if err != nil || !to.After(from) || to.After(from.AddDate(0, 0, 400)) {
		return nil, api.InvalidParams("to must be 1 to 400 days after from")
	}
	return c.occurrences(ctx, store.OccurrenceFilter{From: dayInZone(from, zone), To: dayInZone(to, zone),
		FromDate: p.From, ToDate: p.To, CalendarIDs: p.CalendarIDs}, zone)
}

func dayInZone(day time.Time, zone *time.Location) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, zone).UTC()
}

func (c calendarService) occurrences(ctx context.Context, f store.OccurrenceFilter, zone *time.Location) ([]api.Occurrence, error) {
	from, to, ok, err := c.DB.InstanceWindow(ctx)
	if err != nil {
		return nil, err
	}
	var rows []store.OccurrenceRow
	limit := f.Limit
	f.Limit = 0
	if ok && !f.From.Before(from) && !f.To.After(to) && f.FromDate >= from.Format(time.DateOnly) && f.ToDate <= to.Format(time.DateOnly) {
		rows, err = c.DB.Occurrences(ctx, f)
	} else {
		rows, err = c.expandRange(ctx, f)
	}
	if err != nil {
		return nil, err
	}
	out := make([]api.Occurrence, 0, len(rows))
	for _, row := range rows {
		out = append(out, occurrenceAPI(row, zone))
	}
	slices.SortStableFunc(out, func(a, b api.Occurrence) int {
		if n := a.Start.Compare(b.Start); n != 0 {
			return n
		}
		if a.AllDay != b.AllDay {
			if a.AllDay {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.EventID, b.EventID)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (c calendarService) expandRange(ctx context.Context, f store.OccurrenceFilter) ([]store.OccurrenceRow, error) {
	events, err := c.DB.Events(ctx, store.EventsFilter{Visible: true, CalendarIDs: f.CalendarIDs})
	if err != nil {
		return nil, err
	}
	dateFrom, _ := time.Parse(time.DateOnly, f.FromDate)
	dateTo, _ := time.Parse(time.DateOnly, f.ToDate)
	from, to := f.From, f.To
	if dateFrom.Before(from) {
		from = dateFrom
	}
	if dateTo.After(to) {
		to = dateTo
	}
	out := []store.OccurrenceRow{}
	for len(events) > 0 {
		n := 1
		for n < len(events) && events[n].ObjectID == events[0].ObjectID {
			n++
		}
		group := events[:n]
		instances, err := pimsync.Instances(group, time.Local, from, to)
		if err != nil {
			return nil, err
		}
		byID := make(map[int64]store.EventRow, n)
		for _, e := range group {
			if f.WithEmail != "" {
				e, err = c.DB.Event(ctx, e.ID)
				if err != nil {
					return nil, err
				}
			}
			byID[e.ID] = e
		}
		for _, i := range instances {
			e := byID[i.EventID]
			if instanceMatches(i, f) && eventWithEmail(e, f.WithEmail) {
				out = append(out, store.OccurrenceRow{Instance: i, Event: e})
			}
		}
		events = events[n:]
	}
	return out, nil
}

func instanceMatches(i store.InstanceRow, f store.OccurrenceFilter) bool {
	if i.AllDay {
		return i.Start.Format(time.DateOnly) < f.ToDate && i.End.Format(time.DateOnly) > f.FromDate
	}
	return i.Start.Before(f.To) && (i.End.After(f.From) || i.Start.Equal(i.End) && !i.Start.Before(f.From))
}

func eventWithEmail(e store.EventRow, email string) bool {
	return email == "" || e.Organizer == email || slices.ContainsFunc(e.Attendees, func(a store.AttendeeRow) bool { return a.Email == email })
}

func occurrenceAPI(row store.OccurrenceRow, zone *time.Location) api.Occurrence {
	e, i := row.Event, row.Instance
	out := api.Occurrence{EventID: i.EventID, RecurrenceID: i.RecurrenceID, CalendarID: e.CalendarID,
		AccountID: e.AccountID, Summary: e.Summary, Location: e.Location, AllDay: i.AllDay, Start: i.Start, End: i.End,
		Status: api.EventStatus(e.Status), Transparent: e.Transparent, Recurring: i.RecurrenceID != "", Answer: eventAnswer(e.PartStat)}
	if i.AllDay {
		out.StartDate, out.EndDate = i.Start.Format(time.DateOnly), i.End.Format(time.DateOnly)
		out.Start, out.End = dayInZone(i.Start, zone), dayInZone(i.End, zone)
	}
	return out
}

func eventAnswer(partStat string) *api.PartStat {
	if partStat == "" {
		return nil
	}
	answer := api.PartStat(partStat)
	return &answer
}

// Event returns an event or the requested occurrence of its series.
func (c calendarService) Event(ctx context.Context, p *api.CalendarEventParams) (*api.CalendarEvent, error) {
	e, err := c.DB.Event(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, "calendar event")
	}
	if p.RecurrenceID != nil && *p.RecurrenceID != e.RecurrenceID {
		if e.Recurrence == "" || e.RecurrenceID != "" {
			return nil, api.NotFound("event is not a series master")
		}
		layout := store.TimeFormat
		if e.AllDay {
			layout = time.DateOnly
		}
		start, err := time.Parse(layout, *p.RecurrenceID)
		if err != nil || calendar.RecurrenceKey(start, e.AllDay) != *p.RecurrenceID {
			return nil, api.NotFound("invalid recurrence id")
		}
		e.End, e.Start = start.Add(e.End.Sub(e.Start)), start
		e.RecurrenceID = *p.RecurrenceID
	}
	emails, err := c.eventUserEmails(ctx, e.AccountID)
	if err != nil {
		return nil, err
	}
	out := calendarEventAPI(e, emails)
	return &out, nil
}

func (c calendarService) eventUserEmails(ctx context.Context, id int64) ([]string, error) {
	account, err := c.DB.GetAccount(ctx, id)
	if err != nil {
		return nil, err
	}
	identities, err := c.DB.ListIdentities(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []string{strings.ToLower(account.Email)}
	for _, i := range identities {
		out = append(out, strings.ToLower(i.Email))
	}
	return out, nil
}

func calendarEventAPI(e store.EventRow, emails []string) api.CalendarEvent {
	out := api.CalendarEvent{ID: e.ID, RecurrenceID: e.RecurrenceID, CalendarID: e.CalendarID, AccountID: e.AccountID,
		UID: e.UID, Summary: e.Summary, Location: e.Location, Description: e.Description, AllDay: e.AllDay,
		Start: e.Start, End: e.End, TimeZone: e.TZID, Recurring: e.Recurrence != "" || e.RecurrenceID != "",
		Recurrence: e.Recurrence, Status: api.EventStatus(e.Status), Transparent: e.Transparent,
		Answer: eventAnswer(e.PartStat), ReadOnly: e.ReadOnly, Attendees: []api.Attendee{}, Alarms: []int64{}}
	if strings.HasPrefix(e.TZID, "UTC") {
		out.TimeZone = ""
	}
	if e.AllDay {
		out.StartDate, out.EndDate = e.Start.Format(time.DateOnly), e.End.Format(time.DateOnly)
	}
	if e.Organizer != "" {
		out.Organizer = &api.Attendee{Email: e.Organizer, Name: e.OrganizerName, Role: api.AttendeeRoleChair,
			Answer: api.PartStatAccepted, IsUser: slices.Contains(emails, e.Organizer)}
	}
	for _, a := range e.Attendees {
		out.Attendees = append(out.Attendees, api.Attendee{Email: a.Email, Name: a.Name, Role: api.AttendeeRole(a.Role),
			Answer: api.PartStat(a.PartStat), IsUser: slices.Contains(emails, a.Email)})
	}
	for _, a := range e.Alarms {
		var before time.Duration
		if a.At != nil {
			before = e.Start.Sub(*a.At)
		} else if a.Offset != nil {
			before = -*a.Offset
			if a.RelatedEnd {
				before -= e.End.Sub(e.Start)
			}
		}
		out.Alarms = append(out.Alarms, int64(before/time.Minute))
	}
	return out
}

func (c calendarService) Invitation(context.Context, *api.CalendarInvitationParams) (*api.Invitation, error) {
	return nil, notBuilt("calendar.invitation")
}

func (c calendarService) Respond(context.Context, *api.CalendarRespondParams) (*api.CalendarEvent, error) {
	return nil, notBuilt("calendar.respond")
}

func (c calendarService) Reminders(context.Context, *api.CalendarRemindersParams) ([]api.Reminder, error) {
	return []api.Reminder{}, nil
}

func (c calendarService) Snooze(context.Context, *api.CalendarSnoozeParams) error {
	return notBuilt("calendar.snooze")
}

func (c calendarService) Dismiss(context.Context, *api.CalendarDismissParams) error {
	return notBuilt("calendar.dismiss")
}
