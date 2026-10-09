package calendar

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

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

// ParseTodo reads the first VTODO of the first VCALENDAR.
func ParseTodo(raw []byte, opts Options) (Todo, error) {
	components, err := contentline.Parse(raw)
	if err != nil {
		return Todo{}, fmt.Errorf("parse todo: %w", err)
	}
	if opts.Local == nil {
		opts.Local = time.Local
	}
	for _, c := range components {
		if c.Name != "VCALENDAR" {
			continue
		}
		todos := c.ChildrenNamed("VTODO")
		if len(todos) == 0 {
			return Todo{}, errors.New("parse todo: no VTODO")
		}
		return readTodo(todos[0], timeReader{calendar: c, local: opts.Local}), nil
	}
	return Todo{}, errors.New("parse todo: no VCALENDAR")
}

func readTodo(c *contentline.Component, reader timeReader) Todo {
	todo := Todo{
		UID:     strings.TrimSpace(propertyText(c, "UID")),
		Summary: propertyText(c, "SUMMARY"), Description: propertyText(c, "DESCRIPTION"),
		SortOrder: strings.TrimSpace(propertyValue(c, "X-APPLE-SORT-ORDER")),
		Completed: strings.EqualFold(propertyValue(c, "STATUS"), "COMPLETED"),
	}
	for _, p := range c.PropsNamed("RELATED-TO") {
		if relation := p.Param("RELTYPE"); relation == "" || strings.EqualFold(relation, "PARENT") {
			todo.ParentUID = strings.TrimSpace(p.Text())
			break
		}
	}
	if due, ok := reader.read(c.Prop("DUE")); ok {
		date := due.instant
		if !due.allDay {
			date = date.In(reader.local)
		}
		todo.Due = date.Format(time.DateOnly)
	}
	if completed, ok := reader.read(c.Prop("COMPLETED")); ok {
		todo.Completed, todo.CompletedAt = true, completed.instant
	}
	return todo
}
