package pimsync

import (
	"context"
	"errors"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/store"
)

// The calendar's side of the replay (ADR-0030; docs/design/pim.md,
// Editing): an event put the server refused is applied again when it
// still fits, and a move's delete waits for its put.

// refusedPut handles an event put the server refused: a 412 on an edit
// is applied again to the server's version when it fits; anything else
// leaves the server's version and tells the app.
func (p *pass) refusedPut(ctx context.Context, c *davx.Client, col store.Collection, op store.PIMOp, err error) error {
	payload, edit := ParseEventPayload(op.Payload)
	precondition := errors.Is(err, davx.ErrPrecondition)
	if precondition && edit && !payload.Whole {
		return p.reapplyEvent(ctx, c, col, op, payload)
	}
	p.m.log.Warn("event change refused", "account", p.acct.ID, "href", op.Href, "err", err)
	reason := "refused"
	if precondition {
		reason = "changed"
	}
	return p.refusedEvent(ctx, op, col, nil, reason)
}

// reapplyEvent is reapply for an event edit.
func (p *pass) reapplyEvent(ctx context.Context, c *davx.Client, col store.Collection, op store.PIMOp, payload EventPayload) error {
	for range reapplyAttempts {
		server, err := c.Get(ctx, op.Href)
		if errors.Is(err, davx.ErrNotFound) {
			return p.refusedEvent(ctx, op, col, nil, "deleted")
		}
		if err != nil {
			return p.failed(ctx, op, err)
		}
		if !payload.Fits(server.Data) {
			return p.refusedEvent(ctx, op, col, &server, "changed")
		}
		raw, err := payload.Apply(server.Data, p.m.db.Now())
		if err != nil {
			return p.refusedEvent(ctx, op, col, &server, "changed")
		}
		w, err := c.Put(ctx, op.Href, raw, davx.Calendars, server.ETag)
		if errors.Is(err, davx.ErrPrecondition) {
			continue
		}
		if err != nil {
			return p.failed(ctx, op, err)
		}
		written := davx.Object{Href: op.Href, ETag: w.ETag, Data: raw}
		if w.ETag == "" {
			if written, err = c.Get(ctx, op.Href); err != nil {
				return p.failed(ctx, op, err)
			}
		}
		return p.done(ctx, op, func(tx *store.Tx) error { return p.indexDAV(ctx, tx, col, written) })
	}
	return p.refusedEvent(ctx, op, col, nil, "changed")
}

// refusedEvent ends an event write the server will not take, and names
// the event in calendar.refused: the server's version (when there is one)
// replaces the local one; a creation, or an event the server no longer
// has, is removed here; otherwise the next full listing brings the
// server's version.
func (p *pass) refusedEvent(ctx context.Context, op store.PIMOp, col store.Collection, server *davx.Object, reason string) error {
	summary, id := p.eventOf(ctx, op)
	return p.done(ctx, op, func(tx *store.Tx) error {
		switch {
		case server != nil:
			if err := p.indexDAV(ctx, tx, col, *server); err != nil {
				return err
			}
		case reason == "deleted" || op.IfMatch == "":
			if _, err := tx.DeleteObjects(ctx, col.ID, []string{op.Href}); err != nil {
				return err
			}
			id = nil
			if err := backFromMove(ctx, tx, op); err != nil {
				return err
			}
		default:
			if err := tx.SetCollectionSync(ctx, col.ID, "", ""); err != nil {
				return err
			}
		}
		return tx.Emit(ctx, api.CalendarRefused{AccountID: p.acct.ID, EventID: id, Summary: summary, Reason: reason})
	})
}

// backFromMove drops the delete a refused creation's move waits for, and
// lists the calendar it left in full again, which brings the event back.
func backFromMove(ctx context.Context, tx *store.Tx, op store.PIMOp) error {
	if op.Kind != "put" || op.IfMatch != "" {
		return nil
	}
	cols, err := tx.DropDeletesAfter(ctx, op.ID)
	if err != nil {
		return err
	}
	for _, col := range cols {
		if err := tx.SetCollectionSync(ctx, col, "", ""); err != nil {
			return err
		}
	}
	return nil
}

// eventOf is the summary and event (the master's row) of the object an
// op writes; the summary of a delete comes from its payload.
func (p *pass) eventOf(ctx context.Context, op store.PIMOp) (string, *int64) {
	payload, _ := ParseEventPayload(op.Payload)
	if op.ObjectID == nil {
		return payload.Summary, nil
	}
	obj, err := p.m.db.GetObject(ctx, *op.ObjectID)
	if err != nil {
		return payload.Summary, nil
	}
	events, err := p.m.db.EventsByUID(ctx, p.acct.ID, obj.UID)
	if err != nil {
		return payload.Summary, nil
	}
	for _, e := range events {
		if e.ObjectID == obj.ID && e.RecurrenceID == "" {
			return e.Summary, &e.ID
		}
	}
	if events, err := calendar.Parse(obj.Raw, calendar.Options{}); err == nil && len(events) > 0 {
		return events[0].Summary, nil
	}
	return payload.Summary, nil
}

// deleteEvent writes an event's delete. A move's delete waits while the
// put to the other calendar is queued, and is dropped, the event coming
// back here, if the server refused that put. A 412 or another refusal
// brings the server's version back; a 404 is a delete already done.
func (p *pass) deleteEvent(ctx context.Context, c *davx.Client, col store.Collection, op store.PIMOp) error {
	payload, _ := ParseEventPayload(op.Payload)
	if _, queued, err := p.m.db.PIMOpState(ctx, op.ID); err != nil || !queued {
		return err // dropped earlier in this pass, with the move's refused put
	}
	if payload.After != 0 {
		state, waiting, err := p.m.db.PIMOpState(ctx, payload.After)
		if err != nil {
			return err
		}
		if waiting && state != "failed" {
			return nil
		}
		if waiting {
			return p.refusedEvent(ctx, op, col, nil, "refused")
		}
	}
	err := c.Delete(ctx, op.Href, op.IfMatch)
	switch {
	case err == nil || errors.Is(err, davx.ErrNotFound):
		return p.done(ctx, op, nil)
	case errors.Is(err, davx.ErrPrecondition):
		return p.refusedEvent(ctx, op, col, nil, "changed")
	case refusal(err):
		return p.refusedEvent(ctx, op, col, nil, "refused")
	}
	return p.failed(ctx, op, err)
}
