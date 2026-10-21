package mailsync_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/smtpx/smtpxtest"
	"github.com/frostyard/frostmail/internal/store"
)

// startMicrosoft is maild with a Microsoft account, cy@hotmail.com, not yet
// signed in, on memory IMAP and SMTP servers that take the token "tok-1".
func startMicrosoft(t *testing.T) *sendHarness {
	t.Helper()
	mem := imapxtest.StartMemFullOAuth(t, "tok-1")
	sm := smtpxtest.Start(t, "unused")
	sm.Token = "tok-1"
	ts := tokenServer(t)
	ms := oauth.Microsoft
	ms.AuthURL, ms.TokenURL = "https://login.test/authorize", ts.URL
	cfg := testConfig
	cfg.DraftQuiet = 20 * time.Millisecond
	cfg.SendRetry = 50 * time.Millisecond
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, OAuthEndpoints: map[string]oauth.Endpoint{"microsoft": ms}})
	c := srv.Dial(t)
	ctx := t.Context()
	if _, err := c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	o := mem.DialOptions()
	a, err := c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindMicrosoft, Email: "cy@hotmail.com", DisplayName: "Cy", Auth: api.AuthKindOAuth2,
		IMAP: &api.ServerConfig{Host: o.Host, Port: int64(o.Port), TLS: o.TLS, Username: imapxtest.Username},
		SMTP: sm.Config(imapxtest.Username),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := &sendHarness{harness: &harness{t: t, srv: srv, c: c, acct: a.ID}, mem: mem, smtp: sm}
	h.waitPhase(api.SyncPhaseUnauthorized)
	return h
}

// newMicrosoftHarness is startMicrosoft signed in and idle.
func newMicrosoftHarness(t *testing.T) *sendHarness {
	t.Helper()
	h := startMicrosoft(t)
	h.signInMicrosoft()
	h.waitPhase(api.SyncPhaseIdle)
	return h
}

// signInMicrosoft runs account.authorize and the browser's redirect.
func (h *sendHarness) signInMicrosoft() {
	t, c, ctx := h.t, h.c, h.t.Context()
	t.Helper()
	// No client was set: the built-in one signs in, through localhost.
	res, err := c.Account().Authorize(ctx, &api.AccountAuthorizeParams{ID: h.acct})
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(res.URL)
	if u.Query().Get("client_id") != oauth.MicrosoftClientID || !strings.HasPrefix(u.Query().Get("redirect_uri"), "http://localhost:") {
		t.Fatalf("authorization URL = %s", res.URL)
	}
	resp, err := http.Get(u.Query().Get("redirect_uri") + "?" + url.Values{"code": {"c0de"}, "state": {u.Query().Get("state")}}.Encode())
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

// TestOutlookTransientLoginRetries checks Outlook's "User is authenticated
// but not connected.": the token is good and the mailbox is away for a
// while, so sync retries and never asks for a sign-in.
func TestOutlookTransientLoginRetries(t *testing.T) {
	h := startMicrosoft(t)
	h.mem.Unavailable.Store(3)
	h.signInMicrosoft()
	st := h.waitPhase(api.SyncPhaseOffline)
	if st.Error == nil || !strings.Contains(*st.Error, "not connected") {
		t.Fatalf("offline status = %+v", st)
	}
	h.waitPhase(api.SyncPhaseIdle)
	got, err := h.c.Account().Get(t.Context(), &api.AccountGetParams{ID: h.acct})
	if err != nil || !got.SignedIn {
		t.Fatalf("account = %+v, %v; want signed in", got, err)
	}
}

// TestLoginClearsSignInAgain checks that a login the server takes clears
// "Sign in again", which an earlier refusal left.
func TestLoginClearsSignInAgain(t *testing.T) {
	h := newMicrosoftHarness(t)
	ctx := t.Context()
	if err := h.srv.DB.Tx(ctx, func(tx *store.Tx) error { return tx.SetNeedsReauth(ctx, h.acct, true) }); err != nil {
		t.Fatal(err)
	}
	// A changed account reconnects.
	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, Notify: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "signed in again", func() bool {
		got, err := h.c.Account().Get(ctx, &api.AccountGetParams{ID: h.acct})
		return err == nil && got.SignedIn
	})
}

