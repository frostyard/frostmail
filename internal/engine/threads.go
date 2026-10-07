package engine

import (
	"context"
	"fmt"

	"github.com/frostyard/frostmail/api"
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
	return out, nil
}
