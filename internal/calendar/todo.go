package calendar

import (
	"errors"
	"time"
)

// errTodoNotYet marks what task card T-0079 has yet to write.
var errTodoNotYet = errors.New("calendar: VTODO not implemented yet")

// Todo is a VTODO's index (docs/design/pim.md, Tasks).
type Todo struct {
	UID string
	// ParentUID is RELATED-TO with RELTYPE=PARENT or no RELTYPE; "" when none.
	ParentUID   string
	Summary     string
	Description string
	// Due is DUE's date, YYYY-MM-DD (a DATE-TIME's date in Options.Local), or "".
	Due string
	// Completed is STATUS:COMPLETED, or a COMPLETED time.
	Completed bool
	// CompletedAt is COMPLETED in UTC; zero when absent.
	CompletedAt time.Time
	// SortOrder is X-APPLE-SORT-ORDER, or "".
	SortOrder string
}

// ParseTodo reads the first VTODO of the first VCALENDAR. Task T-0079
// writes it.
func ParseTodo(raw []byte, opts Options) (Todo, error) {
	return Todo{}, errTodoNotYet
}
