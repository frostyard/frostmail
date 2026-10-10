package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	netmail "net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/compose"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/mimex"
	"github.com/frostyard/frostmail/internal/render"
	"github.com/frostyard/frostmail/internal/store"
)

// The draft, outbox, address and identity domains (docs/design/send.md).
// Drafts live in the store; the sync engine saves their server copies and
// sends what the outbox holds.

// AttachmentLimit is the most attachment data a draft may carry when the
// SMTP server's own limit is not known.
const AttachmentLimit = 25_000_000

// DefaultUndoDelay is how long a sent message waits in the outbox.
const DefaultUndoDelay = 10 * time.Second

type drafts struct{ Deps }

func (d drafts) Create(ctx context.Context, p *api.DraftCreateParams) (*api.Draft, error) {
	var src *store.MessageDetail
	if p.Kind != api.DraftKindNew {
		if p.SourceID == nil {
			return nil, api.InvalidParams("a %s needs sourceId", p.Kind)
		}
		m, err := d.DB.GetMessage(ctx, *p.SourceID)
		if err != nil {
			return nil, apiError(err, fmt.Sprintf("message %d", *p.SourceID))
		}
		src = &m
	}
	acctID, err := d.draftAccount(ctx, p.AccountID, src)
	if err != nil {
		return nil, err
	}
	ident, err := d.DB.DefaultIdentity(ctx, acctID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("an identity of account %d", acctID))
	}
	dr := store.Draft{
		AccountID: acctID,
		Kind:      string(p.Kind),
		Content: store.DraftContent{
			IdentityID: ident.ID, To: []store.Address{}, Cc: []store.Address{}, Bcc: []store.Address{},
		},
		MessageID: compose.NewMessageID(ident.Email),
	}
	var attach []mimePart
	if src == nil {
		dr.Content.HTML = "<p><br></p>" + signatureBlock(ident)
		for _, a := range p.To {
			if strings.TrimSpace(a.Address) != "" {
				dr.Content.To = append(dr.Content.To, store.Address{Name: strings.TrimSpace(a.Name), Addr: strings.TrimSpace(a.Address)})
			}
		}
	} else {
		dr.SourceID = src.ID
		if attach, err = d.startFrom(ctx, &dr, *src, ident, p.Kind); err != nil {
			return nil, err
		}
	}
	out, err := d.insertDraft(ctx, dr, attach, 0)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// draftAccount picks a new draft's account: the one asked for, the
// source's, or the first account.
func (d drafts) draftAccount(ctx context.Context, asked *int64, src *store.MessageDetail) (int64, error) {
	switch {
	case src != nil && asked != nil && *asked != src.AccountID:
		return 0, api.InvalidParams("message %d is not in account %d", src.ID, *asked)
	case src != nil:
		return src.AccountID, nil
	case asked != nil:
		if _, err := d.DB.GetAccount(ctx, *asked); err != nil {
			return 0, apiError(err, fmt.Sprintf("account %d", *asked))
		}
		return *asked, nil
	}
	accts, err := d.DB.ListAccounts(ctx)
	if err != nil {
		return 0, err
	}
	if len(accts) == 0 {
		return 0, api.InvalidParams("there is no account to write from")
	}
	return accts[0].ID, nil
}

