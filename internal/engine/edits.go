package engine

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

// The event edits of plan 0009 (ADR-0030 to ADR-0034), which answer
// unavailable until its Phase 3 builds them; the contact edits are in
// contacts.go.

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
