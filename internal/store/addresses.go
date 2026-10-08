package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RecordAddresses counts addresses seen in mail. Each address with a
// non-empty trimmed Addr is counted once per call, keyed by its trimmed,
// lowercased address: a new row gets the trimmed name, count 1 and
// last_seen seen; an existing row gets count + 1, the given name when it
// is not empty (otherwise it keeps its name), and the later last_seen.
func (t *Tx) RecordAddresses(ctx context.Context, as []Address, seen time.Time) error {
	seenAt := FormatTime(seen)
	done := make(map[string]bool, len(as))
	for _, a := range as {
		addr := strings.ToLower(strings.TrimSpace(a.Addr))
		if addr == "" || done[addr] {
			continue
		}
		done[addr] = true
		_, err := t.ExecContext(ctx, `INSERT INTO addresses (address, name, count, last_seen)
			VALUES (?, ?, 1, ?)
			ON CONFLICT (address) DO UPDATE SET
				count = count + 1,
				name = CASE WHEN excluded.name != '' THEN excluded.name ELSE addresses.name END,
				last_seen = max(addresses.last_seen, excluded.last_seen)`,
			addr, strings.TrimSpace(a.Name), seenAt)
		if err != nil {
			return fmt.Errorf("record address %s: %w", addr, err)
		}
	}
	return nil
}

// SuggestAddresses returns up to limit addresses matching a typed prefix,
// highest count first, then most recently seen, then by address. A row
// matches when, case-insensitively, the address starts with the prefix,
// the name starts with it, or any later word of the name starts with it;
// LIKE metacharacters in the prefix are literal. An empty prefix returns
// nil, and a limit at or below 0 means 10.
func (d *DB) SuggestAddresses(ctx context.Context, prefix string, limit int) ([]Address, error) {
	if prefix == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	p := escapeLike(strings.ToLower(prefix))
	rows, err := d.db.QueryContext(ctx, `SELECT address, name FROM addresses
		WHERE address LIKE ? ESCAPE '\'
		   OR name LIKE ? ESCAPE '\'
		   OR name LIKE ? ESCAPE '\'
		ORDER BY count DESC, last_seen DESC, address
		LIMIT ?`,
		p+"%", p+"%", "% "+p+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("suggest addresses: %w", err)
	}
	defer rows.Close()
	var out []Address
	for rows.Next() {
		var a Address
		if err := rows.Scan(&a.Addr, &a.Name); err != nil {
			return nil, fmt.Errorf("suggest addresses: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
