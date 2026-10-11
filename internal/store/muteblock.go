package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/frostyard/frostmail/api"
)

// MuteThreads mutes or unmutes the conversations of messageIDs (ADR-0028)
// and emits api.MessageChanged, per account, for every message of them.
// Messages not yet in a conversation are left as they are.
func (t *Tx) MuteThreads(ctx context.Context, messageIDs []int64, muted bool) error {
	threads, err := t.ThreadsOf(ctx, messageIDs)
	if err != nil || len(threads) == 0 {
		return err
	}
	in, err := json.Marshal(threads)
	if err != nil {
		return fmt.Errorf("mute: %w", err)
	}
	if _, err := t.ExecContext(ctx, `UPDATE threads SET muted = ? WHERE id IN (SELECT value FROM json_each(?))`,
		muted, string(in)); err != nil {
		return fmt.Errorf("mute: %w", err)
	}
	rows, err := t.QueryContext(ctx, `SELECT id, account_id FROM messages
		WHERE thread_id IN (SELECT value FROM json_each(?)) ORDER BY id`, string(in))
	if err != nil {
		return fmt.Errorf("mute: %w", err)
	}
	defer rows.Close()
	changed := map[int64][]int64{}
	for rows.Next() {
		var id, acct int64
		if err := rows.Scan(&id, &acct); err != nil {
			return fmt.Errorf("mute: %w", err)
		}
		changed[acct] = append(changed[acct], id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("mute: %w", err)
	}
	for _, acct := range slices.Sorted(maps.Keys(changed)) {
		if err := t.Emit(ctx, api.MessageChanged{AccountID: acct, IDs: changed[acct]}); err != nil {
			return err
		}
	}
	return nil
}

// BlockedSender is one blocked address (ADR-0028).
type BlockedSender struct {
	Address string // lowercased
	Name    string // the name last seen with it in mail, else the one stored when blocked
	At      time.Time
}

// BlockedSenders returns every blocked sender by address.
func (d *DB) BlockedSenders(ctx context.Context) ([]BlockedSender, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT b.address,
		COALESCE(NULLIF((SELECT name FROM addresses WHERE address = b.address), ''), b.name), b.at
		FROM blocked b ORDER BY b.address`)
	if err != nil {
		return nil, fmt.Errorf("blocked senders: %w", err)
	}
	defer rows.Close()
	out := []BlockedSender{}
	for rows.Next() {
		var b BlockedSender
		var at string
		if err := rows.Scan(&b.Address, &b.Name, &at); err != nil {
			return nil, fmt.Errorf("blocked senders: %w", err)
		}
		if b.At, err = ParseTime(at); err != nil {
			return nil, fmt.Errorf("blocked sender %s: %w", b.Address, err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BlockedAmong returns those of ids whose sender is blocked, ascending.
func (d *DB) BlockedAmong(ctx context.Context, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	in, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("blocked among: %w", err)
	}
	rows, err := d.db.QueryContext(ctx, `SELECT id FROM messages WHERE id IN (SELECT value FROM json_each(?))
		AND lower(from_addr) IN (SELECT address FROM blocked) ORDER BY id`, string(in))
	if err != nil {
		return nil, fmt.Errorf("blocked among: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("blocked among: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Block blocks addresses (lowercased; ones already blocked stay as they
// are), remembering the name seen with each, and emits api.BlockChanged
// when one was added.
func (t *Tx) Block(ctx context.Context, addresses []string) error {
	added := false
	for _, a := range addresses {
		a = strings.ToLower(strings.TrimSpace(a))
		var name string
		err := t.QueryRowContext(ctx, `SELECT name FROM addresses WHERE address = ?`, a).Scan(&name)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("block: %w", err)
		}
		res, err := t.ExecContext(ctx, `INSERT INTO blocked (address, name, at) VALUES (?, ?, ?)
			ON CONFLICT (address) DO NOTHING`, a, name, FormatTime(t.Now()))
		if err != nil {
			return fmt.Errorf("block %s: %w", a, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added = true
		}
	}
	if !added {
		return nil
	}
	return t.Emit(ctx, api.BlockChanged{})
}

// Unblock unblocks addresses and emits api.BlockChanged when one was
// blocked.
func (t *Tx) Unblock(ctx context.Context, addresses []string) error {
	removed := false
	for _, a := range addresses {
		res, err := t.ExecContext(ctx, `DELETE FROM blocked WHERE address = ?`, strings.ToLower(strings.TrimSpace(a)))
		if err != nil {
			return fmt.Errorf("unblock: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			removed = true
		}
	}
	if !removed {
		return nil
	}
	return t.Emit(ctx, api.BlockChanged{})
}
