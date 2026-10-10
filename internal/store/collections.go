package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
)

// Collection is a collections row: an address book, calendar or Google
// Tasks list.
type Collection struct {
	ID          int64
	AccountID   int64
	Kind        api.CollectionKind
	Href        string // the collection's path, or the Google list's ID
	Name        string
	Description string
	Color       string   // #rrggbb, or ""
	Components  []string // calendars: VEVENT, VTODO; stored comma-separated
	ReadOnly    bool
	Enabled     bool
	IsDefault   bool
	Position    int
	SyncToken   string
	CTag        string
	SyncedAt    *time.Time
}

// RemoteCollection is a collection as the server lists it.
type RemoteCollection struct {
	Href        string
	Name        string
	Description string
	Color       string
	Components  []string
	ReadOnly    bool
}

// CollectionFilter selects collections; zero fields match every one.
type CollectionFilter struct {
	AccountID   int64
	Kind        api.CollectionKind
	EnabledOnly bool
}

const collectionColumns = `id, account_id, kind, href, name, description, color, components,
 read_only, enabled, is_default, position, sync_token, ctag, synced_at`

type collectionReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readCollection(row interface{ Scan(...any) error }) (Collection, error) {
	var c Collection
	var components string
	var synced sql.NullString
	err := row.Scan(&c.ID, &c.AccountID, &c.Kind, &c.Href, &c.Name, &c.Description,
		&c.Color, &components, &c.ReadOnly, &c.Enabled, &c.IsDefault, &c.Position,
		&c.SyncToken, &c.CTag, &synced)
	if err == sql.ErrNoRows {
		return c, ErrNotFound
	}
	if err != nil {
		return c, fmt.Errorf("read collection: %w", err)
	}
	if components != "" {
		c.Components = strings.Split(components, ",")
	}
	if synced.Valid {
		at, err := ParseTime(synced.String)
		if err != nil {
			return c, err
		}
		c.SyncedAt = &at
	}
	return c, nil
}

