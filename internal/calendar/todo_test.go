package calendar

// CONTRACT TEST for task card T-0079 (docs/tasks). Do not edit.

import (
	"testing"
	"time"
)

func todo(t *testing.T, local *time.Location, lines ...string) Todo {
	t.Helper()
	got, err := ParseTodo(ics(append(append([]string{"BEGIN:VTODO"}, lines...), "END:VTODO")...), Options{Local: local})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestTodoFromReminders(t *testing.T) {
	got := todo(t, time.UTC, "UID:apple-1", "SUMMARY:Buy milk\\, eggs", "DESCRIPTION:Two dozen\\nfree range",
		"DUE;VALUE=DATE:20261015", "STATUS:NEEDS-ACTION", "X-APPLE-SORT-ORDER:731235684")
	want := Todo{UID: "apple-1", Summary: "Buy milk, eggs", Description: "Two dozen\nfree range", Due: "2026-10-15",
		SortOrder: "731235684"}
	if got != want {
		t.Errorf("todo = %+v\nwant %+v", got, want)
	}
}

func TestTodoCompleted(t *testing.T) {
	got := todo(t, time.UTC, "UID:c", "SUMMARY:Done", "STATUS:COMPLETED", "COMPLETED:20261008T143000Z", "PERCENT-COMPLETE:100")
	if !got.Completed || !got.CompletedAt.Equal(time.Date(2026, 10, 8, 14, 30, 0, 0, time.UTC)) {
		t.Errorf("completed = %+v", got)
	}
	if got := todo(t, time.UTC, "UID:d", "COMPLETED:20261008T143000Z"); !got.Completed {
		t.Errorf("a COMPLETED time alone = %+v", got)
	}
	if got := todo(t, time.UTC, "UID:e", "STATUS:IN-PROCESS"); got.Completed || !got.CompletedAt.IsZero() {
		t.Errorf("in process = %+v", got)
	}
}

func TestTodoDue(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	late := []string{"UID:x", "DUE;TZID=America/New_York:20261015T230000"}
	if got := todo(t, newYork, late...).Due; got != "2026-10-15" {
		t.Errorf("due in New York, read there = %q", got)
	}
	if got := todo(t, time.UTC, late...).Due; got != "2026-10-16" {
		t.Errorf("due in New York, read in UTC = %q", got)
	}
	if got := todo(t, newYork, "UID:y", "DUE:20261015T020000Z").Due; got != "2026-10-14" {
		t.Errorf("a UTC due, read in New York = %q", got)
	}
	if got := todo(t, newYork, "UID:z", "DUE:20261015T090000").Due; got != "2026-10-15" {
		t.Errorf("a floating due = %q", got)
	}
	if got := todo(t, time.UTC, "UID:n").Due; got != "" {
		t.Errorf("no due = %q", got)
	}
}

func TestTodoParent(t *testing.T) {
	for related, want := range map[string]string{
		"RELATED-TO;RELTYPE=PARENT:p1": "p1",
		"RELATED-TO:p2":                "p2",
		"RELATED-TO;RELTYPE=SIBLING:s": "",
		"RELATED-TO;RELTYPE=CHILD:c":   "",
	} {
		if got := todo(t, time.UTC, "UID:k", related).ParentUID; got != want {
			t.Errorf("%s: parent = %q, want %q", related, got, want)
		}
	}
}

func TestTodoErrors(t *testing.T) {
	if _, err := ParseTodo(ics("BEGIN:VEVENT", "UID:e", "DTSTART:20261009T090000Z", "END:VEVENT"), Options{}); err == nil {
		t.Error("an event parsed as a task")
	}
	if _, err := ParseTodo([]byte("not a calendar"), Options{}); err == nil {
		t.Error("garbage parsed")
	}
}