// startFrom fills a reply or forward from its source message and returns
// the source's attachments to carry over (forwards only).
func (d drafts) startFrom(ctx context.Context, dr *store.Draft, src store.MessageDetail, ident store.Identity, kind api.DraftKind) ([]mimePart, error) {
	raw, err := messages(d).raw(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	source := compose.Source{
		From:       compose.Address{Name: src.From.Name, Addr: src.From.Addr},
		ReplyTo:    toComposeAddresses(src.ReplyTo),
		To:         toComposeAddresses(src.To),
		Cc:         toComposeAddresses(src.Cc),
		Subject:    src.Subject,
		MessageID:  src.MessageID,
		References: src.References,
		Date:       src.Date,
	}
	if d.Render != nil {
		r, err := d.Render.Render(ctx, src.ID, raw, false)
		if err != nil {
			return nil, err
		}
		source.HTML, source.Text = r.HTML, r.Text
	} else if source.Text, _, err = mimex.BodyText(raw); err != nil {
		return nil, err
	}
	sig := signatureBlock(ident)
	switch kind {
	case api.DraftKindForward:
		dr.Content.Subject = compose.ForwardSubject(src.Subject)
		dr.Content.HTML = withSignature(compose.ForwardHTML(source, time.Local), sig)
		return attachmentsOf(raw)
	default:
		self, err := d.ownAddresses(ctx, dr.AccountID)
		if err != nil {
			return nil, err
		}
		to, cc := compose.ReplyRecipients(source, self, kind == api.DraftKindReplyall)
		dr.Content.To, dr.Content.Cc = fromComposeAddresses(to), fromComposeAddresses(cc)
		dr.Content.Subject = compose.ReplySubject(src.Subject)
		dr.Content.HTML = withSignature(compose.QuoteHTML(source, time.Local), sig)
		dr.InReplyTo = src.MessageID
		dr.References = compose.ReplyReferences(source)
		return nil, nil
	}
}

// signatureBlock is the identity's signature with a blank line above it, or
// nothing.
func signatureBlock(ident store.Identity) string {
	if strings.TrimSpace(ident.SignatureHTML) == "" {
		return ""
	}
	return "<p><br></p>" + ident.SignatureHTML
}

// withSignature puts the signature between the first (empty) paragraph of a
// reply or forward body and the quoted message.
func withSignature(body, sig string) string {
	if sig == "" {
		return body
	}
	const lead = "<p><br></p>"
	return lead + sig + body
}

// ownAddresses are the account's own addresses, which replies leave out.
func (d drafts) ownAddresses(ctx context.Context, accountID int64) ([]string, error) {
	acct, err := d.DB.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	ids, err := d.DB.ListIdentities(ctx, accountID)
	if err != nil {
		return nil, err
	}
	out := []string{acct.Email}
	for _, i := range ids {
		out = append(out, i.Email)
	}
	return out, nil
}

// mimePart is a source attachment to copy into a new draft.
type mimePart struct {
	filename, contentType string
	data                  []byte
}

// attachmentsOf returns a message's attachments: parts marked attachment,
// and named parts not marked inline.
func attachmentsOf(raw []byte) ([]mimePart, error) {
	var out []mimePart
	err := mimex.WalkParts(raw, func(p mimex.PartInfo, body io.Reader) error {
		if p.Disposition != "attachment" && (p.Filename == "" || p.Disposition == "inline") {
			return nil
		}
		data, err := io.ReadAll(body)
		if err != nil {
			return err
		}
		name := p.Filename
		if name == "" {
			name = "attachment"
		}
		out = append(out, mimePart{filename: name, contentType: p.ContentType, data: data})
		return nil
	})
	return out, err
}

// insertDraft stores a new draft and its attachments, marks its server copy
// current (serverUID 0 means there is none yet: nothing to save until it is
// edited), and announces it.
func (d drafts) insertDraft(ctx context.Context, dr store.Draft, attach []mimePart, serverUID uint32) (*api.Draft, error) {
	stored := make([]store.DraftAttachment, 0, len(attach))
	for _, a := range attach {
		if d.Blobs == nil {
			return nil, api.Unavailable("the blob store is not running")
		}
		id, err := d.Blobs.Put(ctx, bytes.NewReader(a.data))
		if err != nil {
			return nil, err
		}
		stored = append(stored, store.DraftAttachment{
			BlobID: id, Filename: a.filename, ContentType: a.contentType, Size: int64(len(a.data)),
		})
	}
	var out store.Draft
	err := d.DB.Tx(ctx, func(tx *store.Tx) error {
		created, err := tx.CreateDraft(ctx, dr)
		if err != nil {
			return err
		}
		for _, a := range stored {
			if _, err := tx.AddDraftAttachment(ctx, created.ID, a); err != nil {
				return err
			}
		}
		if err := tx.SetDraftServerCopy(ctx, created.ID, serverUID, created.UpdatedAt); err != nil {
			return err
		}
		if out, err = tx.GetDraft(ctx, created.ID); err != nil {
			return err
		}
		return tx.Emit(ctx, api.DraftChanged{ID: created.ID, AccountID: created.AccountID})
	})
	if err != nil {
		return nil, err
	}
	r := toAPIDraft(out)
	return &r, nil
}

func (d drafts) Open(ctx context.Context, p *api.DraftOpenParams) (*api.Draft, error) {
	src, err := d.DB.GetMessage(ctx, p.MessageID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("message %d", p.MessageID))
	}
	if src.MessageID != "" {
		dr, err := d.DB.DraftByMessageID(ctx, src.AccountID, src.MessageID)
		if err == nil {
			r := toAPIDraft(dr)
			return &r, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}
	raw, err := messages(d).raw(ctx, src.ID)
	if err != nil {
		return nil, err
	}
	ident, err := d.DB.DefaultIdentity(ctx, src.AccountID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("an identity of account %d", src.AccountID))
	}
	if from, err := d.identityFor(ctx, src.AccountID, src.From.Addr); err == nil {
		ident = from
	}
	dr := store.Draft{
		AccountID: src.AccountID,
		Kind:      string(api.DraftKindNew),
		Content: store.DraftContent{
			IdentityID: ident.ID, To: nonNil(src.To), Cc: nonNil(src.Cc), Bcc: bccOf(raw),
			Subject: src.Subject,
		},
		MessageID:  src.MessageID,
		InReplyTo:  src.InReplyTo,
		References: src.References,
	}
	if dr.MessageID == "" {
		dr.MessageID = compose.NewMessageID(ident.Email)
	}
	if d.Render != nil {
		r, err := d.Render.Render(ctx, src.ID, raw, false)
		if err != nil {
			return nil, err
		}
		dr.Content.HTML = r.HTML
		if dr.Content.HTML == "" {
			dr.Content.HTML = textHTML(r.Text)
		}
	} else {
		text, _, err := mimex.BodyText(raw)
		if err != nil {
			return nil, err
		}
		dr.Content.HTML = textHTML(text)
	}
	attach, err := attachmentsOf(raw)
	if err != nil {
		return nil, err
	}
	return d.insertDraft(ctx, dr, attach, d.draftsUID(ctx, src))
}

