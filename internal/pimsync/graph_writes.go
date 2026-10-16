package pimsync

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/graph"
	"github.com/frostyard/frostmail/internal/store"
)

// graphObjects is a Graph collection as the replay's objectServer
// (ADR-0040; docs/design/pim.md, Microsoft Graph): it gives the sources
// maild derives from Graph's JSON, keeping the JSON for indexDAV, and
// takes edited sources, writing only the fields that differ from the
// version the edit was made on. It writes no ETag back, so the replay
// reads each object again after writing it, and the JSON stays the
// server's.
type graphObjects struct {
	p   *pass
	c   *graph.Client
	col store.Collection
	op  store.PIMOp // the change being written
}

func (g graphObjects) contacts() bool { return g.col.Kind == api.CollectionKindAddressbook }

// keepRemote holds an object's JSON for indexDAV.
func (p *pass) keepRemote(href string, raw []byte) {
	if p.remotes == nil {
		p.remotes = map[string][]byte{}
	}
	p.remotes[href] = raw
}

// Get reads a series or an event with its exceptions, or a contact with
// its photo.
func (g graphObjects) Get(ctx context.Context, href string) (davx.Object, error) {
	if g.contacts() {
		raw, err := g.c.Contact(ctx, href)
		if err != nil {
			return davx.Object{}, davError(err)
		}
		var gc graph.Contact
		if err := json.Unmarshal(raw, &gc); err != nil {
			return davx.Object{}, fmt.Errorf("graph contact %s: %w", href, err)
		}
		photo, err := g.c.Photo(ctx, href)
		if err != nil {
			return davx.Object{}, davError(err)
		}
		g.p.keepRemote(href, raw)
		return davx.Object{Href: href, ETag: gc.ETag, Data: graph.ContactSource(gc, photo, "")}, nil
	}
	raw, err := g.c.Series(ctx, href)
	if err != nil {
		return davx.Object{}, davError(err)
	}
	var e graph.Event
	if err := json.Unmarshal(raw, &e); err != nil {
		return davx.Object{}, fmt.Errorf("graph event %s: %w", href, err)
	}
	src, err := graph.EventSource(e, e.ExceptionOccurrences, g.p.acct.Email)
	if err != nil {
		return davx.Object{}, fmt.Errorf("graph event %s: %w", href, err)
	}
	g.p.keepRemote(href, raw)
	return davx.Object{Href: href, ETag: e.ETag, Data: src}, nil
}

// base is the JSON of the version an edit was made on, whose ETag is
// ifMatch: one read in this pass, the stored one, or Graph's own, which
// must still be that version.
func (g graphObjects) base(ctx context.Context, href, ifMatch string) ([]byte, error) {
	if raw, ok := g.p.remotes[href]; ok && remoteETag(raw) == ifMatch {
		return raw, nil
	}
	obj, err := g.p.m.db.ObjectByHref(ctx, g.col.ID, href)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if err == nil && obj.ETag == ifMatch && len(obj.Remote) > 0 {
		return obj.Remote, nil
	}
	server, err := g.Get(ctx, href)
	if err != nil {
		return nil, err
	}
	if server.ETag != ifMatch {
		return nil, fmt.Errorf("%w: %s changed in Outlook", davx.ErrPrecondition, href)
	}
	return g.p.remotes[href], nil
}

func remoteETag(raw []byte) string {
	var v struct {
		ETag string `json:"@odata.etag"`
	}
	_ = json.Unmarshal(raw, &v)
	return v.ETag
}

// Put creates an object when ifMatch is "", else writes what changed.
func (g graphObjects) Put(ctx context.Context, href string, data []byte, _ davx.Kind, ifMatch string) (davx.Written, error) {
	if g.contacts() {
		return g.putContact(ctx, href, data, ifMatch)
	}
	return g.putEvent(ctx, href, data, ifMatch)
}

// Delete deletes an event, a series or a contact, which Outlook moves to
// Deleted Items.
func (g graphObjects) Delete(ctx context.Context, href, ifMatch string) error {
	if g.contacts() {
		return davError(g.c.DeleteContact(ctx, href, ifMatch))
	}
	return davError(g.c.DeleteEvent(ctx, href, ifMatch))
}

func (g graphObjects) project(src []byte) (graph.EventProjection, error) {
	return graph.ProjectEvent(src, g.p.userEmails, g.p.m.cfg.Local)
}

