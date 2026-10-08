package engine

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

// The draft, outbox, address and identity domains (docs/design/send.md)
// arrive in M3 phase 3; until then they report unavailable.

type drafts struct{ Deps }

var errNotYet = api.Unavailable("drafts and sending are not implemented yet")

func (drafts) Create(context.Context, *api.DraftCreateParams) (*api.Draft, error) {
	return nil, errNotYet
}
func (drafts) Open(context.Context, *api.DraftOpenParams) (*api.Draft, error)  { return nil, errNotYet }
func (drafts) Get(context.Context, *api.DraftGetParams) (*api.Draft, error)    { return nil, errNotYet }
func (drafts) List(context.Context, *api.DraftListParams) ([]api.Draft, error) { return nil, errNotYet }
func (drafts) Update(context.Context, *api.DraftUpdateParams) (*api.Draft, error) {
	return nil, errNotYet
}
func (drafts) Attach(context.Context, *api.DraftAttachParams) (*api.DraftAttachment, error) {
	return nil, errNotYet
}
func (drafts) Detach(context.Context, *api.DraftDetachParams) error { return errNotYet }
func (drafts) Delete(context.Context, *api.DraftDeleteParams) error { return errNotYet }
func (drafts) Send(context.Context, *api.DraftSendParams) (*api.OutboxItem, error) {
	return nil, errNotYet
}

type outbox struct{ Deps }

func (outbox) List(context.Context, *api.OutboxListParams) ([]api.OutboxItem, error) {
	return nil, errNotYet
}
func (outbox) Cancel(context.Context, *api.OutboxCancelParams) (*api.Draft, error) {
	return nil, errNotYet
}
func (outbox) Retry(context.Context, *api.OutboxRetryParams) error { return errNotYet }

type addresses struct{ Deps }

func (addresses) Suggest(context.Context, *api.AddressSuggestParams) ([]api.Address, error) {
	return nil, errNotYet
}

type identities struct{ Deps }

func (identities) List(context.Context, *api.IdentityListParams) ([]api.Identity, error) {
	return nil, errNotYet
}
func (identities) Update(context.Context, *api.IdentityUpdateParams) (*api.Identity, error) {
	return nil, errNotYet
}
