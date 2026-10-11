package store

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/frostyard/frostmail/api"
)

// VIP is one VIP address (ADR-0026, docs/design/organize.md, VIPs).
type VIP struct {
	Address  string // lowercased
	Name     string // the person's in People, else the one seen in mail, else ""
	PersonID int64  // 0 when People has no person with the address
}

// VIPs returns every VIP by name (case-insensitively), then address.
func (d *DB) VIPs(ctx context.Context) ([]VIP, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT v.address,
		COALESCE((SELECT c.person_id FROM contact_emails ce JOIN contacts c ON c.object_id = ce.contact_id
			WHERE ce.email = v.address AND c.person_id IS NOT NULL ORDER BY c.object_id LIMIT 1), 0),
		COALESCE(NULLIF((SELECT name FROM addresses WHERE address = v.address), ''), v.name)
		FROM vips v`)
	if err != nil {
		return nil, fmt.Errorf("vips: %w", err)
	}
	defer rows.Close()
	out := []VIP{}
	for rows.Next() {
		var v VIP
		if err := rows.Scan(&v.Address, &v.PersonID, &v.Name); err != nil {
			return nil, fmt.Errorf("vips: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vips: %w", err)
	}
	for i := range out {
		if out[i].PersonID == 0 {
			continue
		}
		p, err := d.Person(ctx, out[i].PersonID)
		if err == nil {
			out[i].Name = p.DisplayName
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}
	slices.SortFunc(out, func(a, b VIP) int {
		if n := cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); n != 0 {
			return n
		}
		return cmp.Compare(a.Address, b.Address)
	})
	return out, nil
}

// PersonEmails returns the lowercased addresses of a person's contacts,
// ascending, or ErrNotFound for no such person.
func (d *DB) PersonEmails(ctx context.Context, personID int64) ([]string, error) {
	if _, err := d.Person(ctx, personID); err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, `SELECT DISTINCT ce.email FROM contacts c
		JOIN contact_emails ce ON ce.contact_id = c.object_id WHERE c.person_id = ? ORDER BY ce.email`, personID)
	if err != nil {
		return nil, fmt.Errorf("person emails: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, fmt.Errorf("person emails: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// AddVIPs makes addresses VIPs (lowercased; ones already VIPs are left as
// they are), remembering the name seen with each, and emits
// api.VipChanged when one was added.
func (t *Tx) AddVIPs(ctx context.Context, addresses []string) error {
	added := false
	for _, a := range addresses {
		a = strings.ToLower(strings.TrimSpace(a))
		var name string
		err := t.QueryRowContext(ctx, `SELECT name FROM addresses WHERE address = ?`, a).Scan(&name)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("add vip: %w", err)
		}
		res, err := t.ExecContext(ctx, `INSERT INTO vips (address, name, added_at) VALUES (?, ?, ?)
			ON CONFLICT (address) DO NOTHING`, a, name, FormatTime(t.Now()))
		if err != nil {
			return fmt.Errorf("add vip %s: %w", a, err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added = true
		}
	}
	if !added {
		return nil
	}
	return t.Emit(ctx, api.VipChanged{})
}

// RemoveVIPs stops addresses being VIPs and emits api.VipChanged when one
// was.
func (t *Tx) RemoveVIPs(ctx context.Context, addresses []string) error {
	removed := false
	for _, a := range addresses {
		res, err := t.ExecContext(ctx, `DELETE FROM vips WHERE address = ?`, strings.ToLower(strings.TrimSpace(a)))
		if err != nil {
			return fmt.Errorf("remove vip: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			removed = true
		}
	}
	if !removed {
		return nil
	}
	return t.Emit(ctx, api.VipChanged{})
}
