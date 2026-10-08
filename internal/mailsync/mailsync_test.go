package mailsync_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/notify"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// harness is maild with sync on, an IMAP server, and a subscribed client.
type harness struct {
	t      *testing.T
	srv    *rpctest.Server
	c      *api.Client
	acct   int64
	events []api.EventEnvelope
}

var testConfig = mailsync.Config{PollInterval: time.Hour, IdleMax: time.Minute, MinBackoff: 50 * time.Millisecond, Chunk: 2}

// newHarness starts maild with sync, subscribes, and adds an account for
// opts with password.
func newHarness(t *testing.T, opts imapx.DialOptions, password string) *harness {
	t.Helper()
	return newHarnessWith(t, opts, password, testConfig)
}

// newHarnessWith is newHarness with a sync configuration.
func newHarnessWith(t *testing.T, opts imapx.DialOptions, password string, cfg mailsync.Config) *harness {
	t.Helper()
	cfg.InsecureSkipVerify = opts.InsecureSkipVerify
	var log *slog.Logger
	if os.Getenv("FROSTMAIL_TEST_LOG") != "" {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, Log: log})
	c := srv.Dial(t)
	ctx := t.Context()
	if _, err := c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	server := api.ServerConfig{Host: opts.Host, Port: int64(opts.Port), TLS: opts.TLS, Username: opts.Username}
	a, err := c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: "test@mailtest.test", Auth: api.AuthKindPassword, IMAP: &server, SMTP: &server,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: password}); err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, srv: srv, c: c, acct: a.ID}
}

// waitFor reads events until match returns true or the timeout passes.
func (h *harness) waitFor(timeout time.Duration, what string, match func(api.EventEnvelope, api.Event) bool) {
	h.t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case env, ok := <-h.c.Notifications():
			if !ok {
				h.t.Fatalf("connection closed waiting for %s: %v", what, h.c.Err())
			}
			h.events = append(h.events, env)
			ev, err := api.DecodeEvent(env.Event, env.Data)
			if err != nil {
				h.t.Fatalf("decode %s: %v", env.Event, err)
			}
			if match(env, ev) {
				return
			}
		case <-deadline:
			// What did arrive, for the rare timeouts under load.
			for _, e := range h.events {
				h.t.Logf("event %s %s", e.Event, e.Data)
			}
			h.t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
	}
}

func (h *harness) waitPhase(phase api.SyncPhase) api.SyncStatus {
	h.t.Helper()
	var got api.SyncStatus
	h.waitFor(10*time.Second, "sync phase "+string(phase), func(_ api.EventEnvelope, ev api.Event) bool {
		p, ok := ev.(api.SyncProgress)
		if ok && p.Status.Phase == phase && p.Status.AccountID == h.acct {
			got = p.Status
			return true
		}
		return false
	})
	return got
}

func (h *harness) inbox() api.Mailbox {
	h.t.Helper()
	list, err := h.c.Mailbox().List(h.t.Context(), &api.MailboxListParams{AccountID: &h.acct})
	if err != nil {
		h.t.Fatal(err)
	}
	for _, mb := range list {
		if mb.Role == api.MailboxRoleInbox {
			return mb
		}
	}
	h.t.Fatalf("no INBOX in %+v", list)
	return api.Mailbox{}
}

func (h *harness) subjects(mailboxID int64) []string {
	h.t.Helper()
	ctx := h.t.Context()
	v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &mailboxID}})
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = h.c.View().Close(ctx, &api.ViewCloseParams{ID: v.ID}) }()
	rows, err := h.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: v.Count})
	if err != nil {
		h.t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.Subject)
	}
	return out
}

