package pimsync

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/store"
)

// syncDAV reconciles accepted collections and syncs enabled ones in order,
// continuing past collection failures unless the server refuses credentials.
func (p *pass) syncDAV(ctx context.Context, c *davx.Client, kind davx.Kind, collectionKind api.CollectionKind, want func(davx.Collection) bool) error {
	remote, err := c.Collections(ctx, kind)
	if err != nil {
		return fmt.Errorf("list dav collections: %w", err)
	}
	byHref := map[string]davx.Collection{}
	var list []store.RemoteCollection
	for _, col := range remote {
		if !want(col) {
			continue
		}
		byHref[col.Href] = col
		list = append(list, store.RemoteCollection{Href: col.Href, Name: col.Name,
			Description: col.Description, Color: col.Color, Components: col.Components, ReadOnly: col.ReadOnly})
	}
	var cols []store.Collection
	err = p.m.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		cols, err = tx.ReplaceCollections(ctx, p.acct.ID, collectionKind, list)
		if err != nil {
			return err
		}
		return relinkDAVPeople(ctx, tx, kind)
	})
	if err != nil {
		return fmt.Errorf("replace dav collections: %w", err)
	}
	var first error
	for _, col := range cols {
		server := byHref[col.Href]
		if !col.Enabled || server.CTag != "" && server.CTag == col.CTag && col.SyncedAt != nil {
			continue
		}
		if err := p.syncCollection(ctx, c, kind, col, server); err != nil {
			err = fmt.Errorf("sync dav collection %q: %w", col.Href, err)
			if errors.Is(err, davx.ErrUnauthorized) {
				return err
			}
			if first == nil {
				first = err
			}
		}
	}
	return first
}

func (p *pass) syncCollection(ctx context.Context, c *davx.Client, kind davx.Kind, col store.Collection, remote davx.Collection) error {
	etags, err := p.m.db.ObjectETags(ctx, col.ID)
	if err != nil {
		return fmt.Errorf("read object etags: %w", err)
	}
	pending, err := p.m.db.PendingHrefs(ctx, col.ID)
	if err != nil {
		return fmt.Errorf("read pending hrefs: %w", err)
	}
	delta, full, err := listDAV(ctx, c, col, remote.Sync)
	if err != nil {
		return fmt.Errorf("list dav changes: %w", err)
	}
	fetch, deleted := davHrefs(delta, full, etags, pending)
	if err := p.fetchDAV(ctx, c, kind, col, fetch); err != nil {
		return err
	}
	if err := p.deleteDAV(ctx, kind, col, deleted); err != nil {
		return err
	}
	return p.m.db.Tx(ctx, func(tx *store.Tx) error {
		return tx.SetCollectionSync(ctx, col.ID, delta.Token, remote.CTag)
	})
}

// listDAV accumulates all pages, discarding a stale token's partial results.
func listDAV(ctx context.Context, c *davx.Client, col store.Collection, sync bool) (davx.Delta, bool, error) {
	if !sync {
		changed, err := c.List(ctx, col.Href)
		return davx.Delta{Changed: changed}, true, err
	}
	token := col.SyncToken
	full := token == ""
	var out davx.Delta
	for {
		delta, err := c.Sync(ctx, col.Href, token)
		if errors.Is(err, davx.ErrInvalidToken) && token != "" {
			token, full, out = "", true, davx.Delta{}
			continue
		}
		if token == "" && refusedSync(err) {
			// Google's CardDAV lists sync-collection among its reports yet
			// refuses one without a token: list the collection instead,
			// every pass, as for a server without sync-collection.
			changed, err := c.List(ctx, col.Href)
			return davx.Delta{Changed: changed}, true, err
		}
		if err != nil {
			return davx.Delta{}, false, err
		}
		out.Changed = append(out.Changed, delta.Changed...)
		out.Deleted = append(out.Deleted, delta.Deleted...)
		out.Token = delta.Token
		if !delta.More {
			return out, full, nil
		}
		if delta.Token == token {
			return davx.Delta{}, false, fmt.Errorf("dav sync pagination did not advance")
		}
		token = delta.Token
	}
}

