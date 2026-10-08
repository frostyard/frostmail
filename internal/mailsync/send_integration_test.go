//go:build integration

package mailsync_test

// Sending against the frostmail-mailtest container (dev/incus/mailtest.sh):
// make engine-it sets FROSTMAIL_IT_HOST and restores the clean snapshot.
// test4 sends to test5, so other tests' mailbox counts stay as seeded.

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/mimex"
	"github.com/frostyard/frostmail/internal/rpctest"
)

const itPassword = "frostmail-test"

func itHost(t *testing.T) string {
	host := os.Getenv("FROSTMAIL_IT_HOST")
	if host == "" {
		t.Skip("FROSTMAIL_IT_HOST is not set; run make engine-it")
	}
	return host
}

// itHarness runs maild against the container for test4.
type itHarness struct {
	*harness
	host string
}

func itServers(host string) (imapCfg, smtpCfg api.ServerConfig) {
	imapCfg = api.ServerConfig{Host: host, Port: 993, TLS: api.TLSModeTLS, Username: "test4@mailtest.test"}
	smtpCfg = api.ServerConfig{Host: host, Port: 587, TLS: api.TLSModeStartTLS, Username: "test4@mailtest.test"}
	return imapCfg, smtpCfg
}

func itConfig() mailsync.Config {
	cfg := testConfig
	cfg.InsecureSkipVerify = true
	cfg.DraftQuiet = 100 * time.Millisecond
	cfg.SendRetry = time.Second
	return cfg
}

// startIT starts maild on data (fresh when empty) and, when create is set,
// adds the test4 account with smtp as its submission server.
func startIT(t *testing.T, data string, create bool, smtp *api.ServerConfig) *itHarness {
	t.Helper()
	host := itHost(t)
	cfg := itConfig()
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, DataDir: data})
	h := &itHarness{harness: &harness{t: t, srv: srv, c: srv.Dial(t), acct: 1}, host: host}
	ctx := t.Context()
	if _, err := h.c.Events().Subscribe(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if !create {
		return h
	}
	imapCfg, smtpCfg := itServers(host)
	if smtp != nil {
		smtpCfg = *smtp
	}
	a, err := h.c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: "test4@mailtest.test", DisplayName: "Test Four",
		Auth: api.AuthKindPassword, IMAP: &imapCfg, SMTP: &smtpCfg,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.acct = a.ID
	if err := h.c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: itPassword}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	return h
}

func (h *itHarness) waitOutbox(id int64, state api.OutboxState, timeout time.Duration) {
	h.t.Helper()
	h.waitFor(timeout, "outbox "+string(state), func(_ api.EventEnvelope, ev api.Event) bool {
		o, ok := ev.(api.OutboxChanged)
		return ok && o.ID == id && o.State == state && !o.Deleted
	})
}

// draftTo creates a draft from test4 to test5.
func (h *itHarness) draftTo(subject string, attach ...string) api.Draft {
	h.t.Helper()
	ctx := h.t.Context()
	d, err := h.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew, AccountID: &h.acct})
	if err != nil {
		h.t.Fatal(err)
	}
	c := d.Content
	c.To = []api.Address{{Name: "Test Five", Address: "test5@mailtest.test"}}
	c.Subject = subject
	c.HTML = "<p>Sent by the integration tests.</p>"
	if _, err := h.c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: c}); err != nil {
		h.t.Fatal(err)
	}
	for _, p := range attach {
		if _, err := h.c.Draft().Attach(ctx, &api.DraftAttachParams{ID: d.ID, Path: p}); err != nil {
			h.t.Fatal(err)
		}
	}
	got, err := h.c.Draft().Get(ctx, &api.DraftGetParams{ID: d.ID})
	if err != nil {
		h.t.Fatal(err)
	}
	return *got
}

