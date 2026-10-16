package pimsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/gtasks"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/store"
)

// replay writes the account's due changes to the server before the pass
// reads (docs/design/pim.md, Writes). A read-only account never writes.
func (p *pass) replay(ctx context.Context, services []store.Service) error {
	if p.acct.ReadOnly {
		return nil
	}
	if err := p.dropOldRestores(ctx); err != nil {
		return err
	}
	ops, err := p.m.db.DuePIMOps(ctx, p.acct.ID, p.m.db.Now())
	if err != nil {
		return err
	}
	for _, op := range ops {
		if err := p.replayOne(ctx, op, services); err != nil {
			return err
		}
	}
	return nil
}

// replayOne writes one change. It returns only what ends the pass: a
// refused credential or the store failing; the change's own failures are
// recorded on it.
func (p *pass) replayOne(ctx context.Context, op store.PIMOp, services []store.Service) error {
	col, err := p.m.db.GetCollection(ctx, op.CollectionID)
	if errors.Is(err, store.ErrNotFound) {
		return p.done(ctx, op, nil)
	}
	if err != nil {
		return err
	}
	if col.Kind == api.CollectionKindTasklist && strings.HasPrefix(op.Kind, "tasks.") {
		return p.replayGoogleTask(ctx, op, col, services)
	}
	kind, service := davx.AddressBooks, api.ServiceKindContacts
	switch col.Kind {
	case api.CollectionKindCalendar:
		kind, service = davx.Calendars, api.ServiceKindCalendar
	case api.CollectionKindTasklist:
		kind, service = davx.Calendars, api.ServiceKindTasks
	}
	svc, ok := enabledService(services, service)
	if !ok || p.scope(service) != nil {
		return nil // waits until the service is on and signed in
	}
	var c objectServer
	if providers.ForKind(p.acct.Kind).DAV.Graph {
		gc, err := p.graphClient()
		if err != nil {
			return err
		}
		c = graphObjects{p: p, c: gc, col: col, op: op}
	} else {
		home, err := p.home(ctx, svc, kind)
		if err != nil {
			return p.failed(ctx, op, err)
		}
		c = home
	}
	switch op.Kind {
	case "put":
		return p.put(ctx, c, kind, col, op)
	case "delete":
		if col.Kind == api.CollectionKindCalendar {
			return p.deleteEvent(ctx, c, col, op)
		}
		err := c.Delete(ctx, op.Href, op.IfMatch)
		if err != nil && kind == davx.AddressBooks && refusal(err) {
			return p.refusedDelete(ctx, op, col)
		}
		if err != nil {
			return p.failed(ctx, op, err)
		}
		return p.done(ctx, op, nil)
	}
	return p.failed(ctx, op, fmt.Errorf("pimsync: no replay for %q", op.Kind))
}

// objectServer is where a collection's objects are read and written by
// href: a DAV server, or Microsoft Graph through graphObjects, which takes
// and gives the sources maild derives (ADR-0040).
type objectServer interface {
	Get(ctx context.Context, href string) (davx.Object, error)
	Put(ctx context.Context, href string, data []byte, kind davx.Kind, ifMatch string) (davx.Written, error)
	Delete(ctx context.Context, href, ifMatch string) error
}

func enabledService(services []store.Service, kind api.ServiceKind) (store.Service, bool) {
	for _, s := range services {
		if s.Service == kind && s.Enabled {
			return s, true
		}
	}
	return store.Service{}, false
}

