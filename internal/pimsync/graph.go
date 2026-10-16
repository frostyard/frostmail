package pimsync

import (
	"cmp"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/graph"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/store"
)

// Microsoft accounts' contacts and calendars over Graph (ADR-0040;
// docs/design/pim.md, Microsoft Graph). maild stores the iCalendar or
// vCard it derives from Graph's JSON, with the JSON beside it, so the
// index and everything after it read them as they read DAV sources.

// audienceTokens gives access tokens for an audience besides mail's;
// oauth.Manager does.
type audienceTokens interface {
	AccessTokenFor(ctx context.Context, accountID int64, audience string) (string, error)
}

// deltaMargin is how far the calendar delta's window reaches past the
// instance window, so it is rebased about once a month.
const deltaMargin = 31 * 24 * time.Hour

func (p *pass) graphClient() (*graph.Client, error) {
	tokens, ok := p.m.cfg.Tokens.(audienceTokens)
	if !ok || p.acct.Auth != api.AuthKindOAuth2 {
		return nil, fmt.Errorf("%w: maild has no Microsoft sign-in", errSignIn)
	}
	base := p.m.cfg.GraphURL
	if base == "" {
		base = graph.BaseURL
	}
	httpc := p.m.cfg.HTTP
	if httpc == nil {
		httpc = &http.Client{Timeout: time.Minute}
	}
	return &graph.Client{Base: base, HTTP: httpc, UserAgent: p.m.cfg.UserAgent,
		Token: func(ctx context.Context) (string, error) {
			tok, err := tokens.AccessTokenFor(ctx, p.acct.ID, oauth.AudienceGraph)
			switch {
			case errors.Is(err, oauth.ErrNoScope):
				return "", errScope
			case errors.Is(err, oauth.ErrReauth) || errors.Is(err, oauth.ErrNoClient):
				return "", fmt.Errorf("%w: %w", errSignIn, err)
			}
			return tok, err
		}}, nil
}

// syncGraph syncs one of a Graph account's services.
func (p *pass) syncGraph(ctx context.Context, service api.ServiceKind) error {
	c, err := p.graphClient()
	if err != nil {
		return err
	}
	switch service {
	case api.ServiceKindContacts:
		return p.syncGraphContacts(ctx, c)
	case api.ServiceKindCalendar:
		return p.syncGraphCalendars(ctx, c)
	}
	return nil // tasks: Microsoft To Do is Later
}

// graphToken is a Graph collection's sync token: its delta link, and for
// a calendar the window the delta covers.
type graphToken struct {
	Link string    `json:"link"`
	From time.Time `json:"from,omitzero"`
	To   time.Time `json:"to,omitzero"`
}

func parseGraphToken(s string) graphToken {
	var t graphToken
	if s != "" {
		_ = json.Unmarshal([]byte(s), &t)
	}
	return t
}

func (t graphToken) encode() string {
	b, _ := json.Marshal(t)
	return string(b)
}

func (p *pass) syncGraphCalendars(ctx context.Context, c *graph.Client) error {
	cals, err := c.Calendars(ctx)
	if err != nil {
		return fmt.Errorf("list graph calendars: %w", err)
	}
	list := make([]store.RemoteCollection, 0, len(cals))
	defaultHref := ""
	for _, cal := range cals {
		// Read-only until Graph's writes (plan 0010, Phase 3b).
		list = append(list, store.RemoteCollection{Href: cal.ID, Name: collectionName(cal.Name, api.CollectionKindCalendar),
			Color: calendarColor(cal), Components: []string{"VEVENT"}, ReadOnly: true})
		if cal.IsDefaultCalendar {
			defaultHref = cal.ID
		}
	}
	var cols []store.Collection
	err = p.m.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		if cols, err = tx.ReplaceCollections(ctx, p.acct.ID, api.CollectionKindCalendar, list); err != nil {
			return err
		}
		if defaultHref != "" {
			_, err = tx.UseServerDefault(ctx, p.acct.ID, api.CollectionKindCalendar, defaultHref)
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("replace graph calendars: %w", err)
	}
	var first error
	for _, col := range cols {
		if !col.Enabled {
			continue
		}
		if err := p.syncGraphCalendar(ctx, c, col); err != nil {
			err = fmt.Errorf("sync graph calendar %q: %w", col.Name, err)
			if errors.Is(err, graph.ErrUnauthorized) || errors.Is(err, errScope) || errors.Is(err, errSignIn) {
				return err
			}
			first = cmp.Or(first, err)
		}
	}
	return first
}