func seed(t *testing.T, mem *imapxtest.Mem) {
	t.Helper()
	files, _ := filepath.Glob("../../dev/incus/seed/*.eml")
	slices.Sort(files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := mem.User.Append("INBOX", bytes.NewReader(data), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInitialSyncAgainstMemServer(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	st := h.waitPhase(api.SyncPhaseIdle)
	if st.LastSyncAt == nil || st.Error != nil {
		t.Fatalf("idle status = %+v", st)
	}

	inbox := h.inbox()
	if inbox.Total != 5 || inbox.Unread != 5 {
		t.Fatalf("INBOX = %+v, want 5 messages, 5 unread", inbox)
	}
	got := h.subjects(inbox.ID)
	want := []string{"Q3 numbers", "October deals — up to 40% off", "Re: Lunch on Thursday?", "Lunch on Thursday?", "Welcome to the frostmail test server"}
	if !slices.Equal(got, want) {
		t.Fatalf("INBOX newest first = %q\nwant %q", got, want)
	}

	ctx := t.Context()
	v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Text: ptr("lunch")}})
	if err != nil || v.Count != 2 {
		t.Fatalf("search lunch = %+v, %v", v, err)
	}
	rows, err := h.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 2})
	if err != nil || len(rows) != 2 || rows[0].ThreadID == 0 || rows[0].ThreadID != rows[1].ThreadID {
		t.Fatalf("lunch rows = %+v, %v; want one thread", rows, err)
	}

	msg, err := h.c.Message().Get(ctx, &api.MessageGetParams{ID: rows[0].ID})
	if err != nil || msg.BodyFetched || msg.InReplyTo != "thread-0001@mailtest.test" {
		t.Fatalf("message.get = %+v, %v", msg, err)
	}
	body, err := h.c.Message().Body(ctx, &api.MessageBodyParams{ID: rows[0].ID})
	if err != nil || !strings.HasPrefix(body.Text, "Thursday works. Noon?") || body.HasHTML {
		t.Fatalf("message.body = %+v, %v", body, err)
	}
	if msg, _ := h.c.Message().Get(ctx, &api.MessageGetParams{ID: rows[0].ID}); !msg.BodyFetched {
		t.Fatal("body not recorded as fetched")
	}
}

func TestLiveChangesReachClients(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	inbox := h.inbox()
	ctx := t.Context()
	view, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID}})
	if err != nil {
		t.Fatal(err)
	}

	// New mail: IDLE wakes C2, the pass inserts it, the view grows by one.
	start := time.Now()
	if _, err := mem.User.Append("INBOX", bytes.NewReader([]byte("Subject: Fresh\r\nFrom: z@mailtest.test\r\nDate: Thu, 08 Oct 2026 09:00:00 +0000\r\n\r\nnew\r\n")), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	h.waitFor(5*time.Second, "view.delta for new mail", func(_ api.EventEnvelope, ev api.Event) bool {
		d, ok := ev.(api.ViewDelta)
		return ok && d.ID == view.ID && d.Count == 6 && slices.Equal(d.Ops, []api.ViewOp{{Op: api.ViewOpKindInsert, At: 0, Count: 1}})
	})
	t.Logf("new mail reached the client in %v", time.Since(start))

	// A flag change and an expunge by another client.
	other, err := imapx.Open(ctx, mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}
	if err := other.StoreFlags(ctx, []uint32{1}, []string{`\Seen`, `\Flagged`}, nil); err != nil {
		t.Fatal(err)
	}
	h.waitFor(5*time.Second, "message.changed for the flag change", func(env api.EventEnvelope, ev api.Event) bool {
		c, ok := ev.(api.MessageChanged)
		return ok && env.Seq > 0 && len(c.IDs) == 1
	})
	if got := h.inbox(); got.Unread != 5 { // 6 messages, one now seen
		t.Fatalf("unread after flag change = %d, want 5", got.Unread)
	}
	if err := other.StoreFlags(ctx, []uint32{2}, []string{`\Deleted`}, nil); err != nil {
		t.Fatal(err)
	}
	h.waitFor(5*time.Second, "view.delta hiding the \\Deleted message", func(_ api.EventEnvelope, ev api.Event) bool {
		d, ok := ev.(api.ViewDelta)
		return ok && d.ID == view.ID && d.Count == 5
	})
}