// put writes an object as stored here, conditional on op.IfMatch, and
// records the server's ETag; a server that sends none is asked for the
// object, whose version then replaces the local one, or whose absence
// removes it. A new object the server stored under a name of its own
// moves there, or gives way to the object a pass already brought from
// there.
func (p *pass) put(ctx context.Context, c objectServer, kind davx.Kind, col store.Collection, op store.PIMOp) error {
	if op.ObjectID == nil {
		return p.done(ctx, op, nil)
	}
	obj, err := p.m.db.GetObject(ctx, *op.ObjectID)
	if errors.Is(err, store.ErrNotFound) {
		return p.done(ctx, op, nil)
	}
	if err != nil {
		return err
	}
	w, err := c.Put(ctx, op.Href, obj.Raw, kind, op.IfMatch)
	if err != nil && col.Kind == api.CollectionKindCalendar && refusal(err) {
		return p.refusedPut(ctx, c, col, op, err)
	}
	if err != nil {
		if payload, edit := ParseContactPayload(op.Payload); edit && kind == davx.AddressBooks && refusal(err) {
			if errors.Is(err, davx.ErrPrecondition) && !payload.Whole {
				return p.reapply(ctx, c, col, op, payload)
			}
			reason := "refused"
			if errors.Is(err, davx.ErrPrecondition) {
				reason = "changed"
			}
			return p.refusedContact(ctx, op, col, serverCopy(ctx, c, op), reason)
		}
		return p.failed(ctx, op, err)
	}
	moved := w.Href != op.Href
	there := false
	if moved {
		_, err := p.m.db.ObjectByHref(ctx, col.ID, w.Href)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		there = err == nil
	}
	var fetched *davx.Object
	gone := false
	if w.ETag == "" || there {
		got, err := c.Get(ctx, w.Href)
		switch {
		case errors.Is(err, davx.ErrNotFound):
			// The server took the change and has no object since: Outlook
			// removes an invitation the user declined.
			gone = true
		case err != nil:
			return p.failed(ctx, op, err)
		default:
			fetched = &got
		}
	}
	return p.done(ctx, op, func(tx *store.Tx) error {
		if gone {
			if _, err := tx.DeleteObjects(ctx, col.ID, []string{obj.Href}); err != nil {
				return err
			}
			if kind == davx.AddressBooks {
				return tx.RelinkPeople(ctx)
			}
			return tx.Emit(ctx, api.CalendarChanged{AccountID: p.acct.ID})
		}
		switch {
		case there:
			if _, err := tx.DeleteObjects(ctx, col.ID, []string{obj.Href}); err != nil {
				return err
			}
		case moved:
			if err := tx.SetObjectHref(ctx, obj.ID, w.Href); err != nil {
				return err
			}
		}
		if fetched != nil {
			if err := p.indexDAV(ctx, tx, col, *fetched); err != nil {
				return err
			}
			if kind == davx.AddressBooks {
				return tx.RelinkPeople(ctx)
			}
			return nil
		}
		return tx.SetObjectETag(ctx, obj.ID, w.ETag)
	})
}

// done removes a change the server took (or that no longer applies),
// with the store updates that go with it.
func (p *pass) done(ctx context.Context, op store.PIMOp, also func(*store.Tx) error) error {
	return p.m.db.Tx(ctx, func(tx *store.Tx) error {
		if also != nil {
			if err := also(tx); err != nil {
				return err
			}
		}
		return tx.DonePIMOp(ctx, op.ID)
	})
}

// retryAfter is the wait after each failed attempt; the change fails for
// good after the last.
func retryAfter(attempts int) time.Duration {
	waits := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour}
	if attempts >= len(waits) {
		return 0
	}
	return waits[attempts]
}

// failed records a failed write. The server's refusal of the change itself
// (412, or any 4xx) fails it for good, and the next full listing restores
// the server's version; other failures retry later. A refused credential,
// or a sign-in to do again, ends the pass.
func (p *pass) failed(ctx context.Context, op store.PIMOp, err error) error {
	if errors.Is(err, davx.ErrUnauthorized) || errors.Is(err, gtasks.ErrUnauthorized) ||
		errors.Is(err, errSignIn) || errors.Is(err, errScope) {
		return err
	}
	p.m.log.Warn("pim change not written", "account", p.acct.ID, "href", op.Href, "err", err)
	var next time.Time
	var ae *gtasks.APIError
	refused := refusal(err) || errors.As(err, &ae) && ae.Status/100 == 4
	if wait := retryAfter(op.Attempts); !refused && wait > 0 {
		next = p.m.db.Now().Add(wait)
	}
	return p.m.db.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.RetryPIMOp(ctx, op.ID, next, err.Error()); err != nil {
			return err
		}
		if next.IsZero() && strings.HasPrefix(op.Kind, "tasks.") {
			return refusedTask(ctx, tx, op)
		}
		if !next.IsZero() || op.Kind != "put" || op.IfMatch != "" {
			return nil
		}
		// A creation the server will not take: no server version to keep,
		// and an incremental sync would never remove the local one.
		if _, err := tx.DeleteObjects(ctx, op.CollectionID, []string{op.Href}); err != nil {
			return err
		}
		return tx.RelinkPeople(ctx)
	})
}

// refusedTask restores the server's version after Google refused a task
// change for good: a task it never had is removed; otherwise the list is
// listed in full at the next pass.
func refusedTask(ctx context.Context, tx *store.Tx, op store.PIMOp) error {
	if op.Kind == "tasks.insert" {
		_, err := tx.DeleteObjects(ctx, op.CollectionID, []string{op.Href})
		return err
	}
	return tx.SetCollectionSync(ctx, op.CollectionID, "", "")
}