// refusedSync reports whether a first sync-collection was refused as a
// request the server does not take (400, 405, 501), rather than for the
// credentials, a missing collection or a server failure.
func refusedSync(err error) bool {
	var status *davx.StatusError
	if !errors.As(err, &status) {
		return false
	}
	switch status.Code {
	case http.StatusBadRequest, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true
	}
	return false
}

func davHrefs(delta davx.Delta, full bool, etags map[string]string, pending map[string]bool) (fetch, deleted []string) {
	named := map[string]bool{}
	fetched := map[string]bool{}
	removed := map[string]bool{}
	for _, change := range delta.Changed {
		named[change.Href] = true
		old, exists := etags[change.Href]
		if !pending[change.Href] && !fetched[change.Href] && (!exists || change.ETag == "" || old != change.ETag) {
			fetch = append(fetch, change.Href)
			fetched[change.Href] = true
		}
	}
	for _, href := range delta.Deleted {
		if !pending[href] && !removed[href] {
			deleted = append(deleted, href)
			removed[href] = true
		}
	}
	if full {
		for href := range etags {
			if !named[href] && !pending[href] && !removed[href] {
				deleted = append(deleted, href)
			}
		}
	}
	return fetch, deleted
}

func (p *pass) fetchDAV(ctx context.Context, c *davx.Client, kind davx.Kind, col store.Collection, hrefs []string) error {
	for start := 0; start < len(hrefs); start += p.m.cfg.Batch {
		batch := hrefs[start:min(start+p.m.cfg.Batch, len(hrefs))]
		objects, missing, err := c.Multiget(ctx, kind, col.Href, batch)
		if err != nil {
			return fmt.Errorf("fetch dav objects: %w", err)
		}
		err = p.m.db.Tx(ctx, func(tx *store.Tx) error {
			for _, o := range objects {
				if err := p.indexDAV(ctx, tx, col, o); err != nil {
					return err
				}
			}
			n, err := tx.DeleteObjects(ctx, col.ID, missing)
			if err != nil {
				return err
			}
			if len(objects) > 0 || n > 0 {
				if err := p.emitDAV(ctx, tx, col.Kind); err != nil {
					return err
				}
			}
			return relinkDAVPeople(ctx, tx, kind)
		})
		if err != nil {
			return fmt.Errorf("store dav batch: %w", err)
		}
	}
	return nil
}

func (p *pass) deleteDAV(ctx context.Context, kind davx.Kind, col store.Collection, hrefs []string) error {
	if len(hrefs) == 0 {
		return nil
	}
	err := p.m.db.Tx(ctx, func(tx *store.Tx) error {
		n, err := tx.DeleteObjects(ctx, col.ID, hrefs)
		if err != nil {
			return err
		}
		if n > 0 {
			if err := p.emitDAV(ctx, tx, col.Kind); err != nil {
				return err
			}
		}
		return relinkDAVPeople(ctx, tx, kind)
	})
	if err != nil {
		return fmt.Errorf("delete dav objects: %w", err)
	}
	return nil
}

func (p *pass) emitDAV(ctx context.Context, tx *store.Tx, kind api.CollectionKind) error {
	switch kind {
	case api.CollectionKindAddressbook:
		return tx.Emit(ctx, api.PeopleChanged{AccountID: p.acct.ID})
	case api.CollectionKindTasklist:
		return tx.Emit(ctx, api.TasksChanged{AccountID: p.acct.ID})
	default:
		return tx.Emit(ctx, api.CalendarChanged{AccountID: p.acct.ID})
	}
}

// relinkDAVPeople keeps the people index in step only for address books.
func relinkDAVPeople(ctx context.Context, tx *store.Tx, kind davx.Kind) error {
	if kind == davx.AddressBooks {
		return tx.RelinkPeople(ctx)
	}
	return nil
}
