package engine_test

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/mimex"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// outbox returns the account's queued messages with their text/calendar
// parts.
func (e *inviteEnv) outbox(t *testing.T) ([]store.OutboxItem, []string) {
	t.Helper()
	items, err := e.srv.DB.ListOutbox(t.Context(), e.acct)
	if err != nil {
		t.Fatal(err)
	}
	var cals []string
	for _, it := range items {
		rc, err := e.srv.Blobs.Open(it.BlobID)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(rc)
		rc.Close()
		_ = mimex.WalkParts(raw, func(p mimex.PartInfo, body io.Reader) error {
			if p.ContentType == "text/calendar" {
				b, _ := io.ReadAll(body)
				cals = append(cals, string(b))
			}
			return nil
		})
		if !strings.Contains(string(raw), "method=REPLY") {
			t.Errorf("the reply's part has no method: %s", raw)
		}
	}
	return items, cals
}

// serverCopy is the calendar server's copy of the stored event with uid.
func (e *inviteEnv) serverCopy(t *testing.T, uid string) string {
	t.Helper()
	rows, err := e.srv.DB.EventsByUID(t.Context(), e.acct, uid)
	if err != nil || len(rows) == 0 {
		t.Fatalf("stored %s = %+v, %v", uid, rows, err)
	}
	obj, err := e.srv.DB.GetObject(t.Context(), rows[0].ObjectID)
	if err != nil {
		t.Fatal(err)
	}
	data, _, ok := e.dav.Object(obj.Href)
	if !ok {
		t.Fatalf("the server has no %s", obj.Href)
	}
	return string(unfold(data))
}

func unfold(b []byte) []byte {
	return []byte(strings.NewReplacer("\r\n ", "", "\r\n\t", "").Replace(string(b)))
}

func (e *inviteEnv) respond(t *testing.T, p *api.CalendarRespondParams) *api.CalendarEvent {
	t.Helper()
	ev, err := e.c.Calendar().Respond(t.Context(), p)
	if err != nil {
		t.Fatalf("respond %+v: %v", p, err)
	}
	return ev
}

func TestRespondStoresAnAcceptedInvitation(t *testing.T) {
	e := newInviteEnv(t)
	id := e.mail(t, request("REQUEST", 1, userLine))
	ev := e.respond(t, &api.CalendarRespondParams{MessageID: &id, Answer: api.PartStatAccepted})
	if ev.ID == 0 || ev.Answer == nil || *ev.Answer != api.PartStatAccepted || ev.Summary != "Launch review" {
		t.Errorf("answered = %+v", ev)
	}
	items, cals := e.outbox(t)
	if len(items) != 1 || items[0].Subject != "Accepted: Launch review" || len(items[0].To) != 1 ||
		items[0].To[0].Addr != "maria@example.com" || items[0].From != "user@dav.test" {
		t.Fatalf("outbox = %+v", items)
	}
	if len(cals) != 1 || !strings.Contains(cals[0], "METHOD:REPLY") ||
		!strings.Contains(string(unfold([]byte(cals[0]))), "PARTSTAT=ACCEPTED:mailto:user@dav.test") {
		t.Errorf("the reply = %s", cals)
	}
	e.pass(t)
	copy := e.serverCopy(t, "launch@example.com")
	if strings.Contains(copy, "METHOD") || !strings.Contains(copy, "PARTSTAT=ACCEPTED:mailto:user@dav.test") {
		t.Errorf("the server's copy:\n%s", copy)
	}
	if inv := e.invitation(t, id); inv.EventID == nil || *inv.EventID != ev.ID || *inv.Answer != api.PartStatAccepted {
		t.Errorf("the invitation afterwards = %+v", inv)
	}

	// Undo send drops the reply; there is no draft to return to.
	dr, err := e.c.Outbox().Cancel(t.Context(), &api.OutboxCancelParams{ID: items[0].ID})
	if err != nil || dr.ID != 0 {
		t.Errorf("cancel = %+v, %v", dr, err)
	}
	if items, _ := e.outbox(t); len(items) != 0 {
		t.Errorf("outbox after undo = %+v", items)
	}
}

