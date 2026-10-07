package store

import (
	"context"
	"fmt"

	"github.com/frostyard/frostmail/api"
)

// Mailbox is a mailboxes row with its local counts.
type Mailbox struct {
	ID        int64
	AccountID int64
	Path      string
	Name      string
	Delimiter string
	Role      api.MailboxRole
	Total     int64
	Unread    int64
}

// ListMailboxes returns mailboxes ordered by account, role (inbox first,
// user folders last) and path. accountID 0 means every account.
func (d *DB) ListMailboxes(ctx context.Context, accountID int64) ([]Mailbox, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT mb.id, mb.account_id, mb.path, mb.name, mb.delimiter, mb.role,
		       COUNT(mm.message_id), COALESCE(SUM(m.seen = 0), 0)
		FROM mailboxes mb
		LEFT JOIN message_mailbox mm ON mm.mailbox_id = mb.id
		LEFT JOIN messages m ON m.id = mm.message_id
		WHERE ? = 0 OR mb.account_id = ?
		GROUP BY mb.id
		ORDER BY mb.account_id,
		         CASE mb.role WHEN 'inbox' THEN 0 WHEN 'drafts' THEN 1 WHEN 'sent' THEN 2
		              WHEN 'archive' THEN 3 WHEN 'all' THEN 4 WHEN 'flagged' THEN 5
		              WHEN 'junk' THEN 6 WHEN 'trash' THEN 7 ELSE 8 END,
		         mb.path`, accountID, accountID)
	if err != nil {
		return nil, fmt.Errorf("list mailboxes: %w", err)
	}
	defer rows.Close()
	var out []Mailbox
	for rows.Next() {
		var mb Mailbox
		var role string
		if err := rows.Scan(&mb.ID, &mb.AccountID, &mb.Path, &mb.Name, &mb.Delimiter, &role, &mb.Total, &mb.Unread); err != nil {
			return nil, fmt.Errorf("list mailboxes: %w", err)
		}
		mb.Role = api.MailboxRole(role)
		out = append(out, mb)
	}
	return out, rows.Err()
}