func (g graphObjects) putEvent(ctx context.Context, href string, data []byte, ifMatch string) (davx.Written, error) {
	next, err := g.project(data)
	if err != nil {
		return davx.Written{}, fmt.Errorf("%w: %w", errUnstorable, err)
	}
	if ifMatch == "" {
		return g.createEvent(ctx, next)
	}
	raw, err := g.base(ctx, href, ifMatch)
	if err != nil {
		return davx.Written{}, err
	}
	var remote graph.Event
	if err := json.Unmarshal(raw, &remote); err != nil {
		return davx.Written{}, fmt.Errorf("graph event %s: %w", href, err)
	}
	src, err := graph.EventSource(remote, remote.ExceptionOccurrences, g.p.acct.Email)
	if err != nil {
		return davx.Written{}, fmt.Errorf("graph event %s: %w", href, err)
	}
	base, err := g.project(src)
	if err != nil {
		return davx.Written{}, fmt.Errorf("graph event %s: %w", href, err)
	}
	if err := g.updateEvent(ctx, href, remote, base, next, ifMatch); err != nil {
		return davx.Written{}, err
	}
	return davx.Written{Href: href}, nil
}

// createEvent creates the user's own event or series, its changed and
// deleted occurrences after it. An invitation's copy (an answer to one
// the calendar did not have yet) is not created: Outlook put the
// invitation in the calendar when it came, and the answer goes to that.
func (g graphObjects) createEvent(ctx context.Context, next graph.EventProjection) (davx.Written, error) {
	if !next.Organizer {
		return g.answerInvitation(ctx, next)
	}
	fields := maps.Clone(next.Master.Fields)
	// Graph creates one event for one transactionId, so a create sent
	// again after a lost answer makes no second one; another change (a
	// second restore of the event) has its own.
	fields["transactionId"] = next.UID + "." + strconv.FormatInt(g.op.ID, 10)
	created, err := g.c.CreateEvent(ctx, g.col.Href, fields)
	if err != nil {
		return davx.Written{}, davError(err)
	}
	if len(next.Overrides) > 0 || len(next.Cancelled) > 0 {
		base := next
		base.Overrides, base.Cancelled = nil, nil
		// The series is there: its occurrences that fail to change come
		// back as the series has them, and the creation is not sent again.
		if err := g.occurrences(ctx, created, base, next); err != nil {
			g.p.m.log.Warn("graph occurrences not written", "account", g.p.acct.ID, "href", created.ID, "err", err)
		}
	}
	return davx.Written{Href: created.ID}, nil
}

// answerInvitation answers the calendar's copy of an invitation, by its
// UID (as Outlook keeps an outside one, inside its own), and the stored
// copy moves there.
func (g graphObjects) answerInvitation(ctx context.Context, next graph.EventProjection) (davx.Written, error) {
	found, err := g.c.EventsByUID(ctx, g.col.Href, next.UID)
	if err == nil && len(found) == 0 {
		// An invitation from outside has its UID inside Outlook's.
		found, err = g.c.EventsByUID(ctx, g.col.Href, graph.OutlookUID(next.UID))
	}
	if err != nil {
		return davx.Written{}, davError(err)
	}
	if len(found) == 0 {
		return davx.Written{}, fmt.Errorf("%w: Outlook has no copy of this invitation", errUnstorable)
	}
	e := found[0]
	if answerable(next.Master.Answer) && answerOf(e.ResponseStatus.Response) != next.Master.Answer {
		if err := g.c.Answer(ctx, e.ID, next.Master.Answer); err != nil {
			return davx.Written{}, davError(err)
		}
	}
	return davx.Written{Href: e.ID}, nil
}

// answerOf is Graph's responseStatus as a PARTSTAT.
func answerOf(response string) string {
	switch response {
	case "accepted", "organizer":
		return "accepted"
	case "tentativelyAccepted":
		return "tentative"
	case "declined":
		return "declined"
	}
	return "needsaction"
}

func answerable(partstat string) bool {
	return partstat == "accepted" || partstat == "tentative" || partstat == "declined"
}

