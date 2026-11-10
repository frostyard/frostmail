package activity_test

import (
	"context"
	"crypto/x509"
	"encoding/json/v2"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/graph"
	"github.com/frostyard/frostmail/internal/gtasks"
	"github.com/frostyard/frostmail/internal/oauth"
)

// clock is a settable time for a Hub.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newHub() (*activity.Hub, *clock) {
	c := &clock{t: time.Date(2026, 11, 10, 9, 41, 0, 0, time.UTC)}
	return activity.New(c.now), c
}

func TestHubNumbersKeepsAndReturnsTheNewest(t *testing.T) {
	h, c := newHub()
	for i := range activity.Keep + 5 {
		c.t = c.t.Add(time.Second) // one a second: no account limit
		h.Record(activity.Pushed(1, i))
	}
	got := h.Recent(activity.MaxRecent)
	if len(got) != activity.MaxRecent {
		t.Fatalf("Recent(%d) = %d entries", activity.MaxRecent, len(got))
	}
	last := got[len(got)-1]
	if last.Seq != activity.Keep+5 || *last.N != activity.Keep+4 {
		t.Errorf("newest = seq %d, n %d", last.Seq, *last.N)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Seq != got[i-1].Seq+1 {
			t.Fatalf("not oldest first at %d: %d after %d", i, got[i].Seq, got[i-1].Seq)
		}
	}
	if n := len(h.Recent(0)); n != activity.DefaultRecent {
		t.Errorf("Recent(0) = %d entries, want %d", n, activity.DefaultRecent)
	}
	if n := len(h.Recent(10_000)); n != activity.MaxRecent {
		t.Errorf("Recent(10000) = %d entries, want %d", n, activity.MaxRecent)
	}
}

func TestNilHubRecordsNothing(t *testing.T) {
	var h *activity.Hub
	h.Record(activity.Pushed(1, 1))
	if got := h.Recent(5); len(got) != 0 {
		t.Errorf("nil hub's Recent = %v", got)
	}
}

func TestAnAccountPastItsLimitGetsOneDroppedEntry(t *testing.T) {
	h, c := newHub()
	for range activity.PerSecond + 7 {
		h.Record(activity.Pushed(1, 1))
	}
	h.Record(activity.Pushed(2, 1))                          // another account is not limited by 1
	h.Record(activity.Sent(1, 0))                            // nor another kind
	h.Record(activity.SendFailed(1, errors.New("x"), false)) // an error is always kept
	if n := len(h.Recent(500)); n != activity.PerSecond+3 {
		t.Fatalf("kept %d entries in the first second, want %d", n, activity.PerSecond+3)
	}
	c.t = c.t.Add(time.Second)
	h.Record(activity.Pushed(1, 1))
	got := h.Recent(2)
	if got[0].Kind != api.ActivityKindDropped || *got[0].N != 7 || *got[0].AccountID != 1 {
		t.Errorf("before the next entry = %+v, want 7 dropped for account 1", got[0])
	}
	if got[1].Kind != api.ActivityKindPushed {
		t.Errorf("then %s, want the entry", got[1].Kind)
	}
}

func TestASubscriberBehindLosesEntriesAndLearnsHowMany(t *testing.T) {
	h, c := newHub()
	sub, seq := h.Subscribe()
	defer sub.Close()
	if seq != 0 {
		t.Errorf("seq of an empty hub = %d", seq)
	}
	for i := range activity.SubscriberBuffer + 3 {
		c.t = c.t.Add(time.Second)
		h.Record(activity.Pushed(1, i))
	}
	for range activity.SubscriberBuffer {
		<-sub.C()
	}
	c.t = c.t.Add(time.Second)
	h.Record(activity.Pushed(1, 99))
	d, e := <-sub.C(), <-sub.C()
	if d.Kind != api.ActivityKindDropped || *d.N != 3 || d.Seq != 0 {
		t.Errorf("after catching up = %+v, want 3 dropped with seq 0", d)
	}
	if *e.N != 99 {
		t.Errorf("then n %d, want the new entry", *e.N)
	}
	sub.Close()
	sub.Close() // twice is fine
	if _, ok := <-sub.C(); ok {
		t.Error("a closed subscription's channel is open")
	}
}

// classified is a stand-in for the HTTP clients' errors.
func TestClassifyReadsTypesNotText(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		class api.ErrorClass
		code  int64
	}{
		{"deadline", fmt.Errorf("dial: %w", context.DeadlineExceeded), api.ErrorClassTimeout, 0},
		{"refused connection", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, api.ErrorClassNetwork, 0},
		{"unknown authority", x509.UnknownAuthorityError{}, api.ErrorClassTLS, 0},
		{"dav 412", &davx.StatusError{Code: 412, Body: "secret"}, api.ErrorClassRefused, 412},
		{"dav 401", &davx.StatusError{Code: 401}, api.ErrorClassAuth, 401},
		{"graph 429", &graph.StatusError{Status: 429}, api.ErrorClassQuota, 429},
		{"tasks 503", &gtasks.APIError{Status: 503}, api.ErrorClassNetwork, 503},
		{"invalid grant", &oauth.TokenError{Code: "invalid_grant"}, api.ErrorClassAuth, 0},
		{"other token error", &oauth.TokenError{Code: "temporarily_unavailable"}, api.ErrorClassInternal, 0},
		{"plain", errors.New("timeout reading from the server"), api.ErrorClassInternal, 0},
	}
	for _, tc := range cases {
		class, code, _ := activity.Classify(tc.err)
		var got int64
		if code != nil {
			got = *code
		}
		if class != tc.class || got != tc.code {
			t.Errorf("%s: Classify = %s, %d; want %s, %d", tc.name, class, got, tc.class, tc.code)
		}
	}
}

