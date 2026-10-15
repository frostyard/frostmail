package engine

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/store"
)

// The calendar domain's writes (ADR-0030 to ADR-0033; docs/design/pim.md,
// Editing): each patches or builds the event's object, stores it with its
// events and instances, so the views move at once, keeps a restore, and
// queues the write, in one transaction.

// eventTarget is a stored event a write changes: its row, its object and
// its calendar.
type eventTarget struct {
	row    store.EventRow
	obj    store.Object
	col    store.Collection
	series bool // the object repeats: its changes name an occurrence and a scope
	emails []string
}

// Create adds an event to a calendar.
func (c calendarService) Create(ctx context.Context, p *api.CalendarCreateParams) (*api.EventWrite, error) {
	change, move := eventChangeOf(p.Fields)
	id := p.CalendarID
	if id == nil {
		id = move
	}
	col, err := c.writableCalendar(ctx, id)
	if err != nil {
		return nil, err
	}
	emails, err := c.eventUserEmails(ctx, col.AccountID)
	if err != nil {
		return nil, err
	}
	if p.Fields.Attendees != nil {
		people, err := c.inviteesOf(ctx, col, p.Fields.Attendees, emails)
		if err != nil {
			return nil, err
		}
		change.Attendees, change.Organizer = people.Attendees, people.Organizer
	}
	uid, err := c.newUID()
	if err != nil {
		return nil, err
	}
	raw, err := calendar.NewEvent(uid, change, c.DB.Now())
	if err != nil {
		return nil, api.InvalidParams("%v", err)
	}
	if err := c.checkNotify(ctx, col, nil, [][]byte{raw}, emails, false, p.Notify); err != nil {
		return nil, err
	}
	restoreID, err := newRestoreID()
	if err != nil {
		return nil, err
	}
	obj := store.Object{CollectionID: col.ID, Href: eventHref(col, uid), Kind: store.ObjectVEvent, UID: uid, Raw: raw}
	err = c.DB.Tx(ctx, func(tx *store.Tx) error {
		objID, err := c.storeEvent(ctx, tx, obj)
		if err != nil {
			return err
		}
		if err := tx.RecordRestore(ctx, store.Restore{ID: restoreID, AccountID: col.AccountID,
			Objects: []store.RestoreObject{{CollectionID: col.ID, Href: obj.Href, UID: uid}}}); err != nil {
			return err
		}
		if _, err := tx.QueuePIMOp(ctx, store.PIMOp{AccountID: col.AccountID, CollectionID: col.ID, ObjectID: &objID,
			Kind: "put", Href: obj.Href}); err != nil {
			return err
		}
		return tx.Emit(ctx, api.CalendarChanged{AccountID: col.AccountID})
	})
	if err != nil {
		return nil, err
	}
	return c.eventWritten(ctx, restoreID, []int64{col.AccountID}, uid, "", col.ID)
}