// mailboxMessages returns the raw messages of a user's mailbox whose
// Message-ID contains msgid ("" for all).
func mailboxMessages(t *testing.T, host, user, mailbox, msgid string) [][]byte {
	t.Helper()
	ctx := t.Context()
	s, err := imapx.Open(ctx, imapx.DialOptions{
		Host: host, Port: 993, TLS: api.TLSModeTLS, Username: user, Password: itPassword, InsecureSkipVerify: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Select(ctx, mailbox); err != nil {
		t.Fatal(err)
	}
	var uids []uint32
	if msgid == "" {
		uids, err = s.UIDs(ctx)
	} else {
		uids, err = s.SearchMessageID(ctx, msgid)
	}
	if err != nil {
		t.Fatal(err)
	}
	var out [][]byte
	for _, u := range uids {
		raw, err := s.FetchRaw(ctx, u)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, raw)
	}
	return out
}

// messageID returns the Message-ID an outbox row sends with.
func (h *itHarness) messageID(outboxID int64) string {
	h.t.Helper()
	row, err := h.srv.DB.GetOutbox(h.t.Context(), outboxID)
	if err != nil {
		h.t.Fatal(err)
	}
	return row.MessageID
}

// waitDelivered polls test5's INBOX (Postfix delivers on its own time).
func waitDelivered(t *testing.T, host, msgid string) []byte {
	t.Helper()
	var got [][]byte
	waitUntil(t, 30*time.Second, "delivery to test5", func() bool {
		got = mailboxMessages(t, host, "test5@mailtest.test", "INBOX", msgid)
		return len(got) > 0
	})
	if len(got) != 1 {
		t.Fatalf("test5 got %d copies of %s", len(got), msgid)
	}
	return got[0]
}

func TestPostfixDeliversAndSentIsFiled(t *testing.T) {
	h := startIT(t, "", true, nil)
	d := h.draftTo("Integration: delivery")
	item, err := h.c.Draft().Send(t.Context(), &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent, 30*time.Second)
	id := h.messageID(item.ID)
	delivered := waitDelivered(t, h.host, id)
	if !bytes.Contains(delivered, []byte("Subject: Integration: delivery")) {
		t.Errorf("delivered message:\n%s", delivered)
	}
	if sent := mailboxMessages(t, h.host, "test4@mailtest.test", "Sent", id); len(sent) != 1 {
		t.Errorf("test4's Sent holds %d copies, want 1", len(sent))
	}
}

func TestOutboxFlushesWhenTheServerReturns(t *testing.T) {
	host := itHost(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	down := api.ServerConfig{
		Host: "127.0.0.1", Port: int64(ln.Addr().(*net.TCPAddr).Port), TLS: api.TLSModeStartTLS, Username: "test4@mailtest.test",
	}
	_ = ln.Close()
	h := startIT(t, "", true, &down)
	d := h.draftTo("Integration: queued while down")
	item, err := h.c.Draft().Send(t.Context(), &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSending, 10*time.Second)
	h.waitOutbox(item.ID, api.OutboxStateQueued, 10*time.Second) // refused: waits for a retry
	list, err := h.c.Outbox().List(t.Context(), &api.OutboxListParams{})
	if err != nil || len(list) != 1 || list[0].Attempts != 1 || list[0].Error == nil {
		t.Fatalf("outbox while the server is down = %+v, %v", list, err)
	}

	_, up := itServers(host)
	back := time.Now()
	if _, err := h.c.Account().Update(t.Context(), &api.AccountUpdateParams{ID: h.acct, SMTP: &up}); err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent, time.Minute)
	if d := time.Since(back); d > 30*time.Second {
		t.Errorf("the queued message went out %v after the server returned", d)
	}
	waitDelivered(t, host, h.messageID(item.ID))
}

func TestLargeAttachmentArrivesIntact(t *testing.T) {
	h := startIT(t, "", true, nil)
	path := filepath.Join(t.TempDir(), "big.bin")
	data := make([]byte, 25_000_000)
	if _, err := io.ReadFull(rand.Reader, data); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(data)
	d := h.draftTo("Integration: 25 MB", path)
	item, err := h.c.Draft().Send(t.Context(), &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent, 2*time.Minute)
	raw := waitDelivered(t, h.host, h.messageID(item.ID))
	found := false
	err = mimex.WalkParts(raw, func(p mimex.PartInfo, body io.Reader) error {
		if p.Filename != "big.bin" {
			return nil
		}
		b, err := io.ReadAll(body)
		if err != nil {
			return err
		}
		found = true
		if got := sha256.Sum256(b); got != want {
			t.Errorf("the attachment changed: %d bytes, sha256 %x, want %x", len(b), got, want)
		}
		return nil
	})
	if err != nil || !found {
		t.Fatalf("walk = %v, found %v", err, found)
	}
}

func TestDraftsSurviveARestart(t *testing.T) {
	data := t.TempDir()
	h := startIT(t, data, true, nil)
	d := h.draftTo("Integration: draft before restart")
	msgCopy := func() [][]byte {
		row, err := h.srv.DB.GetDraft(t.Context(), d.ID)
		if err != nil {
			t.Fatal(err)
		}
		return mailboxMessages(t, h.host, "test4@mailtest.test", "Drafts", row.MessageID)
	}
	waitUntil(t, 30*time.Second, "the first draft copy", func() bool { return len(msgCopy()) == 1 })
	row, _ := h.srv.DB.GetDraft(t.Context(), d.ID)
	h.srv.Stop()

	h2 := startIT(t, data, false, nil)
	h2.acct = d.AccountID
	list, err := h2.c.Draft().List(t.Context(), &api.DraftListParams{})
	if err != nil || len(list) != 1 || list[0].Content.Subject != "Integration: draft before restart" {
		t.Fatalf("drafts after restart = %+v, %v", list, err)
	}
	c := list[0].Content
	c.Subject = "Integration: draft after restart"
	if _, err := h2.c.Draft().Update(t.Context(), &api.DraftUpdateParams{ID: d.ID, Content: c}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 30*time.Second, "the replaced draft copy", func() bool {
		copies := mailboxMessages(t, h2.host, "test4@mailtest.test", "Drafts", row.MessageID)
		return len(copies) == 1 && bytes.Contains(copies[0], []byte("Subject: Integration: draft after restart"))
	})
}
