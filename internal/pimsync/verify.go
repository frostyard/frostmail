package pimsync

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"slices"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/gtasks"
	"github.com/frostyard/frostmail/internal/providers"
	"github.com/frostyard/frostmail/internal/store"
)

// maxListed caps the hrefs a check lists.
const maxListed = 20

// Verify compares each enabled collection of an account's services with
// the server, changing nothing (docs/design/pim.md, Operational notes).
// Objects with changes waiting to be written are left out of ETag and
// missing-on-server checks. DAV services without a home set are skipped;
// Google Tasks checks the task lists already discovered by a pass.
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
		google := s.Service == api.ServiceKindTasks && providers.ForKind(acct.Kind).DAV.Tasks == "google"
		if !s.Enabled || !google && s.Home == "" {
			continue
		}
		checks, err := p.verifyService(ctx, s, google)
		if err != nil {
			return nil, err
		}
		out = append(out, checks...)
	}
	return out, nil
}

func (p *pass) verifyService(ctx context.Context, s store.Service, google bool) ([]api.CollectionCheck, error) {
	ck := api.CollectionKindAddressbook
	switch s.Service {
	case api.ServiceKindCalendar:
		ck = api.CollectionKindCalendar
	case api.ServiceKindTasks:
		ck = api.CollectionKindTasklist
	}
	cols, err := p.m.db.Collections(ctx, store.CollectionFilter{AccountID: p.acct.ID, Kind: ck, EnabledOnly: true})
	if err != nil {
		return nil, err
	}
	var verify func(context.Context, store.Collection) (api.CollectionCheck, error)
	if google {
		c := p.googleTasksClient(s)
		verify = func(ctx context.Context, col store.Collection) (api.CollectionCheck, error) {
			return p.m.verifyGoogleTaskList(ctx, c, col)
		}
	} else {
		c, err := p.client(s.Home)
		if err != nil {
			return nil, err
		}
		verify = func(ctx context.Context, col store.Collection) (api.CollectionCheck, error) {
			return p.m.verifyCollection(ctx, c, col)
		}
	}
	var checks []api.CollectionCheck
	for _, col := range cols {
		check, err := verify(ctx, col)
		if err != nil {
			return nil, err
		}
		checks = append(checks, check)
	}
	return checks, nil
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
	server, err = m.confirmDAVObjects(ctx, c, col, server, local)
	if err != nil {
		return api.CollectionCheck{}, err
	}
	return m.compareCollection(ctx, col, server, local)
}

// confirmDAVObjects removes listed hrefs absent both locally and remotely.
func (m *Manager) confirmDAVObjects(ctx context.Context, c *davx.Client, col store.Collection, server []davx.Change, local map[string]string) ([]davx.Change, error) {
	var hrefs []string
	for _, o := range server {
		if _, ok := local[o.Href]; !ok {
			hrefs = append(hrefs, o.Href)
		}
	}
	kind := davx.Calendars
	if col.Kind == api.CollectionKindAddressbook {
		kind = davx.AddressBooks
	}
	phantoms := map[string]bool{}
	for start := 0; start < len(hrefs); start += m.cfg.Batch {
		batch := hrefs[start:min(start+m.cfg.Batch, len(hrefs))]
		_, missing, err := c.Multiget(ctx, kind, col.Href, batch)
		if err != nil {
			return nil, fmt.Errorf("confirm dav objects: %w", err)
		}
		for _, href := range missing {
			phantoms[href] = true
		}
	}
	return slices.DeleteFunc(server, func(o davx.Change) bool { return phantoms[o.Href] }), nil
}

func (m *Manager) verifyGoogleTaskList(ctx context.Context, c *gtasks.Client, col store.Collection) (api.CollectionCheck, error) {
	tasks, err := c.Tasks(ctx, col.Href, time.Time{})
	if err != nil {
		return api.CollectionCheck{}, fmt.Errorf("fetch google tasks: %w", err)
	}
	var server []davx.Change
	for _, task := range tasks {
		if task.Deleted {
			continue
		}
		var meta struct {
			ETag string `json:"etag"`
		}
		if err := json.Unmarshal(task.Raw, &meta); err != nil {
			return api.CollectionCheck{}, fmt.Errorf("read google task etag: %w", err)
		}
		server = append(server, davx.Change{Href: task.ID, ETag: meta.ETag})
	}
	local, err := m.db.ObjectETags(ctx, col.ID)
	if err != nil {
		return api.CollectionCheck{}, err
	}
	return m.compareCollection(ctx, col, server, local)
}

func (m *Manager) compareCollection(ctx context.Context, col store.Collection, server []davx.Change, local map[string]string) (api.CollectionCheck, error) {
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
		case !pending[o.Href] && o.ETag != "" && etag != "" && etag != o.ETag:
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
