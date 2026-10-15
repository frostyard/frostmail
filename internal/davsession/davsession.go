// Package davsession scripts the write sessions that are recorded against
// a provider's servers and replayed in tests (docs/design/testing.md, DAV
// and Tasks recordings). tools/davsession runs a session against the
// provider through a recording transport; the replay test runs the same
// session against the scrubbed recording. Both run this code, so they
// send the same requests in the same order.
package davsession

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/frostyard/frostmail/api"
)

// Session is what a script drives: an engine's calendar and people
// services over one account, and a sync pass of that account.
type Session struct {
	Calendar api.CalendarService
	People   api.PeopleService
	// Pass runs one sync pass of the account.
	Pass func(context.Context) error
	// Step, when set, hears each step's name before it runs.
	Step func(name string)
}

// GoogleZone is the zone the Google session's events are written in.
const GoogleZone = "America/New_York"

// onePixel is a 1×1 PNG, a contact's photo.
const onePixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII="

// Google is the Gmail write session: on the account's default calendar,
// a single event made, changed, deleted and restored; a daily series
// changed in each scope (This Event restored at once, as Ctrl+Z does);
// an all-day event made and deleted; then a contact made, changed, given
// a photo, deleted and restored. Everything it makes is deleted at the
// end, and a last pass finds nothing changed. Each write is followed by
// one pass, which sends it.
func Google(ctx context.Context, s Session) error {
	r := run{ctx: ctx, s: s}
	r.pass("first sync")
	before := r.contacts()

	// October 26–30, 2026 is EDT, four hours behind UTC.
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, 10, day, hour+4, minute, 0, 0, time.UTC)
	}
	timed := func(start, end time.Time) api.EventFields {
		zone := GoogleZone
		return api.EventFields{AllDay: new(false), Start: &start, End: &end, TimeZone: &zone}
	}
	key := func(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

	review := timed(at(27, 10, 0), at(27, 11, 0))
	review.Summary, review.Alarms = new("Replay review"), []int64{15}
	w := r.create("make an event", review)
	moved := timed(at(27, 11, 0), at(27, 12, 0))
	moved.Summary = new("Replay design review")
	w = r.update("rename and move it", &api.CalendarUpdateParams{ID: r.id(w), Fields: moved})
	deleted := r.delete("delete it", &api.CalendarDeleteParams{ID: r.id(w)})
	r.restore("restore it", deleted)

	standup := timed(at(26, 9, 30), at(26, 9, 45))
	standup.Summary, standup.Rrule = new("Replay standup"), new("FREQ=DAILY;COUNT=5")
	series := r.create("make a daily series", standup)
	this := r.update("move one occurrence", &api.CalendarUpdateParams{ID: r.id(series),
		RecurrenceID: new(key(at(27, 9, 30))), Scope: new(api.EventScopeThis), Fields: timed(at(27, 10, 0), at(27, 10, 15))})
	r.restore("restore the occurrence", this)
	series = r.update("change all occurrences", &api.CalendarUpdateParams{ID: r.id(series),
		RecurrenceID: new(key(at(28, 9, 30))), Scope: new(api.EventScopeAll), Fields: api.EventFields{Location: new("Room 4")}})
	split := r.update("change the rest from the fourth", &api.CalendarUpdateParams{ID: r.id(series),
		RecurrenceID: new(key(at(29, 9, 30))), Scope: new(api.EventScopeFuture), Fields: timed(at(29, 9, 45), at(29, 10, 0))})
	r.delete("delete the new series' last occurrence", &api.CalendarDeleteParams{ID: r.id(split),
		RecurrenceID: new(key(at(30, 9, 45))), Scope: new(api.EventScopeThis)})

	day := api.EventFields{Summary: new("Replay day"), AllDay: new(true), StartDate: new("2026-10-28"), EndDate: new("2026-10-29")}
	allDay := r.create("make an all-day event", day)
	r.delete("delete the all-day event", &api.CalendarDeleteParams{ID: r.id(allDay)})

	// The events found again by their times: a replay's summaries are
	// davrec's fakes.
	r.delete("delete the restored event", &api.CalendarDeleteParams{ID: r.event(at(27, 11, 0))})
	r.delete("delete the series", &api.CalendarDeleteParams{ID: r.event(at(26, 9, 30)),
		RecurrenceID: new(key(at(26, 9, 30))), Scope: new(api.EventScopeAll)})
	r.delete("delete the split series", &api.CalendarDeleteParams{ID: r.event(at(29, 9, 45)),
		RecurrenceID: new(key(at(29, 9, 45))), Scope: new(api.EventScopeAll)})

	c := r.contact("make a contact", api.ContactFields{GivenName: new("Replay"), FamilyName: new("Person"),
		Organization: new("Frostyard"), Emails: []api.LabeledValue{{Label: "work", Value: "replay.person@example.com"}},
		Phones: []api.LabeledValue{{Label: "mobile", Value: "+1 555 0100"}}})
	c = r.contactWrite("change it", func() (*api.ContactWrite, error) {
		return s.People.Update(ctx, &api.PeopleUpdateParams{ID: c.ContactID, Fields: api.ContactFields{Title: new("Tester")}})
	})
	c = r.contactWrite("give it a photo", func() (*api.ContactWrite, error) {
		return s.People.SetPhoto(ctx, &api.PeopleSetPhotoParams{ID: c.ContactID, Photo: &api.Photo{ContentType: "image/png", Data: onePixel}})
	})
	gone := r.contactWrite("delete it", func() (*api.ContactWrite, error) {
		return s.People.Delete(ctx, &api.PeopleDeleteParams{ID: c.ContactID})
	})
	r.step("restore it", func() error { return s.People.Restore(ctx, &api.PeopleRestoreParams{RestoreID: gone.RestoreID}) })
	r.contactWrite("delete the restored contact", func() (*api.ContactWrite, error) {
		id, err := r.newContact(before)
		if err != nil {
			return nil, err
		}
		return s.People.Delete(ctx, &api.PeopleDeleteParams{ID: id})
	})

	r.pass("a last pass, with nothing changed")
	return r.err
}

