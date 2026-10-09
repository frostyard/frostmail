package engine

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

// notBuilt answers a method M4.5 has not built yet (docs/plans/0007).
func notBuilt(method string) error {
	return api.Unavailable("%s is not built yet", method)
}

func (a accounts) Services(context.Context, *api.AccountServicesParams) ([]api.ServiceSettings, error) {
	return nil, notBuilt("account.services")
}

func (a accounts) SetService(context.Context, *api.AccountSetServiceParams) (*api.ServiceSettings, error) {
	return nil, notBuilt("account.setService")
}

func (a accounts) Collections(context.Context, *api.AccountCollectionsParams) ([]api.Collection, error) {
	return []api.Collection{}, nil
}

func (a accounts) SetCollection(context.Context, *api.AccountSetCollectionParams) (*api.Collection, error) {
	return nil, notBuilt("account.setCollection")
}

func (s syncService) Pim(context.Context, *api.SyncPimParams) error {
	return nil
}
