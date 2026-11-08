package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/store"
)

const (
	maxSnippetName = 60
	maxSnippetHTML = 65536
)

// Snippets implements the snippet domain.
func (e *Engine) Snippets() api.SnippetService { return snippets{e.d} }

type snippets struct{ Deps }

func (s snippets) List(ctx context.Context, _ *api.SnippetListParams) ([]api.Snippet, error) {
	rows, err := s.DB.Snippets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]api.Snippet, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAPISnippet(row))
	}
	return out, nil
}

func (s snippets) Create(ctx context.Context, p *api.SnippetCreateParams) (*api.Snippet, error) {
	name, err := snippetName(p.Name)
	if err != nil {
		return nil, err
	}
	if err := snippetHTML(p.HTML); err != nil {
		return nil, err
	}
	var out store.Snippet
	err = s.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.CreateSnippet(ctx, store.Snippet{Name: name, HTML: p.HTML})
		return err
	})
	if err != nil {
		return nil, snippetError(err, fmt.Sprintf("snippet %q", name), name)
	}
	a := toAPISnippet(out)
	return &a, nil
}

func (s snippets) Update(ctx context.Context, p *api.SnippetUpdateParams) (*api.Snippet, error) {
	var name string
	u := store.SnippetUpdate{Name: p.Name, HTML: p.HTML}
	if p.Name != nil {
		var err error
		name, err = snippetName(*p.Name)
		if err != nil {
			return nil, err
		}
		u.Name = &name
	}
	if p.HTML != nil {
		if err := snippetHTML(*p.HTML); err != nil {
			return nil, err
		}
	}
	var out store.Snippet
	err := s.DB.Tx(ctx, func(tx *store.Tx) error {
		var err error
		out, err = tx.UpdateSnippet(ctx, p.ID, u)
		return err
	})
	if err != nil {
		// Only a new name can be taken, so name is set on a conflict.
		return nil, snippetError(err, fmt.Sprintf("snippet %d", p.ID), name)
	}
	a := toAPISnippet(out)
	return &a, nil
}

func (s snippets) Delete(ctx context.Context, p *api.SnippetDeleteParams) error {
	err := s.DB.Tx(ctx, func(tx *store.Tx) error { return tx.DeleteSnippet(ctx, p.ID) })
	return apiError(err, fmt.Sprintf("snippet %d", p.ID))
}

// snippetError is a store error as the snippet domain reports it: a taken
// name names the name, and the limit is invalidParams.
func snippetError(err error, what, name string) error {
	switch {
	case errors.Is(err, store.ErrConflict):
		return api.Conflict("a snippet named %q already exists", name)
	case errors.Is(err, store.ErrTooManySnippets):
		return api.InvalidParams("at most %d snippets can be kept", store.MaxSnippets)
	}
	return apiError(err, what)
}

func snippetName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxSnippetName || strings.ContainsFunc(name, unicode.IsControl) {
		return "", api.InvalidParams("snippet name must be 1 to %d characters without control characters", maxSnippetName)
	}
	return name, nil
}

func snippetHTML(html string) error {
	if len(html) > maxSnippetHTML {
		return api.InvalidParams("snippet text must be at most %d bytes", maxSnippetHTML)
	}
	return nil
}

func toAPISnippet(s store.Snippet) api.Snippet {
	return api.Snippet{ID: s.ID, Name: s.Name, HTML: s.HTML, UpdatedAt: s.UpdatedAt}
}
