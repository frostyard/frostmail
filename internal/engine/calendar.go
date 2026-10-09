package engine

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

// Calendar implements the calendar domain.
func (e *Engine) Calendar() api.CalendarService { return calendarService{e.d} }

type calendarService struct{ Deps }

// M4.5's Phase 3 and 4 cards implement these (docs/plans/0007).

func (c calendarService) Range(context.Context, *api.CalendarRangeParams) ([]api.Occurrence, error) {
	return nil, notBuilt("calendar.range")
}

func (c calendarService) Event(context.Context, *api.CalendarEventParams) (*api.CalendarEvent, error) {
	return nil, notBuilt("calendar.event")
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
