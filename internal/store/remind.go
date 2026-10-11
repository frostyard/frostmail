package store

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"time"
)

// Remind Me (ADR-0025, docs/design/organize.md, Remind Me): a pending
// reminder per message in message_reminders, and messages.list_date, which
// views order by: the arrival date until a reminder fires.

// MessageReminder is a message's pending reminder.
type MessageReminder struct {
	MessageID int64
	AccountID int64
	RemindAt  time.Time
}

// SetMessageReminders gives messages a reminder at at, replacing any they
// had. The caller checks that the messages exist.
func (t *Tx) SetMessageReminders(ctx context.Context, ids []int64, at time.Time) error {
	in, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("set reminders: %w", err)
	}
	if _, err := t.ExecContext(ctx, `INSERT INTO message_reminders (message_id, remind_at)
		SELECT value, ? FROM json_each(?) WHERE true
		ON CONFLICT (message_id) DO UPDATE SET remind_at = excluded.remind_at`, FormatTime(at), string(in)); err != nil {
		return fmt.Errorf("set reminders: %w", err)
	}
	return nil
}

// ClearMessageReminders drops the messages' pending reminders and returns
// the IDs that had one.
func (t *Tx) ClearMessageReminders(ctx context.Context, ids []int64) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	in, err := json.Marshal(ids)
	if err != nil {
		return nil, fmt.Errorf("clear reminders: %w", err)
	}
	rows, err := t.QueryContext(ctx, `DELETE FROM message_reminders WHERE message_id IN (SELECT value FROM json_each(?))
		RETURNING message_id`, string(in))
	if err != nil {
		return nil, fmt.Errorf("clear reminders: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("clear reminders: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DueMessageReminders returns the reminders due by now, oldest first.
func (d *DB) DueMessageReminders(ctx context.Context, now time.Time) ([]MessageReminder, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT r.message_id, m.account_id, r.remind_at
		FROM message_reminders r JOIN messages m ON m.id = r.message_id
		WHERE r.remind_at <= ? ORDER BY r.remind_at, r.message_id`, FormatTime(now))
	if err != nil {
		return nil, fmt.Errorf("due reminders: %w", err)
	}
	defer rows.Close()
	var out []MessageReminder
	for rows.Next() {
		var r MessageReminder
		var at string
		if err := rows.Scan(&r.MessageID, &r.AccountID, &at); err != nil {
			return nil, fmt.Errorf("due reminders: %w", err)
		}
		if r.RemindAt, err = ParseTime(at); err != nil {
			return nil, fmt.Errorf("due reminders: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FireMessageReminders drops the messages' reminders and lists them at now,
// at the top of their views.
func (t *Tx) FireMessageReminders(ctx context.Context, ids []int64, now time.Time) error {
	if _, err := t.ClearMessageReminders(ctx, ids); err != nil {
		return err
	}
	in, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("fire reminders: %w", err)
	}
	if _, err := t.ExecContext(ctx, `UPDATE messages SET list_date = ? WHERE id IN (SELECT value FROM json_each(?))`,
		FormatTime(now), string(in)); err != nil {
		return fmt.Errorf("fire reminders: %w", err)
	}
	return nil
}