// graphColors are the hex values of Outlook's named calendar colors, for
// calendars without hexColor.
var graphColors = map[string]string{
	"lightBlue": "#a6d1f5", "lightGreen": "#87d28e", "lightOrange": "#fcab73", "lightGray": "#c0c0c0",
	"lightYellow": "#f4d07a", "lightTeal": "#8ad9d7", "lightPink": "#f88ca9", "lightBrown": "#cba287",
	"lightRed": "#f88c8c", "maxColor": "",
}

func calendarColor(cal graph.Calendar) string {
	if cal.HexColor != "" {
		return cal.HexColor
	}
	return graphColors[cal.Color]
}

func (p *pass) syncGraphCalendar(ctx context.Context, c *graph.Client, col store.Collection) error {
	tok := parseGraphToken(col.SyncToken)
	full := tok.Link == "" || p.from.Before(tok.From) || p.to.After(tok.To)
	if full {
		tok = graphToken{From: p.from.Add(-deltaMargin), To: p.to.Add(deltaMargin)}
		tok.Link = graph.CalendarDelta(col.Href, tok.From, tok.To)
	}
	items, next, err := c.Delta(ctx, tok.Link)
	if errors.Is(err, graph.ErrDeltaExpired) && !full {
		full = true
		tok = graphToken{From: p.from.Add(-deltaMargin), To: p.to.Add(deltaMargin)}
		tok.Link = graph.CalendarDelta(col.Href, tok.From, tok.To)
		items, next, err = c.Delta(ctx, tok.Link)
	}
	if err != nil {
		return fmt.Errorf("calendar delta: %w", err)
	}
	pending, err := p.m.db.PendingHrefs(ctx, col.ID)
	if err != nil {
		return err
	}
	stored, err := p.m.db.ObjectETags(ctx, col.ID)
	if err != nil {
		return err
	}
	var singles []graphItem
	var series []string // masters to read again, in order
	marked := map[string]bool{}
	mark := func(id string) {
		if id != "" && !marked[id] {
			marked[id] = true
			series = append(series, id)
		}
	}
	var removed []string
	named := map[string]bool{}
	for _, raw := range items {
		var e graph.Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return fmt.Errorf("calendar delta item: %w", err)
		}
		switch {
		case e.Removed != nil:
			removed = append(removed, e.ID)
		case e.Type == "seriesMaster":
			mark(e.ID)
		case e.Type == "exception" || e.Type == "occurrence":
			mark(e.SeriesMasterID)
		default:
			singles = append(singles, graphItem{id: e.ID, raw: raw, event: e})
		}
	}
	// A removed exception or occurrence changes its series; a removed
	// event or master goes.
	var gone []string
	if len(removed) > 0 {
		owners, err := p.graphExceptionOwners(ctx, col)
		if err != nil {
			return err
		}
		for _, id := range removed {
			if master, ok := owners[id]; ok {
				mark(master)
				continue
			}
			if _, ok := stored[id]; ok && !marked[id] {
				gone = append(gone, id)
			}
		}
	}
	for _, it := range singles {
		named[it.id] = true
	}
	var fetched []graphItem
	for _, id := range series {
		named[id] = true
		if pending[id] {
			continue
		}
		raw, err := c.Series(ctx, id)
		if errors.Is(err, graph.ErrNotFound) {
			gone = append(gone, id)
			continue
		}
		if err != nil {
			return fmt.Errorf("read series %s: %w", id, err)
		}
		var e graph.Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return fmt.Errorf("series %s: %w", id, err)
		}
		fetched = append(fetched, graphItem{id: id, raw: raw, event: e})
	}
	if full {
		for href := range stored {
			if !named[href] && !pending[href] && !slices.Contains(gone, href) {
				gone = append(gone, href)
			}
		}
	}
	err = p.m.db.Tx(ctx, func(tx *store.Tx) error {
		changed := false
		for _, it := range append(singles, fetched...) {
			if pending[it.id] {
				continue
			}
			src, err := graph.EventSource(it.event, it.event.ExceptionOccurrences, p.acct.Email)
			o := store.Object{CollectionID: col.ID, Href: it.id, ETag: it.event.ETag, Raw: src, Remote: it.raw}
			if err != nil {
				// Kept, with the reason, so verify and a later fix can see it.
				o.Raw, o.ParseError = []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"), err.Error()
			}
			if err := p.indexObject(ctx, tx, col, o); err != nil {
				return err
			}
			changed = true
		}
		n, err := tx.DeleteObjects(ctx, col.ID, slices.DeleteFunc(gone, func(h string) bool { return pending[h] }))
		if err != nil {
			return err
		}
		if changed || n > 0 {
			if err := tx.Emit(ctx, api.CalendarChanged{AccountID: p.acct.ID}); err != nil {
				return err
			}
		}
		tok.Link = next
		return tx.SetCollectionSync(ctx, col.ID, tok.encode(), "")
	})
	if err != nil {
		return fmt.Errorf("store graph events: %w", err)
	}
	return nil
}

