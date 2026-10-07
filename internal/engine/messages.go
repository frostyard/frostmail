package engine

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/mimex"
	"github.com/frostyard/frostmail/internal/store"
)

type messages struct{ Deps }

func (m messages) Get(ctx context.Context, p *api.MessageGetParams) (*api.Message, error) {
	d, err := m.DB.GetMessage(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("message %d", p.ID))
	}
	out := api.Message{
		Summary: toAPISummary(d.Summary), To: toAPIAddresses(d.To), Cc: toAPIAddresses(d.Cc),
		ReplyTo: toAPIAddresses(d.ReplyTo), MessageID: d.MessageID, InReplyTo: d.InReplyTo,
		References: d.References, ListID: d.ListID, ListUnsubscribe: d.ListUnsubscribe,
		BodyFetched: d.BlobID != "",
	}
	for _, pt := range d.Parts {
		out.Parts = append(out.Parts, api.Part{
			Path: pt.Path, ContentType: pt.ContentType, Filename: pt.Filename,
			Disposition: pt.Disposition, ContentID: pt.ContentID, Size: pt.Size,
		})
	}
	return &out, nil
}

func (m messages) Body(ctx context.Context, p *api.MessageBodyParams) (*api.Body, error) {
	if m.Sync == nil || m.Blobs == nil {
		return nil, api.Unavailable("sync is not running")
	}
	blobID, err := m.Sync.FetchBody(ctx, p.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, api.NotFound("message %d does not exist", p.ID)
	}
	if err != nil {
		return nil, api.Unavailable("message %d is not stored locally and cannot be fetched now: %v", p.ID, err)
	}
	rc, err := m.Blobs.Open(blobID)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	raw, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	text, hasHTML, err := mimex.BodyText(raw)
	if err != nil {
		return nil, err
	}
	return &api.Body{Text: text, HasHTML: hasHTML}, nil
}

func (m messages) SetFlags(ctx context.Context, p *api.MessageSetFlagsParams) error {
	if m.Sync == nil {
		return api.Unavailable("sync is not running")
	}
	c := p.Changes
	if c.FlagColor != nil && (*c.FlagColor < 0 || *c.FlagColor > 7) {
		return api.InvalidParams("flagColor %d is not 0-7", *c.FlagColor)
	}
	change := store.FlagChange{Seen: c.Seen, Flagged: c.Flagged, Answered: c.Answered}
	if c.FlagColor != nil {
		color := int(*c.FlagColor)
		change.Color = &color
	}
	return opError(m.Sync.SetFlags(ctx, p.IDs, change))
}

func (m messages) Move(ctx context.Context, p *api.MessageMoveParams) error {
	if m.Sync == nil {
		return api.Unavailable("sync is not running")
	}
	return opError(m.Sync.Move(ctx, p.IDs, p.MailboxID))
}

func (m messages) Delete(ctx context.Context, p *api.MessageDeleteParams) error {
	if m.Sync == nil {
		return api.Unavailable("sync is not running")
	}
	return opError(m.Sync.Delete(ctx, p.IDs))
}

func opError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound):
		return api.NotFound("%v", err)
	case errors.Is(err, mailsync.ErrInvalid):
		return api.InvalidParams("%v", err)
	}
	return err
}

func toAPISummary(s store.Summary) api.MessageSummary {
	return api.MessageSummary{
		ID: s.ID, AccountID: s.AccountID, MailboxIDs: s.MailboxIDs, ThreadID: s.ThreadID,
		Subject: s.Subject, From: api.Address{Name: s.From.Name, Address: s.From.Addr}, Date: s.Date,
		Preview: s.Preview, HasAttachments: s.HasAttachments, Size: s.Size,
		Flags: api.Flags{
			Seen: s.Flags.Seen, Flagged: s.Flags.Flagged, Answered: s.Flags.Answered,
			Forwarded: s.Flags.Forwarded, Draft: s.Flags.Draft, FlagColor: int64(s.Flags.Color),
		},
	}
}

func toAPIAddresses(as []store.Address) []api.Address {
	out := make([]api.Address, 0, len(as))
	for _, a := range as {
		out = append(out, api.Address{Name: a.Name, Address: a.Addr})
	}
	return out
}
