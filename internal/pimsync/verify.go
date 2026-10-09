package pimsync

import (
	"context"
	"slices"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/store"
)

// maxListed caps the hrefs a check lists.
const maxListed = 20

// Verify compares each enabled collection of an account's DAV services
// with the server, changing nothing (docs/design/pim.md, Operational
// notes): the server's hrefs and ETags against the stored objects, leaving
// out objects with changes waiting to be written. Services that have not
// found their home set yet are skipped.
func (m *Manager) Verify(ctx context.Context, accountID int64) ([]api.CollectionCheck, error) {
	acct, err := m.db.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	services, err := m.db.Services(ctx, accountID)
	if err != nil {
		return nil, err
	}
	p := &pass{m: m, acct: acct}
	out := []api.CollectionCheck{}
	for _, s := range services {
		ck := api.CollectionKindAddressbook
		switch {
		case !s.Enabled || s.Home == "" || s.Service == api.ServiceKindTasks:
			continue
		case s.Service == api.ServiceKindCalendar:
			ck = api.CollectionKindCalendar
		}
		c, err := p.client(s.Home)
		if err != nil {
			return nil, err
		}
		cols, err := m.db.Collections(ctx, store.CollectionFilter{AccountID: accountID, Kind: ck, EnabledOnly: true})
		if err != nil {
			return nil, err
		}
		for _, col := range cols {
			check, err := m.verifyCollection(ctx, c, col)
			if err != nil {
				return nil, err
			}
			out = append(out, check)
		}
	}
	return out, nil
}

func (m *Manager) verifyCollection(ctx context.Context, c *davx.Client, col store.Collection) (api.CollectionCheck, error) {
	server, err := c.List(ctx, col.Href)
	if err != nil {
		return api.CollectionCheck{}, err
	}
	local, err := m.db.ObjectETags(ctx, col.ID)
	if err != nil {
		return api.CollectionCheck{}, err
	}
	pending, err := m.db.PendingHrefs(ctx, col.ID)
	if err != nil {
		return api.CollectionCheck{}, err
	}
	check := api.CollectionCheck{CollectionID: col.ID, Name: col.Name, Server: int64(len(server)), Local: int64(len(local)),
		MissingLocally: []string{}, MissingOnServer: []string{}}
	onServer := map[string]bool{}
	for _, o := range server {
		onServer[o.Href] = true
		etag, ok := local[o.Href]
		switch {
		case !ok:
			if len(check.MissingLocally) < maxListed {
				check.MissingLocally = append(check.MissingLocally, o.Href)
			}
		case !pending[o.Href] && o.ETag != "" && etag != o.ETag:
			check.EtagDiffs++
		}
	}
	var gone []string
	for href := range local {
		if !onServer[href] && !pending[href] {
			gone = append(gone, href)
		}
	}
	slices.Sort(gone)
	check.MissingOnServer = append(check.MissingOnServer, gone[:min(len(gone), maxListed)]...)
	return check, nil
}

// Clean reports whether a check found no difference.
func Clean(c api.CollectionCheck) bool {
	return len(c.MissingLocally) == 0 && len(c.MissingOnServer) == 0 && c.EtagDiffs == 0
}
