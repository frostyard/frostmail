package engine

import (
	"context"
	"errors"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/search"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/view"
)

type views struct{ Deps }

func (v views) Open(ctx context.Context, p *api.ViewOpenParams) (*api.ViewInfo, error) {
	if v.Views == nil {
		return nil, api.Unavailable("views are not running")
	}
	q := p.Query
	f := store.ViewFilter{Unread: q.Unread, Flagged: q.Flagged, HasAttachment: q.HasAttachments}
	if q.AccountID != nil {
		f.AccountID = *q.AccountID
	}
	if q.MailboxID != nil {
		f.MailboxID = *q.MailboxID
	}
	if q.Text != nil {
		applySearch(&f, search.Parse(*q.Text, time.Now(), time.Local))
	}
	if q.Role != nil {
		f.Role = string(*q.Role)
	}
	if q.Threads != nil {
		f.Threads = *q.Threads
	}
	if q.Conditions != nil {
		if err := store.CheckConditions(*q.Conditions); err != nil {
			return nil, api.InvalidParams("%v", err)
		}
		f.Conditions = q.Conditions
	}
	if q.SmartMailboxID != nil {
		return nil, notYet(3)
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

// applySearch adds a parsed search (docs/specs/search.md) to a view's
// filter. is:unread, is:flagged and has:attachment apply when the query has
// not set them.
func applySearch(f *store.ViewFilter, q search.Query) {
	f.Match, f.Exclude = q.Match(), q.Exclude()
	if f.Unread == nil {
		f.Unread = q.Unread
	}
	if f.Flagged == nil {
		f.Flagged = q.Flagged
	}
	if f.HasAttachment == nil {
		f.HasAttachment = q.HasAttachment
	}
	f.After, f.Before = q.After, q.Before
	for _, r := range q.Roles {
		f.Roles = append(f.Roles, string(r))
	}
}