// identityFor returns the account's identity with that address.
func (d drafts) identityFor(ctx context.Context, accountID int64, addr string) (store.Identity, error) {
	ids, err := d.DB.ListIdentities(ctx, accountID)
	if err != nil {
		return store.Identity{}, err
	}
	for _, i := range ids {
		if strings.EqualFold(i.Email, addr) {
			return i, nil
		}
	}
	return store.Identity{}, store.ErrNotFound
}

// draftsUID is the message's UID in its account's Drafts mailbox, or 0.
func (d drafts) draftsUID(ctx context.Context, m store.MessageDetail) uint32 {
	mb, err := d.DB.MailboxByRole(ctx, m.AccountID, api.MailboxRoleDrafts)
	if err != nil {
		return 0
	}
	var uid uint32
	_ = d.DB.Tx(ctx, func(tx *store.Tx) error {
		mems, err := tx.Memberships(ctx, []int64{m.ID})
		for _, mem := range mems {
			if mem.MailboxID == mb.ID {
				uid = mem.UID
			}
		}
		return err
	})
	return uid
}

// bccOf reads the Bcc field a draft copy keeps.
func bccOf(raw []byte) []store.Address {
	out := []store.Address{}
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return out
	}
	list, err := msg.Header.AddressList("Bcc")
	if err != nil {
		return out
	}
	for _, a := range list {
		out = append(out, store.Address{Name: a.Name, Addr: a.Address})
	}
	return out
}

// textHTML shows plain text as paragraphs with line breaks.
func textHTML(text string) string {
	return "<p>" + strings.ReplaceAll(html.EscapeString(text), "\n", "<br>") + "</p>"
}

func nonNil(list []store.Address) []store.Address {
	if list == nil {
		return []store.Address{}
	}
	return list
}

func (d drafts) Get(ctx context.Context, p *api.DraftGetParams) (*api.Draft, error) {
	dr, err := d.DB.GetDraft(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("draft %d", p.ID))
	}
	r := toAPIDraft(dr)
	return &r, nil
}

func (d drafts) List(ctx context.Context, p *api.DraftListParams) ([]api.Draft, error) {
	var acct int64
	if p.AccountID != nil {
		acct = *p.AccountID
	}
	list, err := d.DB.ListDrafts(ctx, acct)
	if err != nil {
		return nil, err
	}
	out := make([]api.Draft, 0, len(list))
	for _, dr := range list {
		out = append(out, toAPIDraft(dr))
	}
	return out, nil
}

