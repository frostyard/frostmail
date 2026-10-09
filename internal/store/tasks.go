package store

import (
	"context"
	"errors"
	"time"
)

// errTasksNotYet marks what task card T-0079 has yet to write.
var errTasksNotYet = errors.New("store: tasks not implemented yet")

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

// Task T-0079 writes the functions below.

// IndexTask stores or replaces an object's task.
func (t *Tx) IndexTask(ctx context.Context, objectID int64, r TaskRow) error {
	return errTasksNotYet
}

// RemoveTask deletes an object's task, if it has one.
func (t *Tx) RemoveTask(ctx context.Context, objectID int64) error {
	return errTasksNotYet
}

// Tasks returns the tasks of shown task lists in order.
func (d *DB) Tasks(ctx context.Context, f TaskFilter) ([]TaskRow, error) {
	return nil, errTasksNotYet
}

// Task returns one task.
func (d *DB) Task(ctx context.Context, objectID int64) (TaskRow, error) {
	return TaskRow{}, errTasksNotYet
}