// verifyGraph compares a Graph service's collections with Graph, changing
// nothing: a calendar over its delta's window (its events and series,
// without ETags for series), a contact folder whole. Collections a pass
// has not synced yet are left out.
func (p *pass) verifyGraph(ctx context.Context, service api.ServiceKind) ([]api.CollectionCheck, error) {
	kind := api.CollectionKindAddressbook
	if service == api.ServiceKindCalendar {
		kind = api.CollectionKindCalendar
	} else if service != api.ServiceKindContacts {
		return nil, nil
	}
	c, err := p.graphClient()
	if err != nil {
		return nil, err
	}
	cols, err := p.m.db.Collections(ctx, store.CollectionFilter{AccountID: p.acct.ID, Kind: kind, EnabledOnly: true})
	if err != nil {
		return nil, err
	}
	var checks []api.CollectionCheck
	for _, col := range cols {
		tok := parseGraphToken(col.SyncToken)
		if tok.Link == "" {
			continue
		}
		var server []davx.Change
		if kind == api.CollectionKindCalendar {
			events, err := c.CalendarView(ctx, col.Href, tok.From, tok.To)
			if err != nil {
				return nil, fmt.Errorf("verify graph calendar %q: %w", col.Name, err)
			}
			seen := map[string]bool{}
			for _, e := range events {
				href, etag := e.ID, e.ETag
				if e.SeriesMasterID != "" {
					href, etag = e.SeriesMasterID, "" // a series' own change key is its master's
				}
				if !seen[href] {
					seen[href] = true
					server = append(server, davx.Change{Href: href, ETag: etag})
				}
			}
		} else {
			contacts, err := c.Contacts(ctx, col.Href)
			if err != nil {
				return nil, fmt.Errorf("verify graph contacts %q: %w", col.Name, err)
			}
			for _, gc := range contacts {
				server = append(server, davx.Change{Href: gc.ID, ETag: gc.ETag})
			}
		}
		local, err := p.m.db.ObjectETags(ctx, col.ID)
		if err != nil {
			return nil, err
		}
		check, err := p.m.compareCollection(ctx, col, server, local)
		if err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}
	return checks, nil
}

// graphItem is one event or contact Graph sent, with its JSON.
type graphItem struct {
	id    string
	raw   jsontext.Value
	event graph.Event
}

// graphExceptionOwners maps the exceptions of a calendar's stored series
// to their masters' hrefs.
func (p *pass) graphExceptionOwners(ctx context.Context, col store.Collection) (map[string]string, error) {
	remotes, err := p.m.db.ObjectRemotes(ctx, col.ID)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for href, raw := range remotes {
		var e graph.Event
		if json.Unmarshal(raw, &e) != nil {
			continue
		}
		for _, ex := range e.ExceptionOccurrences {
			out[ex.ID] = href
		}
	}
	return out, nil
}

