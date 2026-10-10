package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/compose"
	"github.com/frostyard/frostmail/internal/itip"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/store"
)

// response is what answering an invitation needs: the account, the
// iCalendar source the answer patches and replies to, and who the user and
// the organizer are in it.
type response struct {
	acct   store.Account
	emails []string
	answer string
	// key answers one occurrence (calendar.Event.RecurrenceID); "" the
	// whole event.
	key     string
	comment string
	// stored is the user's copy in a calendar, with its object; nil when
	// only the message has the invitation.
	stored *store.EventRow
	obj    store.Object
	// src is the stored object's source, or the message's invitation.
	src       []byte
	main      calendar.Event
	attendee  string // the user's address as the event names it
	organizer calendar.Attendee
}

// Respond answers an invitation (ADR-0019, docs/design/pim.md,
// Invitations). The user's PARTSTAT changes in the stored event, whose put
// is queued; an accepted or tentative invitation not yet stored goes into
// the account's default calendar. Unless the account's server schedules,
// maild also mails the organizer an iTIP REPLY through the outbox, with
// the undo delay.
func (c calendarService) Respond(ctx context.Context, p *api.CalendarRespondParams) (*api.CalendarEvent, error) {
	switch p.Answer {
	case api.PartStatAccepted, api.PartStatDeclined, api.PartStatTentative:
	default:
		return nil, api.InvalidParams("an answer is accepted, declined or tentative")
	}
	r, err := c.response(ctx, p)
	if err != nil {
		return nil, err
	}
	if r.acct.ReadOnly || r.stored != nil && r.stored.ReadOnly {
		return nil, api.Conflict("the calendar of account %d is read-only", r.acct.ID)
	}
	if r.organizer.Email == "" || slices.Contains(r.emails, r.organizer.Email) {
		return nil, api.Conflict("the user organizes this event")
	}
	mail := !providers.ForKind(r.acct.Kind).DAV.Schedules
	var id int64
	if r.stored != nil {
		id, err = c.answerStored(ctx, &r, mail)
	} else {
		id, err = c.answerMessage(ctx, &r, &mail)
	}
	if err != nil {
		return nil, err
	}
	if c.PIM != nil {
		c.PIM.Kick(r.acct.ID)
	}
	if c.Sync != nil && mail {
		c.Sync.OutboxChanged(r.acct.ID)
	}
	if id == 0 {
		return c.answered(r)
	}
	ev := &api.CalendarEventParams{ID: id}
	if r.key != "" {
		ev.RecurrenceID = &r.key
	}
	return c.Event(ctx, ev)
}

// response finds the invitation by its stored event or its message.
func (c calendarService) response(ctx context.Context, p *api.CalendarRespondParams) (response, error) {
	r := response{answer: string(p.Answer)}
	if p.RecurrenceID != nil {
		r.key = *p.RecurrenceID
	}
	if p.Comment != nil {
		r.comment = strings.TrimSpace(*p.Comment)
	}
	var err error
	switch {
	case p.EventID != nil:
		err = c.byEvent(ctx, &r, *p.EventID)
	case p.MessageID != nil:
		err = c.byMessage(ctx, &r, *p.MessageID)
	default:
		return r, api.InvalidParams("answer a messageId or an eventId")
	}
	if err != nil {
		return r, err
	}
	for _, a := range r.main.Attendees {
		if slices.Contains(r.emails, a.Email) {
			r.attendee = a.Email
			break
		}
	}
	if r.attendee == "" {
		return r, api.Conflict("the event does not invite the user")
	}
	if r.main.Organizer != nil {
		r.organizer = *r.main.Organizer
	}
	return r, nil
}

func (c calendarService) setAccount(ctx context.Context, r *response, id int64) error {
	acct, err := c.DB.GetAccount(ctx, id)
	if err != nil {
		return apiError(err, "the account")
	}
	r.acct = acct
	r.emails, err = c.eventUserEmails(ctx, id)
	return err
}

// byEvent answers a stored event: its object is the source.
func (c calendarService) byEvent(ctx context.Context, r *response, id int64) error {
	e, err := c.DB.Event(ctx, id)
	if err != nil {
		return apiError(err, "calendar event")
	}
	return c.useStored(ctx, r, e)
}

func (c calendarService) useStored(ctx context.Context, r *response, e store.EventRow) error {
	if err := c.setAccount(ctx, r, e.AccountID); err != nil {
		return err
	}
	obj, err := c.DB.GetObject(ctx, e.ObjectID)
	if err != nil {
		return err
	}
	events, err := calendar.Parse(obj.Raw, calendar.Options{Local: time.Local, UserEmails: r.emails})
	if err != nil {
		return api.InvalidParams("the event cannot be read: %v", err)
	}
	r.stored, r.obj, r.src = &e, obj, obj.Raw
	for _, ev := range events {
		if ev.RecurrenceID == e.RecurrenceID {
			r.main = ev
		}
	}
	return nil
}

