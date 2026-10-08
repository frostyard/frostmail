package mailsync_test

import (
	"bytes"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-smtp"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/smtpx/smtpxtest"
)

// sendHarness is maild with sync on, an IMAP server with UIDPLUS, and an
// SMTP server, for one account ann@x.test.
type sendHarness struct {
	*harness
	mem  *imapxtest.Mem
	smtp *smtpxtest.Server
}

func newSendHarness(t *testing.T, undo time.Duration) *sendHarness {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	sm := smtpxtest.Start(t, imapxtest.Password)
	cfg := testConfig
	cfg.DraftQuiet = 20 * time.Millisecond
	cfg.SendRetry = 50 * time.Millisecond
	var log *slog.Logger
	if os.Getenv("FROSTMAIL_TEST_LOG") != "" {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, UndoDelay: undo, Log: log})
	c := srv.Dial(t)
	ctx := t.Context()
	if _, err := c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	o := mem.DialOptions()
	a, err := c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: "ann@x.test", DisplayName: "Ann Example", Auth: api.AuthKindPassword,
		IMAP: &api.ServerConfig{Host: o.Host, Port: int64(o.Port), TLS: o.TLS, Username: o.Username},
		SMTP: sm.Config(imapxtest.Username),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: imapxtest.Password}); err != nil {
		t.Fatal(err)
	}
	h := &sendHarness{harness: &harness{t: t, srv: srv, c: c, acct: a.ID}, mem: mem, smtp: sm}
	h.waitPhase(api.SyncPhaseIdle)
	return h
}

var (
	bob   = api.Address{Name: "Bob", Address: "bob@x.test"}
	carol = api.Address{Name: "", Address: "carol@x.test"}
	dan   = api.Address{Name: "Dan", Address: "dan@x.test"}
)

// draft creates a new draft and fills it.
func (h *sendHarness) draft(subject string, to, cc, bcc []api.Address) api.Draft {
	h.t.Helper()
	ctx := h.t.Context()
	d, err := h.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew, AccountID: &h.acct})
	if err != nil {
		h.t.Fatal(err)
	}
	content := d.Content
	content.To, content.Cc, content.Bcc = to, cc, bcc
	content.Subject = subject
	content.HTML = "<p>Hello <b>there</b></p>"
	up, err := h.c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: content})
	if err != nil {
		h.t.Fatal(err)
	}
	return *up
}

// waitOutbox waits for an outbox.changed event of one message in a state.
func (h *sendHarness) waitOutbox(id int64, state api.OutboxState) {
	h.t.Helper()
	h.waitFor(10*time.Second, "outbox "+string(state), func(_ api.EventEnvelope, ev api.Event) bool {
		o, ok := ev.(api.OutboxChanged)
		return ok && o.ID == id && o.State == state && !o.Deleted
	})
}

// outboxStates lists the states announced for one message so far.
func (h *sendHarness) outboxStates(id int64) []api.OutboxState {
	var out []api.OutboxState
	for _, env := range h.events {
		ev, err := api.DecodeEvent(env.Event, env.Data)
		if o, ok := ev.(api.OutboxChanged); err == nil && ok && o.ID == id {
			out = append(out, o.State)
		}
	}
	return out
}

