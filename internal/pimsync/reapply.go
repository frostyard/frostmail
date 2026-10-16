package pimsync

import (
	"context"
	"encoding/json/v2"
	"errors"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/graph"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/vcardx"
)

// reapplyAttempts bounds how often a contact edit is applied again to a
// server version that keeps changing under it.
const reapplyAttempts = 3

// restoresKept is how long a write's restore stays (ADR-0032).
const restoresKept = 24 * time.Hour

// reapply handles a contact edit the server refused with 412 (ADR-0030):
// when another client left the properties it touches alone, it is applied
// again to the server's version and written on that ETag; otherwise the
// server's version stands and the app hears so.
func (p *pass) reapply(ctx context.Context, c objectServer, col store.Collection, op store.PIMOp, payload ContactPayload) error {
	for range reapplyAttempts {
		server, err := c.Get(ctx, op.Href)
		if errors.Is(err, davx.ErrNotFound) {
			return p.refusedContact(ctx, op, col, nil, "deleted")
		}
		if err != nil {
			return p.failed(ctx, op, err)
		}
		if !payload.Fits(server.Data) {
			return p.refusedContact(ctx, op, col, &server, "changed")
		}
		raw, err := payload.Apply(server.Data, p.m.db.Now())
		if err != nil {
			return p.refusedContact(ctx, op, col, &server, "changed")
		}
		w, err := c.Put(ctx, op.Href, raw, davx.AddressBooks, server.ETag)
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
		return p.done(ctx, op, func(tx *store.Tx) error {
			if err := p.indexDAV(ctx, tx, col, written); err != nil {
				return err
			}
			return tx.RelinkPeople(ctx)
		})
	}
	return p.refusedContact(ctx, op, col, nil, "changed")
}

// refusedContact ends a contact edit the server will not take: the
// server's version (when there is one) replaces the local one, the
// object is removed when the server has none, and people.refused names
// the contact. With server nil and reason changed, the next full listing
// brings the server's version.
func (p *pass) refusedContact(ctx context.Context, op store.PIMOp, col store.Collection, server *davx.Object, reason string) error {
	name, id := "", op.ObjectID
	if id != nil {
		if obj, err := p.m.db.GetObject(ctx, *id); err == nil {
			if card, err := vcardx.Parse(obj.Raw); err == nil {
				name = card.DisplayName()
			}
		}
	}
	return p.done(ctx, op, func(tx *store.Tx) error {
		switch {
		case server != nil:
			if err := p.indexDAV(ctx, tx, col, *server); err != nil {
				return err
			}
		case reason == "deleted":
			if _, err := tx.DeleteObjects(ctx, col.ID, []string{op.Href}); err != nil {
				return err
			}
			id = nil
		default:
			if err := tx.SetCollectionSync(ctx, col.ID, "", ""); err != nil {
				return err
			}
		}
		if err := tx.RelinkPeople(ctx); err != nil {
			return err
		}
		return tx.Emit(ctx, api.PeopleRefused{AccountID: p.acct.ID, ContactID: id, Name: name, Reason: reason})
	})
}

// refusal reports whether the server refused a change itself (412, or
// another 4xx; or a source Outlook cannot store), so that trying again
// would not help.
func refusal(err error) bool {
	var se *davx.StatusError
	var ge *graph.StatusError
	return errors.Is(err, davx.ErrPrecondition) || errors.As(err, &se) && se.Code/100 == 4 ||
		errors.As(err, &ge) && ge.Status/100 == 4 || errors.Is(err, errUnstorable)
}

// refusedDelete ends a contact delete the server refused, most often
// because another client changed the contact meanwhile (412): the next
// full listing brings the server's version back, and people.refused names
// it.
func (p *pass) refusedDelete(ctx context.Context, op store.PIMOp, col store.Collection) error {
	var payload ContactPayload
	_ = json.Unmarshal([]byte(op.Payload), &payload)
	return p.done(ctx, op, func(tx *store.Tx) error {
		if err := tx.SetCollectionSync(ctx, col.ID, "", ""); err != nil {
			return err
		}
		return tx.Emit(ctx, api.PeopleRefused{AccountID: p.acct.ID, Name: payload.Name, Reason: "changed"})
	})
}

// dropOldRestores removes the restores older than a day.
func (p *pass) dropOldRestores(ctx context.Context) error {
	return p.m.db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.DropRestoresBefore(ctx, p.m.db.Now().Add(-restoresKept))
		return err
	})
}