// byMessage answers the invitation in a message: its stored copy when the
// account's calendar has one, else the message's own.
func (c calendarService) byMessage(ctx context.Context, r *response, id int64) error {
	sums, err := c.DB.Summaries(ctx, []int64{id})
	if err != nil {
		return err
	}
	if len(sums) == 0 {
		return api.NotFound("message %d does not exist", id)
	}
	if err := c.setAccount(ctx, r, sums[0].AccountID); err != nil {
		return err
	}
	msg, err := c.readInvitation(ctx, id, r.emails)
	if err != nil {
		return err
	}
	if msg.Method != itip.MethodRequest {
		return api.Conflict("message %d is not an invitation to answer", id)
	}
	main := msg.Main()
	stored, err := c.DB.EventsByUID(ctx, r.acct.ID, main.UID)
	if err != nil {
		return err
	}
	for _, e := range stored {
		if e.RecurrenceID != main.RecurrenceID {
			continue
		}
		if e.Sequence > main.Sequence {
			return api.Conflict("the calendar has a later version of this invitation")
		}
		full, err := c.DB.Event(ctx, e.ID)
		if err != nil {
			return err
		}
		return c.useStored(ctx, r, full)
	}
	r.main, r.src = main, msg.Source
	return nil
}

// answerStored patches the stored copy, queues its put and, when mail is
// set, the reply. It returns the event's ID.
func (c calendarService) answerStored(ctx context.Context, r *response, mail bool) (int64, error) {
	raw, err := itip.SetPartStat(r.src, r.key, r.attendee, r.answer)
	if err != nil {
		return 0, answerError(err)
	}
	reply, err := c.replyMail(ctx, r, raw, mail)
	if err != nil {
		return 0, err
	}
	obj := r.obj
	obj.Raw = raw
	err = c.DB.Tx(ctx, func(tx *store.Tx) error {
		if err := c.storeAnswer(ctx, tx, r, obj); err != nil {
			return err
		}
		return queueReply(ctx, tx, reply)
	})
	return r.stored.ID, err
}

// answerMessage stores an accepted or tentative invitation in the default
// calendar, queuing its creation, and mails the reply when mail is set or
// nothing was stored. It returns the stored event's ID, or 0.
func (c calendarService) answerMessage(ctx context.Context, r *response, mail *bool) (int64, error) {
	raw, err := itip.SetPartStat(r.src, r.key, r.attendee, r.answer)
	if err != nil {
		return 0, answerError(err)
	}
	cal, ok, err := c.defaultCalendar(ctx, r.acct.ID)
	if err != nil {
		return 0, err
	}
	keep := ok && r.answer != string(api.PartStatDeclined)
	*mail = *mail || !keep
	reply, err := c.replyMail(ctx, r, raw, *mail)
	if err != nil {
		return 0, err
	}
	var obj store.Object
	if keep {
		if raw, err = itip.ForCalendar(raw); err != nil {
			return 0, err
		}
		uid, err := newUUID()
		if err != nil {
			return 0, err
		}
		obj = store.Object{CollectionID: cal.ID, Href: strings.TrimSuffix(cal.Href, "/") + "/" + uid + ".ics",
			Kind: store.ObjectVEvent, UID: r.main.UID, Raw: raw}
	}
	err = c.DB.Tx(ctx, func(tx *store.Tx) error {
		if keep {
			if err := c.storeAnswer(ctx, tx, r, obj); err != nil {
				return err
			}
		}
		return queueReply(ctx, tx, reply)
	})
	if err != nil || !keep {
		return 0, err
	}
	stored, err := c.DB.EventsByUID(ctx, r.acct.ID, r.main.UID)
	for _, e := range stored {
		if e.RecurrenceID == r.main.RecurrenceID {
			return e.ID, err
		}
	}
	return 0, err
}

// storeAnswer stores and indexes the answered object and queues its put
// on the ETag it was read with ("" creates it), unless one is waiting.
func (c calendarService) storeAnswer(ctx context.Context, tx *store.Tx, r *response, obj store.Object) error {
	from, to := pimsync.Window(tx.Now())
	if f, t, ok, err := c.DB.InstanceWindow(ctx); err != nil {
		return err
	} else if ok {
		from, to = f, t
	}
	id, err := pimsync.StoreCalendarObject(ctx, tx, obj, calendar.Options{Local: time.Local, UserEmails: r.emails}, from, to)
	if err != nil {
		return err
	}
	waiting, err := tx.QueuedPIMOp(ctx, id, "put")
	if err != nil {
		return err
	}
	if !waiting {
		if _, err := tx.QueuePIMOp(ctx, store.PIMOp{AccountID: r.acct.ID, CollectionID: obj.CollectionID, ObjectID: &id,
			Kind: "put", Href: obj.Href, IfMatch: obj.ETag}); err != nil {
			return err
		}
	}
	return tx.Emit(ctx, api.CalendarChanged{AccountID: r.acct.ID})
}

