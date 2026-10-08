package mailsync_test

import (
	"bytes"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
)

const plansMessage = "From: Bob <bob@x.test>\r\n" +
	"To: Ann Example <ann@x.test>, Carol <carol@x.test>\r\n" +
	"Cc: dan@x.test\r\n" +
	"Subject: Plans\r\n" +
	"Date: Wed, 07 Oct 2026 09:30:00 +0000\r\n" +
	"Message-ID: <src@x.test>\r\n" +
	"References: <root@x.test>\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: multipart/mixed; boundary=b\r\n\r\n" +
	"--b\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Let's meet <b>Friday</b>.</p>\r\n" +
	"--b\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=plan.pdf\r\n" +
	"Content-Transfer-Encoding: base64\r\n\r\nJVBERi0xLjQK\r\n" +
	"--b--\r\n"

// deliver puts a raw message in a server mailbox, syncs, and returns its ID
// once the message is stored (waiting for a sync phase could match an
// earlier pass).
func (h *sendHarness) deliver(mailbox, raw, subject string, flags ...imap.Flag) int64 {
	h.t.Helper()
	ctx := h.t.Context()
	if _, err := h.mem.User.Append(mailbox, strings.NewReader(raw), &imap.AppendOptions{Flags: flags}); err != nil {
		h.t.Fatal(err)
	}
	if err := h.c.Sync().Now(ctx, &api.SyncNowParams{AccountID: h.acct}); err != nil {
		h.t.Fatal(err)
	}
	var id int64
	find := func() bool {
		id = h.find(mailbox, subject)
		return id != 0
	}
	if !find() {
		h.waitFor(10*time.Second, subject+" in "+mailbox, func(api.EventEnvelope, api.Event) bool { return find() })
	}
	return id
}

// find returns the ID of the message with subject in a mailbox, or 0.
func (h *sendHarness) find(mailbox, subject string) int64 {
	h.t.Helper()
	ctx := h.t.Context()
	list, err := h.c.Mailbox().List(ctx, &api.MailboxListParams{AccountID: &h.acct})
	if err != nil {
		h.t.Fatal(err)
	}
	for _, mb := range list {
		if mb.Path != mailbox {
			continue
		}
		v, err := h.c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{MailboxID: &mb.ID}})
		if err != nil {
			h.t.Fatal(err)
		}
		defer func() { _ = h.c.View().Close(ctx, &api.ViewCloseParams{ID: v.ID}) }()
		rows, err := h.c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: v.Count})
		if err != nil {
			h.t.Fatal(err)
		}
		for _, r := range rows {
			if r.Subject == subject {
				return r.ID
			}
		}
	}
	return 0
}

func addresses(list []api.Address) []string {
	var out []string
	for _, a := range list {
		out = append(out, a.Name+" <"+a.Address+">")
	}
	return out
}

func TestRepliesAndForwardsStartFromTheSource(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	src := h.deliver("INBOX", plansMessage, "Plans")

	reply, err := h.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindReply, SourceID: &src})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Kind != api.DraftKindReply || reply.SourceID == nil || *reply.SourceID != src || reply.AccountID != h.acct {
		t.Errorf("reply = %+v", reply)
	}
	if got := addresses(reply.Content.To); !slices.Equal(got, []string{"Bob <bob@x.test>"}) || len(reply.Content.Cc) != 0 {
		t.Errorf("reply goes to %v, cc %v", got, reply.Content.Cc)
	}
	html := reply.Content.HTML
	if reply.Content.Subject != "Re: Plans" || !strings.Contains(html, "Bob wrote:") ||
		!strings.Contains(html, `<blockquote type="cite">`) || !strings.Contains(html, "Friday") {
		t.Errorf("reply subject %q, html %s", reply.Content.Subject, html)
	}

	all, err := h.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindReplyall, SourceID: &src})
	if err != nil {
		t.Fatal(err)
	}
	if got := addresses(all.Content.Cc); !slices.Equal(got, []string{"Carol <carol@x.test>", " <dan@x.test>"}) {
		t.Errorf("reply all copies %v (Ann is the account and stays out)", got)
	}

	fwd, err := h.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindForward, SourceID: &src})
	if err != nil {
		t.Fatal(err)
	}
	if fwd.Content.Subject != "Fwd: Plans" || len(fwd.Content.To) != 0 || !strings.Contains(fwd.Content.HTML, "Begin forwarded message:") {
		t.Errorf("forward = %+v", fwd.Content)
	}
	if len(fwd.Attachments) != 1 || fwd.Attachments[0].Filename != "plan.pdf" || fwd.Attachments[0].Size != 9 {
		t.Errorf("forward attachments = %+v", fwd.Attachments)
	}

	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: reply.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent)
	head, _, _ := bytes.Cut(h.smtp.Messages()[0].Data, []byte("\r\n\r\n"))
	for _, want := range []string{"In-Reply-To: <src@x.test>", "References: <root@x.test> <src@x.test>", "Subject: Re: Plans"} {
		if !bytes.Contains(head, []byte(want)) {
			t.Errorf("reply header lacks %q:\n%s", want, head)
		}
	}
}

