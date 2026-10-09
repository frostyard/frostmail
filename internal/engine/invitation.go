package engine

import (
	"context"
	"errors"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/itip"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/store"
)

// Invitation returns the message's invitation and its current calendar context.
func (c calendarService) Invitation(ctx context.Context, p *api.CalendarInvitationParams) (*api.Invitation, error) {
	sums, err := c.DB.Summaries(ctx, []int64{p.MessageID})
	if err != nil {
		return nil, err
	}
	if len(sums) == 0 {
		return nil, api.NotFound("message %d does not exist", p.MessageID)
	}
	accountID := sums[0].AccountID
	emails, err := c.eventUserEmails(ctx, accountID)
	if err != nil {
		return nil, err
	}
	msg, err := c.readInvitation(ctx, p.MessageID, emails)
	if err != nil {
		return nil, err
	}
	main := pimsync.EventRow(msg.Main())
	main.AccountID = accountID
	stored, err := c.DB.EventsByUID(ctx, accountID, main.UID)
	if err != nil {
		return nil, err
	}
	account, err := c.DB.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := invitationAPI(msg.Method, main, emails, stored, account.ReadOnly)
	if err := c.invitationOccurrences(ctx, &out, stored); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c calendarService) readInvitation(ctx context.Context, id int64, emails []string) (*itip.Message, error) {
	raw, err := messages(c).raw(ctx, id)
	if err != nil {
		return nil, err
	}
	src, err := itip.FromMessage(raw)
	if errors.Is(err, itip.ErrNoInvitation) {
		return nil, api.NotFound("message %d has no invitation", id)
	}
	if err != nil {
		return nil, api.InvalidParams("the invitation cannot be read: %v", err)
	}
	msg, err := itip.Parse(src, calendar.Options{Local: time.Local, UserEmails: emails})
	if err != nil {
		return nil, api.InvalidParams("the invitation cannot be read: %v", err)
	}
	return msg, nil
}

func invitationAPI(method string, main store.EventRow, emails []string, stored []store.EventRow, readOnly bool) api.Invitation {
	out := api.Invitation{Method: api.ITIPMethod(method), Event: calendarEventAPI(main, emails),
		Answer: eventAnswer(main.PartStat), Conflicts: []api.Occurrence{}, Adjacent: []api.Occurrence{}}
	for _, e := range stored {
		if e.RecurrenceID != main.RecurrenceID {
			continue
		}
		out.EventID = &e.ID
		out.Outdated = e.Sequence > main.Sequence
		if e.PartStat != "" {
			out.Answer = eventAnswer(e.PartStat)
		}
		readOnly = readOnly || e.ReadOnly
		break
	}
	out.From = out.Event.Organizer
	if method == itip.MethodReply {
		out.From = nil
		if len(out.Event.Attendees) > 0 {
			out.From = &out.Event.Attendees[0]
		}
	}
	out.CanRespond = method == itip.MethodRequest && main.PartStat != "" && !out.Outdated && !readOnly && main.Status != "cancelled"
	return out
}

func (c calendarService) invitationOccurrences(ctx context.Context, out *api.Invitation, stored []store.EventRow) error {
	if out.Event.AllDay {
		return nil
	}
	day := out.Event.Start.In(time.Local)
	from := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local)
	to := from.AddDate(0, 0, 1)
	occ, err := c.occurrences(ctx, store.OccurrenceFilter{From: from.UTC(), To: to.UTC(),
		FromDate: from.Format(time.DateOnly), ToDate: to.Format(time.DateOnly)}, time.Local)
	if err != nil {
		return err
	}
	own := make(map[int64]bool, len(stored))
	for _, e := range stored {
		own[e.ID] = true
	}
	var before, after *api.Occurrence
	for _, o := range occ {
		if own[o.EventID] || o.AllDay || o.Status == api.EventStatusCancelled || o.Answer != nil && *o.Answer == api.PartStatDeclined {
			continue
		}
		if o.Start.Before(out.Event.End) && o.End.After(out.Event.Start) {
			if !o.Transparent {
				out.Conflicts = append(out.Conflicts, o)
			}
			continue
		}
		if !o.End.After(out.Event.Start) && (before == nil || o.End.After(before.End)) {
			before = &o
		}
		if !o.Start.Before(out.Event.End) && (after == nil || o.Start.Before(after.Start)) {
			after = &o
		}
	}
	for _, o := range []*api.Occurrence{before, after} {
		if o != nil {
			out.Adjacent = append(out.Adjacent, *o)
		}
	}
	return nil
}
