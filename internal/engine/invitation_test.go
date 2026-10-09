package engine_test

// CONTRACT TEST for task card T-0088 (docs/tasks). Do not edit.

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// inviteEnv is a server with user@dav.test's Work calendar holding a day
// of events around the invitation's hour (2026-10-15 14:00–15:00 UTC; all
// within one day from UTC−10 to UTC+4), and an INBOX for invitation mail.
type inviteEnv struct {
	srv   *rpctest.Server
	c     *api.Client
	dav   *davtest.Server
	acct  int64
	work  string
	inbox int64
	uid   uint32
}

func event(uid, start, end, summary string, extra ...string) []byte {
	lines := append([]string{"BEGIN:VEVENT", "UID:" + uid, "DTSTART:" + start, "DTEND:" + end, "SUMMARY:" + summary}, extra...)
	return vcal(append(lines, "END:VEVENT")...)
}

func newInviteEnv(t *testing.T) *inviteEnv {
	t.Helper()
	ctx := t.Context()
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	dav := davtest.New(t, davtest.Options{})
	srv := rpctest.StartWith(t, rpctest.Options{
		PIM: &pimsync.Config{HTTP: dav.Client, Interval: time.Hour, Local: time.UTC},
		Now: func() time.Time { return now },
	})
	e := &inviteEnv{srv: srv, c: srv.Dial(t), dav: dav}
	e.acct = davAccount(t, e.c)
	e.work = dav.Calendar("work", "Work", "")
	for name, ics := range map[string][]byte{
		"early":   event("early", "20261015T130000Z", "20261015T131500Z", "Early call"),
		"bfast":   event("bfast", "20261015T131500Z", "20261015T134500Z", "Breakfast"),
		"review":  event("review", "20261015T143000Z", "20261015T153000Z", "Design review"),
		"focus":   event("focus", "20261015T140000Z", "20261015T150000Z", "Focus time", "TRANSP:TRANSPARENT"),
		"skipped": event("skipped", "20261015T141500Z", "20261015T144500Z", "Skipped", "ATTENDEE;PARTSTAT=DECLINED:mailto:user@dav.test"),
		"wrap":    event("wrap", "20261015T160000Z", "20261015T163000Z", "Wrap-up"),
		"evening": event("evening", "20261015T190000Z", "20261015T193000Z", "Evening"),
	} {
		dav.Put(e.work+name+".ics", ics)
	}
	if _, err := e.c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: e.acct, Service: api.ServiceKindCalendar,
		Enabled: true, URL: ptr(dav.URL)}); err != nil {
		t.Fatal(err)
	}
	e.pass(t)
	if err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, e.acct, []store.ServerMailbox{{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true}})
		if err != nil {
			return err
		}
		e.inbox = mbs[0].ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *inviteEnv) pass(t *testing.T) {
	t.Helper()
	if err := e.srv.PIM.Pass(t.Context(), e.acct); err != nil {
		t.Fatal(err)
	}
}

