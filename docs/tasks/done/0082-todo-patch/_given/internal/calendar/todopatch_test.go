package calendar

// CONTRACT TEST for task card T-0082 (docs/tasks). Do not edit.

import (
	"strings"
	"testing"
	"time"
)

var patchNow = time.Date(2026, 10, 9, 14, 30, 0, 0, time.UTC)

func ptrTo[T any](v T) *T { return &v }

// lf writes lines ending in CRLF, as servers send them.
func crlf(lines ...string) string { return strings.Join(lines, "\r\n") + "\r\n" }

// reminders is a task as Apple's Reminders writes it, with properties the
// patches must keep as they are.
var reminders = crlf(
	"BEGIN:VCALENDAR",
	"VERSION:2.0",
	"PRODID:-//Apple Inc.//iOS 18.0//EN",
	"BEGIN:VTODO",
	"CREATED:20261001T090000Z",
	"DTSTAMP:20261001T090000Z",
	"LAST-MODIFIED:20261001T090000Z",
	"PRIORITY:0",
	"SEQUENCE:0",
	"STATUS:NEEDS-ACTION",
	"SUMMARY:Buy milk",
	"DUE;TZID=Europe/Berlin:20261012T180000",
	"UID:A1B2C3",
	"X-APPLE-SORT-ORDER:731235684",
	"BEGIN:VALARM",
	"ACTION:DISPLAY",
	"TRIGGER;VALUE=DATE-TIME:20261012T160000Z",
	"END:VALARM",
	"END:VTODO",
	"END:VCALENDAR",
)

func patch(t *testing.T, src string, c TodoChange) string {
	t.Helper()
	out, err := PatchTodo([]byte(src), c, patchNow)
	if err != nil {
		t.Fatal(err)
	}
	// What is written must read back.
	if _, err := ParseTodo(out, Options{Local: time.UTC}); err != nil {
		t.Fatalf("the patched task does not parse: %v\n%s", err, out)
	}
	return string(out)
}

func TestPatchSummaryKeepsTheRest(t *testing.T) {
	got := patch(t, reminders, TodoChange{Summary: ptrTo("Buy oat milk, 2 l")})
	want := strings.NewReplacer(
		"SUMMARY:Buy milk\r\n", "SUMMARY:Buy oat milk\\, 2 l\r\n",
		"DTSTAMP:20261001T090000Z", "DTSTAMP:20261009T143000Z",
		"LAST-MODIFIED:20261001T090000Z", "LAST-MODIFIED:20261009T143000Z",
	).Replace(reminders)
	if got != want {
		t.Errorf("patched:\n%s\nwant:\n%s", got, want)
	}
}

func TestPatchCompleteAndReopen(t *testing.T) {
	done := patch(t, reminders, TodoChange{Completed: ptrTo(true)})
	for _, line := range []string{"STATUS:COMPLETED\r\n", "COMPLETED:20261009T143000Z\r\n", "PERCENT-COMPLETE:100\r\n"} {
		if strings.Count(done, line) != 1 {
			t.Errorf("completed task lacks %q once:\n%s", line, done)
		}
	}
	if strings.Contains(done, "NEEDS-ACTION") {
		t.Errorf("still needs action:\n%s", done)
	}
	// Inserted lines go before END:VTODO, after the alarm, never inside it.
	if i, j := strings.Index(done, "END:VALARM"), strings.Index(done, "COMPLETED:2026"); j < i {
		t.Errorf("COMPLETED landed inside or before the alarm:\n%s", done)
	}
	parsed, _ := ParseTodo([]byte(done), Options{Local: time.UTC})
	if !parsed.Completed || !parsed.CompletedAt.Equal(patchNow) {
		t.Errorf("read back = %+v", parsed)
	}

	open := patch(t, done, TodoChange{Completed: ptrTo(false)})
	if !strings.Contains(open, "STATUS:NEEDS-ACTION\r\n") || strings.Contains(open, "COMPLETED:2026") || strings.Contains(open, "PERCENT-COMPLETE") {
		t.Errorf("reopened:\n%s", open)
	}
	if parsed, _ := ParseTodo([]byte(open), Options{Local: time.UTC}); parsed.Completed {
		t.Errorf("reopened task reads as completed: %+v", parsed)
	}
}