func collections(ctx context.Context, q collectionReader, f CollectionFilter) ([]Collection, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+collectionColumns+` FROM collections
 WHERE (? = 0 OR account_id = ?) AND (? = '' OR kind = ?) AND (? = 0 OR enabled = 1)
 ORDER BY account_id, position, id`, f.AccountID, f.AccountID, f.Kind, f.Kind, f.EnabledOnly)
	if err != nil {
		return nil, fmt.Errorf("collections: %w", err)
	}
	defer rows.Close()
	var out []Collection
	for rows.Next() {
		c, err := readCollection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Collections returns matching collections ordered by account, position and ID.
func (d *DB) Collections(ctx context.Context, f CollectionFilter) ([]Collection, error) {
	return collections(ctx, d.db, f)
}

// GetCollection returns one collection, or ErrNotFound.
func (d *DB) GetCollection(ctx context.Context, id int64) (Collection, error) {
	return readCollection(d.db.QueryRowContext(ctx, "SELECT "+collectionColumns+" FROM collections WHERE id = ?", id))
}

// ReplaceCollections reconciles server metadata and removals, preserving settings
// and sync state. It chooses a writable default and emits AccountChanged on change.
func (t *Tx) ReplaceCollections(ctx context.Context, accountID int64, kind api.CollectionKind, list []RemoteCollection) ([]Collection, error) {
	f := CollectionFilter{AccountID: accountID, Kind: kind}
	old, err := collections(ctx, t, f)
	if err != nil {
		return nil, err
	}
	kept := make(map[string]bool, len(list))
	changed := false
	for i, c := range list {
		kept[c.Href] = true
		n, err := t.replaceCollection(ctx, accountID, kind, c, i)
		if err != nil {
			return nil, err
		}
		changed = changed || n > 0
	}
	for _, c := range old {
		if kept[c.Href] {
			continue
		}
		if _, err := t.ExecContext(ctx, "DELETE FROM collections WHERE id = ?", c.ID); err != nil {
			return nil, fmt.Errorf("delete collection: %w", err)
		}
		changed = true
	}
	res, err := t.ExecContext(ctx, `UPDATE collections SET is_default = 1 WHERE id =
 (SELECT id FROM collections WHERE account_id = ? AND kind = ? AND read_only = 0 ORDER BY position, id LIMIT 1)
 AND NOT EXISTS (SELECT 1 FROM collections WHERE account_id = ? AND kind = ? AND is_default = 1)`,
		accountID, kind, accountID, kind)
	if err != nil {
		return nil, fmt.Errorf("default collection: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("default collection count: %w", err)
	}
	if changed || n > 0 {
		if err := t.Emit(ctx, api.AccountChanged{ID: accountID}); err != nil {
			return nil, err
		}
	}
	return collections(ctx, t, f)
}

func (t *Tx) replaceCollection(ctx context.Context, accountID int64, kind api.CollectionKind, c RemoteCollection, position int) (int64, error) {
	res, err := t.ExecContext(ctx, `INSERT INTO collections
 (account_id, kind, href, name, description, color, components, read_only, position)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
 ON CONFLICT(account_id, kind, href) DO UPDATE SET
 name = excluded.name, description = excluded.description, color = excluded.color,
 components = excluded.components, read_only = excluded.read_only, position = excluded.position
 WHERE name != excluded.name OR description != excluded.description OR color != excluded.color
 OR components != excluded.components OR read_only != excluded.read_only OR position != excluded.position`,
		accountID, kind, c.Href, c.Name, c.Description, c.Color, strings.Join(c.Components, ","), c.ReadOnly, position)
	if err != nil {
		return 0, fmt.Errorf("replace collection: %w", err)
	}
	return res.RowsAffected()
}

// SetCollectionSync records the completed pass's token, ctag and timestamp.
func (t *Tx) SetCollectionSync(ctx context.Context, id int64, token, ctag string) error {
	res, err := t.ExecContext(ctx, "UPDATE collections SET sync_token = ?, ctag = ?, synced_at = ? WHERE id = ?",
		token, ctag, FormatTime(t.Now()), id)
	if err != nil {
		return fmt.Errorf("set collection sync: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("set collection sync count: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateCollection changes enabled and default settings and emits
// AccountChanged; showing or hiding a collection also emits its domain's
// change event, since what the domain's queries return changed.
func (t *Tx) UpdateCollection(ctx context.Context, id int64, enabled *bool, makeDefault bool) (Collection, error) {
	c, err := readCollection(t.QueryRowContext(ctx, "SELECT "+collectionColumns+" FROM collections WHERE id = ?", id))
	if err != nil {
		return Collection{}, err
	}
	if enabled != nil && *enabled != c.Enabled {
		if _, err := t.ExecContext(ctx, "UPDATE collections SET enabled = ? WHERE id = ?", *enabled, id); err != nil {
			return Collection{}, fmt.Errorf("update collection: %w", err)
		}
		c.Enabled = *enabled
		if err := t.Emit(ctx, collectionChanged(c)); err != nil {
			return Collection{}, err
		}
	}
	if makeDefault {
		if _, err := t.ExecContext(ctx, "UPDATE collections SET is_default = (id = ?), default_chosen = (id = ?) WHERE account_id = ? AND kind = ?",
			id, id, c.AccountID, c.Kind); err != nil {
			return Collection{}, fmt.Errorf("update collection default: %w", err)
		}
		c.IsDefault = true
	}
	if err := t.Emit(ctx, api.AccountChanged{ID: c.AccountID}); err != nil {
		return Collection{}, err
	}
	return c, nil
}

// UseServerDefault makes the writable collection at href the default of
// its kind for the account, as the server names it, unless the user chose
// one (UpdateCollection). It reports whether the default changed, and
// emits AccountChanged when it did.
func (t *Tx) UseServerDefault(ctx context.Context, accountID int64, kind api.CollectionKind, href string) (bool, error) {
	res, err := t.ExecContext(ctx, `UPDATE collections SET is_default = (href = ?) WHERE account_id = ? AND kind = ?
 AND EXISTS (SELECT 1 FROM collections WHERE account_id = ? AND kind = ? AND href = ? AND read_only = 0 AND is_default = 0)
 AND NOT EXISTS (SELECT 1 FROM collections WHERE account_id = ? AND kind = ? AND default_chosen = 1)`,
		href, accountID, kind, accountID, kind, href, accountID, kind)
	if err != nil {
		return false, fmt.Errorf("use the server's default: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return false, err
	}
	return true, t.Emit(ctx, api.AccountChanged{ID: accountID})
}

// collectionChanged is the change event of a collection's domain.
func collectionChanged(c Collection) api.Event {
	switch c.Kind {
	case api.CollectionKindAddressbook:
		return api.PeopleChanged{AccountID: c.AccountID}
	case api.CollectionKindTasklist:
		return api.TasksChanged{AccountID: c.AccountID}
	}
	return api.CalendarChanged{AccountID: c.AccountID}
}
