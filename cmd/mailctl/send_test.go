package main

// CONTRACT TEST for task card T-0041 (docs/tasks). Do not edit.

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/smtpx/smtpxtest"
)

type sendEnv struct {
	srv  *rpctest.Server
	c    *api.Client // subscribed to events
	smtp *smtpxtest.Server
}

// newSendEnv runs maild with sync, an IMAP server and an SMTP server for
// one account, ann@x.test; sent mail waits for undo.
func newSendEnv(t *testing.T, undo time.Duration) *sendEnv {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	sm := smtpxtest.Start(t, imapxtest.Password)
	cfg := mailsync.Config{PollInterval: time.Hour, MinBackoff: 50 * time.Millisecond, SendRetry: 50 * time.Millisecond}
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, UndoDelay: undo})
	c := srv.Dial(t)
	ctx := t.Context()
	if _, err := c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	o := mem.DialOptions()
	a, err := c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: "ann@x.test", DisplayName: "Ann", Auth: api.AuthKindPassword,
		IMAP: &api.ServerConfig{Host: o.Host, Port: int64(o.Port), TLS: o.TLS, Username: o.Username},
		SMTP: sm.Config(imapxtest.Username),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: imapxtest.Password}); err != nil {
		t.Fatal(err)
	}
	return &sendEnv{srv: srv, c: c, smtp: sm}
}

// waitOutbox waits for an outbox.changed event of one message in a state.
func (e *sendEnv) waitOutbox(t *testing.T, id int64, state api.OutboxState) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for {
		select {
		case env := <-e.c.Notifications():
			ev, _ := api.DecodeEvent(env.Event, env.Data)
			if o, ok := ev.(api.OutboxChanged); ok && o.ID == id && o.State == state && !o.Deleted {
				return
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for message %d to be %s", id, state)
		}
	}
}

func TestSendComposesFromFlagsAndStdin(t *testing.T) {
	e := newSendEnv(t, 0)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("snacks\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	out, err := runWithStdin(t, "Hello\nWorld <3\n", "--socket", e.srv.Socket, "send",
		"--to", "Bob <bob@x.test>", "--cc", "carol@x.test", "--subject", "Hi", "--attach", "notes.txt", "--wait")
	if err != nil {
		t.Fatalf("send: %v\n%s", err, out)
	}
	if out != "queued message 1 from draft 1\nsent\n" {
		t.Errorf("output = %q", out)
	}
	msgs := e.smtp.Messages()
	if len(msgs) != 1 {
		t.Fatalf("SMTP server got %d messages", len(msgs))
	}
	m := msgs[0]
	if m.From != "ann@x.test" || !slices.Equal(m.To, []string{"bob@x.test", "carol@x.test"}) {
		t.Errorf("envelope = %s → %v", m.From, m.To)
	}
	for _, want := range []string{"Subject: Hi", `To: "Bob" <bob@x.test>`, "Hello<br>World &lt;3", "notes.txt"} {
		if !bytes.Contains(m.Data, []byte(want)) {
			t.Errorf("message lacks %q:\n%s", want, m.Data)
		}
	}
}

func TestSendTakesHTMLAndBcc(t *testing.T) {
	e := newSendEnv(t, 0)
	out, err := runWithStdin(t, "<p>Hi <b>there</b></p>", "--socket", e.srv.Socket, "send",
		"--bcc", "dan@x.test", "--subject", "Quiet", "--html", "--wait")
	if err != nil {
		t.Fatalf("send: %v\n%s", err, out)
	}
	m := e.smtp.Messages()[0]
	head, body, _ := bytes.Cut(m.Data, []byte("\r\n\r\n"))
	if !slices.Equal(m.To, []string{"dan@x.test"}) || bytes.Contains(head, []byte("dan@x.test")) {
		t.Errorf("Bcc: envelope %v, header:\n%s", m.To, head)
	}
	if !bytes.Contains(body, []byte("<p>Hi <b>there</b></p>")) {
		t.Errorf("the HTML body was not sent as given:\n%s", body)
	}
}

func TestSendChecksArgumentsBeforeDialing(t *testing.T) {
	nowhere := filepath.Join(t.TempDir(), "none.sock")
	cases := map[string][]string{
		"no recipients":   {"send", "--subject", "x"},
		"invalid address": {"send", "--to", "not an address"},
	}
	for name, args := range cases {
		_, err := runWithStdin(t, "body", append([]string{"--socket", nowhere}, args...)...)
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if _, err := runWithStdin(t, "", "--socket", nowhere, "outbox", "cancel", "x"); err == nil ||
		!strings.Contains(err.Error(), `invalid outbox id "x"`) {
		t.Errorf("cancel x: err = %v", err)
	}
}

func TestSendReportsARefusal(t *testing.T) {
	e := newSendEnv(t, 0)
	e.smtp.RefuseRecipients(&smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "no such user"})
	out, err := runWithStdin(t, "Hello", "--socket", e.srv.Socket, "send", "--to", "bob@x.test", "--subject", "Nope", "--wait")
	if err == nil || !strings.Contains(err.Error(), "message 1 not sent") || !strings.Contains(err.Error(), "no such user") {
		t.Fatalf("send = %v\n%s", err, out)
	}

	out, err = runWithStdin(t, "", "--socket", e.srv.Socket, "outbox")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "ID") || !strings.Contains(lines[0], "STATE") ||
		!strings.Contains(lines[1], "failed") || !strings.Contains(lines[1], "Nope") || !strings.Contains(lines[1], "no such user") {
		t.Errorf("outbox = %q", out)
	}

	out, err = runWithStdin(t, "", "--socket", e.srv.Socket, "outbox", "retry", "1")
	if err != nil || out != "queued message 1 again\n" {
		t.Fatalf("retry = %q, %v", out, err)
	}
	e.waitOutbox(t, 1, api.OutboxStateSent)
	if n := len(e.smtp.Messages()); n != 1 {
		t.Errorf("SMTP server got %d messages after the retry", n)
	}
}

func TestOutboxListsAndCancels(t *testing.T) {
	e := newSendEnv(t, time.Hour)
	out, err := runWithStdin(t, "Hello", "--socket", e.srv.Socket, "send", "--to", "bob@x.test", "--subject", "Later")
	if err != nil || out != "queued message 1 from draft 1\n" {
		t.Fatalf("send = %q, %v", out, err)
	}
	out, err = runWithStdin(t, "", "--socket", e.srv.Socket, "outbox")
	if err != nil || !strings.Contains(out, "queued") || !strings.Contains(out, "Later") {
		t.Fatalf("outbox = %q, %v", out, err)
	}
	out, err = runWithStdin(t, "", "--socket", e.srv.Socket, "outbox", "--json")
	var items []api.OutboxItem
	if err != nil || json.Unmarshal([]byte(out), &items) != nil || len(items) != 1 ||
		items[0].State != api.OutboxStateQueued || items[0].Subject != "Later" {
		t.Fatalf("outbox --json = %q, %v", out, err)
	}
	out, err = runWithStdin(t, "", "--socket", e.srv.Socket, "outbox", "cancel", "1")
	if err != nil || out != "cancelled message 1; draft 1 kept\n" {
		t.Fatalf("cancel = %q, %v", out, err)
	}
	out, err = runWithStdin(t, "", "--socket", e.srv.Socket, "outbox")
	if err != nil || out != "outbox is empty\n" {
		t.Fatalf("outbox after cancel = %q, %v", out, err)
	}
	if n := len(e.smtp.Messages()); n != 0 {
		t.Errorf("SMTP server got %d messages", n)
	}
}
