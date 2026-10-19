package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

type threads struct{ Deps }

// Messages implements thread.messages: the thread's messages, oldest
// first, as summaries, across every mailbox.
func (t threads) Messages(ctx context.Context, p *api.ThreadMessagesParams) ([]api.MessageSummary, error) {
	ids, err := t.DB.ThreadMessageIDs(ctx, p.ID)
	if err != nil {
		return nil, apiError(err, fmt.Sprintf("thread %d", p.ID))
	}
	sums, err := t.DB.Summaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]api.MessageSummary, 0, len(sums))
	for _, s := range sums {
		out = append(out, toAPISummary(s))
	}
	if out = scoped(ctx, out); len(out) == 0 && len(sums) > 0 {
		return nil, api.NotFound("thread %d does not exist", p.ID)
	}
	return out, nil
}

// The waiting queries (docs/specs/lodge-app.md, Calendar, tasks and
// contacts; ADR-0042): maild, not an agent's model, picks the threads
// waiting on a reply, by fixed rules over what it stores.

// noReplyParts are the local parts of senders nobody answers.
var noReplyParts = []string{"noreply", "no-reply", "donotreply", "do-not-reply", "mailer-daemon", "postmaster"}

// maxAwaitingRecipients bounds the recipients an AwaitingThread carries.
const maxAwaitingRecipients = 10

const day = 24 * time.Hour

// awaitingRange is a waiting query's params, checked, with its defaults.
type awaitingRange struct{ limit, waiting, within int64 }

func checkAwaiting(limit, waiting, within *int64, defLimit, defWaiting int64) (awaitingRange, error) {
	r := awaitingRange{limit: defLimit, waiting: defWaiting, within: 30}
	if limit != nil {
		r.limit = *limit
	}
	if waiting != nil {
		r.waiting = *waiting
	}
	if within != nil {
		r.within = *within
	}
	switch {
	case r.limit < 1 || r.limit > 25:
		return r, api.InvalidParams("limit must be 1 to 25")
	case r.waiting < 0 || r.waiting > 30:
		return r, api.InvalidParams("waitingDays must be 0 to 30")
	case r.within < 1 || r.within > 90:
		return r, api.InvalidParams("withinDays must be 1 to 90")
	}
	return r, nil
}

// AwaitingReply implements thread.awaitingReply.
func (t threads) AwaitingReply(ctx context.Context, p *api.ThreadAwaitingReplyParams) (*api.Awaiting, error) {
	r, err := checkAwaiting(p.Limit, p.WaitingDays, p.WithinDays, 5, 0)
	if err != nil {
		return nil, err
	}
	return t.awaiting(ctx, r, true)
}

// AwaitingOthers implements thread.awaitingOthers.
func (t threads) AwaitingOthers(ctx context.Context, p *api.ThreadAwaitingOthersParams) (*api.Awaiting, error) {
	r, err := checkAwaiting(p.Limit, p.WaitingDays, p.WithinDays, 10, 5)
	if err != nil {
		return nil, err
	}
	return t.awaiting(ctx, r, false)
}

func lowerAddr(a string) string { return strings.ToLower(strings.TrimSpace(a)) }

// waitingAddresses are the address sets the waiting rules read.
type waitingAddresses struct {
	own     map[int64]map[string]bool // each account's identities and its own address
	anyOwn  map[string]bool           // every account's
	vips    map[string]bool
	blocked map[string]bool
}

func (t threads) waitingAddresses(ctx context.Context) (waitingAddresses, error) {
	w := waitingAddresses{own: map[int64]map[string]bool{}, anyOwn: map[string]bool{}, vips: map[string]bool{}, blocked: map[string]bool{}}
	add := func(acct int64, addr string) {
		if w.own[acct] == nil {
			w.own[acct] = map[string]bool{}
		}
		w.own[acct][lowerAddr(addr)] = true
		w.anyOwn[lowerAddr(addr)] = true
	}
	accts, err := t.DB.ListAccounts(ctx)
	if err != nil {
		return w, err
	}
	for _, a := range accts {
		add(a.ID, a.Email)
	}
	ids, err := t.DB.ListIdentities(ctx, 0)
	if err != nil {
		return w, err
	}
	for _, id := range ids {
		add(id.AccountID, id.Email)
	}
	vips, err := t.DB.VIPs(ctx)
	if err != nil {
		return w, err
	}
	for _, v := range vips {
		w.vips[v.Address] = true
	}
	blocked, err := t.DB.BlockedSenders(ctx)
	if err != nil {
		return w, err
	}
	for _, b := range blocked {
		w.blocked[b.Address] = true
	}
	return w, nil
}