// run keeps the first error: after it, every step is skipped.
type run struct {
	ctx context.Context
	s   Session
	err error
}

// step runs one write and the pass that sends it.
func (r *run) step(name string, write func() error) {
	if r.err != nil {
		return
	}
	if r.s.Step != nil {
		r.s.Step(name)
	}
	if err := write(); err != nil {
		r.err = fmt.Errorf("%s: %w", name, err)
		return
	}
	if err := r.s.Pass(r.ctx); err != nil {
		r.err = fmt.Errorf("%s: the pass: %w", name, err)
	}
}

func (r *run) pass(name string) { r.step(name, func() error { return nil }) }

func (r *run) create(name string, fields api.EventFields) *api.EventWrite {
	var w *api.EventWrite
	r.step(name, func() (err error) {
		w, err = r.s.Calendar.Create(r.ctx, &api.CalendarCreateParams{Fields: fields})
		return err
	})
	return w
}

func (r *run) update(name string, p *api.CalendarUpdateParams) *api.EventWrite {
	var w *api.EventWrite
	r.step(name, func() (err error) {
		w, err = r.s.Calendar.Update(r.ctx, p)
		return err
	})
	return w
}

func (r *run) delete(name string, p *api.CalendarDeleteParams) *api.EventWrite {
	var w *api.EventWrite
	r.step(name, func() (err error) {
		w, err = r.s.Calendar.Delete(r.ctx, p)
		return err
	})
	return w
}

func (r *run) restore(name string, w *api.EventWrite) {
	r.step(name, func() error {
		if w == nil {
			return errors.New("nothing to restore")
		}
		return r.s.Calendar.Restore(r.ctx, &api.CalendarRestoreParams{RestoreID: w.RestoreID})
	})
}

// id is the event a write returned; 0 after an error, which stops the
// session at its next step.
func (r *run) id(w *api.EventWrite) int64 {
	if w == nil || w.Event == nil {
		if r.err == nil {
			r.err = errors.New("a write returned no event")
		}
		return 0
	}
	return w.Event.ID
}

// event is the event with an occurrence starting at start, a time no
// event of the account's own shares.
func (r *run) event(start time.Time) int64 {
	if r.err != nil {
		return 0
	}
	loc, err := time.LoadLocation(GoogleZone)
	if err != nil {
		r.err = err
		return 0
	}
	day := start.In(loc).Format(time.DateOnly)
	next := start.In(loc).AddDate(0, 0, 1).Format(time.DateOnly)
	zone := GoogleZone
	occ, err := r.s.Calendar.Range(r.ctx, &api.CalendarRangeParams{From: day, To: next, TimeZone: &zone})
	if err != nil {
		r.err = err
		return 0
	}
	for _, o := range occ {
		if !o.AllDay && o.Start.Equal(start) {
			return o.EventID
		}
	}
	r.err = fmt.Errorf("no event at %s", start.Format(time.RFC3339))
	return 0
}

func (r *run) contact(name string, fields api.ContactFields) *api.ContactWrite {
	return r.contactWrite(name, func() (*api.ContactWrite, error) {
		return r.s.People.Create(r.ctx, &api.PeopleCreateParams{Fields: fields})
	})
}

func (r *run) contactWrite(name string, write func() (*api.ContactWrite, error)) *api.ContactWrite {
	var w *api.ContactWrite
	r.step(name, func() (err error) {
		w, err = write()
		return err
	})
	if w == nil {
		w = &api.ContactWrite{}
	}
	return w
}

// contacts are the IDs of every contact.
func (r *run) contacts() map[int64]bool {
	out := map[int64]bool{}
	if r.err != nil {
		return out
	}
	people, err := r.s.People.List(r.ctx, &api.PeopleListParams{})
	if err != nil {
		r.err = err
		return out
	}
	for _, summary := range people {
		p, err := r.s.People.Get(r.ctx, &api.PeopleGetParams{ID: summary.ID})
		if err != nil {
			r.err = err
			return out
		}
		for _, c := range p.Contacts {
			out[c.ID] = true
		}
	}
	return out
}

// newContact is the one contact that was not among before: a contact is
// found again that way, since a replay's names are davrec's fakes.
func (r *run) newContact(before map[int64]bool) (int64, error) {
	var found []int64
	for id := range r.contacts() {
		if !before[id] {
			found = append(found, id)
		}
	}
	if r.err != nil {
		return 0, r.err
	}
	if len(found) != 1 {
		return 0, fmt.Errorf("%d new contacts", len(found))
	}
	return found[0], nil
}
