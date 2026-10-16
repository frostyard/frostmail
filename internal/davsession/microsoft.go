package davsession

import (
	"context"
	"time"

	"github.com/frostyard/frostmail/api"
)

// MicrosoftZone is the zone the Microsoft session's events are written in.
const MicrosoftZone = "America/New_York"

// eightPixels is an 8×8 JPEG, a contact's photo; Outlook stores its own
// encoding of it.
const eightPixels = "/9j/2wCEABALDA4MChAODQ4SERATGCgaGBYWGDEjJR0oOjM9PDkzODdASFxOQERXRTc4UG1RV19iZ2hnPk1xeXBkeFxlZ2MBERISGBUYLxoaL2NCOEJjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY2NjY//AABEIAAgACAMBIgACEQEDEQH/xAGiAAABBQEBAQEBAQAAAAAAAAAAAQIDBAUGBwgJCgsQAAIBAwMCBAMFBQQEAAABfQECAwAEEQUSITFBBhNRYQcicRQygZGhCCNCscEVUtHwJDNicoIJChYXGBkaJSYnKCkqNDU2Nzg5OkNERUZHSElKU1RVVldYWVpjZGVmZ2hpanN0dXZ3eHl6g4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2drh4uPk5ebn6Onq8fLz9PX29/j5+gEAAwEBAQEBAQEBAQAAAAAAAAECAwQFBgcICQoLEQACAQIEBAMEBwUEBAABAncAAQIDEQQFITEGEkFRB2FxEyIygQgUQpGhscEJIzNS8BVictEKFiQ04SXxFxgZGiYnKCkqNTY3ODk6Q0RFRkdISUpTVFVWV1hZWmNkZWZnaGlqc3R1dnd4eXqCg4SFhoeIiYqSk5SVlpeYmZqio6Slpqeoqaqys7S1tre4ubrCw8TFxsfIycrS09TV1tfY2dri4+Tl5ufo6ery8/T19vf4+fr/2gAMAwEAAhEDEQA/AMuiiivePPP/2Q=="

// Microsoft is the Outlook.com write session over Microsoft Graph (plan
// 0010, Phase 3b), on the account's default calendar: a single event
// made, changed, deleted and restored; a daily series with an occurrence
// moved and the move undone, an occurrence deleted, every occurrence
// changed, the series moved to another time (Outlook drops its exceptions
// then, and they are written again) and split from its fourth; an all-day
// event made and deleted; then a contact made, changed, given a photo,
// deleted and restored. Everything it makes is deleted at the end, and a
// last pass finds nothing changed. Each write is followed by one pass,
// which sends it. Its events and its contact are named "FM replay".
func Microsoft(ctx context.Context, s Session) error {
	r := run{ctx: ctx, s: s, zone: MicrosoftZone}
	r.pass("first sync")
	before := r.contacts()

	// October 26–30, 2026 is EDT, four hours behind UTC.
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, 10, day, hour+4, minute, 0, 0, time.UTC)
	}
	timed := func(start, end time.Time) api.EventFields {
		zone := MicrosoftZone
		return api.EventFields{AllDay: new(false), Start: &start, End: &end, TimeZone: &zone}
	}
	key := func(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

	review := timed(at(27, 10, 0), at(27, 11, 0))
	review.Summary, review.Alarms = new("FM replay review"), []int64{15}
	w := r.create("make an event", review)
	moved := timed(at(27, 11, 0), at(27, 12, 0))
	moved.Summary = new("FM replay design review")
	w = r.update("rename and move it", &api.CalendarUpdateParams{ID: r.id(w), Fields: moved})
	deleted := r.delete("delete it", &api.CalendarDeleteParams{ID: r.id(w)})
	r.restore("restore it", deleted)

	standup := timed(at(26, 9, 30), at(26, 9, 45))
	standup.Summary, standup.Rrule = new("FM replay standup"), new("FREQ=DAILY;COUNT=5")
	series := r.create("make a daily series", standup)
	this := r.update("move one occurrence", &api.CalendarUpdateParams{ID: r.id(series),
		RecurrenceID: new(key(at(27, 9, 30))), Scope: new(api.EventScopeThis), Fields: timed(at(27, 10, 0), at(27, 10, 15))})
	r.restore("undo the move", this)
	r.delete("delete one occurrence", &api.CalendarDeleteParams{ID: r.id(series),
		RecurrenceID: new(key(at(28, 9, 30))), Scope: new(api.EventScopeThis)})
	series = r.update("change all occurrences", &api.CalendarUpdateParams{ID: r.id(series),
		RecurrenceID: new(key(at(29, 9, 30))), Scope: new(api.EventScopeAll), Fields: api.EventFields{Location: new("FM Room 4")}})
	series = r.update("move the series later", &api.CalendarUpdateParams{ID: r.id(series),
		RecurrenceID: new(key(at(26, 9, 30))), Scope: new(api.EventScopeAll), Fields: timed(at(26, 9, 45), at(26, 10, 0))})
	r.update("change the rest from the fourth", &api.CalendarUpdateParams{ID: r.id(series),
		RecurrenceID: new(key(at(29, 9, 45))), Scope: new(api.EventScopeFuture), Fields: api.EventFields{Summary: new("FM replay standup v2")}})

	day := api.EventFields{Summary: new("FM replay day"), AllDay: new(true), StartDate: new("2026-10-28"), EndDate: new("2026-10-29")}
	allDay := r.create("make an all-day event", day)
	r.delete("delete the all-day event", &api.CalendarDeleteParams{ID: r.id(allDay)})

	// The events found again by their times: a replay's summaries are
	// davrec's fakes.
	r.delete("delete the restored event", &api.CalendarDeleteParams{ID: r.event(at(27, 11, 0))})
	r.delete("delete the series", &api.CalendarDeleteParams{ID: r.event(at(26, 9, 45)),
		RecurrenceID: new(key(at(26, 9, 45))), Scope: new(api.EventScopeAll)})
	r.delete("delete the split series", &api.CalendarDeleteParams{ID: r.event(at(29, 9, 45)),
		RecurrenceID: new(key(at(29, 9, 45))), Scope: new(api.EventScopeAll)})

	c := r.contact("make a contact", api.ContactFields{GivenName: new("FM"), FamilyName: new("Replay"),
		Organization: new("Frostyard"), Emails: []api.LabeledValue{{Label: "work", Value: "fm.replay@example.com"}},
		Phones: []api.LabeledValue{{Label: "mobile", Value: "+1 555 0100"}}})
	c = r.contactWrite("change it", func() (*api.ContactWrite, error) {
		return s.People.Update(ctx, &api.PeopleUpdateParams{ID: c.ContactID, Fields: api.ContactFields{Title: new("Tester")}})
	})
	c = r.contactWrite("give it a photo", func() (*api.ContactWrite, error) {
		return s.People.SetPhoto(ctx, &api.PeopleSetPhotoParams{ID: c.ContactID, Photo: &api.Photo{ContentType: "image/jpeg", Data: eightPixels}})
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