// updateEvent writes an event's or a series' changes: the master's fields
// on its change key, the user's answer, then its occurrences. A write
// that cannot carry the change key checks it first.
func (g graphObjects) updateEvent(ctx context.Context, id string, remote graph.Event, base, next graph.EventProjection, ifMatch string) error {
	for _, key := range base.Cancelled {
		if !slices.Contains(next.Cancelled, key) && next.Generates(key) {
			// An occurrence deleted in Outlook, back here (an undo): Graph
			// cannot cancel a cancellation.
			return fmt.Errorf("%w: Outlook cannot bring back a deleted occurrence", errUnstorable)
		}
	}
	fields := graph.Diff(base.Master.Fields, next.Master.Fields)
	answer := !next.Organizer && answerable(next.Master.Answer) && next.Master.Answer != base.Master.Answer
	if len(fields) == 0 && (answer || occurrencesChanged(base, next)) {
		etag, err := g.c.ChangeKey(ctx, id)
		if err != nil {
			return davError(err)
		}
		if etag != ifMatch {
			return fmt.Errorf("%w: %s changed in Outlook", davx.ErrPrecondition, id)
		}
	}
	if len(fields) > 0 {
		if err := g.c.UpdateEvent(ctx, id, fields, ifMatch); err != nil {
			return davError(err)
		}
	}
	if answer {
		if err := g.c.Answer(ctx, id, next.Master.Answer); err != nil {
			return davError(err)
		}
	}
	if reshaped(fields) && (len(next.Overrides) > 0 || len(next.Cancelled) > 0 || len(base.Overrides) > 0) {
		// Outlook may drop a series' exceptions with its pattern or time:
		// its occurrences are compared with the series as it is now.
		raw, err := g.c.Series(ctx, id)
		if err != nil {
			return davError(err)
		}
		remote = graph.Event{}
		if err := json.Unmarshal(raw, &remote); err != nil {
			return fmt.Errorf("graph event %s: %w", id, err)
		}
		src, err := graph.EventSource(remote, remote.ExceptionOccurrences, g.p.acct.Email)
		if err != nil {
			return fmt.Errorf("graph event %s: %w", id, err)
		}
		if base, err = g.project(src); err != nil {
			return fmt.Errorf("graph event %s: %w", id, err)
		}
	}
	return g.occurrences(ctx, remote, base, next)
}

// reshaped reports whether a master's changes move its occurrences.
func reshaped(fields map[string]any) bool {
	for _, k := range []string{"recurrence", "start", "end", "isAllDay"} {
		if _, ok := fields[k]; ok {
			return true
		}
	}
	return false
}

func occurrencesChanged(base, next graph.EventProjection) bool {
	for key, part := range next.Overrides {
		was, ok := base.Overrides[key]
		if !ok || len(graph.Diff(was.Fields, part.Fields)) > 0 || was.Answer != part.Answer {
			return true
		}
	}
	for key := range base.Overrides {
		if _, ok := next.Overrides[key]; !ok && next.Generates(key) {
			return true
		}
	}
	for _, key := range next.Cancelled {
		if !slices.Contains(base.Cancelled, key) {
			return true
		}
	}
	return false
}

// occurrences writes a series' changed occurrences and deletes its
// cancelled ones. A new override is compared with the occurrence the
// series makes. Graph cannot undo an exception, so an override undone
// here (This Event's undo) is written back to that occurrence; one the
// series no longer makes (past a split's end) is left to Outlook.
func (g graphObjects) occurrences(ctx context.Context, master graph.Event, base, next graph.EventProjection) error {
	if len(next.Overrides) == 0 && len(next.Cancelled) == 0 && len(base.Overrides) == 0 {
		return nil
	}
	series, err := graph.SeriesOf(master)
	if err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(base.Overrides)) {
		id, ok := series.Exception(key)
		if _, kept := next.Overrides[key]; kept || !ok || !next.Generates(key) {
			continue
		}
		fields, err := next.Occurrence(key)
		if err != nil {
			return err
		}
		if fields = graph.Diff(base.Overrides[key].Fields, fields); len(fields) > 0 {
			if err := g.c.UpdateEvent(ctx, id, fields, ""); err != nil {
				return davError(err)
			}
		}
	}
	for _, key := range slices.Sorted(maps.Keys(next.Overrides)) {
		part := next.Overrides[key]
		was, ok := base.Overrides[key]
		if !ok {
			fields, err := next.Occurrence(key)
			if err != nil {
				return err
			}
			was = graph.EventPart{Fields: fields, Answer: next.Master.Answer}
		}
		fields := graph.Diff(was.Fields, part.Fields)
		answer := !next.Organizer && answerable(part.Answer) && part.Answer != was.Answer
		if len(fields) == 0 && !answer {
			continue
		}
		id, err := g.occurrenceID(ctx, master.ID, series, key)
		if errors.Is(err, davx.ErrNotFound) {
			g.p.m.log.Warn("graph occurrence not found", "account", g.p.acct.ID, "href", master.ID, "occurrence", key)
			continue
		}
		if err != nil {
			return err
		}
		if len(fields) > 0 {
			if err := g.c.UpdateEvent(ctx, id, fields, ""); err != nil {
				return davError(err)
			}
		}
		if answer {
			if err := g.c.Answer(ctx, id, part.Answer); err != nil {
				return davError(err)
			}
		}
	}
	for _, key := range next.Cancelled {
		if slices.Contains(base.Cancelled, key) {
			continue
		}
		id, err := g.occurrenceID(ctx, master.ID, series, key)
		if errors.Is(err, davx.ErrNotFound) {
			continue // not there to delete
		}
		if err != nil {
			return err
		}
		if err := g.c.DeleteEvent(ctx, id, ""); err != nil && !errors.Is(err, graph.ErrNotFound) {
			return davError(err)
		}
	}
	return nil
}