// serverMessages returns the raw messages in a mailbox on the IMAP server.
func (h *sendHarness) serverMessages(mailbox string) [][]byte {
	h.t.Helper()
	ctx := h.t.Context()
	s, err := imapx.Open(ctx, h.mem.DialOptions())
	if err != nil {
		h.t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Select(ctx, mailbox); err != nil {
		h.t.Fatal(err)
	}
	uids, err := s.UIDs(ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	var out [][]byte
	for _, u := range uids {
		raw, err := s.FetchRaw(ctx, u)
		if errors.Is(err, imapx.ErrNoMessage) {
			continue // expunged while we read (a replaced draft copy)
		}
		if err != nil {
			h.t.Fatal(err)
		}
		out = append(out, raw)
	}
	return out
}

// waitServer checks a mailbox on the IMAP server after every event until
// ok accepts its messages.
func (h *sendHarness) waitServer(mailbox, what string, ok func([][]byte) bool) {
	h.t.Helper()
	if ok(h.serverMessages(mailbox)) {
		return
	}
	h.waitFor(10*time.Second, what, func(api.EventEnvelope, api.Event) bool {
		return ok(h.serverMessages(mailbox))
	})
}

func TestSendDeliversAndFilesTheSentCopy(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	d := h.draft("Lunch", []api.Address{bob}, []api.Address{carol}, []api.Address{dan})
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("bring snacks\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	att, err := h.c.Draft().Attach(ctx, &api.DraftAttachParams{ID: d.ID, Path: path})
	if err != nil || att.Filename != "notes.txt" || att.ContentType != "text/plain" || att.Size != 13 {
		t.Fatalf("attach = %+v, %v", att, err)
	}

	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if item.State != api.OutboxStateQueued || item.SendAt == nil || item.Subject != "Lunch" {
		t.Fatalf("send = %+v", item)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent)
	if got := h.outboxStates(item.ID); !slices.Equal(got, []api.OutboxState{"queued", "sending", "accepted", "sent"}) {
		t.Errorf("states = %v", got)
	}

	msgs := h.smtp.Messages()
	if len(msgs) != 1 {
		t.Fatalf("SMTP server got %d messages, want 1", len(msgs))
	}
	m := msgs[0]
	if m.From != "ann@x.test" || !slices.Equal(m.To, []string{"bob@x.test", "carol@x.test", "dan@x.test"}) {
		t.Errorf("envelope = %s → %v", m.From, m.To)
	}
	head, _, _ := bytes.Cut(m.Data, []byte("\r\n\r\n"))
	for _, want := range []string{"Subject: Lunch", `From: "Ann Example" <ann@x.test>`, `To: "Bob" <bob@x.test>`, "Cc: <carol@x.test>"} {
		if !bytes.Contains(head, []byte(want)) {
			t.Errorf("header lacks %q:\n%s", want, head)
		}
	}
	if bytes.Contains(head, []byte("dan@x.test")) {
		t.Errorf("the sent header names the Bcc recipient:\n%s", head)
	}
	if !bytes.Contains(m.Data, []byte(`filename=notes.txt`)) && !bytes.Contains(m.Data, []byte(`filename="notes.txt"`)) {
		t.Errorf("the attachment is missing")
	}

	sent := h.serverMessages("Sent")
	if len(sent) != 1 || !bytes.Equal(sent[0], m.Data) {
		t.Fatalf("Sent holds %d messages; want the sent one", len(sent))
	}
	if list, err := h.c.Draft().List(ctx, &api.DraftListParams{}); err != nil || len(list) != 0 {
		t.Errorf("drafts after sending = %+v, %v", list, err)
	}
	if list, err := h.c.Outbox().List(ctx, &api.OutboxListParams{}); err != nil || len(list) != 0 {
		t.Errorf("outbox after sending = %+v, %v", list, err)
	}
	got, err := h.c.Address().Suggest(ctx, &api.AddressSuggestParams{Prefix: "bo"})
	if err != nil || len(got) != 1 || got[0] != bob {
		t.Errorf("suggest(bo) = %+v, %v", got, err)
	}
}

func TestUndoCancelsASend(t *testing.T) {
	h := newSendHarness(t, time.Hour)
	ctx := t.Context()
	d := h.draft("Oops", []api.Address{bob}, nil, nil)
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID}); !isCode(err, api.CodeConflict) {
		t.Errorf("a second send = %v, want conflict", err)
	}
	back, err := h.c.Outbox().Cancel(ctx, &api.OutboxCancelParams{ID: item.ID})
	if err != nil || back.ID != d.ID || back.Content.Subject != "Oops" {
		t.Fatalf("cancel = %+v, %v", back, err)
	}
	if list, _ := h.c.Outbox().List(ctx, &api.OutboxListParams{}); len(list) != 0 {
		t.Errorf("outbox after undo = %+v", list)
	}
	if _, err := h.c.Outbox().Cancel(ctx, &api.OutboxCancelParams{ID: item.ID}); !isCode(err, api.CodeNotFound) {
		t.Errorf("cancel again = %v, want notFound", err)
	}
	if n := len(h.smtp.Messages()); n != 0 {
		t.Errorf("SMTP server got %d messages", n)
	}
	// The draft can be sent again.
	if _, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID}); err != nil {
		t.Errorf("send after undo: %v", err)
	}
}

