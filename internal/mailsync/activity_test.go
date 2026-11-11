package mailsync_test

import (
	"encoding/json/v2"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-smtp"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/smtpx/smtpxtest"
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

// newestActivity is the newest entry's seq, 0 when there is none.
func (h *harness) newestActivity() int64 {
	entries := h.srv.Activity.Recent(1)
	if len(entries) == 0 {
		return 0
	}
	return entries[0].Seq
}

// The emit points of ADR-0055: each test below fails when its point stops
// recording. IDLE needs a server that speaks it, which the memory server
// does not: TestActivityAgainstDovecot holds that one.

func TestActivityOfASession(t *testing.T) {
	mem := imapxtest.StartMem(t)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	if e := h.waitActivity(api.ActivityKindConnect, nil); e.Level != api.ActivityLevelDebug || *e.Service != api.ActivityServerIMAP {
		t.Errorf("connect entry = %+v", e)
	}
	if e := h.waitActivity(api.ActivityKindList, nil); e.N == nil || *e.N < 1 {
		t.Errorf("list entry = %+v", e)
	}
}

// A server that cannot be reached is an offline entry, retried.
func TestActivityOfAnOfflineServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, imapx.DialOptions{Host: "127.0.0.1", Port: port, TLS: api.TLSModeInsecure, Username: imapxtest.Username}, imapxtest.Password)
	e := h.waitActivity(api.ActivityKindOffline, nil)
	if e.Level != api.ActivityLevelWarn || *e.Service != api.ActivityServerIMAP || *e.ErrorClass != api.ErrorClassNetwork {
		t.Errorf("offline entry = %+v, want a warning, IMAP, network", e)
	}
}

// A pass counts the messages whose flags changed, not the rows the server
// reported: a server without CONDSTORE reports every row on every pass.
func TestActivityCountsTheFlagsThatChanged(t *testing.T) {
	mem := imapxtest.StartMem(t)
	for _, subject := range []string{"a", "b", "c"} {
		appendNew(t, mem, "INBOX", subject)
	}
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	inbox := h.inbox()

	other, err := imapx.Open(ctx, mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}
	if err := other.StoreFlags(ctx, []uint32{1}, []string{`\Flagged`}, nil); err != nil {
		t.Fatal(err)
	}
	before := h.newestActivity()
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	e := h.waitActivity(api.ActivityKindMailboxsynced, func(e api.ActivityEntry) bool {
		return e.Seq > before && *e.MailboxID == inbox.ID && *e.Changed > 0
	})
	if *e.Changed != 1 || e.Level != api.ActivityLevelInfo {
		t.Errorf("the pass of one flag change = %+v, want 1 changed, info", e)
	}

	// Nothing changed since: the server reports the same rows, and the pass
	// is debug.
	before = e.Seq
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	e = h.waitActivity(api.ActivityKindMailboxsynced, func(e api.ActivityEntry) bool {
		return e.Seq > before && *e.MailboxID == inbox.ID
	})
	if *e.Changed != 0 || e.Level != api.ActivityLevelDebug {
		t.Errorf("a pass that changed nothing = %+v, want 0 changed, debug", e)
	}
}

func TestActivityOfPrefetch(t *testing.T) {
	mem := imapxtest.StartMem(t)
	if _, err := mem.User.Append("INBOX", strings.NewReader(plain("bob@example.test", "Recent")),
		&imap.AppendOptions{Time: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig
	cfg.NoPrefetch = false
	h := newHarnessWith(t, mem.DialOptions(), imapxtest.Password, cfg)
	if e := h.waitActivity(api.ActivityKindFetched, nil); *e.N != 1 || e.Level != api.ActivityLevelDebug {
		t.Errorf("fetched entry = %+v, want 1 body, debug", e)
	}
}

func TestActivityOfADraftCopy(t *testing.T) {
	h := newSendHarness(t, 0)
	h.draft("Saved", []api.Address{bob}, nil, nil)
	if err := h.c.Sync().Now(t.Context(), &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	if e := h.waitActivity(api.ActivityKindDraftsaved, nil); *e.N != 1 || e.Level != api.ActivityLevelDebug {
		t.Errorf("draft entry = %+v, want 1 draft, debug", e)
	}
}

// A temporary refusal is a warning with its code, and the retry goes out.
func TestActivityOfATemporarySendFailure(t *testing.T) {
	h := newSendHarness(t, 0)
	h.smtp.RefuseRecipients(&smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 2, 1}, Message: "try later"})
	d := h.draft("Later", []api.Address{bob}, nil, nil)
	item, err := h.c.Draft().Send(t.Context(), &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent)
	e := h.waitActivity(api.ActivityKindSendfailed, nil)
	if e.Level != api.ActivityLevelWarn || *e.Service != api.ActivityServerSMTP || *e.ErrorClass != api.ErrorClassRefused || *e.Code != 451 {
		t.Errorf("send failed entry = %+v, want a warning, SMTP, refused, 451", e)
	}
	sent := h.waitActivity(api.ActivityKindSent, nil)
	if sent.Seq <= e.Seq {
		t.Errorf("sent entry %d came before the failure %d", sent.Seq, e.Seq)
	}
}

// Only a send past the undo delay is scheduled.
func TestActivityOfSendLater(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	now := h.draft("Now", []api.Address{bob}, nil, nil)
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: now.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent)
	for _, e := range h.srv.Activity.Recent(activity.MaxRecent) {
		if e.Kind == api.ActivityKindSendlater {
			t.Errorf("a send now recorded %+v", e)
		}
	}

	later := h.draft("Later", []api.Address{bob}, nil, nil)
	at := time.Now().Add(time.Hour)
	if _, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: later.ID, SendAt: &at}); err != nil {
		t.Fatal(err)
	}
	if e := h.waitActivity(api.ActivityKindSendlater, nil); *e.N != 1 || e.Level != api.ActivityLevelInfo || *e.AccountID != h.acct {
		t.Errorf("send later entry = %+v", e)
	}
}

