package store

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// LocalCopy is what the store holds at one UID of a mailbox, for the safety
// check (docs/design/accounts.md, Safety check).
type LocalCopy struct {
	UID     uint32
	Message int64
	// Queued is true when a queued action covers the message, whose local
	// state then differs from the server's until the replay.
	Queued bool
	// Labels are the message's Gmail label mailboxes, ascending.
	Labels []int64
	flags  flagCols
}

// FlagsDiffer reports whether flags, as the server reports them, differ
// from the stored ones.
func (c LocalCopy) FlagsDiffer(flags Flags) bool {
	want, err := flagValues(flags)
	return err != nil || want != c.flags
}

// LocalCopies returns every stored message of a mailbox that has a UID, in
// UID order.
func (d *DB) LocalCopies(ctx context.Context, mailboxID int64) ([]LocalCopy, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT mm.uid, m.id,
		m.seen, m.flagged, m.answered, m.forwarded, m.draft, m.deleted, m.flag_color, m.keywords_json,
		EXISTS (SELECT 1 FROM pending_op_messages pm JOIN pending_ops p ON p.id = pm.op_id
			WHERE pm.message_id = m.id AND p.state != 'failed'),
		COALESCE((SELECT GROUP_CONCAT(l.mailbox_id) FROM message_mailbox l
			JOIN mailboxes b ON b.id = l.mailbox_id
			WHERE l.message_id = m.id AND b.is_gmail_label = 1), '')
		FROM message_mailbox mm JOIN messages m ON m.id = mm.message_id
		WHERE mm.mailbox_id = ? AND mm.uid IS NOT NULL ORDER BY mm.uid`, mailboxID)
	if err != nil {
		return nil, fmt.Errorf("local copies of %d: %w", mailboxID, err)
	}
	defer rows.Close()
	var out []LocalCopy
	for rows.Next() {
		var c LocalCopy
		var uid int64
		var labels string
		f := &c.flags
		if err := rows.Scan(&uid, &c.Message, &f.seen, &f.flagged, &f.answered, &f.forwarded, &f.draft, &f.deleted,
			&f.color, &f.keywords, &c.Queued, &labels); err != nil {
			return nil, fmt.Errorf("local copies of %d: %w", mailboxID, err)
		}
		c.UID = uint32(uid)
		for s := range strings.SplitSeq(labels, ",") {
			if id, err := strconv.ParseInt(s, 10, 64); err == nil {
				c.Labels = append(c.Labels, id)
			}
		}
		slices.Sort(c.Labels)
		out = append(out, c)
	}
	return out, rows.Err()
}