// occurrenceID finds the ID of the occurrence a key names: an
// exception's, else that of the occurrence Graph lists around its start.
func (g graphObjects) occurrenceID(ctx context.Context, masterID string, series graph.Series, key string) (string, error) {
	if id, ok := series.Exception(key); ok {
		return id, nil
	}
	at, err := graph.KeyTime(key)
	if err != nil {
		return "", err
	}
	found, err := g.c.Instances(ctx, masterID, at.Add(-48*time.Hour), at.Add(48*time.Hour))
	if err != nil {
		return "", davError(err)
	}
	for _, o := range found {
		if k, err := series.Key(o); err == nil && k == key && o.Type == "occurrence" {
			return o.ID, nil
		}
	}
	return "", fmt.Errorf("%w: occurrence %s of %s", davx.ErrNotFound, key, masterID)
}

func (g graphObjects) putContact(ctx context.Context, href string, data []byte, ifMatch string) (davx.Written, error) {
	next, err := graph.ProjectContact(data)
	if err != nil {
		return davx.Written{}, fmt.Errorf("%w: %w", errUnstorable, err)
	}
	if ifMatch == "" {
		created, err := g.c.CreateContact(ctx, g.col.Href, next.Fields)
		if err != nil {
			return davx.Written{}, davError(err)
		}
		if len(next.Photo) > 0 {
			// The contact is there: a photo that fails is left out, and the
			// creation is not sent again.
			if err := g.c.SetPhoto(ctx, created.ID, next.Photo, next.PhotoType); err != nil {
				g.p.m.log.Warn("graph contact photo not written", "account", g.p.acct.ID, "href", created.ID, "err", err)
			}
		}
		return davx.Written{Href: created.ID}, nil
	}
	raw, err := g.base(ctx, href, ifMatch)
	if err != nil {
		return davx.Written{}, err
	}
	var remote graph.Contact
	if err := json.Unmarshal(raw, &remote); err != nil {
		return davx.Written{}, fmt.Errorf("graph contact %s: %w", href, err)
	}
	base, err := graph.ProjectContact(graph.ContactSource(remote, nil, ""))
	if err != nil {
		return davx.Written{}, fmt.Errorf("graph contact %s: %w", href, err)
	}
	if fields := graph.Diff(base.Fields, next.Fields); len(fields) > 0 {
		if err := g.c.UpdateContact(ctx, href, fields, ifMatch); err != nil {
			return davx.Written{}, davError(err)
		}
	}
	photo, err := g.c.Photo(ctx, href)
	if err != nil {
		return davx.Written{}, davError(err)
	}
	switch {
	case bytes.Equal(photo, next.Photo):
	case len(next.Photo) == 0:
		// Graph cannot delete a contact's photo (405, Outlook.com,
		// 2026-10-16): the rest is written, and the photo refused.
		return davx.Written{}, fmt.Errorf("%w: Outlook cannot remove a contact's photo", errUnstorable)
	default:
		if err := g.c.SetPhoto(ctx, href, next.Photo, next.PhotoType); err != nil {
			return davx.Written{}, davError(err)
		}
	}
	return davx.Written{Href: href}, nil
}

// errUnstorable is a source Outlook cannot store, a refusal like a 4xx.
var errUnstorable = errors.New("outlook cannot store this")

// davError gives Graph's errors the meanings the replay reads in davx's:
// a refused token, a missing item, a stale change key.
func davError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, graph.ErrUnauthorized):
		return fmt.Errorf("%w: %w", davx.ErrUnauthorized, err)
	case errors.Is(err, graph.ErrNotFound):
		return fmt.Errorf("%w: %w", davx.ErrNotFound, err)
	case errors.Is(err, graph.ErrPrecondition):
		return fmt.Errorf("%w: %w", davx.ErrPrecondition, err)
	case errors.Is(err, graph.ErrUnsupported):
		return fmt.Errorf("%w: %w", errUnstorable, err)
	}
	return err
}
