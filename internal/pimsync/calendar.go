package pimsync

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/store"
)

// Window returns the UTC day's instances window: 365 days back and 730 ahead.
func Window(now time.Time) (from, to time.Time) {
	day := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return day.AddDate(0, 0, -365), day.AddDate(0, 0, 730)
}

// Instances expands one object's stored events over [from, to).
func Instances(events []store.EventRow, local *time.Location, from, to time.Time) ([]store.InstanceRow, error) {
	if local == nil {
		local = time.Local
	}
	parsed := make([]calendar.Event, 0, len(events))
	for _, e := range events {
		zone := time.UTC
		if !e.AllDay {
			zone = calendar.ZoneNamed(e.TZID, local)
		}
		parsed = append(parsed, calendar.Event{UID: e.UID, RecurrenceID: e.RecurrenceID, AllDay: e.AllDay,
			Start: e.Start, End: e.End, Recurrence: e.Recurrence, Zone: zone})
	}
	occurrences, err := calendar.Expand(parsed, from, to)
	if err != nil {
		return nil, fmt.Errorf("expand events: %w", err)
	}
	out := make([]store.InstanceRow, 0, len(occurrences))
	for _, o := range occurrences {
		out = append(out, store.InstanceRow{EventID: events[o.Event].ID, RecurrenceID: o.RecurrenceID,
			AllDay: o.AllDay, Start: o.Start, End: o.End})
	}
	return out, nil
}

func (p *pass) prepareCalendar(ctx context.Context) error {
	identities, err := p.m.db.ListIdentities(ctx, p.acct.ID)
	if err != nil {
		return err
	}
	p.userEmails = []string{strings.ToLower(p.acct.Email)}
	for _, i := range identities {
		p.userEmails = append(p.userEmails, strings.ToLower(i.Email))
	}
	p.m.windowMu.Lock()
	defer p.m.windowMu.Unlock()
	p.from, p.to = Window(p.m.cfg.Now())
	from, to, ok, err := p.m.db.InstanceWindow(ctx)
	if err != nil || ok && from.Equal(p.from) && to.Equal(p.to) {
		return err
	}
	return p.moveWindow(ctx)
}

func (p *pass) moveWindow(ctx context.Context) error {
	return p.m.db.Tx(ctx, func(tx *store.Tx) error {
		events, err := p.m.db.Events(ctx, store.EventsFilter{})
		if err != nil {
			return err
		}
		for len(events) > 0 {
			n := 1
			for n < len(events) && events[n].ObjectID == events[0].ObjectID {
				n++
			}
			rows, err := Instances(events[:n], p.m.cfg.Local, p.from, p.to)
			if err != nil {
				return err
			}
			if err := tx.ReplaceInstances(ctx, events[0].ObjectID, rows); err != nil {
				return err
			}
			events = events[n:]
		}
		return tx.SetInstanceWindow(ctx, p.from, p.to)
	})
}

func (p *pass) indexCalendar(ctx context.Context, tx *store.Tx, o store.Object) error {
	calendarMetadata(&o)
	var events []store.EventRow
	if o.Kind == store.ObjectVEvent {
		parsed, err := calendar.Parse(o.Raw, calendar.Options{Local: p.m.cfg.Local, UserEmails: p.userEmails})
		if err != nil {
			o.ParseError = err.Error()
		} else {
			// The store holds one event per recurrence ID in an object:
			// keep the first of any that repeat one (two UIDs in a file).
			seen := map[string]bool{}
			for _, e := range parsed {
				if !seen[e.RecurrenceID] {
					seen[e.RecurrenceID] = true
					events = append(events, indexedEvent(e))
				}
			}
		}
	}
	id, err := tx.PutObject(ctx, o)
	if err != nil {
		return fmt.Errorf("store calendar object: %w", err)
	}
	ids, err := tx.IndexEvents(ctx, id, events)
	if err != nil {
		return err
	}
	for i := range events {
		events[i].ID = ids[i]
	}
	// Another account's pass may have moved the window while this one fetched.
	// The writer transaction keeps that committed window stable while indexing.
	from, to, ok, err := p.m.db.InstanceWindow(ctx)
	if err != nil {
		return err
	}
	if !ok {
		from, to = p.from, p.to
	}
	rows, err := Instances(events, p.m.cfg.Local, from, to)
	if err != nil {
		return err
	}
	return tx.ReplaceInstances(ctx, id, rows)
}

func indexedEvent(e calendar.Event) store.EventRow {
	row := store.EventRow{UID: e.UID, RecurrenceID: e.RecurrenceID, Summary: e.Summary, Location: e.Location,
		Description: e.Description, AllDay: e.AllDay, Start: e.Start, End: e.End, TZID: e.TZID,
		Recurrence: e.Recurrence, Status: e.Status, Transparent: e.Transparent, PartStat: e.PartStat, Sequence: e.Sequence}
	if e.Organizer != nil {
		row.Organizer, row.OrganizerName = e.Organizer.Email, e.Organizer.Name
	}
	for _, a := range e.Attendees {
		row.Attendees = append(row.Attendees, store.AttendeeRow{Email: a.Email, Name: a.Name, Role: a.Role, PartStat: a.PartStat})
	}
	for _, a := range e.Alarms {
		alarm := store.AlarmRow{Action: a.Action, RelatedEnd: a.RelatedEnd}
		if a.At.IsZero() {
			alarm.Offset = &a.Offset
		} else {
			alarm.At = &a.At
		}
		row.Alarms = append(row.Alarms, alarm)
	}
	return row
}