func TestWrongPasswordIsUnauthorized(t *testing.T) {
	mem := imapxtest.StartMem(t)
	h := newHarness(t, mem.DialOptions(), "wrong")
	// The account exists before its password does, so the first
	// unauthorized status says no password is stored; wait for the login.
	h.waitFor(10*time.Second, "a rejected login", func(_ api.EventEnvelope, ev api.Event) bool {
		p, ok := ev.(api.SyncProgress)
		return ok && p.Status.Phase == api.SyncPhaseUnauthorized && p.Status.Error != nil &&
			strings.Contains(*p.Status.Error, "login rejected")
	})
	// A new password restarts the actor, which then syncs.
	if err := h.c.Account().SetPassword(t.Context(), &api.AccountSetPasswordParams{ID: h.acct, Password: imapxtest.Password}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	statuses, err := h.c.Sync().Status(context.Background(), &api.SyncStatusParams{AccountID: &h.acct})
	if err != nil || len(statuses) != 1 || statuses[0].Phase != api.SyncPhaseIdle {
		t.Fatalf("sync.status = %+v, %v", statuses, err)
	}
}

func ptr[T any](v T) *T { return &v }

// waitUntil polls an external system (the IMAP server) until ok or timeout.
func waitUntil(t *testing.T, timeout time.Duration, what string, ok func() bool) {
	t.Helper()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	deadline := time.After(timeout)
	for !ok() {
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
	}
}

func TestFlagChangesReplayToServer(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	inbox := h.inbox()
	v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := h.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 4, End: 5}) // the oldest: UID 1
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %v, %v", rows, err)
	}
	id := rows[0].ID
	if err := h.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{id}, Changes: api.FlagChanges{Seen: ptr(true), FlagColor: ptr(int64(5))}}); err != nil {
		t.Fatal(err)
	}
	// The local change is immediate.
	if got := h.inbox(); got.Unread != 4 {
		t.Fatalf("unread right after setFlags = %d, want 4", got.Unread)
	}
	other, err := imapx.Open(ctx, mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "flags on the server", func() bool {
		ups, err := other.FetchFlags(ctx, []uint32{1}, 0)
		return err == nil && len(ups) == 1 && ups[0].Flags.Seen && ups[0].Flags.Flagged && ups[0].Flags.Color == 5
	})
	if err := h.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{id}, Changes: api.FlagChanges{Flagged: ptr(false)}}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "unflagged on the server", func() bool {
		ups, err := other.FetchFlags(ctx, []uint32{1}, 0)
		return err == nil && len(ups) == 1 && ups[0].Flags.Seen && !ups[0].Flags.Flagged && ups[0].Flags.Color == 0
	})
	if err := h.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{999}, Changes: api.FlagChanges{Seen: ptr(true)}}); err == nil {
		t.Fatal("setFlags on a missing message succeeded")
	}
}

// A local flag change moves mailbox counts, so clients showing them must hear
// mailbox.changed, as they do for changes found on the server.
func TestLocalFlagChangeAnnouncesMailboxCounts(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	inbox := h.inbox()
	v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := h.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 1})
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %v, %v", rows, err)
	}
	if err := h.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{rows[0].ID}, Changes: api.FlagChanges{Seen: ptr(!rows[0].Flags.Seen)}}); err != nil {
		t.Fatal(err)
	}
	h.waitFor(5*time.Second, "mailbox.changed for INBOX", func(_ api.EventEnvelope, ev api.Event) bool {
		c, ok := ev.(api.MailboxChanged)
		return ok && c.ID == inbox.ID
	})
}

