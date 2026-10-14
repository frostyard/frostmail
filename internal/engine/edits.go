package engine

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

// The contact and event edits of plan 0009 (ADR-0030 to ADR-0034). Each
// method answers unavailable until its phase builds it: Phase 2 the
// contacts, Phase 3 the events.

func notYet(method string, phase int) *api.Error {
	return api.Unavailable("%s comes with plan 0009's Phase %d", method, phase)
}

func (calendarService) Create(context.Context, *api.CalendarCreateParams) (*api.EventWrite, error) {
	return nil, notYet("calendar.create", 3)
}

func (calendarService) Update(context.Context, *api.CalendarUpdateParams) (*api.EventWrite, error) {
	return nil, notYet("calendar.update", 3)
}

func (calendarService) Delete(context.Context, *api.CalendarDeleteParams) (*api.EventWrite, error) {
	return nil, notYet("calendar.delete", 3)
}

func (calendarService) Restore(context.Context, *api.CalendarRestoreParams) error {
	return notYet("calendar.restore", 3)
}

func (people) Create(context.Context, *api.PeopleCreateParams) (*api.ContactWrite, error) {
	return nil, notYet("people.create", 2)
}

func (people) Update(context.Context, *api.PeopleUpdateParams) (*api.ContactWrite, error) {
	return nil, notYet("people.update", 2)
}

func (people) Delete(context.Context, *api.PeopleDeleteParams) (*api.ContactWrite, error) {
	return nil, notYet("people.delete", 2)
}

func (people) SetPhoto(context.Context, *api.PeopleSetPhotoParams) (*api.ContactWrite, error) {
	return nil, notYet("people.setPhoto", 2)
}

func (people) Restore(context.Context, *api.PeopleRestoreParams) error {
	return notYet("people.restore", 2)
}