func TestActivityOfRulesAndAnnouncements(t *testing.T) {
	r := newRulesHarness(t)
	r.rule("Read", cond(api.ConditionFieldSubject, api.ConditionOpContains, "receipt"), api.RuleAction{Kind: api.RuleActionKindRead})

	r.deliver("Ann <ann@x.test>", "Lunch")
	r.settledWith("Lunch")
	if e := r.waitActivity(api.ActivityKindAnnounced, nil); *e.N != 1 || e.Level != api.ActivityLevelInfo {
		t.Errorf("announced entry = %+v, want 1 message, info", e)
	}

	r.deliver("Ann <ann@x.test>", "Your receipt")
	if e := r.waitActivity(api.ActivityKindRulesapplied, nil); *e.N != 1 || e.Level != api.ActivityLevelInfo {
		t.Errorf("rules entry = %+v, want 1 message, info", e)
	}
}

func TestActivityOfAReminder(t *testing.T) {
	r := newRulesHarness(t)
	ctx := t.Context()
	r.deliver("Ann <ann@x.test>", "Remind me")
	r.settledWith("Remind me")
	at := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := r.c.Message().Remind(ctx, &api.MessageRemindParams{IDs: []int64{r.summary("Remind me").ID}, At: &at}); err != nil {
		t.Fatal(err)
	}
	if err := r.srv.Sync.FireReminders(ctx, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if e := r.waitActivity(api.ActivityKindRemindfired, nil); *e.N != 1 || e.Level != api.ActivityLevelInfo {
		t.Errorf("reminder entry = %+v, want 1 message, info", e)
	}
}

func TestActivityOfErasing(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	t0 := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	clock := newTestClock(t0.AddDate(0, 0, -30))
	appendNew(t, mem, "Junk", "old")
	h := newHarnessAt(t, mem.DialOptions(), imapxtest.Password, clock.Now)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	junk := h.mailboxByPath("Junk")
	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, EraseJunkDays: ptr(int64(7))}); err != nil {
		t.Fatal(err)
	}
	clock.Set(t0)
	if err := h.srv.Sync.EraseOld(ctx, t0); err != nil {
		t.Fatal(err)
	}
	if e := h.waitActivity(api.ActivityKindErased, nil); *e.N != 1 || *e.MailboxID != junk.ID || e.Level != api.ActivityLevelInfo {
		t.Errorf("erased entry = %+v, want 1 message of mailbox %d, info", e, junk.ID)
	}
}

// A refresh of a signed-in account's token records its entry.
func TestActivityOfATokenRefresh(t *testing.T) {
	mem := imapxtest.StartMemFullOAuth(t, "tok-1")
	sm := smtpxtest.Start(t, "unused")
	sm.Token = "tok-1"
	ts := tokenServer(t)
	cfg := testConfig
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg,
		OAuthEndpoints: map[string]oauth.Endpoint{"google": {AuthURL: "https://accounts.test/auth", TokenURL: ts.URL, Scopes: oauth.Google.Scopes}}})
	c := srv.Dial(t)
	ctx := t.Context()
	if _, err := c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Oauth().SetClient(ctx, &api.OauthSetClientParams{Provider: api.OAuthProviderGoogle, ClientID: "cid", ClientSecret: ptr("cs")}); err != nil {
		t.Fatal(err)
	}
	o := mem.DialOptions()
	a, err := c.Account().Create(ctx, &api.AccountCreateParams{Kind: api.AccountKindGmail, Email: "ann@gmail.com", Auth: api.AuthKindOAuth2,
		IMAP: &api.ServerConfig{Host: o.Host, Port: int64(o.Port), TLS: o.TLS, Username: imapxtest.Username}, SMTP: sm.Config(imapxtest.Username)})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, srv: srv, c: c, acct: a.ID}
	h.waitPhase(api.SyncPhaseUnauthorized)
	res, err := c.Account().Authorize(ctx, &api.AccountAuthorizeParams{ID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(res.URL)
	if err != nil {
		t.Fatal(err)
	}
	back := u.Query().Get("redirect_uri") + "?" + url.Values{"code": {"c0de"}, "state": {u.Query().Get("state")}}.Encode()
	resp, err := http.Get(back)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	h.waitPhase(api.SyncPhaseIdle)

	srv.OAuth.Invalidate(a.ID)
	if _, err := srv.OAuth.AccessToken(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if e := h.waitActivity(api.ActivityKindTokenrefreshed, nil); e.Level != api.ActivityLevelDebug || *e.Service != api.ActivityServerOauth || *e.AccountID != a.ID {
		t.Errorf("refresh entry = %+v", e)
	}
}
