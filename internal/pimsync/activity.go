package pimsync

import (
	"errors"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/activity"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/graph"
	"github.com/frostyard/frostmail/internal/gtasks"
	"github.com/frostyard/frostmail/internal/providers"
)

// server is the server or service that s of this pass's account syncs
// with, as activity names it.
func (p *pass) server(s api.ServiceKind) api.ActivityServer {
	q := providers.ForKind(p.acct.Kind).DAV
	switch {
	case q.Graph:
		return api.ActivityServerGraph
	case s == api.ServiceKindContacts:
		return api.ActivityServerCarddav
	case s == api.ServiceKindTasks && q.Tasks == "google":
		return api.ActivityServerGtasks
	default:
		return api.ActivityServerCaldav
	}
}

// record keeps the activity entry of one service's pass (ADR-0055): the
// collections it changed, or why it failed.
func (p *pass) record(s api.ServiceKind, changed int, d time.Duration, err error) {
	h, srv := p.m.cfg.Activity, p.server(s)
	switch {
	case err == nil:
		h.Record(activity.PIMSynced(p.acct.ID, srv, changed, d))
	case errors.Is(err, davx.ErrUnauthorized), errors.Is(err, gtasks.ErrUnauthorized), errors.Is(err, graph.ErrUnauthorized):
		h.Record(activity.SignInNeeded(p.acct.ID, srv, err))
	default:
		h.Record(activity.Lost(p.acct.ID, srv, err))
	}
}

// refusal records that the server refused a change to a collection of kind.
func (p *pass) refusal(kind api.CollectionKind, err error) {
	s := api.ServiceKindCalendar
	switch kind {
	case api.CollectionKindAddressbook:
		s = api.ServiceKindContacts
	case api.CollectionKindTasklist:
		s = api.ServiceKindTasks
	}
	p.m.cfg.Activity.Record(activity.PIMRefused(p.acct.ID, p.server(s), err))
}