// Update changes an event, one occurrence of a series, or a series from
// an occurrence on (ADR-0031), and moves it to another calendar.
func (c calendarService) Update(ctx context.Context, p *api.CalendarUpdateParams) (*api.EventWrite, error) {
	change, move := eventChangeOf(p.Fields)
	t, err := c.writableEvent(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if p.Fields.Attendees != nil {
		people, err := c.inviteesOf(ctx, t.col, p.Fields.Attendees, t.emails)
		if err != nil {
			return nil, err
		}
		change.Attendees, change.Organizer = people.Attendees, people.Organizer
	}
	edit, err := t.edit(p.RecurrenceID, p.Scope)
	if err != nil {
		return nil, err
	}
	edit.Change = change
	if t.invited() && (edit.Change != (calendar.EventChange{Alarms: change.Alarms}) || move != nil || edit.Scope == "future") {
		return nil, api.InvalidParams("an event the user is invited to takes only alerts, on the occurrence or the series")
	}
	if move != nil && *move != t.col.ID && t.series && edit.Scope != "all" {
		return nil, api.InvalidParams("an occurrence moves with its series: change All Events")
	}
	var target *store.Collection
	if move != nil && *move != t.col.ID {
		acct, err := c.DB.GetAccount(ctx, t.col.AccountID)
		if err != nil {
			return nil, err
		}
		if invites(acct) && (hasInvitees(t.obj.Raw, t.emails) || change.Attendees != nil && len(*change.Attendees) > 0) {
			return nil, api.InvalidParams("an event with invitees stays in its calendar: its server would cancel and invite again")
		}
		col, err := c.writableCalendar(ctx, move)
		if err != nil {
			return nil, err
		}
		target = &col
	}
	raw, split, err := t.apply(&edit, c.DB.Now(), c.newUID)
	if err != nil {
		return nil, err
	}
	if err := c.checkNotify(ctx, t.col, [][]byte{t.obj.Raw}, [][]byte{raw, split}, t.emails, false, p.Notify); err != nil {
		return nil, err
	}
	restoreID, err := newRestoreID()
	if err != nil {
		return nil, err
	}
	w := eventWrite{c: c, t: t, raw: raw, split: split, edit: edit, target: target, restoreID: restoreID}
	if err := c.DB.Tx(ctx, func(tx *store.Tx) error { return w.run(ctx, tx) }); err != nil {
		return nil, err
	}
	return c.eventWritten(ctx, restoreID, w.accounts(), w.resultUID(), w.resultRecurrenceID(), w.resultCalendar())
}

// Delete deletes an event, one occurrence of a series, or a series from
// an occurrence on.
func (c calendarService) Delete(ctx context.Context, p *api.CalendarDeleteParams) (*api.EventWrite, error) {
	t, err := c.writableEvent(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	edit, err := t.edit(p.RecurrenceID, p.Scope)
	if err != nil {
		return nil, err
	}
	if t.invited() && t.series && edit.Scope != "all" {
		return nil, api.InvalidParams("an event the user is invited to is deleted whole: delete All Events")
	}
	edit.Delete = true
	var raw []byte
	if t.series && edit.Scope != "all" {
		scope, _ := pimsync.Scope(edit.Scope)
		raw, err = calendar.DeleteOccurrence(t.obj.Raw, edit.RecurrenceID, scope, c.DB.Now())
		if err != nil && !errors.Is(err, calendar.ErrWholeSeries) {
			return nil, api.InvalidParams("%v", err)
		}
	}
	if err := c.checkNotify(ctx, t.col, [][]byte{t.obj.Raw}, [][]byte{raw}, t.emails, true, p.Notify); err != nil {
		return nil, err
	}
	restoreID, err := newRestoreID()
	if err != nil {
		return nil, err
	}
	w := eventWrite{c: c, t: t, raw: raw, edit: edit, restoreID: restoreID}
	if err := c.DB.Tx(ctx, func(tx *store.Tx) error { return w.run(ctx, tx) }); err != nil {
		return nil, err
	}
	if c.PIM != nil {
		c.PIM.Kick(t.col.AccountID)
	}
	return &api.EventWrite{RestoreID: restoreID}, nil
}

// Restore writes back what an event write replaced (ADR-0032).
func (c calendarService) Restore(ctx context.Context, p *api.CalendarRestoreParams) error {
	accounts := map[int64]bool{}
	err := c.DB.Tx(ctx, func(tx *store.Tx) error {
		r, err := tx.TakeRestore(ctx, p.RestoreID)
		if errors.Is(err, store.ErrNotFound) {
			return api.NotFound("restore %s is gone or was never made", p.RestoreID)
		}
		if err != nil {
			return err
		}
		// A refused notify returns before anything is written, and the
		// transaction keeps the restore.
		if err := c.checkRestoreNotify(ctx, r, p.Notify); err != nil {
			return err
		}
		for _, o := range r.Objects {
			accountID, err := c.restoreEvent(ctx, tx, o)
			if err != nil {
				return err
			}
			accounts[accountID] = true
		}
		for id := range accounts {
			if err := tx.Emit(ctx, api.CalendarChanged{AccountID: id}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for id := range accounts {
		if c.PIM != nil {
			c.PIM.Kick(id)
		}
	}
	return nil
}

// checkNotify refuses a write in col that would tell invitees without
// the user's yes (ADR-0035). Only calendars that take invitees tell them.
func (c calendarService) checkNotify(ctx context.Context, col store.Collection, before, after [][]byte, emails []string, deleting bool, notify *bool) error {
	acct, err := c.DB.GetAccount(ctx, col.AccountID)
	if err != nil {
		return err
	}
	if !invites(acct) {
		return nil
	}
	return requireNotify(inviteNotice(before, after, emails, deleting), notify)
}

// checkRestoreNotify is checkNotify for a restore: the objects as they
// are now, and as it writes them back.
func (c calendarService) checkRestoreNotify(ctx context.Context, r store.Restore, notify *bool) error {
	acct, err := c.DB.GetAccount(ctx, r.AccountID)
	if err != nil {
		return err
	}
	if !invites(acct) {
		return nil
	}
	emails, err := c.eventUserEmails(ctx, r.AccountID)
	if err != nil {
		return err
	}
	var before, after [][]byte
	for _, o := range r.Objects {
		current, err := c.DB.ObjectByHref(ctx, o.CollectionID, o.Href)
		switch {
		case err == nil:
			before = append(before, current.Raw)
		case !errors.Is(err, store.ErrNotFound):
			return err
		}
		after = append(after, o.Raw)
	}
	return requireNotify(inviteNotice(before, after, emails, false), notify)
}

// eventWrite is an update or delete, stored in one transaction: the
// object patched (raw) or deleted (raw nil), a split's new series, and a
// move's copy in the other calendar.
type eventWrite struct {
	c         calendarService
	t         eventTarget
	raw       []byte
	split     []byte
	edit      pimsync.EventEdit
	target    *store.Collection
	restoreID string
	moved     store.Object
}

func (w *eventWrite) run(ctx context.Context, tx *store.Tx) error {
	t := w.t
	objects := []store.RestoreObject{{CollectionID: t.col.ID, Href: t.obj.Href, UID: t.obj.UID, Raw: t.obj.Raw}}
	var splitObj store.Object
	if w.split != nil {
		splitObj = store.Object{CollectionID: t.col.ID, Href: eventHref(t.col, w.edit.NewUID), Kind: store.ObjectVEvent,
			UID: w.edit.NewUID, Raw: w.split}
		objects = append(objects, store.RestoreObject{CollectionID: t.col.ID, Href: splitObj.Href, UID: splitObj.UID})
	}
	if w.target != nil {
		w.moved = store.Object{CollectionID: w.target.ID, Href: strings.TrimSuffix(w.target.Href, "/") + "/" + path.Base(t.obj.Href),
			Kind: store.ObjectVEvent, UID: t.obj.UID, Raw: w.raw}
		objects = append(objects, store.RestoreObject{CollectionID: w.target.ID, Href: w.moved.Href, UID: t.obj.UID})
	}
	if err := tx.RecordRestore(ctx, store.Restore{ID: w.restoreID, AccountID: t.col.AccountID, Objects: objects}); err != nil {
		return err
	}
	switch {
	case w.target != nil:
		if err := w.move(ctx, tx); err != nil {
			return err
		}
	case w.raw == nil:
		if err := deleteEventObject(ctx, tx, t.obj, t.col, t.row.Summary, 0); err != nil {
			return err
		}
	default:
		if err := w.c.putEvent(ctx, tx, t.obj, t.col, w.raw, w.edit); err != nil {
			return err
		}
	}
	if w.split != nil {
		if _, err := w.c.createEvent(ctx, tx, splitObj, w.target == nil); err != nil {
			return err
		}
	}
	for _, id := range w.accounts() {
		if err := tx.Emit(ctx, api.CalendarChanged{AccountID: id}); err != nil {
			return err
		}
	}
	return nil
}

// move writes the changed object into the other calendar and deletes it
// here once that put is written (ADR-0030).
func (w *eventWrite) move(ctx context.Context, tx *store.Tx) error {
	opID, err := w.c.createEvent(ctx, tx, w.moved, true)
	if err != nil {
		return err
	}
	return deleteEventObject(ctx, tx, w.t.obj, w.t.col, w.t.row.Summary, opID)
}

func (w *eventWrite) accounts() []int64 {
	out := []int64{w.t.col.AccountID}
	if w.target != nil && w.target.AccountID != w.t.col.AccountID {
		out = append(out, w.target.AccountID)
	}
	return out
}

// result names the event the write returns: the occurrence changed alone,
// a split's new series, or the event.
func (w *eventWrite) resultUID() string {
	if w.split != nil {
		return w.edit.NewUID
	}
	return w.t.obj.UID
}

func (w *eventWrite) resultRecurrenceID() string {
	if w.edit.Scope == "this" {
		return w.edit.RecurrenceID
	}
	return ""
}

func (w *eventWrite) resultCalendar() int64 {
	if w.target != nil {
		return w.target.ID
	}
	return w.t.col.ID
}

// edit is the change's occurrence and scope: required for a series,
// ignored for a single event. An override named by its own ID changes
// that occurrence.
func (t eventTarget) edit(recurrenceID *string, scope *api.EventScope) (pimsync.EventEdit, error) {
	if !t.series {
		return pimsync.EventEdit{}, nil
	}
	id := t.row.RecurrenceID
	if recurrenceID != nil && *recurrenceID != "" {
		id = *recurrenceID
	}
	if id == "" || scope == nil {
		return pimsync.EventEdit{}, api.InvalidParams("a change to a repeating event names its occurrence and a scope")
	}
	if _, ok := pimsync.Scope(string(*scope)); !ok {
		return pimsync.EventEdit{}, api.InvalidParams("scope %q is not this, future or all", *scope)
	}
	return pimsync.EventEdit{RecurrenceID: id, Scope: string(*scope)}, nil
}

// apply patches the object: the patched source, and a split's new series
// (whose UID it sets in the edit).
func (t eventTarget) apply(edit *pimsync.EventEdit, now time.Time, newUID func() (string, error)) ([]byte, []byte, error) {
	if edit.Change == (calendar.EventChange{}) {
		return t.obj.Raw, nil, nil
	}
	if !t.series {
		raw, err := calendar.PatchEvent(t.obj.Raw, edit.Change, now)
		if err != nil {
			return nil, nil, api.InvalidParams("%v", err)
		}
		return raw, nil, nil
	}
	scope, _ := pimsync.Scope(edit.Scope)
	if scope == calendar.ScopeFuture {
		uid, err := newUID()
		if err != nil {
			return nil, nil, err
		}
		edit.NewUID = uid
	}
	raw, split, err := calendar.PatchOccurrence(t.obj.Raw, edit.RecurrenceID, scope, edit.Change, edit.NewUID, now)
	if err != nil {
		return nil, nil, api.InvalidParams("%v", err)
	}
	if split == nil {
		edit.NewUID = ""
	}
	return raw, split, nil
}

// invited reports whether the user is invited to the event rather than
// organizing it.
func (t eventTarget) invited() bool {
	return t.row.Organizer != "" && !slices.Contains(t.emails, t.row.Organizer)
}

// writableEvent is an event a write may change, with its object and
// calendar.
func (c calendarService) writableEvent(ctx context.Context, id int64) (eventTarget, error) {
	row, err := c.DB.Event(ctx, id)
	if err != nil {
		return eventTarget{}, apiError(err, fmt.Sprintf("event %d", id))
	}
	obj, err := c.DB.GetObject(ctx, row.ObjectID)
	if err != nil {
		return eventTarget{}, err
	}
	col, err := c.writableCalendar(ctx, &obj.CollectionID)
	if err != nil {
		return eventTarget{}, err
	}
	events, err := calendar.Parse(obj.Raw, calendar.Options{Local: time.Local})
	if err != nil {
		return eventTarget{}, err
	}
	series := slices.ContainsFunc(events, func(e calendar.Event) bool { return e.Recurrence != "" || e.RecurrenceID != "" })
	emails, err := c.eventUserEmails(ctx, col.AccountID)
	if err != nil {
		return eventTarget{}, err
	}
	return eventTarget{row: row, obj: obj, col: col, series: series, emails: emails}, nil
}

// writableCalendar is the calendar named, or the default calendar of the
// first account with the calendar service on, when the user can write to
// it.
func (c calendarService) writableCalendar(ctx context.Context, id *int64) (store.Collection, error) {
	if id == nil {
		def, err := c.defaultTarget(ctx)
		if err != nil {
			return store.Collection{}, err
		}
		id = &def
	}
	col, err := c.DB.GetCollection(ctx, *id)
	if err == nil && col.Kind != api.CollectionKindCalendar {
		err = store.ErrNotFound
	}
	if err != nil {
		return store.Collection{}, apiError(err, fmt.Sprintf("calendar %d", *id))
	}
	account, err := c.DB.GetAccount(ctx, col.AccountID)
	if err != nil {
		return store.Collection{}, err
	}
	if col.ReadOnly || account.ReadOnly {
		return store.Collection{}, api.Conflict("calendar %q is read-only", col.Name)
	}
	return col, nil
}

func (c calendarService) defaultTarget(ctx context.Context) (int64, error) {
	accounts, err := c.DB.ListAccounts(ctx)
	if err != nil {
		return 0, err
	}
	services, err := c.DB.Services(ctx, 0)
	if err != nil {
		return 0, err
	}
	for _, a := range accounts {
		on := slices.ContainsFunc(services, func(s store.Service) bool {
			return s.AccountID == a.ID && s.Service == api.ServiceKindCalendar && s.Enabled
		})
		if !on || a.ReadOnly {
			continue
		}
		col, ok, err := c.defaultCalendar(ctx, a.ID)
		if err != nil {
			return 0, err
		}
		if ok {
			return col.ID, nil
		}
	}
	return 0, api.NotFound("no calendar to add the event to")
}

// storeEvent stores an object and indexes its events, expanded over the
// instances window.
func (c calendarService) storeEvent(ctx context.Context, tx *store.Tx, obj store.Object) (int64, error) {
	from, to := pimsync.Window(tx.Now())
	if f, t, ok, err := c.DB.InstanceWindow(ctx); err != nil {
		return 0, err
	} else if ok {
		from, to = f, t
	}
	col, err := c.DB.GetCollection(ctx, obj.CollectionID)
	if err != nil {
		return 0, err
	}
	emails, err := c.eventUserEmails(ctx, col.AccountID)
	if err != nil {
		return 0, err
	}
	return pimsync.StoreCalendarObject(ctx, tx, obj, calendar.Options{Local: time.Local, UserEmails: emails}, from, to)
}

// createEvent stores a new object and queues its creation, returning
// the op's ID.
func (c calendarService) createEvent(ctx context.Context, tx *store.Tx, obj store.Object, queue bool) (int64, error) {
	id, err := c.storeEvent(ctx, tx, obj)
	if err != nil || !queue {
		return 0, err
	}
	col, err := c.DB.GetCollection(ctx, obj.CollectionID)
	if err != nil {
		return 0, err
	}
	return tx.QueuePIMOp(ctx, store.PIMOp{AccountID: col.AccountID, CollectionID: col.ID, ObjectID: &id,
		Kind: "put", Href: obj.Href})
}

// putEvent stores a patched object and queues its put on the ETag it
// was read with, or adds the edit to the put already waiting.
func (c calendarService) putEvent(ctx context.Context, tx *store.Tx, obj store.Object, col store.Collection, raw []byte, e pimsync.EventEdit) error {
	op, waiting, err := tx.QueuedPut(ctx, obj.ID)
	if err != nil {
		return err
	}
	payload := pimsync.EventPayload{}
	creation := waiting && op.IfMatch == ""
	if waiting {
		payload, _ = pimsync.ParseEventPayload(op.Payload)
	}
	if !creation {
		if payload, err = payload.Extend(obj.Raw, e); err != nil {
			return err
		}
	}
	next := obj
	next.Raw = raw
	if _, err := c.storeEvent(ctx, tx, next); err != nil {
		return err
	}
	encoded, err := payload.Encode()
	if err != nil {
		return err
	}
	switch {
	case creation:
		return nil // a creation writes the source as it is when it runs
	case waiting:
		return tx.SetPIMOpPayload(ctx, op.ID, encoded)
	}
	_, err = tx.QueuePIMOp(ctx, store.PIMOp{AccountID: col.AccountID, CollectionID: col.ID, ObjectID: &obj.ID,
		Kind: "put", Href: obj.Href, IfMatch: obj.ETag, Payload: encoded})
	return err
}

// deleteEventObject removes an event's object here and queues its delete,
// after the put after names when it is not 0; one the server never had is
// only dropped, with its waiting changes.
func deleteEventObject(ctx context.Context, tx *store.Tx, obj store.Object, col store.Collection, summary string, after int64) error {
	if obj.ETag == "" {
		if _, err := tx.DropPIMOps(ctx, obj.ID); err != nil {
			return err
		}
	} else {
		payload, err := pimsync.EventPayload{Summary: summary, After: after}.Encode()
		if err != nil {
			return err
		}
		if _, err := tx.QueuePIMOp(ctx, store.PIMOp{AccountID: col.AccountID, CollectionID: col.ID,
			Kind: "delete", Href: obj.Href, IfMatch: obj.ETag, Payload: payload}); err != nil {
			return err
		}
	}
	_, err := tx.DeleteObjects(ctx, col.ID, []string{obj.Href})
	return err
}

// restoreEvent puts one kept source back, as people's restoreObject does
// for contacts, and returns the account it wrote in.
func (c calendarService) restoreEvent(ctx context.Context, tx *store.Tx, o store.RestoreObject) (int64, error) {
	col, err := c.writableCalendar(ctx, &o.CollectionID)
	if err != nil {
		return 0, err
	}
	current, err := c.DB.ObjectByHref(ctx, col.ID, o.Href)
	exists := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return 0, err
	}
	if o.Raw == nil {
		if exists {
			return col.AccountID, deleteEventObject(ctx, tx, current, col, "", 0)
		}
		return col.AccountID, nil
	}
	next := store.Object{CollectionID: col.ID, Href: o.Href, Kind: store.ObjectVEvent, UID: o.UID, Raw: o.Raw}
	switch {
	case exists:
		next.ETag = current.ETag
		kept := o.Raw
		if account, err := c.DB.GetAccount(ctx, col.AccountID); err == nil && account.Kind == api.AccountKindGmail {
			// Google keeps an exception a PUT leaves out: reset it instead.
			if kept, err = calendar.KeepOverrides(o.Raw, current.Raw, c.DB.Now()); err != nil {
				return 0, err
			}
		}
		// The kept source goes over a later version: its SEQUENCE must not
		// go back, or Google refuses it (409).
		raised, err := calendar.RaiseSequence(kept, calendar.MaxSequence(current.Raw))
		if err != nil {
			return 0, err
		}
		next.Raw = raised
	default:
		deleted, waiting, err := tx.TakeQueuedDelete(ctx, col.ID, o.Href)
		if err != nil {
			return 0, err
		}
		if waiting {
			next.ETag = deleted.IfMatch
			break
		}
		name, err := c.newUID()
		if err != nil {
			return 0, err
		}
		next.Href = eventHref(col, name)
	}
	id, err := c.storeEvent(ctx, tx, next)
	if err != nil {
		return 0, err
	}
	whole, err := pimsync.EventPayload{Whole: true}.Encode()
	if err != nil {
		return 0, err
	}
	if op, waiting, err := tx.QueuedPut(ctx, id); err != nil {
		return 0, err
	} else if waiting {
		if op.IfMatch == "" {
			return col.AccountID, nil
		}
		return col.AccountID, tx.SetPIMOpPayload(ctx, op.ID, whole)
	}
	_, err = tx.QueuePIMOp(ctx, store.PIMOp{AccountID: col.AccountID, CollectionID: col.ID, ObjectID: &id,
		Kind: "put", Href: next.Href, IfMatch: next.ETag, Payload: whole})
	return col.AccountID, err
}

// eventWritten kicks the accounts' passes and returns the write: the
// event of uid in the calendar (its occurrence recurrenceID, else its
// master), and the restore.
func (c calendarService) eventWritten(ctx context.Context, restoreID string, accounts []int64, uid, recurrenceID string, calendarID int64) (*api.EventWrite, error) {
	for _, id := range accounts {
		if c.PIM != nil {
			c.PIM.Kick(id)
		}
	}
	out := &api.EventWrite{RestoreID: restoreID}
	col, err := c.DB.GetCollection(ctx, calendarID)
	if err != nil {
		return nil, err
	}
	rows, err := c.DB.EventsByUID(ctx, col.AccountID, uid)
	if err != nil {
		return nil, err
	}
	var master *store.EventRow
	for i, r := range rows {
		if r.CalendarID != calendarID {
			continue
		}
		if r.RecurrenceID == recurrenceID {
			master = &rows[i]
			break
		}
		if r.RecurrenceID == "" {
			master = &rows[i]
		}
	}
	if master == nil {
		return out, nil
	}
	params := &api.CalendarEventParams{ID: master.ID}
	if recurrenceID != "" {
		params.RecurrenceID = &recurrenceID
	}
	if out.Event, err = c.Event(ctx, params); err != nil {
		return nil, err
	}
	return out, nil
}

// eventChangeOf turns the API's fields into a change, and the calendar
// to move to; calendar checks the times and the rule.
func eventChangeOf(f api.EventFields) (calendar.EventChange, *int64) {
	c := calendar.EventChange{Summary: f.Summary, Location: f.Location, Description: f.Description, AllDay: f.AllDay,
		Start: f.Start, End: f.End, Zone: f.TimeZone, StartDate: f.StartDate, EndDate: f.EndDate, RRule: f.Rrule,
		Transparent: f.Transparent}
	if f.Rrule != nil {
		rule := strings.TrimPrefix(strings.TrimSpace(*f.Rrule), "RRULE:")
		c.RRule = &rule
	}
	if f.Alarms != nil {
		alarms := make([]int, len(f.Alarms))
		for i, a := range f.Alarms {
			alarms[i] = int(a)
		}
		c.Alarms = &alarms
	}
	return c, f.CalendarID
}

// eventHref is a new object's name in a calendar.
func eventHref(col store.Collection, uid string) string {
	return strings.TrimSuffix(col.Href, "/") + "/" + uid + ".ics"
}
