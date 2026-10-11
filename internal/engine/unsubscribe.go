package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/compose"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/unsubscribe"
)

// Unsubscriber sends one-click unsubscribe requests (unsubscribe.Poster).
type Unsubscriber interface {
	Post(ctx context.Context, url string) error
}

// unsubscribeOf works out how a message can be left (ADR-0027): its stored
// headers, and List-Unsubscribe-Post from the stored message.
func (m messages) unsubscribeOf(ctx context.Context, id int64) (store.MessageDetail, unsubscribe.Info, error) {
	detail, err := m.DB.GetMessage(ctx, id)
	if err != nil {
		return store.MessageDetail{}, unsubscribe.Info{}, apiError(err, fmt.Sprintf("message %d", id))
	}
	if detail.ListUnsubscribe == "" {
		return detail, unsubscribe.Info{}, nil
	}
	raw, err := m.raw(ctx, id)
	if err != nil {
		return store.MessageDetail{}, unsubscribe.Info{}, err
	}
	var post string
	if msg, err := mail.ReadMessage(bytes.NewReader(raw)); err == nil {
		post = msg.Header.Get("List-Unsubscribe-Post")
	}
	return detail, unsubscribe.Parse(detail.ListUnsubscribe, post, detail.AuthResults), nil
}

// listName is a list's name: List-Id's phrase, else its ID, else the
// sender's name or address.
func listName(detail store.MessageDetail) string {
	id := strings.TrimSpace(detail.ListID)
	if open := strings.Index(id, "<"); open >= 0 {
		if phrase := strings.Trim(strings.TrimSpace(id[:open]), `"`); phrase != "" {
			return phrase
		}
		id = strings.Trim(id[open:], "<> ")
	}
	if id != "" {
		return id
	}
	if detail.From.Name != "" {
		return detail.From.Name
	}
	return detail.From.Addr
}