func TestPatchDue(t *testing.T) {
	moved := patch(t, reminders, TodoChange{Due: ptrTo("2026-10-20")})
	if !strings.Contains(moved, "DUE;VALUE=DATE:20261020\r\n") || strings.Contains(moved, "TZID=Europe/Berlin") {
		t.Errorf("moved due:\n%s", moved)
	}
	cleared := patch(t, reminders, TodoChange{Due: ptrTo("")})
	if strings.Contains(cleared, "DUE") {
		t.Errorf("cleared due:\n%s", cleared)
	}
	added := patch(t, cleared, TodoChange{Due: ptrTo("2026-11-01")})
	if strings.Count(added, "DUE;VALUE=DATE:20261101\r\n") != 1 {
		t.Errorf("added due:\n%s", added)
	}
}

func TestPatchDescription(t *testing.T) {
	noted := patch(t, reminders, TodoChange{Description: ptrTo("Two litres;\noat")})
	if !strings.Contains(noted, "DESCRIPTION:Two litres\\;\\noat\r\n") {
		t.Errorf("noted:\n%s", noted)
	}
	parsed, _ := ParseTodo([]byte(noted), Options{Local: time.UTC})
	if parsed.Description != "Two litres;\noat" {
		t.Errorf("read back = %q", parsed.Description)
	}
	if bare := patch(t, noted, TodoChange{Description: ptrTo("")}); strings.Contains(bare, "DESCRIPTION") {
		t.Errorf("removed description:\n%s", bare)
	}
}

func TestPatchKeepsLineEndingsAndFolds(t *testing.T) {
	lf := strings.ReplaceAll(reminders, "\r\n", "\n")
	got := patch(t, lf, TodoChange{Summary: ptrTo(strings.Repeat("long title ", 10))})
	if strings.Contains(got, "\r") {
		t.Errorf("LF source got CRLF lines:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if len(line) > 75 {
			t.Errorf("unfolded line of %d octets: %q", len(line), line)
		}
	}
	parsed, _ := ParseTodo([]byte(got), Options{Local: time.UTC})
	if parsed.Summary != strings.Repeat("long title ", 10) {
		t.Errorf("read back = %q", parsed.Summary)
	}
}

func TestPatchErrors(t *testing.T) {
	if _, err := PatchTodo([]byte(crlf("BEGIN:VCALENDAR", "BEGIN:VEVENT", "UID:x", "END:VEVENT", "END:VCALENDAR")),
		TodoChange{Summary: ptrTo("x")}, patchNow); err == nil {
		t.Error("patched an event as a task")
	}
}

func TestNewTodo(t *testing.T) {
	got := string(NewTodo("new-1", TodoChange{Summary: ptrTo("Call Ann"), Description: ptrTo("About, the offsite"),
		Due: ptrTo("2026-10-12")}, "parent-9", patchNow))
	want := crlf(
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//Frostyard//Frostmail//EN",
		"BEGIN:VTODO",
		"UID:new-1",
		"DTSTAMP:20261009T143000Z",
		"CREATED:20261009T143000Z",
		"LAST-MODIFIED:20261009T143000Z",
		"SUMMARY:Call Ann",
		"DESCRIPTION:About\\, the offsite",
		"DUE;VALUE=DATE:20261012",
		"STATUS:NEEDS-ACTION",
		"RELATED-TO;RELTYPE=PARENT:parent-9",
		"END:VTODO",
		"END:VCALENDAR",
	)
	if got != want {
		t.Errorf("new task:\n%s\nwant:\n%s", got, want)
	}
	bare := string(NewTodo("new-2", TodoChange{Summary: ptrTo("Done already"), Completed: ptrTo(true)}, "", patchNow))
	parsed, err := ParseTodo([]byte(bare), Options{Local: time.UTC})
	if err != nil || parsed.UID != "new-2" || !parsed.Completed || parsed.ParentUID != "" || parsed.Due != "" ||
		strings.Contains(bare, "DESCRIPTION") || strings.Contains(bare, "RELATED-TO") {
		t.Errorf("bare new task = %+v, %v\n%s", parsed, err, bare)
	}
}
