package engine

import (
	"context"
	"errors"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/render"
)

// Render implements message.render (docs/design/rendering.md).
func (m messages) Render(ctx context.Context, p *api.MessageRenderParams) (*api.Rendering, error) {
	if m.Deps.Render == nil {
		return nil, api.Unavailable("rendering is not running")
	}
	raw, err := m.raw(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	r, err := m.Deps.Render.Render(ctx, p.ID, raw, p.Remote != nil && *p.Remote)
	if err != nil {
		return nil, err
	}
	return &api.Rendering{HTML: r.HTML, Text: r.Text, Remote: int64(r.Remote), Trackers: int64(r.Trackers)}, nil
}

// Part implements message.part.
func (m messages) Part(ctx context.Context, p *api.MessagePartParams) (*api.PartFile, error) {
	if m.Deps.Render == nil {
		return nil, api.Unavailable("rendering is not running")
	}
	raw, err := m.raw(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	f, err := m.Deps.Render.Part(p.ID, raw, p.Path)
	if errors.Is(err, render.ErrNoPart) {
		return nil, api.NotFound("message %d has no part %q", p.ID, p.Path)
	}
	if err != nil {
		return nil, err
	}
	return &api.PartFile{Path: f.Path, ContentType: f.ContentType, Filename: f.Filename, Size: f.Size}, nil
}
