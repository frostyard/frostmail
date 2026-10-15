package mailsync_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
)

// withPDF is a message over the whole-fetch limit: its text, and a PDF of
// about 400 KB attached.
func withPDF(subject string) string {
	pdf := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("%PDF-1.7 0123456789abcdef"), 12000))
	return strings.Join([]string{
		"From: Ann <ann@example.test>", "To: test@mailtest.test", "Subject: " + subject, "MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="b"`, "",
		"--b", "Content-Type: text/plain; charset=utf-8", "", "The report is attached.",
		"--b", `Content-Type: application/pdf; name="report.pdf"`, `Content-Disposition: attachment; filename="report.pdf"`,
		"Content-Transfer-Encoding: base64", "", pdf,
		"--b--", "",
	}, "\r\n")
}

func plain(from, subject string) string {
	return "From: " + from + "\r\nTo: test@mailtest.test\r\nSubject: " + subject + "\r\n\r\nHello.\r\n"
}

// ids are the inbox's message IDs by subject.
func (h *harness) ids(mailboxID int64) map[string]int64 {
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
	out := map[string]int64{}
	for _, r := range rows {
		out[r.Subject] = r.ID
	}
	return out
}

func (h *harness) bodyFetched(id int64) bool {
	h.t.Helper()
	m, err := h.c.Message().Get(h.t.Context(), &api.MessageGetParams{ID: id})
	if err != nil {
		h.t.Fatal(err)
	}
	return m.BodyFetched
}

func TestOpeningFetchesWhatTheReaderShows(t *testing.T) {
	mem, counts := imapxtest.StartMemCounted(t)
	for _, m := range []string{withPDF("Report"), plain("bob@example.test", "Hi")} {
		if _, err := mem.User.Append("INBOX", strings.NewReader(m), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	ids := h.ids(h.inbox().ID)

	// The report opens without its PDF.
	r, err := h.c.Message().Render(ctx, &api.MessageRenderParams{ID: ids["Report"]})
	if err != nil || !strings.Contains(r.Text, "The report is attached.") {
		t.Fatalf("render = %+v, %v", r, err)
	}
	if h.bodyFetched(ids["Report"]) || counts.Whole.Load() != 0 {
		t.Errorf("the whole report was fetched to read it (%d whole fetches)", counts.Whole.Load())
	}
	// A small message is fetched whole, in the mailbox C3 has open.
	if _, err := h.c.Message().Render(ctx, &api.MessageRenderParams{ID: ids["Hi"]}); err != nil {
		t.Fatal(err)
	}
	if !h.bodyFetched(ids["Hi"]) || counts.Whole.Load() != 1 {
		t.Errorf("the small message: fetched %v, %d whole fetches", h.bodyFetched(ids["Hi"]), counts.Whole.Load())
	}
	if n := counts.Examines.Load(); n != 1 {
		t.Errorf("INBOX was examined %d times for two messages in it", n)
	}

	// Opening the PDF fetches the whole report.
	m, err := h.c.Message().Get(ctx, &api.MessageGetParams{ID: ids["Report"]})
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, p := range m.Parts {
		if p.ContentType == "application/pdf" {
			path = p.Path
		}
	}
	f, err := h.c.Message().Part(ctx, &api.MessagePartParams{ID: ids["Report"], Path: path})
	if err != nil || f.Size != 12000*25 {
		t.Fatalf("part = %+v, %v", f, err)
	}
	if !h.bodyFetched(ids["Report"]) || counts.Whole.Load() != 2 {
		t.Errorf("the PDF: body fetched %v, %d whole fetches", h.bodyFetched(ids["Report"]), counts.Whole.Load())
	}
	// From now on the report is read from the store.
	if _, err := h.c.Message().Render(ctx, &api.MessageRenderParams{ID: ids["Report"]}); err != nil {
		t.Fatal(err)
	}
	src, err := h.c.Message().Source(ctx, &api.MessageSourceParams{ID: ids["Report"]})
	if err != nil || !strings.Contains(src.Text, "JVBERi0xLjcg") || counts.Whole.Load() != 2 {
		t.Errorf("source after the PDF: %d whole fetches, %v", counts.Whole.Load(), err)
	}
}

func TestSourceOfADisplayCopyFetchesTheWholeMessage(t *testing.T) {
	mem, counts := imapxtest.StartMemCounted(t)
	if _, err := mem.User.Append("INBOX", strings.NewReader(withPDF("Report")), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	h := newHarness(t, mem.DialOptions(), imapxtest.Password)
	h.waitPhase(api.SyncPhaseIdle)
	ctx := t.Context()
	id := h.ids(h.inbox().ID)["Report"]
	if _, err := h.c.Message().Render(ctx, &api.MessageRenderParams{ID: id}); err != nil {
		t.Fatal(err)
	}
	src, err := h.c.Message().Source(ctx, &api.MessageSourceParams{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(src.Text, "X-Frostmail-Omitted") || !strings.Contains(src.Text, "JVBERi0xLjcg") || counts.Whole.Load() != 1 {
		t.Errorf("Raw Source is not the whole message (%d whole fetches)", counts.Whole.Load())
	}
}

func TestPrefetchFetchesRecentInboxAndVIPMail(t *testing.T) {
	mem := imapxtest.StartMem(t)
	now := time.Now()
	for _, m := range []struct {
		mailbox, raw string
		at           time.Time
	}{
		{"INBOX", plain("bob@example.test", "Recent"), now.Add(-24 * time.Hour)},
		{"INBOX", withPDF("Big"), now.Add(-30 * time.Hour)},
		{"INBOX", plain("bob@example.test", "Old"), now.AddDate(0, 0, -100)},
		{"Archive", plain("Vip@Example.test", "From a VIP"), now.Add(-48 * time.Hour)},
		{"Archive", plain("bob@example.test", "Archived"), now.Add(-48 * time.Hour)},
	} {
		if _, err := mem.User.Append(m.mailbox, strings.NewReader(m.raw), &imap.AppendOptions{Time: m.at}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := testConfig
	cfg.NoPrefetch = false
	h := newHarnessWith(t, mem.DialOptions(), imapxtest.Password, cfg)
	ctx := t.Context()
	if _, err := h.c.Vip().Add(ctx, &api.VipAddParams{Addresses: []string{"vip@example.test"}}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(api.SyncPhaseIdle)
	// A pass that settles after the VIP was added.
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		t.Fatal(err)
	}
	ids := h.ids(h.inbox().ID)
	var archive int64
	list, err := h.c.Mailbox().List(ctx, &api.MailboxListParams{AccountID: &h.acct})
	if err != nil {
		t.Fatal(err)
	}
	for _, mb := range list {
		if mb.Name == "Archive" {
			archive = mb.ID
		}
	}
	for k, v := range h.ids(archive) {
		ids[k] = v
	}
	waitUntil(t, 10*time.Second, "the recent inbox and VIP mail prefetched", func() bool {
		return h.bodyFetched(ids["Recent"]) && h.bodyFetched(ids["From a VIP"])
	})
	// The big message got its display copy, not its whole body.
	waitUntil(t, 10*time.Second, "the big message's display copy", func() bool {
		d, err := h.srv.DB.GetMessage(ctx, ids["Big"])
		return err == nil && d.DisplayBlobID != ""
	})
	if h.bodyFetched(ids["Big"]) {
		t.Error("the big message was prefetched whole")
	}
	for _, s := range []string{"Old", "Archived"} {
		if h.bodyFetched(ids[s]) {
			t.Errorf("%q was prefetched", s)
		}
	}
}