// TestMicrosoftAccountKeepsColorsLocal checks the profile's NoKeywords:
// Outlook.com keeps no keywords (plan 0010, Phase 1), so a flag color
// reaches the server as \Flagged alone, and the passes that read flags
// back leave the color as it is.
func TestMicrosoftAccountKeepsColorsLocal(t *testing.T) {
	h := newMicrosoftHarness(t)
	ctx := t.Context()
	if _, err := h.mem.User.Append("INBOX", strings.NewReader("From: bob@x.test\r\nSubject: Colors\r\n\r\nHi.\r\n"), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	var id int64
	waitUntil(t, 5*time.Second, "the message synced", func() bool { id = h.ids(h.inbox().ID)["Colors"]; return id != 0 })
	if err := h.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{id}, Changes: api.FlagChanges{Seen: ptr(true), FlagColor: ptr(int64(4))}}); err != nil {
		t.Fatal(err)
	}
	o := h.mem.DialOptions()
	other, err := imapx.Open(ctx, imapx.DialOptions{Host: o.Host, Port: o.Port, TLS: o.TLS, Username: imapxtest.Username, Password: "tok-1", OAuth: true})
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "the flag on the server", func() bool {
		ups, err := other.FetchFlags(ctx, []uint32{1}, 0)
		return err == nil && len(ups) == 1 && ups[0].Flags.Seen && ups[0].Flags.Flagged
	})
	if ups, _ := other.FetchFlags(ctx, []uint32{1}, 0); ups[0].Flags.Color != 1 {
		t.Errorf("the server got color %d's bits; it keeps none", ups[0].Flags.Color)
	}

	// Read elsewhere as unread: the next pass takes that, and keeps the color.
	if err := other.StoreFlags(ctx, []uint32{1}, nil, []string{`\Seen`}); err != nil {
		t.Fatal(err)
	}
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	summary := func() api.MessageSummary {
		rows, err := h.c.Message().Summaries(ctx, &api.MessageSummariesParams{IDs: []int64{id}})
		if err != nil || len(rows) != 1 {
			t.Fatalf("summaries = %v, %v", rows, err)
		}
		return rows[0]
	}
	waitUntil(t, 5*time.Second, "unread from the server", func() bool { return !summary().Flags.Seen })
	if f := summary().Flags; !f.Flagged || f.FlagColor != 4 {
		t.Errorf("after the pass: flags %+v, want flagged with color 4", f)
	}
}

// Outlook.com files a copy of what it sends in Sent itself (found
// 2026-10-16), so maild appends none (Quirks.SavesSent).
func TestMicrosoftSendLeavesTheSentCopyToOutlook(t *testing.T) {
	h := newMicrosoftHarness(t)
	ctx := t.Context()
	d := h.draft("Hello", []api.Address{bob}, nil, nil)
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent)
	if n := len(h.smtp.Messages()); n != 1 {
		t.Fatalf("SMTP server got %d messages, want 1", n)
	}
	if n := len(h.serverMessages("Sent")); n != 0 {
		t.Errorf("maild appended %d Sent copies; Outlook.com files its own", n)
	}
}

// Outlook.com gives what it sends a Message-ID of its own and keeps the
// client's in X-Microsoft-Original-Message-ID (found 2026-10-16). A send
// interrupted after delivery is found in Sent by that header, and is not
// sent again.
func TestInterruptedMicrosoftSendFoundUnderItsOriginalID(t *testing.T) {
	h := newMicrosoftHarness(t)
	ctx := t.Context()
	release := h.smtp.Hold()
	d := h.draft("Already", []api.Address{bob}, nil, nil)
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.smtp.WaitHeld(t, 1, 10*time.Second)
	row, err := h.srv.DB.GetOutbox(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	copyRaw := "Message-ID: <DM6PR19MB3404A@x.prod.outlook.com>\r\nX-Microsoft-Original-Message-ID: <" + row.MessageID +
		">\r\nSubject: Already\r\n\r\nHello\r\n"
	if _, err := h.mem.User.Append("Sent", strings.NewReader(copyRaw), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := h.srv.Sync.Restart(ctx, h.acct); err != nil {
		t.Fatal(err)
	}
	release(false)
	h.waitOutbox(item.ID, api.OutboxStateSent)
	if n := len(h.smtp.Messages()); n != 0 {
		t.Errorf("SMTP server got %d more messages, want none", n)
	}
	if n := len(h.serverMessages("Sent")); n != 1 {
		t.Errorf("Sent holds %d messages, want Outlook's one", n)
	}
}