func (d drafts) Update(ctx context.Context, p *api.DraftUpdateParams) (*api.Draft, error) {
	cur, err := d.DB.GetDraft(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("draft %d", p.ID))
	}
	if err := d.checkContent(ctx, cur.AccountID, p.Content); err != nil {
		return nil, err
	}
	content := store.DraftContent{
		IdentityID: p.Content.IdentityID,
		To:         fromAPIAddresses(p.Content.To),
		Cc:         fromAPIAddresses(p.Content.Cc),
		Bcc:        fromAPIAddresses(p.Content.Bcc),
		Subject:    p.Content.Subject,
		HTML:       p.Content.HTML,
	}
	var out store.Draft
	err = d.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		if out, err = tx.UpdateDraftContent(ctx, p.ID, content); err != nil {
			return err
		}
		return tx.Emit(ctx, api.DraftChanged{ID: out.ID, AccountID: out.AccountID})
	})
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("draft %d", p.ID))
	}
	d.draftsChanged(out.AccountID)
	r := toAPIDraft(out)
	return &r, nil
}

// checkContent refuses what a draft can never hold: another account's
// identity, and line breaks in the subject or in an address or name (a
// half-typed address is fine until sending).
func (d drafts) checkContent(ctx context.Context, accountID int64, c api.DraftContent) error {
	ident, err := d.DB.GetIdentity(ctx, c.IdentityID)
	if errors.Is(err, store.ErrNotFound) || err == nil && ident.AccountID != accountID {
		return api.InvalidParams("identity %d does not belong to account %d", c.IdentityID, accountID)
	}
	if err != nil {
		return err
	}
	if strings.ContainsAny(c.Subject, "\r\n") {
		return api.InvalidParams("the subject contains a line break")
	}
	for _, list := range [][]api.Address{c.To, c.Cc, c.Bcc} {
		for _, a := range list {
			if strings.ContainsAny(a.Name+a.Address, "\r\n") {
				return api.InvalidParams("address %q contains a line break", a.Address)
			}
		}
	}
	return nil
}

func (d drafts) Attach(ctx context.Context, p *api.DraftAttachParams) (*api.DraftAttachment, error) {
	if d.Blobs == nil {
		return nil, api.Unavailable("the blob store is not running")
	}
	dr, err := d.DB.GetDraft(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("draft %d", p.ID))
	}
	if !filepath.IsAbs(p.Path) {
		return nil, api.InvalidParams("path %q is not absolute", p.Path)
	}
	st, err := os.Stat(p.Path)
	if err != nil {
		return nil, api.InvalidParams("cannot read %s: %v", p.Path, err)
	}
	if !st.Mode().IsRegular() {
		return nil, api.InvalidParams("%s is not a regular file", p.Path)
	}
	if st.Size() > AttachmentLimit {
		return nil, api.InvalidParams("%s is larger than the %d MB limit", filepath.Base(p.Path), AttachmentLimit/1_000_000)
	}
	f, err := os.Open(p.Path)
	if err != nil {
		return nil, api.InvalidParams("cannot read %s: %v", p.Path, err)
	}
	defer f.Close()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	blobID, err := d.Blobs.Put(ctx, f)
	if err != nil {
		return nil, err
	}
	a := store.DraftAttachment{
		BlobID: blobID, Filename: filepath.Base(p.Path), ContentType: contentTypeOf(p.Path, head[:n]), Size: st.Size(),
	}
	err = d.DB.Tx(ctx, func(tx *store.Tx) error {
		if a, err = tx.AddDraftAttachment(ctx, dr.ID, a); err != nil {
			return err
		}
		if err := tx.TouchDraft(ctx, dr.ID); err != nil {
			return err
		}
		return tx.Emit(ctx, api.DraftChanged{ID: dr.ID, AccountID: dr.AccountID})
	})
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("draft %d", p.ID))
	}
	d.draftsChanged(dr.AccountID)
	r := toAPIAttachment(a)
	return &r, nil
}

// contentTypeOf names a file's type from its extension, else from its
// first bytes; lowercase type/subtype without parameters.
func contentTypeOf(path string, head []byte) string {
	t := mime.TypeByExtension(filepath.Ext(path))
	if t == "" {
		t = http.DetectContentType(head)
	}
	if mt, _, err := mime.ParseMediaType(t); err == nil {
		return mt
	}
	return "application/octet-stream"
}

