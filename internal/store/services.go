package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/frostyard/frostmail/api"
)

// Service is an account_services row: one of an account's services besides
// mail (ADR-0017).
type Service struct {
	AccountID  int64
	Service    api.ServiceKind
	Enabled    bool
	URL        string     // where discovery starts, or the Tasks API's base URL
	Home       string     // the DAV home set found from URL; "" until discovered
	LastSyncAt *time.Time // the end of the last complete pass
	LastError  string
}

// SetService stores whether an account's service is on and where it
// lives; a new URL forgets the home set and the last pass. It emits
// api.AccountChanged.
func (t *Tx) SetService(ctx context.Context, accountID int64, service api.ServiceKind, enabled bool, url string) error {
	_, err := t.ExecContext(ctx, `INSERT INTO account_services (account_id, service, enabled, url)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (account_id, service) DO UPDATE SET enabled = excluded.enabled, url = excluded.url,
			home = CASE WHEN url = excluded.url THEN home ELSE '' END,
			last_sync_at = CASE WHEN url = excluded.url THEN last_sync_at END,
			last_error = CASE WHEN url = excluded.url THEN last_error ELSE '' END`,
		accountID, service, enabled, url)
	if err != nil {
		if IsForeignKeyViolation(err) {
			return ErrNotFound
		}
		return fmt.Errorf("set service: %w", err)
	}
	return t.Emit(ctx, api.AccountChanged{ID: accountID})
}

// ServiceSynced records the end of a pass of an account's service: the
// time of a complete pass, or the error that ended it. It emits
// api.AccountChanged only when the error changes.
func (t *Tx) ServiceSynced(ctx context.Context, accountID int64, service api.ServiceKind, at time.Time, errText string) error {
	var old string
	err := t.QueryRowContext(ctx, `SELECT last_error FROM account_services WHERE account_id = ? AND service = ?`,
		accountID, service).Scan(&old)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("service synced: %w", err)
	}
	if errText == "" {
		_, err = t.ExecContext(ctx, `UPDATE account_services SET last_sync_at = ?, last_error = ''
			WHERE account_id = ? AND service = ?`, FormatTime(at), accountID, service)
	} else {
		_, err = t.ExecContext(ctx, `UPDATE account_services SET last_error = ?
			WHERE account_id = ? AND service = ?`, errText, accountID, service)
	}
	if err != nil {
		return fmt.Errorf("service synced: %w", err)
	}
	if old == errText {
		return nil
	}
	return t.Emit(ctx, api.AccountChanged{ID: accountID})
}

// Services returns an account's services rows, contacts, calendar, tasks;
// a service never turned on has none. accountID 0 returns every account's.
func (d *DB) Services(ctx context.Context, accountID int64) ([]Service, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT account_id, service, enabled, url, home, last_sync_at, last_error
		FROM account_services WHERE ? = 0 OR account_id = ?
		ORDER BY account_id, CASE service WHEN 'contacts' THEN 0 WHEN 'calendar' THEN 1 ELSE 2 END`,
		accountID, accountID)
	if err != nil {
		return nil, fmt.Errorf("services: %w", err)
	}
	defer rows.Close()
	var out []Service
	for rows.Next() {
		var (
			s    Service
			last sql.NullString
		)
		if err := rows.Scan(&s.AccountID, &s.Service, &s.Enabled, &s.URL, &s.Home, &last, &s.LastError); err != nil {
			return nil, fmt.Errorf("services: %w", err)
		}
		if last.Valid {
			at, err := ParseTime(last.String)
			if err != nil {
				return nil, err
			}
			s.LastSyncAt = &at
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SetServiceHome records the home set discovery found for a service, or
// clears it ("") so the next pass discovers again.
func (t *Tx) SetServiceHome(ctx context.Context, accountID int64, service api.ServiceKind, home string) error {
	res, err := t.ExecContext(ctx, `UPDATE account_services SET home = ? WHERE account_id = ? AND service = ?`,
		home, accountID, service)
	if err != nil {
		return fmt.Errorf("set service home: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