// A read-only account syncs with EXAMINE and refuses every change to its
// mail; nothing reaches the server.
func TestReadOnlyAccountChangesNothing(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, ReadOnly: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	inbox := h.inbox()
	v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &inbox.ID}})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := h.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 4, End: 5}) // UID 1
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %v, %v", rows, err)
	}
	id := rows[0].ID
	refused := func(what string, err error) {
		t.Helper()
		var apiErr *api.Error
		if !errors.As(err, &apiErr) || apiErr.Code != api.CodeConflict {
			t.Errorf("%s on a read-only account = %v, want conflict", what, err)
		}
	}
	refused("setFlags", h.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{id}, Changes: api.FlagChanges{Seen: ptr(true)}}))
	refused("delete", h.c.Message().Delete(ctx, &api.MessageDeleteParams{IDs: []int64{id}}))
	refused("move", h.c.Message().Move(ctx, &api.MessageMoveParams{IDs: []int64{id}, MailboxID: inbox.ID}))
	if got := h.inbox(); got.Unread != inbox.Unread || got.Total != inbox.Total {
		t.Errorf("inbox after refused changes = %+v, was %+v", got, inbox)
	}

	// New mail still arrives.
	if _, err := mem.User.Append("INBOX", strings.NewReader("Subject: late\r\nMessage-ID: <late@x.test>\r\n\r\nhi\r\n"), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "new mail on a read-only account", func() bool { return h.inbox().Total == inbox.Total+1 })

	other, err := imapx.Open(ctx, mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}
	ups, err := other.FetchFlags(ctx, []uint32{1}, 0)
	if err != nil || len(ups) != 1 || ups[0].Flags.Seen {
		t.Errorf("server flags of UID 1 = %+v, %v; a read-only account changed them", ups, err)
	}

	// Writable again, changes go through.
	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, ReadOnly: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if err := h.c.Message().SetFlags(ctx, &api.MessageSetFlagsParams{IDs: []int64{id}, Changes: api.FlagChanges{Seen: ptr(true)}}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 5*time.Second, "seen on the server", func() bool {
		ups, err := other.FetchFlags(ctx, []uint32{1}, 0)
		return err == nil && len(ups) == 1 && ups[0].Flags.Seen
	})
}

// account.verify over RPC: a clean sync matches the server. Detection is
// tested in TestGmailVerify, where no actor can repair the planted
// differences before the check.
func TestVerifyAfterACleanSync(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	r, err := h.c.Account().Verify(ctx, &api.AccountVerifyParams{ID: h.acct})
	if err != nil {
		t.Fatal(err)
	}
	inbox := h.inbox()
	if !r.Ok || len(r.Mailboxes) == 0 {
		t.Fatalf("verify after a clean sync = %+v", r)
	}
	for _, mb := range r.Mailboxes {
		if mb.MailboxID == inbox.ID && (mb.Server != 5 || mb.Local != 5) {
			t.Errorf("INBOX check = %+v, want 5 on both sides", mb)
		}
	}
	if _, err := h.c.Account().Verify(ctx, &api.AccountVerifyParams{ID: 999}); !isCode(err, api.CodeNotFound) {
		t.Errorf("verify of a missing account = %v", err)
	}
}

// New unread inbox mail is announced once it is synced; the first sync of
// a folder announces nothing, and neither do read-only accounts.
func TestNewMailIsAnnounced(t *testing.T) {
	mem := imapxtest.StartMem(t)
	seed(t, mem)
	announced := make(chan []notify.Mail, 8)
	cfg := testConfig
	cfg.Announce = func(_ context.Context, _ int64, mail []notify.Mail) error {
		announced <- mail
		return nil
	}
	h := newHarnessWith(t, mem.DialOptions(), imapxtest.Password, cfg)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	deliver := func(subject string, flags ...imap.Flag) {
		t.Helper()
		raw := "From: Ann <ann@x.test>\r\nSubject: " + subject + "\r\n\r\nSee you at noon.\r\n"
		if _, err := mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{Flags: flags}); err != nil {
			t.Fatal(err)
		}
		if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
			t.Fatal(err)
		}
	}
	deliver("Read elsewhere", imap.FlagSeen)
	deliver("Lunch")
	select {
	case mail := <-announced:
		if len(mail) != 1 || mail[0].Subject != "Lunch" || mail[0].FromName != "Ann" || mail[0].FromAddr != "ann@x.test" {
			t.Errorf("announced %+v, want only Lunch from Ann", mail)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("new mail was not announced")
	}

	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, ReadOnly: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	deliver("Quiet")
	waitUntil(t, 5*time.Second, "Quiet synced", func() bool { return slices.Contains(h.subjects(h.inbox().ID), "Quiet") })

	// Writable again, the next mail is announced; had Quiet been announced
	// it would come first.
	if _, err := h.c.Account().Update(ctx, &api.AccountUpdateParams{ID: h.acct, ReadOnly: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	deliver("Loud")
	select {
	case mail := <-announced:
		if len(mail) != 1 || mail[0].Subject != "Loud" {
			t.Errorf("announced %+v, want only Loud (nothing while read-only)", mail)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mail after the account became writable was not announced")
	}
}