func (d drafts) Detach(ctx context.Context, p *api.DraftDetachParams) error {
	dr, err := d.DB.GetDraft(ctx, p.ID)
	if err != nil {
		return apiError(err, fmt.Sprintf("draft %d", p.ID))
	}
	err = d.DB.Tx(ctx, func(tx *store.Tx) error {
		if err := tx.RemoveDraftAttachment(ctx, p.ID, p.AttachmentID); err != nil {
			return err
		}
		if err := tx.TouchDraft(ctx, p.ID); err != nil {
			return err
		}
		return tx.Emit(ctx, api.DraftChanged{ID: dr.ID, AccountID: dr.AccountID})
	})
	if err != nil {
		return apiError(err, fmt.Sprintf("attachment %d of draft %d", p.AttachmentID, p.ID))
	}
	d.draftsChanged(dr.AccountID)
	return nil
}

func (d drafts) Delete(ctx context.Context, p *api.DraftDeleteParams) error {
	var acct int64
	err := d.DB.Tx(ctx, func(tx *store.Tx) error {
		dr, err := tx.DeleteDraft(ctx, p.ID)
		if err != nil {
			return err
		}
		acct = dr.AccountID
		if err := mailsync.QueueRemoveDraftCopy(ctx, tx, dr.AccountID, dr.ServerUID); err != nil {
			return err
		}
		return tx.Emit(ctx, api.DraftChanged{ID: dr.ID, AccountID: dr.AccountID, Deleted: true})
	})
	if err != nil {
		return apiError(err, fmt.Sprintf("draft %d", p.ID))
	}
	if d.Sync != nil {
		d.Sync.Kick(acct)
	}
	return nil
}

func (d drafts) Send(ctx context.Context, p *api.DraftSendParams) (*api.OutboxItem, error) {
	if d.Blobs == nil {
		return nil, api.Unavailable("the blob store is not running")
	}
	dr, err := d.DB.GetDraft(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("draft %d", p.ID))
	}
	ident, err := d.DB.GetIdentity(ctx, dr.Content.IdentityID)
	if err != nil || ident.AccountID != dr.AccountID {
		return nil, api.InvalidParams("the draft's identity %d is not in its account", dr.Content.IdentityID)
	}
	if err := checkSendable(dr); err != nil {
		return nil, err
	}
	acct, err := d.DB.GetAccount(ctx, dr.AccountID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("account %d", dr.AccountID))
	}
	if acct.ReadOnly {
		return nil, api.Conflict("account %d is read-only", dr.AccountID)
	}
	m := mailsync.DraftMessage(dr, ident, d.Blobs, d.parts(), time.Now())
	var buf bytes.Buffer
	if err := compose.Build(&buf, m); errors.Is(err, compose.ErrInvalid) {
		return nil, api.InvalidParams("%v", err)
	} else if err != nil {
		return nil, err
	}
	blobID, err := d.Blobs.Put(ctx, &buf)
	if err != nil {
		return nil, err
	}
	delay := d.UndoDelay
	if delay == 0 {
		delay = DefaultUndoDelay
	}
	var item store.OutboxItem
	err = d.DB.Tx(ctx, func(tx *store.Tx) error {
		prior, err := tx.OutboxOfDraft(ctx, dr.ID)
		if err != nil {
			return err
		}
		for _, o := range prior {
			if o.State != "failed" {
				return api.Conflict("draft %d is already being sent", dr.ID)
			}
			if err := tx.DeleteOutbox(ctx, o.ID); err != nil {
				return err
			}
			if err := tx.Emit(ctx, api.OutboxChanged{ID: o.ID, AccountID: o.AccountID, State: api.OutboxStateFailed, Deleted: true}); err != nil {
				return err
			}
		}
		item, err = tx.QueueOutbox(ctx, store.OutboxItem{
			AccountID: dr.AccountID, DraftID: dr.ID, SendAt: tx.Now().Add(delay), BlobID: blobID,
			MessageID: dr.MessageID, From: ident.Email, Recipients: m.Envelope(),
			Subject: dr.Content.Subject, To: dr.Content.To,
		})
		if err != nil {
			return err
		}
		return tx.Emit(ctx, api.OutboxChanged{ID: item.ID, AccountID: item.AccountID, State: api.OutboxStateQueued})
	})
	if err != nil {
		return nil, err
	}
	if d.Sync != nil {
		d.Sync.OutboxChanged(dr.AccountID)
	}
	r := toAPIOutbox(item)
	return &r, nil
}

