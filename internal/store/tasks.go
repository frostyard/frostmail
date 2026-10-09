package store

import (
	"cmp"
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"
)

// TaskRow is a tasks row (docs/design/pim.md, Tasks).
type TaskRow struct {
	// ObjectID is the task's object: the task's ID in the API.
	ObjectID  int64
	UID       string
	ParentUID string
	Title     string
	Notes     string
	Due       string // YYYY-MM-DD, or ""
	Completed bool
	// CompletedAt is set when known.
	CompletedAt *time.Time
	Position    string
	// GmThrID is the Gmail thread a task made from mail links to; 0 for none.
	GmThrID int64
	// Set by reads: the task's list and account, whether either is
	// read-only, and the parent's object (0 when it has none in the list).
	ListID    int64
	AccountID int64
	ReadOnly  bool
	ParentID  int64
}

// TaskFilter selects tasks.
type TaskFilter struct {
	// ListID, when set, keeps one list.
	ListID int64
	// Completed includes completed tasks.
	Completed bool
	// DueBefore, when set (YYYY-MM-DD), keeps tasks due before that day.
	DueBefore string
}

// IndexTask stores or replaces an object's task.
func (t *Tx) IndexTask(ctx context.Context, objectID int64, r TaskRow) error {
	var completedAt any
	if r.CompletedAt != nil {
		completedAt = FormatTime(*r.CompletedAt)
	}
	_, err := t.ExecContext(ctx, `INSERT INTO tasks
 (object_id, uid, parent_uid, title, notes, due, completed, completed_at, position, gm_thrid)
 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(object_id) DO UPDATE SET
 uid = excluded.uid, parent_uid = excluded.parent_uid, title = excluded.title,
 notes = excluded.notes, due = excluded.due, completed = excluded.completed,
 completed_at = excluded.completed_at, position = excluded.position, gm_thrid = excluded.gm_thrid`,
		objectID, r.UID, r.ParentUID, r.Title, r.Notes, r.Due, r.Completed, completedAt, r.Position, nullInt64(r.GmThrID))
	if err != nil {
		return fmt.Errorf("index task: %w", err)
	}
	return nil
}

// RemoveTask deletes an object's task, if it has one.
func (t *Tx) RemoveTask(ctx context.Context, objectID int64) error {
	if _, err := t.ExecContext(ctx, "DELETE FROM tasks WHERE object_id = ?", objectID); err != nil {
		return fmt.Errorf("remove task: %w", err)
	}
	return nil
}

const taskColumns = `t.object_id, t.uid, t.parent_uid, t.title, t.notes, t.due,
 t.completed, t.completed_at, t.position, COALESCE(t.gm_thrid, 0), col.id,
 col.account_id, (col.read_only OR a.read_only), COALESCE((SELECT p.object_id
 FROM tasks p JOIN objects po ON po.id = p.object_id
 WHERE t.parent_uid != '' AND p.uid = t.parent_uid AND po.collection_id = col.id
 ORDER BY p.object_id LIMIT 1), 0)`

const taskJoins = ` FROM tasks t JOIN objects o ON o.id = t.object_id
 JOIN collections col ON col.id = o.collection_id JOIN accounts a ON a.id = col.account_id`

// Tasks returns the tasks of shown task lists in order.
func (d *DB) Tasks(ctx context.Context, f TaskFilter) ([]TaskRow, error) {
	query := "SELECT " + taskColumns + taskJoins + ` WHERE col.kind = 'tasklist'
 AND col.enabled = 1 AND EXISTS (SELECT 1 FROM account_services s
 WHERE s.account_id = col.account_id AND s.service = 'tasks' AND s.enabled = 1)`
	var args []any
	if f.ListID != 0 {
		query += " AND col.id = ?"
		args = append(args, f.ListID)
	}
	if !f.Completed {
		query += " AND t.completed = 0"
	}
	if f.DueBefore != "" {
		query += " AND t.due != '' AND t.due < ?"
		args = append(args, f.DueBefore)
	}
	rows, err := d.db.QueryContext(ctx, query+" ORDER BY col.position, col.id", args...)
	if err != nil {
		return nil, fmt.Errorf("tasks: %w", err)
	}
	defer rows.Close()
	out := []TaskRow{}
	for rows.Next() {
		r, err := readTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: %w", err)
	}
	return orderTasks(out), nil
}

// Task returns one task.
func (d *DB) Task(ctx context.Context, objectID int64) (TaskRow, error) {
	r, err := readTask(d.db.QueryRowContext(ctx, "SELECT "+taskColumns+taskJoins+" WHERE t.object_id = ?", objectID))
	if err != nil {
		return TaskRow{}, err
	}
	return r, nil
}

func readTask(row rowScanner) (TaskRow, error) {
	var r TaskRow
	var completedAt sql.NullString
	err := row.Scan(&r.ObjectID, &r.UID, &r.ParentUID, &r.Title, &r.Notes, &r.Due,
		&r.Completed, &completedAt, &r.Position, &r.GmThrID, &r.ListID,
		&r.AccountID, &r.ReadOnly, &r.ParentID)
	if err == sql.ErrNoRows {
		return TaskRow{}, ErrNotFound
	}
	if err != nil {
		return TaskRow{}, fmt.Errorf("read task: %w", err)
	}
	if completedAt.Valid {
		at, err := ParseTime(completedAt.String)
		if err != nil {
			return TaskRow{}, fmt.Errorf("read task completion: %w", err)
		}
		r.CompletedAt = &at
	}
	return r, nil
}

func orderTasks(rows []TaskRow) []TaskRow {
	out := make([]TaskRow, 0, len(rows))
	for start := 0; start < len(rows); {
		end := start + 1
		for end < len(rows) && rows[end].ListID == rows[start].ListID {
			end++
		}
		out = append(out, orderTaskList(rows[start:end])...)
		start = end
	}
	return out
}

func compareTasks(a, b TaskRow) int {
	if (a.Position == "") != (b.Position == "") {
		if a.Position == "" {
			return 1
		}
		return -1
	}
	if order := cmp.Compare(a.Position, b.Position); order != 0 {
		return order
	}
	if order := cmp.Compare(a.Title, b.Title); order != 0 {
		return order
	}
	return cmp.Compare(a.ObjectID, b.ObjectID)
}

func orderTaskList(rows []TaskRow) []TaskRow {
	slices.SortFunc(rows, compareTasks)
	present := make(map[int64]bool, len(rows))
	children := make(map[int64][]TaskRow)
	for _, r := range rows {
		present[r.ObjectID] = true
		children[r.ParentID] = append(children[r.ParentID], r)
	}
	out := make([]TaskRow, 0, len(rows))
	visited := make(map[int64]bool, len(rows))
	var visit func(TaskRow)
	visit = func(r TaskRow) {
		if visited[r.ObjectID] {
			return
		}
		visited[r.ObjectID] = true
		out = append(out, r)
		for _, child := range children[r.ObjectID] {
			visit(child)
		}
	}
	for _, r := range rows {
		if r.ParentID == 0 || !present[r.ParentID] {
			visit(r)
		}
	}
	// Keep malformed cycles visible without visiting a task twice.
	for _, r := range rows {
		visit(r)
	}
	return out
}
