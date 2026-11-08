package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
)

// MaxSnippets is how many snippets the store keeps (docs/design/send.md,
// Snippets).
const MaxSnippets = 200

// ErrTooManySnippets is CreateSnippet's error when MaxSnippets are kept.
var ErrTooManySnippets = errors.New("store: too many snippets")

// Snippet is a named piece of HTML kept in the local maild store.
type Snippet struct {
	ID        int64
	Name      string
	HTML      string
	UpdatedAt time.Time
}

const snippetColumns = `SELECT id, name, html, updated_at FROM snippets`

func scanSnippet(row rowScanner) (Snippet, error) {
	var s Snippet
	var updated string
	if err := row.Scan(&s.ID, &s.Name, &s.HTML, &updated); err != nil {
		return Snippet{}, err
	}
	at, err := ParseTime(updated)
	if err != nil {
		return Snippet{}, err
	}
	s.UpdatedAt = at
	return s, nil
}

// Snippets returns every snippet by name without case, then by id.
// The sort is in Go so it also applies to non-ASCII names.
func (d *DB) Snippets(ctx context.Context) ([]Snippet, error) {
	rows, err := d.db.QueryContext(ctx, snippetColumns)
	if err != nil {
		return nil, fmt.Errorf("snippets: %w", err)
	}
	defer rows.Close()
	out := []Snippet{}
	for rows.Next() {
		s, err := scanSnippet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b Snippet) int {
		if n := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); n != 0 {
			return n
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return out, nil
}

// Snippet returns one snippet, or ErrNotFound.
func (d *DB) Snippet(ctx context.Context, id int64) (Snippet, error) {
	s, err := scanSnippet(d.db.QueryRowContext(ctx, snippetColumns+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Snippet{}, ErrNotFound
	}
	return s, err
}

// CreateSnippet adds s and emits api.SnippetChanged; ErrConflict when its
// name is taken without case, ErrTooManySnippets at MaxSnippets.
func (t *Tx) CreateSnippet(ctx context.Context, s Snippet) (Snippet, error) {
	var count int
	if err := t.QueryRowContext(ctx, `SELECT COUNT(*) FROM snippets`).Scan(&count); err != nil {
		return Snippet{}, fmt.Errorf("count snippets: %w", err)
	}
	if count >= MaxSnippets {
		return Snippet{}, ErrTooManySnippets
	}
	if err := t.checkSnippetName(ctx, s.Name, 0); err != nil {
		return Snippet{}, err
	}
	var id int64
	updated := FormatTime(t.Now())
	err := t.QueryRowContext(ctx, `INSERT INTO snippets (name, html, updated_at) VALUES (?, ?, ?) RETURNING id`, s.Name, s.HTML, updated).Scan(&id)
	if err != nil {
		if IsUniqueViolation(err) {
			return Snippet{}, ErrConflict
		}
		return Snippet{}, fmt.Errorf("create snippet: %w", err)
	}
	if err := t.Emit(ctx, api.SnippetChanged{ID: id}); err != nil {
		return Snippet{}, err
	}
	return t.Snippet(ctx, id)
}

// UpdateSnippet stores s and emits api.SnippetChanged; ErrNotFound for an
// unknown id, ErrConflict when another snippet has its name without case.
func (t *Tx) UpdateSnippet(ctx context.Context, s Snippet) (Snippet, error) {
	var exists int
	if err := t.QueryRowContext(ctx, `SELECT COUNT(*) FROM snippets WHERE id = ?`, s.ID).Scan(&exists); err != nil {
		return Snippet{}, fmt.Errorf("find snippet: %w", err)
	}
	if exists == 0 {
		return Snippet{}, ErrNotFound
	}
	if err := t.checkSnippetName(ctx, s.Name, s.ID); err != nil {
		return Snippet{}, err
	}
	updated := FormatTime(t.Now())
	_, err := t.ExecContext(ctx, `UPDATE snippets SET name = ?, html = ?, updated_at = ? WHERE id = ?`, s.Name, s.HTML, updated, s.ID)
	if err != nil {
		if IsUniqueViolation(err) {
			return Snippet{}, ErrConflict
		}
		return Snippet{}, fmt.Errorf("update snippet: %w", err)
	}
	if err := t.Emit(ctx, api.SnippetChanged{ID: s.ID}); err != nil {
		return Snippet{}, err
	}
	return t.Snippet(ctx, s.ID)
}

// DeleteSnippet removes a snippet and emits api.SnippetChanged.
func (t *Tx) DeleteSnippet(ctx context.Context, id int64) error {
	var deleted int64
	err := t.QueryRowContext(ctx, `DELETE FROM snippets WHERE id = ? RETURNING id`, id).Scan(&deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete snippet: %w", err)
	}
	if err := t.Emit(ctx, api.SnippetChanged{ID: id, Deleted: true}); err != nil {
		return err
	}
	return nil
}

func (t *Tx) checkSnippetName(ctx context.Context, name string, ownID int64) error {
	rows, err := t.QueryContext(ctx, `SELECT id, name FROM snippets`)
	if err != nil {
		return fmt.Errorf("check snippet name: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var other string
		if err := rows.Scan(&id, &other); err != nil {
			return err
		}
		if id != ownID && strings.EqualFold(name, other) {
			return ErrConflict
		}
	}
	return rows.Err()
}

// Snippet returns one snippet from a transaction.
func (t *Tx) Snippet(ctx context.Context, id int64) (Snippet, error) {
	s, err := scanSnippet(t.QueryRowContext(ctx, snippetColumns+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Snippet{}, ErrNotFound
	}
	return s, err
}
