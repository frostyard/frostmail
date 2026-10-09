package calendar

import (
	"errors"
	"time"
)

// errPatchNotYet marks what task card T-0082 has yet to write.
var errPatchNotYet = errors.New("calendar: VTODO patches not implemented yet")

// TodoChange is a change to a task; nil fields keep their values.
type TodoChange struct {
	Summary     *string
	Description *string // "" removes DESCRIPTION
	Due         *string // YYYY-MM-DD, or "" to remove DUE
	Completed   *bool
}

// PatchTodo applies a change to the first VTODO of src, rewriting only the
// lines it touches (ADR-0018: the rest stays as the server sent it). Task
// T-0082 writes it.
func PatchTodo(src []byte, c TodoChange, now time.Time) ([]byte, error) {
	return nil, errPatchNotYet
}

// NewTodo writes a calendar object holding one new VTODO, a subtask of
// parentUID when it is set. Task T-0082 writes it.
func NewTodo(uid string, c TodoChange, parentUID string, now time.Time) []byte {
	return nil
}
