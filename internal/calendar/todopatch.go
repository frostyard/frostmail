package calendar

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/frostyard/frostmail/internal/contentline"
)

// TodoChange is a change to a task; nil fields keep their values.
type TodoChange struct {
	Summary     *string
	Description *string // "" removes DESCRIPTION
	Due         *string // YYYY-MM-DD, or "" to remove DUE
	Completed   *bool
}

// PatchTodo applies a change to the first VTODO of src, rewriting only the
// lines it touches (ADR-0018: the rest stays as the server sent it).
func PatchTodo(src []byte, c TodoChange, now time.Time) ([]byte, error) {
	components, err := contentline.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("patch todo: %w", err)
	}
	for _, calendar := range components {
		if calendar.Name != "VCALENDAR" {
			continue
		}
		todos := calendar.ChildrenNamed("VTODO")
		if len(todos) == 0 {
			return nil, errors.New("patch todo: no VTODO")
		}
		todo := todos[0]
		eol := contentline.LineEnding(src)
		at := todoEndStart(src, todo.End)
		var edits []contentline.Edit
		for _, field := range todoFields(c, now) {
			edits = append(edits, todoFieldEdits(todo, field, at, eol)...)
		}
		out, err := contentline.Apply(src, edits...)
		if err != nil {
			return nil, fmt.Errorf("patch todo: %w", err)
		}
		return out, nil
	}
	return nil, errors.New("patch todo: no VCALENDAR")
}

// NewTodo writes a calendar object holding one new VTODO, a subtask of
// parentUID when it is set.
func NewTodo(uid string, c TodoChange, parentUID string, now time.Time) []byte {
	stamp := now.UTC().Format("20060102T150405Z")
	lines := []string{
		"BEGIN:VCALENDAR", "VERSION:2.0", "PRODID:-//Frostyard//Frostmail//EN",
		"BEGIN:VTODO", "UID:" + contentline.EscapeText(uid),
		"DTSTAMP:" + stamp, "CREATED:" + stamp, "LAST-MODIFIED:" + stamp,
	}
	if c.Summary == nil {
		summary := ""
		c.Summary = &summary
	}
	if c.Completed == nil {
		completed := false
		c.Completed = &completed
	}
	// The timestamps are already in their required creation order.
	for _, field := range todoFields(c, now)[2:] {
		if field.line != "" {
			lines = append(lines, field.line)
		}
	}
	if parentUID != "" {
		lines = append(lines, "RELATED-TO;RELTYPE=PARENT:"+contentline.EscapeText(parentUID))
	}
	lines = append(lines, "END:VTODO", "END:VCALENDAR")
	var out strings.Builder
	for _, line := range lines {
		out.WriteString(contentline.Fold(line, "\r\n"))
	}
	return []byte(out.String())
}

type todoField struct {
	name string
	line string // empty removes the property
}

func todoFields(c TodoChange, now time.Time) []todoField {
	stamp := now.UTC().Format("20060102T150405Z")
	fields := []todoField{{"DTSTAMP", "DTSTAMP:" + stamp}, {"LAST-MODIFIED", "LAST-MODIFIED:" + stamp}}
	if c.Summary != nil {
		fields = append(fields, todoField{"SUMMARY", "SUMMARY:" + contentline.EscapeText(*c.Summary)})
	}
	if c.Description != nil {
		line := ""
		if *c.Description != "" {
			line = "DESCRIPTION:" + contentline.EscapeText(*c.Description)
		}
		fields = append(fields, todoField{"DESCRIPTION", line})
	}
	if c.Due != nil {
		line := ""
		if *c.Due != "" {
			line = "DUE;VALUE=DATE:" + strings.ReplaceAll(*c.Due, "-", "")
		}
		fields = append(fields, todoField{"DUE", line})
	}
	if c.Completed != nil {
		status, completed, percent := "STATUS:NEEDS-ACTION", "", ""
		if *c.Completed {
			status, completed, percent = "STATUS:COMPLETED", "COMPLETED:"+stamp, "PERCENT-COMPLETE:100"
		}
		fields = append(fields, todoField{"STATUS", status}, todoField{"COMPLETED", completed},
			todoField{"PERCENT-COMPLETE", percent})
	}
	return fields
}

func todoFieldEdits(todo *contentline.Component, field todoField, at int, eol string) []contentline.Edit {
	text := ""
	if field.line != "" {
		text = contentline.Fold(field.line, eol)
	}
	props := todo.PropsNamed(field.name)
	if len(props) == 0 {
		if text == "" {
			return nil
		}
		return []contentline.Edit{{Start: at, End: at, Text: text}}
	}
	edits := make([]contentline.Edit, 0, len(props))
	for _, prop := range props {
		edits = append(edits, contentline.Edit{Start: prop.Start, End: prop.End, Text: text})
		text = "" // only the first occurrence gets a replacement
	}
	return edits
}

// todoEndStart walks back over the END line, including any continuations.
func todoEndStart(src []byte, end int) int {
	for {
		lineEnd := end
		if lineEnd > 0 && src[lineEnd-1] == '\n' {
			lineEnd--
		}
		start := bytes.LastIndexByte(src[:lineEnd], '\n') + 1
		if src[start] != ' ' && src[start] != '\t' {
			return start
		}
		end = start
	}
}