// mail stores a message whose body has ics as its text/calendar part, or
// only text when ics is empty.
func (e *inviteEnv) mail(t *testing.T, ics string) int64 {
	t.Helper()
	ctx := t.Context()
	body := "Content-Type: text/plain; charset=UTF-8\r\n\r\nNo calendar here.\r\n"
	if ics != "" {
		body = "Content-Type: multipart/alternative; boundary=b\r\n\r\n--b\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nYou are invited.\r\n" +
			"--b\r\nContent-Type: text/calendar; charset=UTF-8; method=REQUEST\r\n\r\n" + ics + "--b--\r\n"
	}
	raw := "From: Maria Lopez <maria@example.com>\r\nTo: user@dav.test\r\nSubject: Invitation\r\n" +
		"Date: Fri, 09 Oct 2026 14:00:00 +0000\r\nMessage-ID: <m@example.com>\r\nMIME-Version: 1.0\r\n" + body
	blobID, err := e.srv.Blobs.Put(ctx, bytes.NewReader([]byte(raw)))
	if err != nil {
		t.Fatal(err)
	}
	e.uid++
	var id int64
	if err := e.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		ids, err := tx.InsertHeaders(ctx, e.acct, e.inbox, []store.MessageHeader{{UID: e.uid, Subject: "Invitation",
			InternalDate: time.Date(2026, 10, 9, 14, 0, 0, 0, time.UTC), From: store.Address{Name: "Maria Lopez", Addr: "maria@example.com"}}})
		if err != nil {
			return err
		}
		id = ids[0]
		return tx.SetBody(ctx, id, blobID)
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

// request is the invitation to Launch review, at SEQUENCE seq, with the
// user's line (or none) among the attendees.
func request(method string, seq int, user string) string {
	lines := []string{"METHOD:" + method, "BEGIN:VEVENT", "UID:launch@example.com", "SEQUENCE:" + itoa(seq),
		"DTSTAMP:20261009T140000Z", "DTSTART:20261015T140000Z", "DTEND:20261015T150000Z", "SUMMARY:Launch review",
		"LOCATION:Room 4", "ORGANIZER;CN=Maria Lopez:mailto:maria@example.com",
		"ATTENDEE;CN=Maria Lopez;PARTSTAT=ACCEPTED:mailto:maria@example.com",
		"ATTENDEE;ROLE=OPT-PARTICIPANT;PARTSTAT=NEEDS-ACTION:mailto:bob@example.com"}
	if user != "" {
		lines = append(lines, user)
	}
	if method == "CANCEL" {
		lines = append(lines, "STATUS:CANCELLED")
	}
	return string(vcal(append(lines, "END:VEVENT")...))
}

func itoa(n int) string { return string(rune('0' + n)) }

const userLine = "ATTENDEE;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:user@dav.test"

func summaries(occ []api.Occurrence) []string {
	out := []string{}
	for _, o := range occ {
		out = append(out, o.Summary)
	}
	return out
}

func (e *inviteEnv) invitation(t *testing.T, id int64) *api.Invitation {
	t.Helper()
	inv, err := e.c.Calendar().Invitation(t.Context(), &api.CalendarInvitationParams{MessageID: id})
	if err != nil {
		t.Fatalf("invitation %d: %v", id, err)
	}
	return inv
}

func TestInvitationNotStored(t *testing.T) {
	e := newInviteEnv(t)
	inv := e.invitation(t, e.mail(t, request("REQUEST", 1, userLine)))
	ev := inv.Event
	if inv.Method != api.ITIPMethodRequest || ev.ID != 0 || ev.UID != "launch@example.com" || ev.Summary != "Launch review" ||
		ev.Location != "Room 4" || !ev.Start.Equal(time.Date(2026, 10, 15, 14, 0, 0, 0, time.UTC)) || ev.AccountID != e.acct {
		t.Errorf("event = %s %+v", inv.Method, ev)
	}
	if inv.EventID != nil || inv.Outdated || !inv.CanRespond {
		t.Errorf("eventId %v, outdated %v, canRespond %v", inv.EventID, inv.Outdated, inv.CanRespond)
	}
	if inv.From == nil || inv.From.Email != "maria@example.com" || inv.From.Name != "Maria Lopez" {
		t.Errorf("from = %+v", inv.From)
	}
	if inv.Answer == nil || *inv.Answer != api.PartStatNeedsaction {
		t.Errorf("answer = %v", inv.Answer)
	}
	var user *api.Attendee
	for i, a := range ev.Attendees {
		if a.IsUser {
			user = &ev.Attendees[i]
		}
	}
	if user == nil || user.Email != "user@dav.test" || ev.Organizer == nil || ev.Organizer.Email != "maria@example.com" {
		t.Errorf("people = %+v / %+v", ev.Organizer, ev.Attendees)
	}
	if got := summaries(inv.Conflicts); strings.Join(got, ",") != "Design review" {
		t.Errorf("conflicts = %q", got)
	}
	if got := summaries(inv.Adjacent); strings.Join(got, ",") != "Breakfast,Wrap-up" {
		t.Errorf("adjacent = %q", got)
	}
}

func TestInvitationStored(t *testing.T) {
	e := newInviteEnv(t)
	stored := strings.Replace(strings.Replace(request("REQUEST", 1, "ATTENDEE;PARTSTAT=ACCEPTED:mailto:user@dav.test"),
		"METHOD:REQUEST\r\n", "", 1), "SUMMARY:Launch review", "SUMMARY:Launch review (stored)", 1)
	e.dav.Put(e.work+"launch.ics", []byte(stored))
	e.pass(t)
	id := e.mail(t, request("REQUEST", 1, userLine))
	inv := e.invitation(t, id)
	if inv.EventID == nil || inv.Answer == nil || *inv.Answer != api.PartStatAccepted || inv.Outdated || !inv.CanRespond {
		t.Errorf("stored: eventId %v answer %v outdated %v canRespond %v", inv.EventID, inv.Answer, inv.Outdated, inv.CanRespond)
	}
	if inv.EventID != nil {
		ev, err := e.c.Calendar().Event(t.Context(), &api.CalendarEventParams{ID: *inv.EventID})
		if err != nil || ev.Summary != "Launch review (stored)" {
			t.Errorf("the stored event = %+v, %v", ev, err)
		}
	}
	if inv.Event.Summary != "Launch review" || inv.Event.ID != 0 {
		t.Errorf("the message's event = %+v", inv.Event)
	}
	if got := summaries(inv.Conflicts); strings.Join(got, ",") != "Design review" {
		t.Errorf("conflicts = %q (the event itself is not one)", got)
	}

	// The calendar has a later version than the message.
	e.dav.Put(e.work+"launch.ics", []byte(strings.Replace(stored, "SEQUENCE:1", "SEQUENCE:3", 1)))
	e.pass(t)
	if inv := e.invitation(t, id); !inv.Outdated || inv.CanRespond {
		t.Errorf("outdated: %v, canRespond %v", inv.Outdated, inv.CanRespond)
	}
}

func TestInvitationCannotRespond(t *testing.T) {
	e := newInviteEnv(t)
	cancel := e.invitation(t, e.mail(t, request("CANCEL", 2, userLine)))
	if cancel.Method != api.ITIPMethodCancel || cancel.CanRespond || cancel.Event.Status != api.EventStatusCancelled {
		t.Errorf("cancel = %s canRespond %v status %s", cancel.Method, cancel.CanRespond, cancel.Event.Status)
	}
	uninvited := e.invitation(t, e.mail(t, request("REQUEST", 1, "")))
	if uninvited.CanRespond || uninvited.Answer != nil {
		t.Errorf("not invited: canRespond %v, answer %v", uninvited.CanRespond, uninvited.Answer)
	}
	reply := e.invitation(t, e.mail(t, string(vcal("METHOD:REPLY", "BEGIN:VEVENT", "UID:planning", "DTSTAMP:20261009T160000Z",
		"DTSTART:20261016T130000Z", "DTEND:20261016T140000Z", "SUMMARY:Planning", "ORGANIZER:mailto:user@dav.test",
		"ATTENDEE;PARTSTAT=DECLINED;CN=Bob:mailto:bob@example.com", "END:VEVENT"))))
	if reply.Method != api.ITIPMethodReply || reply.CanRespond || reply.From == nil || reply.From.Email != "bob@example.com" ||
		reply.From.Answer != api.PartStatDeclined || reply.Answer != nil {
		t.Errorf("reply = %s %+v canRespond %v answer %v", reply.Method, reply.From, reply.CanRespond, reply.Answer)
	}
	id := e.mail(t, request("REQUEST", 1, userLine))
	if err := e.srv.DB.Tx(t.Context(), func(tx *store.Tx) error {
		_, err := tx.UpdateAccount(t.Context(), e.acct, store.AccountUpdate{ReadOnly: ptr(true)})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if inv := e.invitation(t, id); inv.CanRespond {
		t.Error("a read-only account can respond")
	}
}

func TestInvitationErrors(t *testing.T) {
	e := newInviteEnv(t)
	ctx := t.Context()
	for name, tc := range map[string]struct {
		id   int64
		code api.ErrorCode
	}{
		"unknown":       {99999, api.CodeNotFound},
		"no invitation": {e.mail(t, ""), api.CodeNotFound},
		"unreadable":    {e.mail(t, "BEGIN:VCALENDAR\r\nMETHOD:REQUEST\r\nEND:VCALENDAR\r\n"), api.CodeInvalidParams},
	} {
		if _, err := e.c.Calendar().Invitation(ctx, &api.CalendarInvitationParams{MessageID: tc.id}); code(err) != tc.code {
			t.Errorf("%s: %v", name, err)
		}
	}
}