// checkSendable refuses a draft without recipients, with an address that
// does not parse, or with too much attached.
func checkSendable(dr store.Draft) error {
	c := dr.Content
	n := 0
	for _, list := range [][]store.Address{c.To, c.Cc, c.Bcc} {
		for _, a := range list {
			if !compose.ValidAddress(compose.Address{Name: a.Name, Addr: a.Addr}) {
				return api.InvalidParams("%q is not a valid address", a.Addr)
			}
			n++
		}
	}
	if n == 0 {
		return api.InvalidParams("the message has no recipients")
	}
	var size int64
	for _, a := range dr.Attachments {
		size += a.Size
	}
	if size > AttachmentLimit {
		return api.InvalidParams("the attachments are larger than the %d MB limit", AttachmentLimit/1_000_000)
	}
	return nil
}

func (d drafts) draftsChanged(accountID int64) {
	if d.Sync != nil {
		d.Sync.DraftsChanged(accountID)
	}
}

func (d Deps) parts() *render.PartsCache {
	if d.Render == nil {
		return nil
	}
	return d.Render.Parts
}

type outbox struct{ Deps }

func (o outbox) List(ctx context.Context, p *api.OutboxListParams) ([]api.OutboxItem, error) {
	var acct int64
	if p.AccountID != nil {
		acct = *p.AccountID
	}
	list, err := o.DB.ListOutbox(ctx, acct)
	if err != nil {
		return nil, err
	}
	out := make([]api.OutboxItem, 0, len(list))
	for _, it := range list {
		out = append(out, toAPIOutbox(it))
	}
	return out, nil
}

func (o outbox) Cancel(ctx context.Context, p *api.OutboxCancelParams) (*api.Draft, error) {
	var dr store.Draft
	err := o.DB.Tx(ctx, func(tx *store.Tx) error {
		it, err := tx.CancelOutbox(ctx, p.ID)
		if errors.Is(err, store.ErrConflict) {
			return api.Conflict("message %d is already being sent", p.ID)
		}
		if err != nil {
			return err
		}
		if err := tx.Emit(ctx, api.OutboxChanged{ID: it.ID, AccountID: it.AccountID, State: api.OutboxStateQueued, Deleted: true}); err != nil {
			return err
		}
		if it.DraftID == 0 {
			// A message maild wrote itself, such as an invitation's
			// answer, has no draft to return to.
			return nil
		}
		dr, err = tx.GetDraft(ctx, it.DraftID)
		return err
	})
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("outbox message %d", p.ID))
	}
	r := toAPIDraft(dr)
	return &r, nil
}

func (o outbox) Retry(ctx context.Context, p *api.OutboxRetryParams) error {
	var acct int64
	err := o.DB.Tx(ctx, func(tx *store.Tx) error {
		err := tx.RequeueOutbox(ctx, p.ID, tx.Now())
		if errors.Is(err, store.ErrConflict) {
			return api.Conflict("message %d has not failed", p.ID)
		}
		if err != nil {
			return err
		}
		it, err := tx.GetOutbox(ctx, p.ID)
		if err != nil {
			return err
		}
		acct = it.AccountID
		return tx.Emit(ctx, api.OutboxChanged{ID: it.ID, AccountID: it.AccountID, State: api.OutboxStateQueued})
	})
	if err != nil {
		return apiError(err, fmt.Sprintf("outbox message %d", p.ID))
	}
	if o.Sync != nil {
		o.Sync.OutboxChanged(acct)
	}
	return nil
}

type addresses struct{ Deps }

func (a addresses) Suggest(ctx context.Context, p *api.AddressSuggestParams) ([]api.Address, error) {
	limit := 10
	if p.Limit != nil && *p.Limit > 0 {
		limit = int(*p.Limit)
	}
	list, err := a.DB.SuggestContacts(ctx, p.Prefix, limit)
	if err != nil {
		return nil, err
	}
	if len(list) == limit {
		return toAPIAddresses(list), nil
	}
	seen, err := a.DB.SuggestAddresses(ctx, p.Prefix, limit)
	if err != nil {
		return nil, err
	}
	done := make(map[string]bool, len(list))
	for _, addr := range list {
		done[addr.Addr] = true
	}
	for _, addr := range seen {
		if !done[addr.Addr] {
			list = append(list, addr)
			done[addr.Addr] = true
			if len(list) == limit {
				break
			}
		}
	}
	return toAPIAddresses(list), nil
}

type identities struct{ Deps }