// UnsubscribeInfo implements message.unsubscribeInfo.
func (m messages) UnsubscribeInfo(ctx context.Context, p *api.MessageUnsubscribeInfoParams) (*api.Unsubscribe, error) {
	detail, info, err := m.unsubscribeOf(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	done, err := m.DB.Unsubscribed(ctx, store.UnsubscribeKey(detail.ListID, detail.ID))
	if err != nil {
		return nil, err
	}
	out := &api.Unsubscribe{Methods: []api.UnsubscribeMethod{}, List: listName(detail), Done: done}
	for _, method := range info.Methods {
		out.Methods = append(out.Methods, api.UnsubscribeMethod(method))
	}
	if slices.Contains(info.Methods, unsubscribe.OneClick) {
		out.Host = &info.Host
	}
	if info.Address != "" {
		out.Address = &info.Address
	}
	if slices.Contains(info.Methods, unsubscribe.Web) {
		out.URL = &info.URL
	}
	return out, nil
}

// Unsubscribe implements message.unsubscribe.
func (m messages) Unsubscribe(ctx context.Context, p *api.MessageUnsubscribeParams) (*api.UnsubscribeResult, error) {
	detail, info, err := m.unsubscribeOf(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(info.Methods, unsubscribe.Method(p.Method)) {
		return nil, api.InvalidParams("message %d does not offer %q", p.ID, p.Method)
	}
	out := &api.UnsubscribeResult{}
	var queued *store.OutboxItem
	switch unsubscribe.Method(p.Method) {
	case unsubscribe.OneClick:
		if m.Unsubscriber == nil {
			return nil, api.Unavailable("one-click unsubscribe is not available")
		}
		if err := m.Unsubscriber.Post(ctx, info.URL); err != nil {
			return nil, api.Unavailable("%v", err)
		}
	case unsubscribe.Mail:
		item, err := m.queueOwn(ctx, detail.AccountID, []compose.Address{{Addr: info.Address}}, info.Subject,
			"<p>"+html.EscapeString(info.Body)+"</p>")
		if err != nil {
			return nil, err
		}
		queued = &item
	case unsubscribe.Web:
		out.URL = &info.URL
	}
	err = m.DB.Tx(ctx, func(tx *store.Tx) error {
		if queued != nil {
			item, err := tx.QueueOutbox(ctx, *queued)
			if err != nil {
				return err
			}
			if err := tx.Emit(ctx, api.OutboxChanged{ID: item.ID, AccountID: item.AccountID, State: api.OutboxStateQueued}); err != nil {
				return err
			}
		}
		return tx.RecordUnsubscribe(ctx, store.UnsubscribeKey(detail.ListID, detail.ID))
	})
	if err != nil {
		return nil, err
	}
	if queued != nil && m.Sync != nil {
		m.Sync.OutboxChanged(detail.AccountID)
	}
	return out, nil
}

// queueOwn builds a message maild writes itself, from the account's own
// address, and returns its outbox row, due after the undo delay.
func (m messages) queueOwn(ctx context.Context, accountID int64, to []compose.Address, subject, body string) (store.OutboxItem, error) {
	ident, acct, err := m.ownIdentity(ctx, accountID)
	if err != nil {
		return store.OutboxItem{}, err
	}
	now := m.DB.Now()
	msg := compose.Message{From: compose.Address{Name: ident.Name, Addr: ident.Email}, To: to, Subject: subject,
		MessageID: compose.NewMessageID(ident.Email), Date: now, HTML: body}
	var buf bytes.Buffer
	if err := compose.Build(&buf, msg); errors.Is(err, compose.ErrInvalid) {
		return store.OutboxItem{}, api.InvalidParams("%v", err)
	} else if err != nil {
		return store.OutboxItem{}, err
	}
	return m.outboxItem(ctx, acct.ID, &buf, msg.MessageID, ident.Email, msg.Envelope(), subject, to, now)
}

// ownIdentity is a writable account's default identity.
func (m messages) ownIdentity(ctx context.Context, accountID int64) (store.Identity, store.Account, error) {
	acct, err := m.DB.GetAccount(ctx, accountID)
	if err != nil {
		return store.Identity{}, store.Account{}, apiError(err, fmt.Sprintf("account %d", accountID))
	}
	if acct.ReadOnly {
		return store.Identity{}, store.Account{}, api.Conflict("account %d is read-only", accountID)
	}
	ident, err := m.DB.DefaultIdentity(ctx, accountID)
	if err != nil {
		return store.Identity{}, store.Account{}, err
	}
	return ident, acct, nil
}

// outboxItem stores a built message and returns its outbox row.
func (m messages) outboxItem(ctx context.Context, accountID int64, raw *bytes.Buffer, messageID, from string,
	recipients []string, subject string, to []compose.Address, now time.Time,
) (store.OutboxItem, error) {
	if m.Blobs == nil {
		return store.OutboxItem{}, api.Unavailable("the blob store is not running")
	}
	blobID, err := m.Blobs.Put(ctx, raw)
	if err != nil {
		return store.OutboxItem{}, err
	}
	delay, err := m.undoDelay(ctx)
	if err != nil {
		return store.OutboxItem{}, err
	}
	addrs := make([]store.Address, len(to))
	for i, a := range to {
		addrs[i] = store.Address{Name: a.Name, Addr: a.Addr}
	}
	return store.OutboxItem{AccountID: accountID, SendAt: now.Add(delay), BlobID: blobID, MessageID: messageID,
		From: from, Recipients: recipients, Subject: subject, To: addrs}, nil
}

// Redirect implements message.redirect: the message as it is, with
// Resent-* fields above its header, to other people.
func (m messages) Redirect(ctx context.Context, p *api.MessageRedirectParams) (*api.OutboxItem, error) {
	if len(p.To) == 0 {
		return nil, api.InvalidParams("a redirect needs at least one recipient")
	}
	to := make([]compose.Address, 0, len(p.To))
	resentTo := make([]string, 0, len(p.To))
	rcpts := make([]string, 0, len(p.To))
	for _, a := range p.To {
		ca := compose.Address{Name: strings.TrimSpace(a.Name), Addr: strings.TrimSpace(a.Address)}
		if !compose.ValidAddress(ca) {
			return nil, api.InvalidParams("%q is not a valid address", a.Address)
		}
		to = append(to, ca)
		resentTo = append(resentTo, (&mail.Address{Name: ca.Name, Address: ca.Addr}).String())
		rcpts = append(rcpts, ca.Addr)
	}
	detail, err := m.DB.GetMessage(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("message %d", p.ID))
	}
	ident, acct, err := m.ownIdentity(ctx, detail.AccountID)
	if err != nil {
		return nil, err
	}
	raw, err := m.raw(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	now := m.DB.Now()
	resentID := compose.NewMessageID(ident.Email)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Resent-From: %s\r\nResent-To: %s\r\nResent-Date: %s\r\nResent-Message-ID: <%s>\r\n",
		(&mail.Address{Name: ident.Name, Address: ident.Email}).String(), strings.Join(resentTo, ", "),
		now.Format(time.RFC1123Z), resentID)
	buf.Write(raw)
	item, err := m.outboxItem(ctx, acct.ID, &buf, resentID, ident.Email, rcpts, detail.Subject, to, now)
	if err != nil {
		return nil, err
	}
	err = m.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		if item, err = tx.QueueOutbox(ctx, item); err != nil {
			return err
		}
		return tx.Emit(ctx, api.OutboxChanged{ID: item.ID, AccountID: item.AccountID, State: api.OutboxStateQueued})
	})
	if err != nil {
		return nil, err
	}
	if m.Sync != nil {
		m.Sync.OutboxChanged(acct.ID)
	}
	r := toAPIOutbox(item)
	return &r, nil
}