func answerError(err error) error {
	switch {
	case errors.Is(err, itip.ErrNoOccurrence):
		return api.InvalidParams("answering one occurrence of a series without its own copy is not supported yet")
	case errors.Is(err, itip.ErrNotInvited):
		return api.Conflict("the event does not invite the user")
	}
	return err
}

// defaultCalendar is the account's default calendar that can be written,
// else its first. A default the user did not choose first follows the one
// the server names, asked for here, where it matters; a server that does
// not answer in time leaves it as it was.
func (c calendarService) defaultCalendar(ctx context.Context, accountID int64) (store.Collection, bool, error) {
	if c.PIM != nil {
		ask, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := c.PIM.UseServerDefaultCalendar(ask, accountID)
		cancel()
		if err != nil && c.Log != nil {
			c.Log.Warn("the server's default calendar is unknown", "account", accountID, "err", err)
		}
	}
	cols, err := c.DB.Collections(ctx, store.CollectionFilter{AccountID: accountID, Kind: api.CollectionKindCalendar, EnabledOnly: true})
	if err != nil {
		return store.Collection{}, false, err
	}
	cols = slices.DeleteFunc(cols, func(col store.Collection) bool { return col.ReadOnly })
	if i := slices.IndexFunc(cols, func(col store.Collection) bool { return col.IsDefault }); i >= 0 {
		return cols[i], true, nil
	}
	if len(cols) == 0 {
		return store.Collection{}, false, nil
	}
	return cols[0], true, nil
}

// reply is a built iTIP reply waiting to be queued.
type reply struct {
	item store.OutboxItem
}

var answerVerb = map[string]string{"accepted": "Accepted", "declined": "Declined", "tentative": "Tentative"}

// replyMail builds the REPLY mail to the organizer and stores its blob;
// nil when mail is not set.
func (c calendarService) replyMail(ctx context.Context, r *response, src []byte, mail bool) (*reply, error) {
	if !mail {
		return nil, nil
	}
	if c.Blobs == nil {
		return nil, api.Unavailable("the blob store is not running")
	}
	now := c.DB.Now()
	data, err := itip.Reply(src, r.key, r.attendee, r.answer, r.comment, now)
	if err != nil {
		return nil, answerError(err)
	}
	ident, err := drafts(c).identityFor(ctx, r.acct.ID, r.attendee)
	if errors.Is(err, store.ErrNotFound) {
		ident = store.Identity{Name: r.acct.DisplayName, Email: r.acct.Email}
	} else if err != nil {
		return nil, err
	}
	name := cmpOr(ident.Name, r.attendee)
	summary := cmpOr(r.main.Summary, "No Title")
	body := "<p>" + html.EscapeString(fmt.Sprintf("%s has %s the invitation to %s.", name,
		strings.ToLower(answerVerb[r.answer]), summary)) + "</p>"
	if r.comment != "" {
		body += "<p>" + html.EscapeString(r.comment) + "</p>"
	}
	to := compose.Address{Name: r.organizer.Name, Addr: r.organizer.Email}
	m := compose.Message{From: compose.Address{Name: ident.Name, Addr: ident.Email}, To: []compose.Address{to},
		Subject: answerVerb[r.answer] + ": " + summary, MessageID: compose.NewMessageID(ident.Email), Date: now,
		HTML: body, Calendar: &compose.Calendar{Method: "REPLY", Data: data}}
	var buf bytes.Buffer
	if err := compose.Build(&buf, m); err != nil {
		return nil, fmt.Errorf("build the reply: %w", err)
	}
	blobID, err := c.Blobs.Put(ctx, &buf)
	if err != nil {
		return nil, err
	}
	delay := c.UndoDelay
	if delay == 0 {
		delay = DefaultUndoDelay
	}
	return &reply{item: store.OutboxItem{AccountID: r.acct.ID, SendAt: now.Add(delay), BlobID: blobID,
		MessageID: m.MessageID, From: ident.Email, Recipients: m.Envelope(), Subject: m.Subject,
		To: []store.Address{{Name: to.Name, Addr: to.Addr}}}}, nil
}

func queueReply(ctx context.Context, tx *store.Tx, r *reply) error {
	if r == nil {
		return nil
	}
	item, err := tx.QueueOutbox(ctx, r.item)
	if err != nil {
		return err
	}
	return tx.Emit(ctx, api.OutboxChanged{ID: item.ID, AccountID: item.AccountID, State: api.OutboxStateQueued})
}

// answered is the message's event with the user's answer, for an
// invitation answered by mail alone.
func (c calendarService) answered(r response) (*api.CalendarEvent, error) {
	row := pimsync.EventRow(r.main)
	row.AccountID, row.PartStat = r.acct.ID, r.answer
	for i, a := range row.Attendees {
		if a.Email == r.attendee {
			row.Attendees[i].PartStat = r.answer
		}
	}
	out := calendarEventAPI(row, r.emails)
	return &out, nil
}

func cmpOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
