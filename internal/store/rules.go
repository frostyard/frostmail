package store

import (
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/frostyard/frostmail/api"
)

// Rule acts on new inbox mail and on chosen messages (ADR-0024,
// docs/design/organize.md, Rules).
type Rule struct {
	ID         int64
	Name       string
	Position   int // from 0; rules run in position order
	Enabled    bool
	Conditions api.Conditions
	Actions    []api.RuleAction
}

const ruleColumns = `SELECT id, name, position, enabled, conditions_json, actions_json FROM rules`

func scanRule(row rowScanner) (Rule, error) {
	var r Rule
	var conds, actions string
	if err := row.Scan(&r.ID, &r.Name, &r.Position, &r.Enabled, &conds, &actions); err != nil {
		return Rule{}, err
	}
	c, err := decodeConditions(conds)
	if err != nil {
		return Rule{}, fmt.Errorf("rule %d: %w", r.ID, err)
	}
	r.Conditions = c
	if err := json.Unmarshal([]byte(actions), &r.Actions); err != nil {
		return Rule{}, fmt.Errorf("rule %d actions: %w", r.ID, err)
	}
	return r, nil
}

// Rules returns every rule in position order.
func (d *DB) Rules(ctx context.Context) ([]Rule, error) {
	rows, err := d.db.QueryContext(ctx, ruleColumns+` ORDER BY position, id`)
	if err != nil {
		return nil, fmt.Errorf("rules: %w", err)
	}
	defer rows.Close()
	out := []Rule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Rule returns one rule, or ErrNotFound.
func (d *DB) Rule(ctx context.Context, id int64) (Rule, error) {
	return getRule(d.db.QueryRowContext(ctx, ruleColumns+` WHERE id = ?`, id))
}

func getRule(row *sql.Row) (Rule, error) {
	r, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, ErrNotFound
	}
	return r, err
}

func encodeRule(r Rule) (conds, actions string, err error) {
	if conds, err = encodeConditions(r.Conditions); err != nil {
		return "", "", err
	}
	if r.Actions == nil {
		r.Actions = []api.RuleAction{}
	}
	data, err := json.Marshal(r.Actions)
	return conds, string(data), err
}

// CreateRule adds r at the end of the list and emits api.RuleChanged. The
// caller has checked it; r.ID and r.Position are ignored.
func (t *Tx) CreateRule(ctx context.Context, r Rule) (Rule, error) {
	conds, actions, err := encodeRule(r)
	if err != nil {
		return Rule{}, fmt.Errorf("create rule: %w", err)
	}
	var id int64
	err = t.QueryRowContext(ctx, `INSERT INTO rules (name, position, enabled, conditions_json, actions_json)
		VALUES (?, (SELECT COALESCE(MAX(position) + 1, 0) FROM rules), ?, ?, ?) RETURNING id`,
		r.Name, bit(r.Enabled), conds, actions).Scan(&id)
	if err != nil {
		return Rule{}, fmt.Errorf("create rule: %w", err)
	}
	if err := t.Emit(ctx, api.RuleChanged{ID: id}); err != nil {
		return Rule{}, err
	}
	return getRule(t.QueryRowContext(ctx, ruleColumns+` WHERE id = ?`, id))
}

// UpdateRule stores r's name, enabled, conditions and actions, and emits
// api.RuleChanged; ErrNotFound when there is no such rule.
func (t *Tx) UpdateRule(ctx context.Context, r Rule) (Rule, error) {
	conds, actions, err := encodeRule(r)
	if err != nil {
		return Rule{}, fmt.Errorf("update rule: %w", err)
	}
	res, err := t.ExecContext(ctx, `UPDATE rules SET name = ?, enabled = ?, conditions_json = ?, actions_json = ?
		WHERE id = ?`, r.Name, bit(r.Enabled), conds, actions, r.ID)
	if err != nil {
		return Rule{}, fmt.Errorf("update rule: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Rule{}, ErrNotFound
	}
	if err := t.Emit(ctx, api.RuleChanged{ID: r.ID}); err != nil {
		return Rule{}, err
	}
	return getRule(t.QueryRowContext(ctx, ruleColumns+` WHERE id = ?`, r.ID))
}

// DeleteRule removes a rule, closes the gap in the positions and emits
// api.RuleChanged.
func (t *Tx) DeleteRule(ctx context.Context, id int64) error {
	if err := t.deleteRow(ctx, "rules", id); err != nil {
		return err
	}
	return t.Emit(ctx, api.RuleChanged{ID: id, Deleted: true})
}

// MoveRule puts a rule at position (clamped to the list), moving the others
// along, and emits api.RuleChanged.
func (t *Tx) MoveRule(ctx context.Context, id int64, position int) error {
	moved, err := t.moveRow(ctx, "rules", id, position)
	if err != nil || !moved {
		return err
	}
	return t.Emit(ctx, api.RuleChanged{ID: id})
}

// MarkRulesWaiting marks those of ids that are in an inbox (\Inbox on
// Gmail) as waiting for the rules (ADR-0024), in the transaction that
// stored them.
func (t *Tx) MarkRulesWaiting(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	in, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("mark rules waiting: %w", err)
	}
	_, err = t.ExecContext(ctx, `UPDATE messages SET rules_waiting = 1
		WHERE id IN (SELECT value FROM json_each(?)) AND EXISTS (SELECT 1 FROM message_mailbox mm
			JOIN mailboxes mb ON mb.id = mm.mailbox_id WHERE mm.message_id = messages.id AND mb.role = 'inbox')`, string(in))
	if err != nil {
		return fmt.Errorf("mark rules waiting: %w", err)
	}
	return nil
}

// RulesWaiting returns an account's messages waiting for the rules,
// ascending.
func (d *DB) RulesWaiting(ctx context.Context, accountID int64) ([]int64, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT id FROM messages WHERE account_id = ? AND rules_waiting = 1 ORDER BY id`, accountID)
	if err != nil {
		return nil, fmt.Errorf("rules waiting: %w", err)
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("rules waiting: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ClearRulesWaiting marks messages as no longer waiting for the rules.
func (t *Tx) ClearRulesWaiting(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	in, err := json.Marshal(ids)
	if err != nil {
		return fmt.Errorf("clear rules waiting: %w", err)
	}
	if _, err := t.ExecContext(ctx, `UPDATE messages SET rules_waiting = 0 WHERE id IN (SELECT value FROM json_each(?))`,
		string(in)); err != nil {
		return fmt.Errorf("clear rules waiting: %w", err)
	}
	return nil
}
