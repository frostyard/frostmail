package engine

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/itip"
	"github.com/frostyard/frostmail/internal/store"
)

// errHeld says a calendar holds the event already.
var errHeld = errors.New("engine: a calendar holds the event")

// AddFromMessage adds the published event of a message to a calendar as
// the user's own event (ADR-0059): the message's source without its
// METHOD, its organizer and attendees and its alarms, stored under a new
// href with the event's UID and queued to be put on the server. An event
// that a calendar of any account holds is not added again: the check runs
// in the transaction that stores the copy, so two adds at once store one.
func (c calendarService) AddFromMessage(ctx context.Context, p *api.CalendarAddFromMessageParams) (*api.CalendarEvent, error) {
	sums, err := c.DB.Summaries(ctx, []int64{p.MessageID})
	if err != nil {
		return nil, err
	}
	if len(sums) == 0 {
		return nil, api.NotFound("message %d does not exist", p.MessageID)
	}
	cal, err := c.addTarget(ctx, sums[0].AccountID, p.CalendarID)
	if err != nil {
		return nil, err
	}
	r := response{}
	if err := c.setAccount(ctx, &r, cal.AccountID); err != nil {
		return nil, err
	}
	if r.acct.ReadOnly {
		return nil, api.Conflict("the calendar of account %d is read-only", r.acct.ID)
	}
	msg, err := c.readInvitation(ctx, p.MessageID, r.emails)
	if err != nil {
		return nil, err
	}
	if msg.Method != itip.MethodPublish {
		return nil, api.Conflict("message %d has no published event to add", p.MessageID)
	}
	main := msg.Main()
	raw, err := itip.Published(msg.Source)
	if err != nil {
		return nil, api.InvalidParams("the event cannot be read: %v", err)
	}
	uid, err := c.newUID()
	if err != nil {
		return nil, err
	}
	obj := store.Object{CollectionID: cal.ID, Href: strings.TrimSuffix(cal.Href, "/") + "/" + uid + ".ics",
		Kind: store.ObjectVEvent, UID: main.UID, Raw: raw}
	err = c.DB.Tx(ctx, func(tx *store.Tx) error {
		held, err := tx.HoldsEventUID(ctx, main.UID)
		if err != nil {
			return err
		}
		if held {
			return errHeld
		}
		return c.storeAnswer(ctx, tx, &r, obj)
	})
	if errors.Is(err, errHeld) {
		return nil, api.Conflict("a calendar holds this event already")
	}
	if err != nil {
		return nil, err
	}
	if c.PIM != nil {
		c.PIM.Kick(r.acct.ID)
	}
	stored, err := c.DB.EventsByUID(ctx, r.acct.ID, main.UID)
	if err != nil {
		return nil, err
	}
	for _, e := range stored {
		if e.RecurrenceID == main.RecurrenceID {
			return c.Event(ctx, &api.CalendarEventParams{ID: e.ID})
		}
	}
	return nil, api.NotFound("the added event is not in the calendar")
}

// addTarget is the calendar an event from a message of account goes to:
// the one named, which must take writes, else the account's default.
func (c calendarService) addTarget(ctx context.Context, account int64, id *int64) (store.Collection, error) {
	if id == nil {
		cal, ok, err := c.defaultCalendar(ctx, account)
		if err != nil {
			return cal, err
		}
		if !ok {
			return cal, api.Conflict("account %d has no calendar that can be written", account)
		}
		return cal, nil
	}
	cal, err := c.DB.GetCollection(ctx, *id)
	if err != nil || cal.Kind != api.CollectionKindCalendar {
		return cal, api.NotFound("calendar %d does not exist", *id)
	}
	if cal.ReadOnly {
		return cal, api.Conflict("calendar %d is read-only", *id)
	}
	return cal, nil
}

// canAdd reports whether the published event of a message of account can
// be added: no calendar holds it (stored), and one of the account's takes
// writes.
func (c calendarService) canAdd(ctx context.Context, method string, account store.Account, stored []store.EventRow) (bool, error) {
	if method != itip.MethodPublish || account.ReadOnly || len(stored) > 0 {
		return false, nil
	}
	cols, err := c.DB.Collections(ctx, store.CollectionFilter{AccountID: account.ID, Kind: api.CollectionKindCalendar, EnabledOnly: true})
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(cols, func(col store.Collection) bool { return !col.ReadOnly }), nil
}