func anyAddress(list []store.Address, in map[string]bool) bool {
	return slices.ContainsFunc(list, func(a store.Address) bool { return in[lowerAddr(a.Addr)] })
}

func noReplySender(addr string) bool {
	local, _, _ := strings.Cut(lowerAddr(addr), "@")
	return slices.Contains(noReplyParts, local)
}

// awaiting answers the threads whose newest message waits on the user
// (reply) or on others, in the request's scope.
func (t threads) awaiting(ctx context.Context, r awaitingRange, reply bool) (*api.Awaiting, error) {
	now := t.DB.Now()
	rows, err := t.DB.NewestMessages(ctx, store.NewestFilter{Accounts: scopeAccounts(ctx),
		After: now.Add(-time.Duration(r.within) * day), Until: now.Add(-time.Duration(r.waiting) * day)})
	if err != nil {
		return nil, err
	}
	w, err := t.waitingAddresses(ctx)
	if err != nil {
		return nil, err
	}
	type pick struct {
		n   store.NewestMessage
		vip bool
	}
	var picked []pick
	for _, n := range rows {
		own, from := w.own[n.AccountID], lowerAddr(n.From.Addr)
		recipients := slices.Concat(n.To, n.Cc)
		if reply {
			if own[from] || !anyAddress(n.To, own) || n.ListID != "" || n.ListUnsubscribe != "" || noReplySender(from) ||
				n.Answered || n.Muted || w.blocked[from] || !n.InInbox {
				continue
			}
			picked = append(picked, pick{n, w.vips[from]})
			continue
		}
		someoneElse := slices.ContainsFunc(recipients, func(a store.Address) bool { return !w.anyOwn[lowerAddr(a.Addr)] })
		if !own[from] || !someoneElse || n.Muted {
			continue
		}
		picked = append(picked, pick{n, anyAddress(recipients, w.vips)})
	}
	// The rows come newest first; a stable sort keeps that order among VIPs
	// and among the rest.
	slices.SortStableFunc(picked, func(a, b pick) int {
		switch {
		case a.vip == b.vip:
			return 0
		case a.vip:
			return -1
		}
		return 1
	})
	out := &api.Awaiting{Total: int64(len(picked)), Threads: []api.AwaitingThread{}}
	picked = picked[:min(int64(len(picked)), r.limit)]
	ids := make([]int64, 0, len(picked))
	for _, p := range picked {
		ids = append(ids, p.n.MessageID)
	}
	sums, err := t.DB.Summaries(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]store.Summary, len(sums))
	for _, s := range sums {
		byID[s.ID] = s
	}
	for _, p := range picked {
		s, ok := byID[p.n.MessageID]
		if !ok {
			continue
		}
		recipients := slices.Concat(p.n.To, p.n.Cc)
		a := api.AwaitingThread{ThreadID: p.n.ThreadID, AccountID: p.n.AccountID, Message: toAPISummary(s),
			Recipients: make([]api.Address, 0, min(len(recipients), maxAwaitingRecipients)), RecipientCount: int64(len(recipients)),
			Vip: p.vip, WaitingDays: int64(now.Sub(p.n.Date) / day)}
		for _, rc := range recipients[:min(len(recipients), maxAwaitingRecipients)] {
			a.Recipients = append(a.Recipients, api.Address{Name: rc.Name, Address: rc.Addr})
		}
		out.Threads = append(out.Threads, a)
	}
	return out, nil
}
