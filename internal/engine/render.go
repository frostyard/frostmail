package engine

import (
	"context"

	"github.com/frostyard/frostmail/api"
)

// Render implements message.render (docs/design/rendering.md). It arrives in
// M2 phase 3 with the sanitizer.
func (m messages) Render(_ context.Context, _ *api.MessageRenderParams) (*api.Rendering, error) {
	return nil, api.Unavailable("message.render is not implemented yet")
}

// Part implements message.part. It arrives in M2 phase 3 with the parts cache.
func (m messages) Part(_ context.Context, _ *api.MessagePartParams) (*api.PartFile, error) {
	return nil, api.Unavailable("message.part is not implemented yet")
}
