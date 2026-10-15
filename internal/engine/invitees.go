package engine

import (
	"context"
	"encoding/json/v2"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/store"
)

// Invitations as organizer (ADR-0035): an event takes invitees only in a
// calendar whose server tells them about the events Frostmail writes, and
// a write that would tell them waits for the user's yes.

// inviteesOf checks the invitees an edit sets and returns them, without
// the user's own addresses, with the organizer an event needs when it has
// any: the account's default identity.
func (c calendarService) inviteesOf(ctx context.Context, col store.Collection, list []api.Invitee, emails []string) (calendar.EventChange, error) {
	out := []calendar.Attendee{}
	seen := map[string]bool{}
	for _, in := range list {
		email := strings.ToLower(strings.TrimSpace(in.Email))
		addr, err := mail.ParseAddress(email)
		if err != nil || addr.Address != email {
			return calendar.EventChange{}, api.InvalidParams("%q is not an address", in.Email)
		}
		role := "required"
		if in.Role != nil {
			switch *in.Role {
			case api.AttendeeRoleRequired, api.AttendeeRoleOptional:
				role = string(*in.Role)
			default:
				return calendar.EventChange{}, api.InvalidParams("an invitee is required or optional")
			}
		}
		if seen[email] || slices.Contains(emails, email) {
			continue
		}
		seen[email] = true
		name := ""
		if in.Name != nil {
			name = strings.TrimSpace(*in.Name)
		}
		out = append(out, calendar.Attendee{Email: email, Name: name, Role: role})
	}
	change := calendar.EventChange{Attendees: &out}
	if len(out) == 0 {
		return change, nil
	}
	acct, err := c.DB.GetAccount(ctx, col.AccountID)
	if err != nil {
		return change, err
	}
	if !invites(acct) {
		return change, api.InvalidParams("invitations aren't available in calendar %q: its server does not send them", col.Name)
	}
	organizer, err := c.organizer(ctx, acct)
	if err != nil {
		return change, err
	}
	change.Organizer = &organizer
	return change, nil
}

// organizer is the account's default identity, else the account.
func (c calendarService) organizer(ctx context.Context, acct store.Account) (calendar.Attendee, error) {
	identities, err := c.DB.ListIdentities(ctx, acct.ID)
	if err != nil {
		return calendar.Attendee{}, err
	}
	for _, i := range identities {
		if i.IsDefault {
			return calendar.Attendee{Email: strings.ToLower(i.Email), Name: i.Name}, nil
		}
	}
	return calendar.Attendee{Email: strings.ToLower(acct.Email), Name: acct.DisplayName}, nil
}

// noticeEvents are the events of calendar objects the user organizes, by
// UID and recurrence ID, and their invitees.
type noticeEvents struct {
	events   map[string]calendar.Event
	invitees map[string]bool
}

func readNoticeEvents(objects [][]byte, emails []string) noticeEvents {
	out := noticeEvents{events: map[string]calendar.Event{}, invitees: map[string]bool{}}
	for _, raw := range objects {
		if raw == nil {
			continue
		}
		events, err := calendar.Parse(raw, calendar.Options{Local: time.UTC, UserEmails: emails})
		if err != nil {
			continue
		}
		for _, e := range events {
			if e.Organizer != nil && !slices.Contains(emails, e.Organizer.Email) {
				continue // an invitation: changing the user's copy tells nobody
			}
			out.events[e.UID+"\x00"+e.RecurrenceID] = e
			for _, a := range e.Attendees {
				if !slices.Contains(emails, a.Email) {
					out.invitees[a.Email] = true
				}
			}
		}
	}
	return out
}

// occurrence is the event as the series' master would give the
// occurrence at key, for comparing with an override only one side has.
func (n noticeEvents) occurrence(e calendar.Event) (calendar.Event, bool) {
	master, ok := n.events[e.UID+"\x00"]
	if !ok {
		return calendar.Event{}, false
	}
	layout := "2006-01-02T15:04:05.000Z"
	if master.AllDay {
		layout = time.DateOnly
	}
	start, err := time.Parse(layout, e.RecurrenceID)
	if err != nil {
		return calendar.Event{}, false
	}
	out := master
	out.RecurrenceID, out.Recurrence = e.RecurrenceID, ""
	out.Start, out.End = start, start.Add(master.End.Sub(master.Start))
	return out, true
}

// seen reports whether invitees see a difference between two versions of
// an event or occurrence: ADR-0035's title, location, notes, times,
// recurrence and status.
func seen(a, b calendar.Event) bool {
	return a.Summary != b.Summary || a.Location != b.Location || a.Description != b.Description ||
		a.AllDay != b.AllDay || !a.Start.Equal(b.Start) || !a.End.Equal(b.End) ||
		a.Recurrence != b.Recurrence || a.Status != b.Status
}

// inviteNotice is what a write tells invitees through the server, from
// the calendar objects it replaces (before) and writes (after), or nil
// when it tells nobody (ADR-0035). deleting marks the delete of an
// occurrence or of the series from one on, which cancels for everyone.
func inviteNotice(before, after [][]byte, emails []string, deleting bool) *api.InviteNotice {
	b, a := readNoticeEvents(before, emails), readNoticeEvents(after, emails)
	notice := func(kind api.NoticeKind, n int) *api.InviteNotice {
		if n == 0 {
			return nil
		}
		return &api.InviteNotice{Kind: kind, Count: int64(n)}
	}
	switch {
	case deleting || len(a.events) == 0:
		return notice(api.NoticeKindCancellations, len(b.invitees))
	case len(b.events) == 0:
		return notice(api.NoticeKindInvitations, len(a.invitees))
	}
	visible := false
	for key, e := range a.events {
		old, ok := b.events[key]
		if !ok {
			old, ok = b.occurrence(e)
		}
		if !ok || seen(old, e) {
			visible = true
			break
		}
	}
	for key, e := range b.events {
		if visible {
			break
		}
		if _, ok := a.events[key]; ok {
			continue
		}
		occ, ok := a.occurrence(e)
		visible = !ok || seen(e, occ)
	}
	var added, removed []string
	for email := range a.invitees {
		if !b.invitees[email] {
			added = append(added, email)
		}
	}
	for email := range b.invitees {
		if !a.invitees[email] {
			removed = append(removed, email)
		}
	}
	switch {
	case visible:
		all := len(b.invitees) + len(added)
		return notice(api.NoticeKindUpdates, all)
	case len(removed) == 0:
		return notice(api.NoticeKindInvitations, len(added))
	case len(added) == 0:
		return notice(api.NoticeKindCancellations, len(removed))
	}
	return notice(api.NoticeKindUpdates, len(added)+len(removed))
}

// requireNotify refuses a write that would tell invitees unless the call
// said notify: true (ADR-0035).
func requireNotify(notice *api.InviteNotice, notify *bool) error {
	if notice == nil {
		return nil
	}
	if notify == nil {
		e := api.NotifyRequired("the change tells %d invitees: ask the user, then send it with notify", notice.Count)
		if data, err := json.Marshal(notice); err == nil {
			e.Data = data
		}
		return e
	}
	if !*notify {
		return api.InvalidParams("the calendar's server tells invitees itself and cannot be kept quiet")
	}
	return nil
}

// hasInvitees reports whether a calendar object invites anyone the user
// does not stand for.
func hasInvitees(raw []byte, emails []string) bool {
	return len(readNoticeEvents([][]byte{raw}, emails).invitees) > 0
}