func isCode(err error, code api.ErrorCode) bool {
	var e *api.Error
	return errors.As(err, &e) && e.Code == code
}

func TestSendRetriesATemporaryRefusal(t *testing.T) {
	h := newSendHarness(t, 0)
	h.smtp.RefuseRecipients(&smtp.SMTPError{Code: 451, EnhancedCode: smtp.EnhancedCode{4, 2, 1}, Message: "try later"})
	d := h.draft("Later", []api.Address{bob}, nil, nil)
	item, err := h.c.Draft().Send(t.Context(), &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent)
	want := []api.OutboxState{"queued", "sending", "queued", "sending", "accepted", "sent"}
	if got := h.outboxStates(item.ID); !slices.Equal(got, want) {
		t.Errorf("states = %v, want %v", got, want)
	}
	if n := len(h.smtp.Messages()); n != 1 {
		t.Errorf("SMTP server got %d messages, want 1", n)
	}
}

func TestPermanentRefusalWaitsForRetry(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	h.smtp.RefuseRecipients(&smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "no such user"})
	d := h.draft("Hello", []api.Address{bob}, nil, nil)
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateFailed)
	list, err := h.c.Outbox().List(ctx, &api.OutboxListParams{})
	if err != nil || len(list) != 1 || list[0].State != api.OutboxStateFailed || list[0].Error == nil ||
		!strings.Contains(*list[0].Error, "no such user") || list[0].Attempts != 1 {
		t.Fatalf("outbox = %+v, %v", list, err)
	}
	if _, err := h.c.Draft().Get(ctx, &api.DraftGetParams{ID: d.ID}); err != nil {
		t.Errorf("the draft of a failed message is gone: %v", err)
	}
	if err := h.c.Outbox().Retry(ctx, &api.OutboxRetryParams{ID: item.ID}); err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent)
	if n := len(h.smtp.Messages()); n != 1 {
		t.Errorf("SMTP server got %d messages, want 1", n)
	}
	if err := h.c.Outbox().Retry(ctx, &api.OutboxRetryParams{ID: item.ID}); !isCode(err, api.CodeConflict) {
		t.Errorf("retrying a sent message = %v, want conflict", err)
	}
}

func TestDraftCopyIsSavedReplacedAndDeleted(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	subjects := func(raws [][]byte) []string {
		var out []string
		for _, r := range raws {
			for line := range strings.SplitSeq(string(r), "\r\n") {
				if s, ok := strings.CutPrefix(line, "Subject: "); ok {
					out = append(out, s)
				}
			}
		}
		return out
	}
	d := h.draft("First", []api.Address{bob}, nil, []api.Address{dan})
	h.waitServer("Drafts", "the first copy", func(raws [][]byte) bool {
		return slices.Equal(subjects(raws), []string{"First"})
	})
	copyRaw := h.serverMessages("Drafts")[0]
	if !bytes.Contains(copyRaw, []byte("dan@x.test")) {
		t.Errorf("the draft copy lost its Bcc:\n%s", copyRaw)
	}

	content := d.Content
	content.Subject = "Second"
	content.To = append(content.To, api.Address{Address: "half-typed"})
	if _, err := h.c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: content}); err != nil {
		t.Fatal(err)
	}
	h.waitServer("Drafts", "the replaced copy", func(raws [][]byte) bool {
		return slices.Equal(subjects(raws), []string{"Second"})
	})

	if err := h.c.Draft().Delete(ctx, &api.DraftDeleteParams{ID: d.ID}); err != nil {
		t.Fatal(err)
	}
	h.waitServer("Drafts", "the copy deleted", func(raws [][]byte) bool { return len(raws) == 0 })
}

func TestUntouchedDraftHasNoServerCopy(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	if _, err := h.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew}); err != nil {
		t.Fatal(err)
	}
	// A later draft's copy proves the saver ran past the untouched one.
	h.draft("Touched", []api.Address{bob}, nil, nil)
	h.waitServer("Drafts", "the touched draft's copy", func(raws [][]byte) bool { return len(raws) == 1 })
	if raws := h.serverMessages("Drafts"); !bytes.Contains(raws[0], []byte("Subject: Touched")) {
		t.Fatalf("Drafts holds %q", raws[0])
	}
}

