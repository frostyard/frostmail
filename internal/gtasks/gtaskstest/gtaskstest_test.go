package gtaskstest_test

import (
	"encoding/json/v2"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/gtasks/gtaskstest"
)

type call struct {
	t   *testing.T
	srv *gtaskstest.Server
}

// do sends a request and decodes the JSON answer into out (when not nil).
func (c call) do(method, path, body string, out any) int {
	c.t.Helper()
	req, err := http.NewRequestWithContext(c.t.Context(), method, c.srv.Base+path, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+gtaskstest.Token)
	resp, err := c.srv.Client.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: %v in %s", method, path, err, data)
		}
	}
	return resp.StatusCode
}

// all follows nextPageToken through a listing.
func (c call) all(path string) []gtaskstest.Task {
	c.t.Helper()
	var out []gtaskstest.Task
	token := ""
	for {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		var p page
		c.do("GET", path+sep+"pageToken="+token, "", &p)
		out = append(out, p.Items...)
		if p.NextPageToken == "" {
			return out
		}
		token = p.NextPageToken
	}
}

type page struct {
	Items         []gtaskstest.Task `json:"items"`
	NextPageToken string            `json:"nextPageToken"`
}

func titles(ts []gtaskstest.Task) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.Title)
	}
	return out
}

func TestServer(t *testing.T) {
	srv := gtaskstest.New(t, gtaskstest.Options{PageSize: 2})
	c := call{t, srv}
	list := srv.AddList("My Tasks")
	srv.AddTask(list, gtaskstest.Task{Title: "Older"})
	newer := srv.AddTask(list, gtaskstest.Task{Title: "Newer", Due: "2026-10-09T00:00:00.000Z"})
	srv.AddTask(list, gtaskstest.Task{Title: "Sub", Parent: newer})

	// Lists, then tasks in pages: parents by position (the newest on top),
	// each followed by its subtasks.
	var lists struct {
		Items []gtaskstest.List `json:"items"`
	}
	if code := c.do("GET", "/users/@me/lists", "", &lists); code != 200 || len(lists.Items) != 1 || lists.Items[0].Title != "My Tasks" {
		t.Fatalf("lists = %d %+v", code, lists)
	}
	var first, second page
	c.do("GET", "/lists/"+list+"/tasks", "", &first)
	c.do("GET", "/lists/"+list+"/tasks?pageToken="+first.NextPageToken, "", &second)
	if got := titles(append(first.Items, second.Items...)); !slices.Equal(got, []string{"Newer", "Sub", "Older"}) || second.NextPageToken != "" {
		t.Fatalf("tasks = %q (next %q)", got, second.NextPageToken)
	}
	if sub := first.Items[1]; sub.Parent != newer || sub.Status != "needsAction" || sub.Kind != "tasks#task" {
		t.Errorf("subtask = %+v", sub)
	}

	// Insert at the top, patch to completed, delete with subtasks.
	var made gtaskstest.Task
	if code := c.do("POST", "/lists/"+list+"/tasks", `{"title":"Top","notes":"n"}`, &made); code != 200 || made.ID == "" || made.Notes != "n" {
		t.Fatalf("insert = %d %+v", code, made)
	}
	mark := srv.Now()
	var done gtaskstest.Task
	c.do("PATCH", "/lists/"+list+"/tasks/"+made.ID, `{"status":"completed"}`, &done)
	if done.Status != "completed" || done.Completed == "" || done.Updated <= made.Updated {
		t.Errorf("completed = %+v", done)
	}
	var reopened gtaskstest.Task
	c.do("PATCH", "/lists/"+list+"/tasks/"+made.ID, `{"status":"needsAction","due":null}`, &reopened)
	if reopened.Status != "needsAction" || reopened.Completed != "" {
		t.Errorf("reopened = %+v", reopened)
	}
	if code := c.do("DELETE", "/lists/"+list+"/tasks/"+newer, "", nil); code != 204 {
		t.Errorf("delete = %d", code)
	}

	// updatedMin and showDeleted bring changes since a mark.
	var changed []string
	for _, t := range c.all("/lists/" + list + "/tasks?showDeleted=true&updatedMin=" + url.QueryEscape(mark.Format(time.RFC3339Nano))) {
		changed = append(changed, t.Title+map[bool]string{true: " (deleted)"}[t.Deleted])
	}
	if !slices.Equal(changed, []string{"Top", "Newer (deleted)", "Sub (deleted)"}) {
		t.Errorf("changed since the mark = %q", changed)
	}
	visible := c.all("/lists/" + list + "/tasks")
	if got := titles(visible); !slices.Equal(got, []string{"Top", "Older"}) {
		t.Fatalf("visible = %q", got)
	}

	// Move under another task; another client's change; errors.
	var moved gtaskstest.Task
	older := visible[1].ID
	c.do("POST", "/lists/"+list+"/tasks/"+made.ID+"/move?parent="+older, "", &moved)
	if moved.Parent != older {
		t.Errorf("moved = %+v", moved)
	}
	srv.Update(list, older, func(t *gtaskstest.Task) { t.Title = "Renamed" })
	if got, _ := srv.Task(list, older); got.Title != "Renamed" {
		t.Errorf("updated = %+v", got)
	}
	if code := c.do("GET", "/lists/nope/tasks", "", nil); code != 404 {
		t.Errorf("unknown list = %d", code)
	}
	req, _ := http.NewRequestWithContext(t.Context(), "GET", srv.Base+"/users/@me/lists", nil)
	if resp, err := srv.Client.Do(req); err != nil || resp.StatusCode != 401 {
		t.Errorf("without a token = %v %v", resp, err)
	} else {
		_ = resp.Body.Close()
	}
	srv.FailWith(func(*http.Request) int { return 503 })
	if code := c.do("GET", "/users/@me/lists", "", nil); code != 503 {
		t.Errorf("failing = %d", code)
	}
}