func (p *pass) syncGraphContacts(ctx context.Context, c *graph.Client) error {
	folders, err := c.ContactFolders(ctx)
	if err != nil {
		return fmt.Errorf("list graph contact folders: %w", err)
	}
	// Read-only until Graph's writes (plan 0010, Phase 3b).
	list := []store.RemoteCollection{{Href: graph.DefaultFolder, Name: "Contacts", ReadOnly: true}}
	for _, f := range folders {
		list = append(list, store.RemoteCollection{Href: f.ID, Name: collectionName(f.DisplayName, api.CollectionKindAddressbook), ReadOnly: true})
	}
	var cols []store.Collection
	err = p.m.db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		cols, err = tx.ReplaceCollections(ctx, p.acct.ID, api.CollectionKindAddressbook, list)
		if err != nil {
			return err
		}
		return tx.RelinkPeople(ctx)
	})
	if err != nil {
		return fmt.Errorf("replace graph contact folders: %w", err)
	}
	var first error
	for _, col := range cols {
		if !col.Enabled {
			continue
		}
		if err := p.syncGraphFolder(ctx, c, col); err != nil {
			err = fmt.Errorf("sync graph contacts %q: %w", col.Name, err)
			if errors.Is(err, graph.ErrUnauthorized) || errors.Is(err, errScope) || errors.Is(err, errSignIn) {
				return err
			}
			first = cmp.Or(first, err)
		}
	}
	return first
}

func (p *pass) syncGraphFolder(ctx context.Context, c *graph.Client, col store.Collection) error {
	tok := parseGraphToken(col.SyncToken)
	full := tok.Link == ""
	if full {
		tok.Link = graph.ContactsDelta(col.Href)
	}
	items, next, err := c.Delta(ctx, tok.Link)
	if errors.Is(err, graph.ErrDeltaExpired) && !full {
		full, tok.Link = true, graph.ContactsDelta(col.Href)
		items, next, err = c.Delta(ctx, tok.Link)
	}
	if err != nil {
		return fmt.Errorf("contacts delta: %w", err)
	}
	pending, err := p.m.db.PendingHrefs(ctx, col.ID)
	if err != nil {
		return err
	}
	stored, err := p.m.db.ObjectETags(ctx, col.ID)
	if err != nil {
		return err
	}
	type contact struct {
		raw     jsontext.Value
		c       graph.Contact
		photo   []byte
		removed bool
	}
	var got []contact
	named := map[string]bool{}
	for _, raw := range items {
		var gc graph.Contact
		if err := json.Unmarshal(raw, &gc); err != nil {
			return fmt.Errorf("contacts delta item: %w", err)
		}
		named[gc.ID] = true
		if pending[gc.ID] {
			continue
		}
		if gc.Removed != nil {
			got = append(got, contact{c: gc, removed: true})
			continue
		}
		photo, err := c.Photo(ctx, gc.ID)
		if err != nil {
			return fmt.Errorf("contact photo: %w", err)
		}
		got = append(got, contact{raw: raw, c: gc, photo: photo})
	}
	var gone []string
	if full {
		for href := range stored {
			if !named[href] && !pending[href] {
				gone = append(gone, href)
			}
		}
	}
	err = p.m.db.Tx(ctx, func(tx *store.Tx) error {
		for _, it := range got {
			if it.removed {
				gone = append(gone, it.c.ID)
				continue
			}
			o := store.Object{CollectionID: col.ID, Href: it.c.ID, ETag: it.c.ETag,
				Raw: graph.ContactSource(it.c, it.photo, ""), Remote: it.raw}
			if err := p.indexObject(ctx, tx, col, o); err != nil {
				return err
			}
		}
		n, err := tx.DeleteObjects(ctx, col.ID, gone)
		if err != nil {
			return err
		}
		if len(got) > 0 || n > 0 {
			if err := tx.Emit(ctx, api.PeopleChanged{AccountID: p.acct.ID}); err != nil {
				return err
			}
		}
		if err := tx.RelinkPeople(ctx); err != nil {
			return err
		}
		tok.Link = next
		return tx.SetCollectionSync(ctx, col.ID, tok.encode(), "")
	})
	if err != nil {
		return fmt.Errorf("store graph contacts: %w", err)
	}
	return nil
}