func TestInterruptedSendIsSentOnce(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	release := h.smtp.Hold()
	d := h.draft("Once", []api.Address{bob}, nil, nil)
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.smtp.WaitHeld(t, 1, 10*time.Second)
	// Restart the account mid-send; the server never answers that DATA.
	if err := h.c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: h.acct, Password: imapxtest.Password}); err != nil {
		t.Fatal(err)
	}
	release(false)
	h.waitOutbox(item.ID, api.OutboxStateSent)
	if n := len(h.smtp.Messages()); n != 1 {
		t.Fatalf("SMTP server got %d messages, want 1", n)
	}
	if n := len(h.serverMessages("Sent")); n != 1 {
		t.Errorf("Sent holds %d messages, want 1", n)
	}
}

func TestInterruptedSendFoundInSentIsNotResent(t *testing.T) {
	h := newSendHarness(t, 0)
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
	// The server delivered it and filed it in Sent (as Gmail does), but the
	// answer never reached maild.
	copyRaw := "Message-ID: <" + row.MessageID + ">\r\nSubject: Already\r\n\r\nHello\r\n"
	if _, err := h.mem.User.Append("Sent", strings.NewReader(copyRaw), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := h.c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: h.acct, Password: imapxtest.Password}); err != nil {
		t.Fatal(err)
	}
	release(false)
	h.waitOutbox(item.ID, api.OutboxStateSent)
	if n := len(h.smtp.Messages()); n != 0 {
		t.Errorf("SMTP server got %d more messages, want none", n)
	}
	if n := len(h.serverMessages("Sent")); n != 1 {
		t.Errorf("Sent holds %d messages, want 1", n)
	}
	if _, err := h.c.Draft().Get(ctx, &api.DraftGetParams{ID: d.ID}); !isCode(err, api.CodeNotFound) {
		t.Errorf("the draft of a delivered message = %v, want notFound", err)
	}
}

// TestCrashAfterAcceptanceSendsOnce stops maild after the SMTP server
// accepted a message but before its Sent copy was written (IMAP is down),
// then restarts it on the same data: the copy is written once and the
// message is not sent again.
func TestCrashAfterAcceptanceSendsOnce(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	sm := smtpxtest.Start(t, imapxtest.Password)
	data := t.TempDir()
	cfg := testConfig
	cfg.SendRetry = 50 * time.Millisecond
	ctx := t.Context()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadPort := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, DataDir: data})
	h := &sendHarness{harness: &harness{t: t, srv: srv, c: srv.Dial(t)}, mem: mem, smtp: sm}
	if _, err := h.c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	a, err := h.c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: "ann@x.test", Auth: api.AuthKindPassword,
		IMAP: &api.ServerConfig{Host: "127.0.0.1", Port: int64(deadPort), TLS: api.TLSModeInsecure, Username: imapxtest.Username},
		SMTP: sm.Config(imapxtest.Username),
	})
	if err != nil {
		t.Fatal(err)
	}
	h.acct = a.ID
	if err := h.c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: imapxtest.Password}); err != nil {
		t.Fatal(err)
	}
	d := h.draft("Once only", []api.Address{bob}, nil, nil)
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateAccepted)
	srv.Stop()

	srv2 := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, DataDir: data})
	h2 := &sendHarness{harness: &harness{t: t, srv: srv2, c: srv2.Dial(t), acct: a.ID}, mem: mem, smtp: sm}
	if _, err := h2.c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	o := mem.DialOptions()
	imapCfg := api.ServerConfig{Host: o.Host, Port: int64(o.Port), TLS: o.TLS, Username: o.Username}
	if _, err := h2.c.Account().Update(ctx, &api.AccountUpdateParams{ID: a.ID, IMAP: &imapCfg}); err != nil {
		t.Fatal(err)
	}
	h2.waitOutbox(item.ID, api.OutboxStateSent)
	if n := len(sm.Messages()); n != 1 {
		t.Errorf("SMTP server got %d messages, want 1", n)
	}
	sent := h2.serverMessages("Sent")
	if len(sent) != 1 || !bytes.Equal(sent[0], sm.Messages()[0].Data) {
		t.Errorf("Sent holds %d messages; want the one sent", len(sent))
	}
}
