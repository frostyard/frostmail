package gtasks_test

// CONTRACT TEST for task card T-0078 (docs/tasks). Do not edit.

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/gtasks"
	"github.com/frostyard/frostmail/internal/gtasks/gtaskstest"
)

func client(t *testing.T, srv *gtaskstest.Server, base string) *gtasks.Client {
	t.Helper()
	return gtasks.New(base, gtasks.Options{HTTP: srv.Client, UserAgent: "Frostmail/test",
		Token: func(context.Context) (string, error) { return gtaskstest.Token, nil }})
}

func titles(ts []gtasks.Task) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Title)
	}
	return out
}

func ptr[T any](v T) *T { return &v }

func TestRead(t *testing.T) {
	srv := gtaskstest.New(t, gtaskstest.Options{PageSize: 2})
	ctx := t.Context()
	c := client(t, srv, srv.Base+"/") // a trailing slash, as providers.GoogleTasksURL has
	work := srv.AddList("Work")
	srv.AddList("Home")
	srv.AddList("Errands")
	lists, err := c.Lists(ctx)
	if err != nil || len(lists) != 3 || lists[0].ID != work || lists[0].Title != "Work" || lists[2].Title != "Errands" ||
		lists[0].Updated.IsZero() {
		t.Fatalf("lists = %+v, %v", lists, err)
	}

	srv.AddTask(work, gtaskstest.Task{Title: "Done", Status: "completed"})
	mark := srv.Now().Add(time.Millisecond) // updatedMin is inclusive: just after Done
	parent := srv.AddTask(work, gtaskstest.Task{Title: "Report", Notes: "Q4 numbers", Due: "2026-10-09T00:00:00.000Z",
		Links: []gtaskstest.Link{{Type: "email", Description: "Re: report", Link: "https://mail.google.com/mail/#all/1234"}}})
	srv.AddTask(work, gtaskstest.Task{Title: "Charts", Parent: parent})
	gone := srv.AddTask(work, gtaskstest.Task{Title: "Gone"})
	srv.Remove(work, gone)

	all, err := c.Tasks(ctx, work, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if got := titles(all); !slices.Equal(got, []string{"Gone", "Report", "Charts", "Done"}) {
		t.Fatalf("tasks = %q", got)
	}
	gone2, report, charts, done := all[0], all[1], all[2], all[3]
	if !gone2.Deleted || report.Notes != "Q4 numbers" || report.Due != "2026-10-09" || report.Completed || report.Parent != "" ||
		report.Position == "" || report.Updated.IsZero() || report.WebViewLink == "" ||
		len(report.Links) != 1 || report.Links[0].Type != "email" || report.Links[0].Link != "https://mail.google.com/mail/#all/1234" {
		t.Errorf("report = %+v", report)
	}
	if charts.Parent != parent || charts.Due != "" {
		t.Errorf("charts = %+v", charts)
	}
	if !done.Completed || done.CompletedAt.IsZero() {
		t.Errorf("done = %+v", done)
	}
	// Raw is the task's JSON as sent.
	var raw gtaskstest.Task
	if err := json.Unmarshal(report.Raw, &raw); err != nil || raw.ID != report.ID || raw.Title != "Report" || raw.Kind != "tasks#task" {
		t.Errorf("raw = %s, %v", report.Raw, err)
	}

	since, err := c.Tasks(ctx, work, mark)
	if err != nil || !slices.Equal(titles(since), []string{"Gone", "Report", "Charts"}) {
		t.Errorf("tasks since the mark = %q, %v", titles(since), err)
	}
	var query string
	for _, r := range srv.Requests() {
		if strings.HasSuffix(r.Path, "/lists/"+work+"/tasks") && strings.Contains(r.Query, "updatedMin") {
			query = r.Query
		}
	}
	for _, want := range []string{"showCompleted=true", "showHidden=true", "showDeleted=true", "updatedMin=2026-10-01T00%3A00%3A00"} {
		if !strings.Contains(query, want) {
			t.Errorf("query %q lacks %q", query, want)
		}
	}
}

func TestWrite(t *testing.T) {
	srv := gtaskstest.New(t, gtaskstest.Options{})
	ctx := t.Context()
	c := client(t, srv, srv.Base)
	list := srv.AddList("Tasks")
	first := srv.AddTask(list, gtaskstest.Task{Title: "First"})

	made, err := c.Insert(ctx, list, gtasks.Fields{Title: ptr("Call Ann"), Notes: ptr("About the offsite"), Due: ptr("2026-10-12")}, "", "")
	if err != nil || made.ID == "" || made.Title != "Call Ann" || made.Due != "2026-10-12" || len(made.Raw) == 0 {
		t.Fatalf("insert = %+v, %v", made, err)
	}
	if got, _ := srv.Task(list, made.ID); got.Due != "2026-10-12T00:00:00.000Z" || got.Notes != "About the offsite" {
		t.Errorf("stored = %+v", got)
	}
	sub, err := c.Insert(ctx, list, gtasks.Fields{Title: ptr("Book a room")}, made.ID, "")
	if err != nil || sub.Parent != made.ID {
		t.Fatalf("subtask = %+v, %v", sub, err)
	}

	doneTask, err := c.Patch(ctx, list, made.ID, gtasks.Fields{Completed: ptr(true)})
	if err != nil || !doneTask.Completed || doneTask.CompletedAt.IsZero() || doneTask.Title != "Call Ann" {
		t.Errorf("completed = %+v, %v", doneTask, err)
	}
	cleared, err := c.Patch(ctx, list, made.ID, gtasks.Fields{Completed: ptr(false), Due: ptr(""), Title: ptr("Call Ann back")})
	if err != nil || cleared.Completed || cleared.Due != "" || cleared.Title != "Call Ann back" || cleared.Notes != "About the offsite" {
		t.Errorf("reopened = %+v, %v", cleared, err)
	}

	moved, err := c.Move(ctx, list, first, "", made.ID)
	if err != nil || moved.Position <= cleared.Position {
		t.Errorf("moved after = %+v (made %s), %v", moved, cleared.Position, err)
	}
	if err := c.Delete(ctx, list, made.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := srv.Task(list, made.ID); !got.Deleted {
		t.Errorf("not deleted: %+v", got)
	}
	if _, err := c.Patch(ctx, list, made.ID, gtasks.Fields{Title: ptr("x")}); !errors.Is(err, gtasks.ErrNotFound) {
		t.Errorf("patching a deleted task: %v", err)
	}
}

func TestErrors(t *testing.T) {
	srv := gtaskstest.New(t, gtaskstest.Options{})
	ctx := t.Context()
	bad := gtasks.New(srv.Base, gtasks.Options{HTTP: srv.Client,
		Token: func(context.Context) (string, error) { return "wrong", nil }})
	if _, err := bad.Lists(ctx); !errors.Is(err, gtasks.ErrUnauthorized) {
		t.Errorf("a refused token: %v", err)
	}
	failing := gtasks.New(srv.Base, gtasks.Options{HTTP: srv.Client,
		Token: func(context.Context) (string, error) { return "", errors.New("no token") }})
	if _, err := failing.Lists(ctx); err == nil || !strings.Contains(err.Error(), "no token") {
		t.Errorf("a failing token source: %v", err)
	}
	c := client(t, srv, srv.Base)
	if _, err := c.Tasks(ctx, "nope", time.Time{}); !errors.Is(err, gtasks.ErrNotFound) {
		t.Errorf("an unknown list: %v", err)
	}
	srv.FailWith(func(*http.Request) int { return http.StatusServiceUnavailable })
	var apiErr *gtasks.APIError
	if _, err := c.Lists(ctx); !errors.As(err, &apiErr) || apiErr.Status != 503 || apiErr.Message != "Service Unavailable" {
		t.Errorf("a 503: %v", err)
	}
}
