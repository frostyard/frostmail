package engine

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

type threads struct{ Deps }

// Messages implements thread.messages: the thread's message IDs from the
// store, as summaries. Task T-0025 implements it; the stub finds none.
func (t threads) Messages(_ context.Context, _ *api.ThreadMessagesParams) ([]api.MessageSummary, error) {
	return []api.MessageSummary{}, nil
}
