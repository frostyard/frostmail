package engine

import (
	"context"
	"errors"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/view"
)

type views struct{ Deps }

func (v views) Open(ctx context.Context, p *api.ViewOpenParams) (*api.ViewInfo, error) {
	if v.Views == nil {
		return nil, api.Unavailable("views are not running")
	}
	q := p.Query
	f := store.ViewFilter{Unread: q.Unread, Flagged: q.Flagged}
	if q.AccountID != nil {
		f.AccountID = *q.AccountID
	}
	if q.MailboxID != nil {
		f.MailboxID = *q.MailboxID
	}
	if q.Text != nil {
		f.Text = *q.Text
	}
	id, count, err := v.Views.Open(ctx, api.ConnFrom(ctx), f)
	if err != nil {
		return nil, err
	}
	return &api.ViewInfo{ID: id, Count: int64(count)}, nil
}

func (v views) Range(ctx context.Context, p *api.ViewRangeParams) ([]api.MessageSummary, error) {
	if v.Views == nil {
		return nil, api.Unavailable("views are not running")
	}
	if p.Start < 0 || p.End < p.Start {
		return nil, api.InvalidParams("range [%d, %d) is invalid", p.Start, p.End)
	}
	ids, err := v.Views.Range(api.ConnFrom(ctx), p.ID, int(p.Start), int(p.End))
	if errors.Is(err, view.ErrNotFound) {
		return nil, api.NotFound("view %d is not open on this connection", p.ID)
	}
	if err != nil {
		return nil, err
	}
	sums, err := v.DB.Summaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]api.MessageSummary, 0, len(sums))
	for _, s := range sums {
		out = append(out, toAPISummary(s))
	}
	return out, nil
}

func (v views) Close(ctx context.Context, p *api.ViewCloseParams) error {
	if v.Views == nil {
		return api.Unavailable("views are not running")
	}
	if err := v.Views.Close(api.ConnFrom(ctx), p.ID); err != nil {
		return api.NotFound("view %d is not open on this connection", p.ID)
	}
	return nil
}

type syncService struct{ Deps }

func (s syncService) Status(_ context.Context, p *api.SyncStatusParams) ([]api.SyncStatus, error) {
	if s.Sync == nil {
		return []api.SyncStatus{}, nil
	}
	var id int64
	if p.AccountID != nil {
		id = *p.AccountID
	}
	return s.Sync.Status(id), nil
}

func (s syncService) Now(ctx context.Context, p *api.SyncNowParams) error {
	if _, err := s.DB.GetAccount(ctx, p.AccountID); err != nil {
		return apiError(err, "account")
	}
	if s.Sync == nil {
		return api.Unavailable("sync is not running")
	}
	return s.Sync.SyncNow(p.AccountID)
}
