package engine

import (
	"context"
	"slices"

	"github.com/frostyard/frostmail/api"
)

// An agent's scope (docs/specs/lodge-app.md, /mcp) is enforced here, on the
// connection its MCP tools use, never by the tools alone: the methods an
// agent may call (schema/rpc's callers) answer for its accounts only, and
// another account's messages, threads and mailboxes do not exist for it.

// agentScope is the scope of a request from an agent, and false for any
// other caller, whose reach the engine does not narrow.
func agentScope(ctx context.Context) (api.Scope, bool) {
	c := api.CallerFrom(ctx)
	return c.Scope, c.Kind == api.CallerKindAgent
}

// admits reports whether the request may reach account id.
func admits(ctx context.Context, id int64) bool {
	s, agent := agentScope(ctx)
	return !agent || s.Admits(id)
}

// scopeAccounts is the store's account restriction for the request: nil for
// every account.
func scopeAccounts(ctx context.Context) []int64 {
	s, agent := agentScope(ctx)
	if !agent || s.All {
		return nil
	}
	return append([]int64{}, s.Accounts...)
}

// admitMessage answers notFound for a message an agent's scope does not
// reach, as for one that does not exist.
func (m messages) admitMessage(ctx context.Context, id int64) error {
	if _, agent := agentScope(ctx); !agent {
		return nil
	}
	sums, err := m.DB.Summaries(ctx, []int64{id})
	if err != nil {
		return err
	}
	if len(sums) == 0 || !admits(ctx, sums[0].AccountID) {
		return api.NotFound("message %d does not exist", id)
	}
	return nil
}

// admitMessages answers notFound unless an agent's scope reaches every one
// of ids, as for a message that does not exist.
func (m messages) admitMessages(ctx context.Context, ids []int64) error {
	if _, agent := agentScope(ctx); !agent {
		return nil
	}
	sums, err := m.DB.Summaries(ctx, ids)
	if err != nil {
		return err
	}
	account := make(map[int64]int64, len(sums))
	for _, s := range sums {
		account[s.ID] = s.AccountID
	}
	for _, id := range ids {
		if a, ok := account[id]; !ok || !admits(ctx, a) {
			return api.NotFound("message %d does not exist", id)
		}
	}
	return nil
}

// admitMailbox answers notFound for a mailbox an agent's scope does not
// reach.
func (d Deps) admitMailbox(ctx context.Context, id int64) error {
	if _, agent := agentScope(ctx); !agent {
		return nil
	}
	mb, err := d.DB.GetMailbox(ctx, id)
	if err != nil || !admits(ctx, mb.AccountID) {
		return api.NotFound("mailbox %d does not exist", id)
	}
	return nil
}

// admitDraft answers notFound for a draft an agent's scope does not reach.
func admitDraft(ctx context.Context, id, accountID int64) error {
	if !admits(ctx, accountID) {
		return api.NotFound("draft %d does not exist", id)
	}
	return nil
}

// scoped keeps the summaries an agent's scope reaches.
func scoped(ctx context.Context, sums []api.MessageSummary) []api.MessageSummary {
	if _, agent := agentScope(ctx); !agent {
		return sums
	}
	return slices.DeleteFunc(sums, func(s api.MessageSummary) bool { return !admits(ctx, s.AccountID) })
}
