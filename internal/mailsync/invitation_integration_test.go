//go:build integration

package mailsync_test

// An invitation from test5 reaches test3 through Postfix and Dovecot; test3
// answers from Frostmail; Radicale, which does not schedule, gets the
// accepted event, and test5 receives exactly one REPLY (plan 0007, Phase
// 4). test3's INBOX and calendar are used by no other test.

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/itip"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/smtpx"
)

func TestInvitationAnsweredOnce(t *testing.T) {
	host := itHost(t)
	ctx := t.Context()
	insecure := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // the test container's certificate
	}}
	cfg := itConfig()
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, PIM: &pimsync.Config{HTTP: insecure, Interval: time.Hour}})
	h := &harness{t: t, srv: srv, c: srv.Dial(t)}
	if _, err := h.c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	const user = "test3@mailtest.test"
	imapCfg := api.ServerConfig{Host: host, Port: 993, TLS: api.TLSModeTLS, Username: user}
	smtpCfg := api.ServerConfig{Host: host, Port: 587, TLS: api.TLSModeStartTLS, Username: user}
	a, err := h.c.Account().Create(ctx, &api.AccountCreateParams{Kind: api.AccountKindIMAP, Email: user, DisplayName: "Test Three",
		Auth: api.AuthKindPassword, IMAP: &imapCfg, SMTP: &smtpCfg})
	if err != nil {
		t.Fatal(err)
	}
	h.acct = a.ID
	if err := h.c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: itPassword}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	start := "https://" + host + ":5232/"
	if _, err := h.c.Account().SetService(ctx, &api.AccountSetServiceParams{ID: a.ID, Service: api.ServiceKindCalendar,
		Enabled: true, URL: &start}); err != nil {
		t.Fatal(err)
	}
	if err := srv.PIM.Pass(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	// test5 invites test3.
	var nonce [6]byte
	_, _ = rand.Read(nonce[:])
	uid := "it-" + hex.EncodeToString(nonce[:]) + "@mailtest.test"
	day := time.Now().UTC().AddDate(0, 0, 1).Format("20060102")
	ics := strings.Join([]string{"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//frostmail//integration//EN", "METHOD:REQUEST",
		"BEGIN:VEVENT", "UID:" + uid, "SEQUENCE:0", "DTSTAMP:" + time.Now().UTC().Format("20060102T150405Z"),
		"DTSTART:" + day + "T150000Z", "DTEND:" + day + "T153000Z", "SUMMARY:Integration sync " + uid[:9],
		"ORGANIZER;CN=Test Five:mailto:test5@mailtest.test",
		"ATTENDEE;CN=Test Five;PARTSTAT=ACCEPTED:mailto:test5@mailtest.test",
		"ATTENDEE;CN=Test Three;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:test3@mailtest.test",
		"END:VEVENT", "END:VCALENDAR"}, "\r\n") + "\r\n"
	subject := "Invitation: Integration sync " + uid[:9]
	msg := "From: Test Five <test5@mailtest.test>\r\nTo: test3@mailtest.test\r\nSubject: " + subject + "\r\n" +
		"Date: " + time.Now().Format(time.RFC1123Z) + "\r\nMessage-ID: <" + uid + ">\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=b\r\n\r\n--b\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" +
		"You are invited.\r\n--b\r\nContent-Type: text/calendar; charset=UTF-8; method=REQUEST\r\n\r\n" + ics + "--b--\r\n"
	err = smtpx.Send(ctx, smtpx.Options{Host: host, Port: 587, TLS: api.TLSModeStartTLS, Username: "test5@mailtest.test",
		Password: itPassword, InsecureSkipVerify: true}, smtpx.Envelope{From: "test5@mailtest.test", To: []string{user}},
		int64(len(msg)), strings.NewReader(msg))
	if err != nil {
		t.Fatal(err)
	}

	// It reaches Frostmail's INBOX and reads as an invitation.
	inbox := h.inbox().ID
	var messageID int64
	waitUntil(t, 60*time.Second, "the invitation in test3's INBOX", func() bool {
		if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
			return false
		}
		for _, s := range inboxSummaries(t, h, inbox) {
			if s.Subject == subject {
				messageID = s.ID
				return true
			}
		}
		return false
	})
	inv, err := h.c.Calendar().Invitation(ctx, &api.CalendarInvitationParams{MessageID: messageID})
	if err != nil || inv.Method != api.ITIPMethodRequest || !inv.CanRespond || inv.EventID != nil {
		t.Fatalf("invitation = %+v, %v", inv, err)
	}

	// test3 accepts: the event goes into Radicale, the reply into the outbox.
	ev, err := h.c.Calendar().Respond(ctx, &api.CalendarRespondParams{MessageID: &messageID, Answer: api.PartStatAccepted})
	if err != nil || ev.ID == 0 || ev.Answer == nil || *ev.Answer != api.PartStatAccepted {
		t.Fatalf("respond = %+v, %v", ev, err)
	}
	items, err := h.c.Outbox().List(ctx, &api.OutboxListParams{AccountID: &h.acct})
	if err != nil || len(items) != 1 {
		t.Fatalf("outbox = %+v, %v", items, err)
	}
	h.waitFor(30*time.Second, "the reply sent", func(_ api.EventEnvelope, e api.Event) bool {
		o, ok := e.(api.OutboxChanged)
		return ok && o.ID == items[0].ID && o.State == api.OutboxStateSent
	})
	if err := srv.PIM.Pass(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if ops, _ := srv.DB.DuePIMOps(ctx, a.ID, srv.DB.Now()); len(ops) != 0 {
		t.Errorf("the event did not reach Radicale: %+v", ops)
	}
	if inv, _ := h.c.Calendar().Invitation(ctx, &api.CalendarInvitationParams{MessageID: messageID}); inv == nil ||
		inv.EventID == nil || inv.Answer == nil || *inv.Answer != api.PartStatAccepted {
		t.Errorf("the invitation after answering = %+v", inv)
	}

	// test5 receives exactly one reply, from test3, accepted: maild queued
	// one, and Radicale sends none of its own.
	var replies [][]byte
	waitUntil(t, 30*time.Second, "the reply in test5's INBOX", func() bool {
		replies = replies[:0]
		for _, raw := range mailboxMessages(t, host, "test5@mailtest.test", "INBOX", "") {
			if src, err := itip.FromMessage(raw); err == nil && bytes.Contains(src, []byte("UID:"+uid)) &&
				bytes.Contains(src, []byte("METHOD:REPLY")) {
				replies = append(replies, raw)
			}
		}
		return len(replies) > 0
	})
	if len(replies) != 1 {
		t.Fatalf("test5 received %d replies", len(replies))
	}
	src, err := itip.FromMessage(replies[0])
	if err != nil {
		t.Fatal(err)
	}
	reply, err := itip.Parse(src, calendar.Options{Local: time.UTC})
	if err != nil || reply.Method != itip.MethodReply || len(reply.Main().Attendees) != 1 ||
		reply.Main().Attendees[0].Email != user || reply.Main().Attendees[0].PartStat != "accepted" {
		t.Errorf("the reply = %+v, %v\n%s", reply, err, src)
	}
}

// inboxSummaries lists a mailbox's messages.
func inboxSummaries(t *testing.T, h *harness, mailbox int64) []api.MessageSummary {
	t.Helper()
	ctx := t.Context()
	v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &mailbox}})
	if err != nil {
		t.Fatal(err)
	}
	defer h.c.View().Close(ctx, &api.ViewCloseParams{ID: v.ID})
	rows, err := h.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: v.Count})
	if err != nil {
		t.Fatal(err)
	}
	return rows
}