func TestRespondPatchesTheStoredCopy(t *testing.T) {
	e := newInviteEnv(t)
	stored := strings.Replace(request("REQUEST", 1, "ATTENDEE;PARTSTAT=ACCEPTED;CN=User:mailto:user@dav.test"), "METHOD:REQUEST\r\n", "", 1)
	e.dav.Put(e.work+"launch.ics", []byte(stored))
	e.pass(t)
	id := e.mail(t, request("REQUEST", 1, userLine))
	ev := e.respond(t, &api.CalendarRespondParams{MessageID: &id, Answer: api.PartStatDeclined})
	if ev.Answer == nil || *ev.Answer != api.PartStatDeclined {
		t.Errorf("answered = %+v", ev)
	}
	e.pass(t)
	got := e.serverCopy(t, "launch@example.com")
	want := strings.Replace(stored, "ATTENDEE;PARTSTAT=ACCEPTED;CN=User:mailto:user@dav.test", "ATTENDEE;PARTSTAT=DECLINED;CN=User:mailto:user@dav.test", 1)
	if got != want {
		t.Errorf("the server's copy =\n%s\nwant\n%s", got, want)
	}
	if paths := e.dav.Paths(e.work); len(paths) != 8 {
		t.Errorf("objects = %q (no new one)", paths)
	}

	// By the event, with a note: only the reply carries it.
	ev = e.respond(t, &api.CalendarRespondParams{EventID: &ev.ID, Answer: api.PartStatTentative, Comment: ptr("Running late")})
	_, cals := e.outbox(t)
	if *ev.Answer != api.PartStatTentative || len(cals) != 2 || !strings.Contains(cals[1], "COMMENT:Running late") ||
		!strings.Contains(string(unfold([]byte(cals[1]))), "PARTSTAT=TENTATIVE") {
		t.Errorf("tentative = %+v, replies %q", ev, cals)
	}
}

func TestRespondDeclinesByMailAlone(t *testing.T) {
	e := newInviteEnv(t)
	id := e.mail(t, request("REQUEST", 1, userLine))
	ev := e.respond(t, &api.CalendarRespondParams{MessageID: &id, Answer: api.PartStatDeclined})
	if ev.ID != 0 || ev.Answer == nil || *ev.Answer != api.PartStatDeclined {
		t.Errorf("answered = %+v", ev)
	}
	if rows, _ := e.srv.DB.EventsByUID(t.Context(), e.acct, "launch@example.com"); len(rows) != 0 {
		t.Errorf("a declined invitation was stored: %+v", rows)
	}
	if items, _ := e.outbox(t); len(items) != 1 || items[0].Subject != "Declined: Launch review" {
		t.Errorf("outbox = %+v", items)
	}
}