func TestSignInNeededIsAuthWhenTheErrorDoesNotSay(t *testing.T) {
	h, _ := newHub()
	h.Record(activity.SignInNeeded(1, api.ActivityServerCarddav, errors.New("davx: unauthorized")))
	if e := h.Recent(1)[0]; *e.ErrorClass != api.ErrorClassAuth {
		t.Errorf("class = %s, want auth", *e.ErrorClass)
	}
}

// names is a client's names for IDs.
type names map[int64]string

func (n names) Account(id int64) string { return n[id] }
func (n names) Mailbox(id int64) string { return n[100+id] }

func TestLineWritesEachKind(t *testing.T) {
	h, c := newHub()
	refused := &davx.StatusError{Code: 412}
	entries := []activity.Entry{
		activity.Connected(1, 400*time.Millisecond),
		activity.Listed(1, 12, 0),
		activity.MailboxSynced(1, 7, 3, 12, 0, 400*time.Millisecond),
		activity.MailboxSynced(1, 7, 0, 0, 0, 0),
		activity.IdleWoke(1, 7),
		activity.Fetched(1, 20, 3<<20, 800*time.Millisecond),
		activity.Pushed(1, 1),
		activity.DraftSaved(1, 2),
		activity.Sent(1, 1200*time.Millisecond),
		activity.SendFailed(1, refused, false),
		activity.SendLater(1, 1),
		activity.RulesApplied(1, 4),
		activity.RemindFired(1, 2),
		activity.Announced(1, 3),
		activity.Erased(1, 7, 40),
		activity.PIMSynced(1, api.ActivityServerCarddav, 2, 300*time.Millisecond),
		activity.PIMRefused(1, api.ActivityServerCaldav, refused),
		activity.Refreshed(1, nil),
		activity.Refreshed(1, &oauth.TokenError{Code: "invalid_grant"}),
		activity.Offline(1, api.ActivityServerIMAP, &net.OpError{Op: "dial", Err: errors.New("x")}),
		activity.Failed(1, api.ActivityServerIMAP, errors.New("x")),
	}
	want := []string{
		"connected to IMAP (0.4 s)",
		"listed 12 mailboxes",
		"Inbox synced: 3 new, 12 changed (0.4 s)",
		"Inbox synced, no changes",
		"Inbox: the server reported changes (IDLE)",
		"fetched 20 message bodies, 3.0 MB (0.8 s)",
		"sent 1 offline change to the server",
		"saved 2 drafts on the server",
		"sent a message (1.2 s)",
		"send failed: refused (412)",
		"scheduled 1 message to send later",
		"rules acted on 4 messages",
		"Remind Me brought back 2 messages",
		"announced 3 new messages",
		"Inbox: erased 40 old messages",
		"CardDAV pass: changes in 2 collections (0.3 s)",
		"CalDAV refused a change: refused (412)",
		"refreshed the sign-in token",
		"the sign-in service refused the sign-in (auth); sign in again",
		"IMAP cannot be reached (network); retrying",
		"IMAP failed (internal); retrying",
	}
	for _, e := range entries {
		c.t = c.t.Add(time.Second)
		h.Record(e)
	}
	got := h.Recent(len(entries))
	for i, e := range got {
		src, text := activity.Line(e, names{1: "Work", 107: "Inbox"})
		if src != "Work" || text != want[i] {
			t.Errorf("entry %d (%s) = %q, %q; want Work, %q", i, e.Kind, src, text, want[i])
		}
	}
	if src, _ := activity.Line(api.ActivityEntry{Kind: api.ActivityKindDropped}, names{}); src != "maild" {
		t.Errorf("an entry of no account's source = %q", src)
	}
}

// TestErrorTextNeverReachesAnEntry is ADR-0055's canary at the
// constructors: every error type a server's words can reach, carrying a
// canary, through every constructor that takes an error.
func TestErrorTextNeverReachesAnEntry(t *testing.T) {
	const canary = "CANARY-5f3a"
	errs := []error{
		&davx.StatusError{Method: "PUT", URL: "https://dav.test/" + canary, Code: 412, Body: canary},
		&graph.StatusError{Status: 400, Code: canary, Message: canary},
		&gtasks.APIError{Status: 403, Message: canary},
		&oauth.TokenError{Code: "invalid_grant", Description: canary},
		&oauth.ChallengeError{Status: "401", Scope: canary},
		&net.OpError{Op: "dial", Net: "tcp", Err: errors.New(canary)},
		fmt.Errorf("imap: NO [ALERT] %s: %w", canary, context.DeadlineExceeded),
		errors.New(canary),
	}
	h, c := newHub()
	for _, err := range errs {
		for _, e := range []activity.Entry{
			activity.SendFailed(1, err, true), activity.PIMRefused(1, api.ActivityServerCaldav, err),
			activity.SignInNeeded(1, api.ActivityServerOauth, err), activity.Offline(1, api.ActivityServerIMAP, err),
			activity.Failed(1, api.ActivityServerSMTP, err), activity.Lost(1, api.ActivityServerGraph, err),
			activity.Refreshed(1, err),
		} {
			c.t = c.t.Add(time.Second)
			h.Record(e)
		}
	}
	out, err := json.Marshal(h.Recent(activity.MaxRecent))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), canary) {
		t.Errorf("an entry holds the canary: %s", out)
	}
}
