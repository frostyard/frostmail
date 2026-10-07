package engine

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

// pending implements the domains that arrive in M1 phase 3
// (docs/plans/0003-m1-headless-read-path.md); every method is unavailable.
type pending struct{}

func notYet() error { return api.Unavailable("not implemented yet (M1 phase 3)") }

func (pending) Get(context.Context, *api.MessageGetParams) (*api.Message, error) {
	return nil, notYet()
}
func (pending) Body(context.Context, *api.MessageBodyParams) (*api.Body, error) { return nil, notYet() }
func (pending) SetFlags(context.Context, *api.MessageSetFlagsParams) error      { return notYet() }
func (pending) Move(context.Context, *api.MessageMoveParams) error              { return notYet() }
func (pending) Delete(context.Context, *api.MessageDeleteParams) error          { return notYet() }
func (pending) Status(context.Context, *api.SyncStatusParams) ([]api.SyncStatus, error) {
	return nil, notYet()
}
func (pending) Now(context.Context, *api.SyncNowParams) error { return notYet() }
func (pending) Open(context.Context, *api.ViewOpenParams) (*api.ViewInfo, error) {
	return nil, notYet()
}
func (pending) Range(context.Context, *api.ViewRangeParams) ([]api.MessageSummary, error) {
	return nil, notYet()
}
func (pending) Close(context.Context, *api.ViewCloseParams) error { return notYet() }