func TestOpenADraftFromTheDraftsMailbox(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	raw := "From: Ann Example <ann@x.test>\r\nTo: Bob <bob@x.test>\r\nBcc: dan@x.test\r\n" +
		"Subject: Saved elsewhere\r\nMessage-ID: <elsewhere@x.test>\r\nContent-Type: text/plain\r\n\r\nHalf done\r\n"
	id := h.deliver("Drafts", raw, "Saved elsewhere", imap.FlagDraft)

	d, err := h.c.Draft().Open(ctx, &api.DraftOpenParams{MessageID: id})
	if err != nil {
		t.Fatal(err)
	}
	c := d.Content
	if c.Subject != "Saved elsewhere" || !slices.Equal(addresses(c.To), []string{"Bob <bob@x.test>"}) ||
		!slices.Equal(addresses(c.Bcc), []string{" <dan@x.test>"}) || !strings.Contains(c.HTML, "Half done") {
		t.Errorf("opened draft = %+v", c)
	}
	again, err := h.c.Draft().Open(ctx, &api.DraftOpenParams{MessageID: id})
	if err != nil || again.ID != d.ID {
		t.Errorf("opening again = %+v, %v; want draft %d", again, err, d.ID)
	}

	c.Subject = "Finished"
	if _, err := h.c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: c}); err != nil {
		t.Fatal(err)
	}
	h.waitServer("Drafts", "the opened draft's copy replaced", func(raws [][]byte) bool {
		return len(raws) == 1 && bytes.Contains(raws[0], []byte("Subject: Finished"))
	})
}

func TestIdentitySignatureAndReplyTo(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	ids, err := h.c.Identity().List(ctx, &api.IdentityListParams{AccountID: &h.acct})
	if err != nil || len(ids) != 1 || ids[0].Email != "ann@x.test" || ids[0].Name != "Ann Example" || !ids[0].IsDefault {
		t.Fatalf("identities = %+v, %v", ids, err)
	}
	sig := "<p>-- Ann</p>"
	up, err := h.c.Identity().Update(ctx, &api.IdentityUpdateParams{ID: ids[0].ID, ReplyTo: ptr("team@x.test"), SignatureHTML: &sig})
	if err != nil || up.ReplyTo != "team@x.test" || up.SignatureHTML != sig {
		t.Fatalf("update = %+v, %v", up, err)
	}
	if _, err := h.c.Identity().Update(ctx, &api.IdentityUpdateParams{ID: ids[0].ID, ReplyTo: ptr("not an address")}); !isCode(err, api.CodeInvalidParams) {
		t.Errorf("a bad Reply-To = %v, want invalidParams", err)
	}
	d, err := h.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew})
	if err != nil || !strings.HasSuffix(d.Content.HTML, sig) {
		t.Fatalf("new draft html = %q, %v", d.Content.HTML, err)
	}
	c := d.Content
	c.To = []api.Address{bob}
	if _, err := h.c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: c}); err != nil {
		t.Fatal(err)
	}
	item, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	h.waitOutbox(item.ID, api.OutboxStateSent)
	if data := h.smtp.Messages()[0].Data; !bytes.Contains(data, []byte("Reply-To: <team@x.test>")) {
		t.Errorf("no Reply-To in:\n%s", data)
	}
}

func TestDraftRequestsAreChecked(t *testing.T) {
	h := newSendHarness(t, 0)
	ctx := t.Context()
	if _, err := h.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindReply}); !isCode(err, api.CodeInvalidParams) {
		t.Errorf("a reply without a source = %v", err)
	}
	missing := int64(999)
	if _, err := h.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindForward, SourceID: &missing}); !isCode(err, api.CodeNotFound) {
		t.Errorf("a forward of a missing message = %v", err)
	}
	d, err := h.c.Draft().Create(ctx, &api.DraftCreateParams{Kind: api.DraftKindNew})
	if err != nil {
		t.Fatal(err)
	}
	bad := map[string]func(*api.DraftContent){
		"another identity": func(c *api.DraftContent) { c.IdentityID = 999 },
		"subject newline":  func(c *api.DraftContent) { c.Subject = "a\r\nBcc: x@y.test" },
		"address newline":  func(c *api.DraftContent) { c.To = []api.Address{{Address: "a@x.test\nBcc: x@y.test"}} },
	}
	for name, mutate := range bad {
		c := d.Content
		mutate(&c)
		if _, err := h.c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: c}); !isCode(err, api.CodeInvalidParams) {
			t.Errorf("update with %s = %v, want invalidParams", name, err)
		}
	}
	if _, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID}); !isCode(err, api.CodeInvalidParams) {
		t.Errorf("sending without recipients = %v", err)
	}
	c := d.Content
	c.To = []api.Address{bob, {Address: "half-typed"}}
	if _, err := h.c.Draft().Update(ctx, &api.DraftUpdateParams{ID: d.ID, Content: c}); err != nil {
		t.Fatalf("a draft may hold a half-typed address: %v", err)
	}
	if _, err := h.c.Draft().Send(ctx, &api.DraftSendParams{ID: d.ID}); !isCode(err, api.CodeInvalidParams) {
		t.Errorf("sending to a half-typed address = %v", err)
	}
	for _, path := range []string{"relative.txt", t.TempDir()} {
		if _, err := h.c.Draft().Attach(ctx, &api.DraftAttachParams{ID: d.ID, Path: path}); !isCode(err, api.CodeInvalidParams) {
			t.Errorf("attach %q = %v", path, err)
		}
	}
	if n := len(h.smtp.Messages()); n != 0 {
		t.Errorf("SMTP server got %d messages", n)
	}
}