func (i identities) List(ctx context.Context, p *api.IdentityListParams) ([]api.Identity, error) {
	var acct int64
	if p.AccountID != nil {
		acct = *p.AccountID
	}
	list, err := i.DB.ListIdentities(ctx, acct)
	if err != nil {
		return nil, err
	}
	out := make([]api.Identity, 0, len(list))
	for _, id := range list {
		out = append(out, toAPIIdentity(id))
	}
	return out, nil
}

func (i identities) Update(ctx context.Context, p *api.IdentityUpdateParams) (*api.Identity, error) {
	if p.Name != nil && strings.ContainsAny(*p.Name, "\r\n") {
		return nil, api.InvalidParams("the name contains a line break")
	}
	if p.ReplyTo != nil && *p.ReplyTo != "" && !compose.ValidAddress(compose.Address{Addr: *p.ReplyTo}) {
		return nil, api.InvalidParams("%q is not a valid address", *p.ReplyTo)
	}
	var out store.Identity
	err := i.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.UpdateIdentity(ctx, p.ID, store.IdentityUpdate{Name: p.Name, ReplyTo: p.ReplyTo, SignatureHTML: p.SignatureHTML})
		return err
	})
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("identity %d", p.ID))
	}
	r := toAPIIdentity(out)
	return &r, nil
}

// Create adds an address to an account. Task T-0096 builds it.
func (i identities) Create(context.Context, *api.IdentityCreateParams) (*api.Identity, error) {
	return nil, errors.New("identity.create: task T-0096 builds it")
}

// Delete removes an added address. Task T-0096 builds it.
func (i identities) Delete(context.Context, *api.IdentityDeleteParams) error {
	return errors.New("identity.delete: task T-0096 builds it")
}

func toAPIDraft(d store.Draft) api.Draft {
	out := api.Draft{
		ID:        d.ID,
		AccountID: d.AccountID,
		Content: api.DraftContent{
			IdentityID: d.Content.IdentityID,
			To:         toAPIAddresses(d.Content.To),
			Cc:         toAPIAddresses(d.Content.Cc),
			Bcc:        toAPIAddresses(d.Content.Bcc),
			Subject:    d.Content.Subject,
			HTML:       d.Content.HTML,
		},
		Attachments: make([]api.DraftAttachment, 0, len(d.Attachments)),
		Kind:        api.DraftKind(d.Kind),
		UpdatedAt:   d.UpdatedAt,
	}
	if d.SourceID != 0 {
		out.SourceID = &d.SourceID
	}
	for _, a := range d.Attachments {
		out.Attachments = append(out.Attachments, toAPIAttachment(a))
	}
	return out
}

func toAPIAttachment(a store.DraftAttachment) api.DraftAttachment {
	return api.DraftAttachment{ID: a.ID, Filename: a.Filename, ContentType: a.ContentType, Size: a.Size}
}

func toAPIOutbox(o store.OutboxItem) api.OutboxItem {
	out := api.OutboxItem{
		ID: o.ID, AccountID: o.AccountID, Subject: o.Subject, To: toAPIAddresses(o.To),
		State: api.OutboxState(o.State), Attempts: int64(o.Attempts),
	}
	if o.DraftID != 0 {
		out.DraftID = &o.DraftID
	}
	if o.State == "queued" {
		at := o.SendAt
		out.SendAt = &at
	}
	if o.LastError != "" {
		e := o.LastError
		out.Error = &e
	}
	return out
}

func toAPIIdentity(i store.Identity) api.Identity {
	return api.Identity{
		ID: i.ID, AccountID: i.AccountID, Name: i.Name, Email: i.Email, ReplyTo: i.ReplyTo,
		SignatureHTML: i.SignatureHTML, IsDefault: i.IsDefault,
	}
}

func fromAPIAddresses(list []api.Address) []store.Address {
	out := make([]store.Address, 0, len(list))
	for _, a := range list {
		out = append(out, store.Address{Name: strings.TrimSpace(a.Name), Addr: strings.TrimSpace(a.Address)})
	}
	return out
}

func toComposeAddresses(list []store.Address) []compose.Address {
	out := make([]compose.Address, 0, len(list))
	for _, a := range list {
		out = append(out, compose.Address{Name: a.Name, Addr: a.Addr})
	}
	return out
}

func fromComposeAddresses(list []compose.Address) []store.Address {
	out := make([]store.Address, 0, len(list))
	for _, a := range list {
		out = append(out, store.Address{Name: a.Name, Addr: a.Addr})
	}
	return out
}
