package mailsync_test

import (
	"encoding/json/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-smtp"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
)

// waitActivity waits until the server has recorded an entry of kind that
// match accepts.
func (h *harness) waitActivity(kind api.ActivityKind, match func(api.ActivityEntry) bool) api.ActivityEntry {
	h.t.Helper()
	var found api.ActivityEntry
	waitUntil(h.t, 10*time.Second, "an activity entry "+string(kind), func() bool {
		for _, e := range h.srv.Activity.Recent(activity.MaxRecent) {
			if e.Kind == kind && (match == nil || match(e)) {
				found = e
				return true
			}
		}
		return false
	})
	return found
}

// TestActivityHoldsNoCanary is ADR-0055's canary run, end to end: mail
// whose subject, body and addresses carry canaries is synced, flagged,
// sent, refused by the SMTP server with a canary in its reply, and the
// account's password is changed to a canary the server refuses. Every
// recorded entry names its account, and none holds a canary or a secret.
func TestActivityHoldsNoCanary(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	const (
		subject  = "CANARY-subject-91"
		body     = "CANARY-body-91"
		from     = "canary-from-91@x.test"
		to       = "canary-to-91@x.test"
		refusal  = "CANARY-smtp-reply-91"
		password = "CANARY-password-91"
	)

	// Sync: a new message in INBOX.
	raw := "From: Canary <" + from + ">\r\nTo: ann@x.test\r\nSubject: " + subject +
		"\r\nMessage-ID: <canary91@x.test>\r\nDate: Mon, 09 Nov 2026 09:41:00 +0000\r\n\r\n" + body + "\r\n"
	if _, err := h.mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	inbox := h.inbox()
	h.waitActivity(api.ActivityKindMailboxsynced, func(e api.ActivityEntry) bool {
		return *e.MailboxID == inbox.ID && e.Added != nil && *e.Added >= 1
	})

	// A flag change pushed to the server.
	v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := h.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: v.Count})
	if err != nil || !slices.ContainsFunc(rows, func(r api.MessageSummary) bool { return r.Subject == subject }) {
		t.Fatalf("inbox rows = %+v, %v", rows, err)
	}
	if err := h.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{rows[0].ID}, Changes: api.FlagChanges{Flagged: ptr(true)}}); err != nil {
		t.Fatal(err)
	}
	h.waitActivity(api.ActivityKindPushed, nil)

	// A send the server refuses for good, then one it takes.
	h.smtp.RefuseRecipients(&smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: refusal})
	d := h.draft(subject, []api.Address{{Name: "Canary", Address: to}}, nil, nil)
	content := d.Content
	content.HTML = "<p>" + body + "</p>"
	if _, err := h.c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: content}); err != nil {
		t.Fatal(err)
	}
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateFailed)
	failed := h.waitActivity(api.ActivityKindSendfailed, nil)
	if failed.Level != api.ActivityLevelError || *failed.ErrorClass != api.ErrorClassRefused || *failed.Code != 550 {
		t.Errorf("send failed entry = %+v, want an error, refused, 550", failed)
	}
	if err := h.c.Outbox().Retry(ctx, &api.OutboxRetryParams{ID: item.ID}); err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent)
	h.waitActivity(api.ActivityKindSent, nil)

	// A password the server refuses.
	if err := h.c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: h.acct, Password: password}); err != nil {
		t.Fatal(err)
	}
	h.waitActivity(api.ActivityKindSigninneeded, func(e api.ActivityEntry) bool {
		return *e.ErrorClass == api.ErrorClassAuth
	})

	entries := h.srv.Activity.Recent(activity.MaxRecent)
	for _, e := range entries {
		if e.AccountID == nil || *e.AccountID != h.acct {
			t.Errorf("entry %d (%s) names account %v, want %d", e.Seq, e.Kind, e.AccountID, h.acct)
		}
	}
	out, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{subject, body, from, to, refusal, password, "canary91",
		imapxtest.Password, "ann@x.test", "Ann Example", "INBOX"} {
		if strings.Contains(string(out), secret) {
			t.Errorf("an activity entry holds %q: %s", secret, out)
		}
	}
	t.Logf("%d entries, kinds %v", len(entries), kinds(entries))
}

func kinds(entries []api.ActivityEntry) []api.ActivityKind {
	var out []api.ActivityKind
	for _, e := range entries {
		if !slices.Contains(out, e.Kind) {
			out = append(out, e.Kind)
		}
	}
	return out
}