func TestRespondRefuses(t *testing.T) {
	e := newInviteEnv(t)
	ctx := t.Context()
	invite := e.mail(t, request("REQUEST", 1, userLine))
	for name, tc := range map[string]struct {
		p    api.CalendarRespondParams
		code api.ErrorCode
	}{
		"no answer":     {api.CalendarRespondParams{MessageID: &invite, Answer: api.PartStatNeedsaction}, api.CodeInvalidParams},
		"nothing":       {api.CalendarRespondParams{Answer: api.PartStatAccepted}, api.CodeInvalidParams},
		"unknown":       {api.CalendarRespondParams{MessageID: ptr(int64(99999)), Answer: api.PartStatAccepted}, api.CodeNotFound},
		"unknown event": {api.CalendarRespondParams{EventID: ptr(int64(99999)), Answer: api.PartStatAccepted}, api.CodeNotFound},
		"cancelled":     {api.CalendarRespondParams{MessageID: ptr(e.mail(t, request("CANCEL", 2, userLine))), Answer: api.PartStatAccepted}, api.CodeConflict},
		"not invited":   {api.CalendarRespondParams{MessageID: ptr(e.mail(t, request("REQUEST", 1, ""))), Answer: api.PartStatAccepted}, api.CodeConflict},
		"one occurrence": {api.CalendarRespondParams{MessageID: &invite, Answer: api.PartStatAccepted,
			RecurrenceID: ptr("2026-10-15T14:00:00.000Z")}, api.CodeInvalidParams},
	} {
		if _, err := e.c.Calendar().Respond(ctx, &tc.p); code(err) != tc.code {
			t.Errorf("%s: %v", name, err)
		}
	}
	e.dav.Put(e.work+"launch.ics", []byte(strings.Replace(strings.Replace(request("REQUEST", 3, userLine),
		"METHOD:REQUEST\r\n", "", 1), "SEQUENCE:3", "SEQUENCE:3", 1)))
	e.pass(t)
	if _, err := e.c.Calendar().Respond(ctx, &api.CalendarRespondParams{MessageID: &invite, Answer: api.PartStatAccepted}); code(err) != api.CodeConflict {
		t.Errorf("outdated: %v", err)
	}
	if err := e.srv.DB.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.UpdateAccount(ctx, e.acct, store.AccountUpdate{ReadOnly: ptr(true)})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	fresh := e.mail(t, strings.Replace(request("REQUEST", 1, userLine), "launch@example.com", "other@example.com", 1))
	if _, err := e.c.Calendar().Respond(ctx, &api.CalendarRespondParams{MessageID: &fresh, Answer: api.PartStatAccepted}); code(err) != api.CodeConflict {
		t.Errorf("read-only: %v", err)
	}
	if items, _ := e.outbox(t); len(items) != 0 {
		t.Errorf("refusals queued %+v", items)
	}
}

// TestRespondWhenTheServerSchedules: on Google, the patched copy's put is
// the answer; maild mails nothing.
func TestRespondWhenTheServerSchedules(t *testing.T) {
	srv := rpctest.Start(t)
	ctx := t.Context()
	var acct, eventID int64
	err := srv.DB.Tx(ctx, func(tx *store.Tx) error {
		server := store.ServerConfig{Host: "imap.gmail.com", Port: 993, TLS: api.TLSModeTLS, Username: "ann@gmail.example"}
		a, err := tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindGmail, Email: "ann@gmail.example",
			Auth: api.AuthKindOAuth2, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		acct = a.ID
		if err := tx.SetService(ctx, acct, api.ServiceKindCalendar, true, "https://apidata.googleusercontent.com/caldav/v2/"); err != nil {
			return err
		}
		cols, err := tx.ReplaceCollections(ctx, acct, api.CollectionKindCalendar, []store.RemoteCollection{{Href: "/cal/", Name: "Ann"}})
		if err != nil {
			return err
		}
		raw := strings.ReplaceAll(strings.Replace(request("REQUEST", 1, "ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:ann@gmail.example"),
			"METHOD:REQUEST\r\n", "", 1), "user@dav.test", "ann@gmail.example")
		from, to := pimsync.Window(time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC))
		_, err = pimsync.StoreCalendarObject(ctx, tx, store.Object{CollectionID: cols[0].ID, Href: "/cal/launch.ics", ETag: `"7"`,
			Kind: store.ObjectVEvent, Raw: []byte(raw)}, calendar.Options{Local: time.UTC, UserEmails: []string{"ann@gmail.example"}}, from, to)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := srv.DB.EventsByUID(ctx, acct, "launch@example.com")
	if len(rows) != 1 {
		t.Fatalf("stored = %+v", rows)
	}
	eventID = rows[0].ID
	c := srv.Dial(t)
	ev, err := c.Calendar().Respond(ctx, &api.CalendarRespondParams{EventID: &eventID, Answer: api.PartStatAccepted})
	if err != nil || ev.Answer == nil || *ev.Answer != api.PartStatAccepted {
		t.Fatalf("respond = %+v, %v", ev, err)
	}
	if items, _ := srv.DB.ListOutbox(ctx, acct); len(items) != 0 {
		t.Errorf("mailed a reply the server sends: %+v", items)
	}
	ops, _ := srv.DB.DuePIMOps(ctx, acct, srv.DB.Now())
	if len(ops) != 1 || ops[0].Kind != "put" || ops[0].IfMatch != `"7"` || ops[0].Href != "/cal/launch.ics" {
		t.Errorf("ops = %+v", ops)
	}
}
