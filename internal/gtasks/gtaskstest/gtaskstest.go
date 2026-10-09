// Package gtaskstest is an in-process Google Tasks API (v1) for tests, as
// davtest is for DAV (docs/design/pim.md, Testing): task lists and tasks in
// memory with Google's JSON, paging, updatedMin, hidden and deleted tasks,
// positions among siblings, a bearer token, and hooks to change tasks as
// another client would and to make requests fail.
package gtaskstest

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Token is the bearer token the server accepts unless Options sets one.
const Token = "gtasks-test-token"

// Options tune the server.
type Options struct {
	// Token is the accepted bearer token; "" means Token.
	Token string
	// PageSize caps maxResults, so tests see paging; 0 means 100.
	PageSize int
}

// Link is a task's link (a Gmail message for a task made from mail).
type Link struct {
	Type        string `json:"type,omitzero"`
	Description string `json:"description,omitzero"`
	Link        string `json:"link,omitzero"`
}

// Task is a task as the API writes it.
type Task struct {
	Kind        string `json:"kind"`
	ID          string `json:"id"`
	ETag        string `json:"etag"`
	Title       string `json:"title"`
	Updated     string `json:"updated"`
	SelfLink    string `json:"selfLink"`
	Parent      string `json:"parent,omitzero"`
	Position    string `json:"position"`
	Notes       string `json:"notes,omitzero"`
	Status      string `json:"status"` // needsAction or completed
	Due         string `json:"due,omitzero"`
	Completed   string `json:"completed,omitzero"`
	Deleted     bool   `json:"deleted,omitzero"`
	Hidden      bool   `json:"hidden,omitzero"`
	Links       []Link `json:"links,omitzero"`
	WebViewLink string `json:"webViewLink,omitzero"`
}

// List is a task list as the API writes it.
type List struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	ETag     string `json:"etag"`
	Title    string `json:"title"`
	Updated  string `json:"updated"`
	SelfLink string `json:"selfLink"`
}

// Request is one request the server received.
type Request struct {
	Method string
	Path   string
	Query  string
}

// Server is a running Tasks API. Base is the API root to give a client
// (…/tasks/v1).
type Server struct {
	Base   string
	Client *http.Client

	opts Options
	ts   *httptest.Server

	mu    sync.Mutex
	clock time.Time
	next  int
	lists []*List
	tasks map[string][]*Task // by list ID, in no order
	pos   map[string]int64   // by task ID
	reqs  []Request
	fail  func(*http.Request) int
}

// New starts a server, stopped when the test ends.
func New(t testing.TB, opts Options) *Server {
	if opts.Token == "" {
		opts.Token = Token
	}
	if opts.PageSize == 0 {
		opts.PageSize = 100
	}
	s := &Server{opts: opts, clock: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		tasks: map[string][]*Task{}, pos: map[string]int64{}}
	s.ts = httptest.NewTLSServer(s)
	s.Base, s.Client = s.ts.URL+"/tasks/v1", s.ts.Client()
	t.Cleanup(s.ts.Close)
	return s
}

// tick advances the server's clock: every change is a millisecond later.
func (s *Server) tick() string {
	s.clock = s.clock.Add(time.Millisecond)
	return s.clock.Format("2006-01-02T15:04:05.000Z")
}

func (s *Server) id(prefix string) string {
	s.next++
	return fmt.Sprintf("%s%04d", prefix, s.next)
}

// Now is the server clock's last change time, for updatedMin.
func (s *Server) Now() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clock
}

// AddList adds a task list and returns its ID.
func (s *Server) AddList(title string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := &List{Kind: "tasks#taskList", ID: s.id("L"), Title: title, Updated: s.tick()}
	l.ETag = etag(l.Updated)
	l.SelfLink = s.Base + "/users/@me/lists/" + l.ID
	s.lists = append(s.lists, l)
	return l.ID
}

// AddTask adds a task as another client would, at the top of its parent's
// subtasks (or of the list), and returns its ID. Title, Notes, Status,
// Due, Parent and Links are taken from t.
func (s *Server) AddTask(list string, t Task) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.insert(list, t, t.Parent, "").ID
}

// Update changes a task as another client would.
func (s *Server) Update(list, id string, f func(*Task)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.find(list, id); t != nil {
		f(t)
		s.touch(t)
	}
}

// Remove deletes a task as another client would: it stays, deleted.
func (s *Server) Remove(list, id string) {
	s.Update(list, id, func(t *Task) { t.Deleted = true })
}

// RemoveList deletes a task list and its tasks.
func (s *Server) RemoveList(list string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lists = slices.DeleteFunc(s.lists, func(l *List) bool { return l.ID == list })
	delete(s.tasks, list)
}

// Task returns a copy of a task.
func (s *Server) Task(list, id string) (Task, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.find(list, id); t != nil {
		return *t, true
	}
	return Task{}, false
}

// Tasks returns copies of a list's tasks, deleted ones included, by
// parent and position.
func (s *Server) Tasks(list string) []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Task
	for _, t := range s.ordered(list) {
		out = append(out, *t)
	}
	return out
}

// Requests returns the requests received since the last ResetRequests.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.reqs)
}

// ResetRequests forgets the requests received.
func (s *Server) ResetRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = nil
}

// FailWith makes the server answer requests with the status f returns; 0
// serves them. nil stops failing.
func (s *Server) FailWith(f func(*http.Request) int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = f
}

func etag(updated string) string { return `"` + updated + `"` }

func (s *Server) find(list, id string) *Task {
	for _, t := range s.tasks[list] {
		if t.ID == id {
			return t
		}
	}
	return nil
}

func (s *Server) touch(t *Task) {
	t.Updated = s.tick()
	t.ETag = etag(t.Updated)
}

// ordered is a list's tasks: parents by position, each followed by its
// subtasks by position.
func (s *Server) ordered(list string) []*Task {
	byPos := func(ts []*Task) []*Task {
		ts = slices.Clone(ts)
		slices.SortFunc(ts, func(a, b *Task) int { return strings.Compare(a.Position, b.Position) })
		return ts
	}
	var out []*Task
	for _, p := range byPos(s.children(list, "")) {
		out = append(out, p)
		out = append(out, byPos(s.children(list, p.ID))...)
	}
	return out
}

func (s *Server) children(list, parent string) []*Task {
	var out []*Task
	for _, t := range s.tasks[list] {
		if t.Parent == parent {
			out = append(out, t)
		}
	}
	return out
}

// place puts a task among its parent's subtasks after previous ("" for the
// top), giving it a position between its neighbors.
func (s *Server) place(list string, t *Task, parent, previous string) {
	siblings := s.children(list, parent)
	siblings = slices.DeleteFunc(siblings, func(o *Task) bool { return o == t })
	slices.SortFunc(siblings, func(a, b *Task) int { return strings.Compare(a.Position, b.Position) })
	at := 0
	for i, o := range siblings {
		if o.ID == previous {
			at = i + 1
		}
	}
	lo, hi := int64(0), int64(1<<40)
	if at > 0 {
		lo = s.pos[siblings[at-1].ID]
	}
	if at < len(siblings) {
		hi = s.pos[siblings[at].ID]
	}
	if hi-lo < 2 { // no room: spread the siblings out again
		for i, o := range siblings {
			s.pos[o.ID] = int64(i+1) << 20
			o.Position = fmt.Sprintf("%020d", s.pos[o.ID])
		}
		s.place(list, t, parent, previous)
		return
	}
	t.Parent = parent
	s.pos[t.ID] = lo + (hi-lo)/2
	t.Position = fmt.Sprintf("%020d", s.pos[t.ID])
}

func (s *Server) insert(list string, in Task, parent, previous string) *Task {
	t := &Task{Kind: "tasks#task", ID: s.id("T"), Title: in.Title, Notes: in.Notes, Status: in.Status, Due: in.Due,
		Links: in.Links}
	if t.Status == "" {
		t.Status = "needsAction"
	}
	if t.Status == "completed" {
		t.Completed = s.clock.Add(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
	}
	t.SelfLink = s.Base + "/lists/" + list + "/tasks/" + t.ID
	t.WebViewLink = "https://tasks.google.com/task/" + t.ID
	s.tasks[list] = append(s.tasks[list], t)
	s.place(list, t, parent, previous)
	s.touch(t)
	return t
}

// ServeHTTP implements the API.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = append(s.reqs, Request{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery})
	if s.fail != nil {
		if code := s.fail(r); code != 0 {
			apiError(w, code, http.StatusText(code))
			return
		}
	}
	if r.Header.Get("Authorization") != "Bearer "+s.opts.Token {
		apiError(w, http.StatusUnauthorized, "Request had invalid authentication credentials.")
		return
	}
	path, ok := strings.CutPrefix(r.URL.Path, "/tasks/v1/")
	if !ok {
		apiError(w, http.StatusNotFound, "Not Found")
		return
	}
	parts := strings.Split(path, "/")
	switch {
	case path == "users/@me/lists" && r.Method == http.MethodGet:
		s.serveLists(w, r)
	case len(parts) == 3 && parts[0] == "lists" && parts[2] == "tasks":
		s.serveTasks(w, r, parts[1])
	case len(parts) == 4 && parts[0] == "lists" && parts[2] == "tasks":
		s.serveTask(w, r, parts[1], parts[3])
	case len(parts) == 5 && parts[0] == "lists" && parts[2] == "tasks" && parts[4] == "move" && r.Method == http.MethodPost:
		s.serveMove(w, r, parts[1], parts[3])
	default:
		apiError(w, http.StatusNotFound, "Not Found")
	}
}

func (s *Server) serveLists(w http.ResponseWriter, r *http.Request) {
	items := make([]any, len(s.lists))
	for i, l := range s.lists {
		items[i] = l
	}
	s.page(w, r, "tasks#taskLists", items)
}

// page writes items in pages of maxResults (capped by PageSize), the page
// token being the next index.
func (s *Server) page(w http.ResponseWriter, r *http.Request, kind string, items []any) {
	size := s.opts.PageSize
	if n, err := strconv.Atoi(r.URL.Query().Get("maxResults")); err == nil && n > 0 && n < size {
		size = n
	}
	start, _ := strconv.Atoi(r.URL.Query().Get("pageToken"))
	if start < 0 || start > len(items) {
		apiError(w, http.StatusBadRequest, "Invalid page token")
		return
	}
	end := min(start+size, len(items))
	out := map[string]any{"kind": kind, "etag": etag(s.clock.Format(time.RFC3339Nano)), "items": items[start:end]}
	if end < len(items) {
		out["nextPageToken"] = strconv.Itoa(end)
	}
	write(w, http.StatusOK, out)
}

func (s *Server) hasList(list string) bool {
	return slices.ContainsFunc(s.lists, func(l *List) bool { return l.ID == list })
}

func (s *Server) serveTasks(w http.ResponseWriter, r *http.Request, list string) {
	if !s.hasList(list) {
		apiError(w, http.StatusNotFound, "Task list not found.")
		return
	}
	q := r.URL.Query()
	switch r.Method {
	case http.MethodGet:
		flag := func(name string, def bool) bool {
			if v := q.Get(name); v != "" {
				return v == "true"
			}
			return def
		}
		completed, hidden, deleted := flag("showCompleted", true), flag("showHidden", false), flag("showDeleted", false)
		var since time.Time
		if v := q.Get("updatedMin"); v != "" {
			var err error
			if since, err = time.Parse(time.RFC3339Nano, v); err != nil {
				apiError(w, http.StatusBadRequest, "Invalid updatedMin")
				return
			}
		}
		var items []any
		for _, t := range s.ordered(list) {
			updated, _ := time.Parse(time.RFC3339Nano, t.Updated)
			if (!completed && t.Status == "completed") || (!hidden && t.Hidden) || (!deleted && t.Deleted) || updated.Before(since) {
				continue
			}
			items = append(items, t)
		}
		s.page(w, r, "tasks#tasks", items)
	case http.MethodPost:
		var in Task
		if err := json.UnmarshalRead(r.Body, &in); err != nil {
			apiError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		parent := q.Get("parent")
		if parent != "" && s.find(list, parent) == nil {
			apiError(w, http.StatusNotFound, "Parent task not found.")
			return
		}
		write(w, http.StatusOK, s.insert(list, in, parent, q.Get("previous")))
	default:
		apiError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) serveTask(w http.ResponseWriter, r *http.Request, list, id string) {
	t := s.find(list, id)
	if t == nil || (t.Deleted && r.Method != http.MethodGet) {
		apiError(w, http.StatusNotFound, "Task not found.")
		return
	}
	switch r.Method {
	case http.MethodGet:
		write(w, http.StatusOK, t)
	case http.MethodPatch:
		var patch map[string]any
		if err := json.UnmarshalRead(r.Body, &patch); err != nil {
			apiError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		s.apply(t, patch)
		s.touch(t)
		write(w, http.StatusOK, t)
	case http.MethodDelete:
		t.Deleted = true
		for _, c := range s.children(list, id) {
			c.Deleted = true
			s.touch(c)
		}
		s.touch(t)
		w.WriteHeader(http.StatusNoContent)
	default:
		apiError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

// apply sets the fields a PATCH names; null clears due and notes.
// Completing sets completed; reopening clears it.
func (s *Server) apply(t *Task, patch map[string]any) {
	str := func(v any) string { s, _ := v.(string); return s }
	for k, v := range patch {
		switch k {
		case "title":
			t.Title = str(v)
		case "notes":
			t.Notes = str(v)
		case "due":
			t.Due = str(v)
		case "status":
			t.Status = str(v)
			if t.Status == "completed" && t.Completed == "" {
				t.Completed = s.clock.Add(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
			}
			if t.Status != "completed" {
				t.Completed = ""
				t.Hidden = false
			}
		}
	}
}

func (s *Server) serveMove(w http.ResponseWriter, r *http.Request, list, id string) {
	t := s.find(list, id)
	if t == nil || t.Deleted {
		apiError(w, http.StatusNotFound, "Task not found.")
		return
	}
	q := r.URL.Query()
	s.place(list, t, q.Get("parent"), q.Get("previous"))
	s.touch(t)
	write(w, http.StatusOK, t)
}

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(code)
	_ = json.MarshalWrite(w, v)
}

func apiError(w http.ResponseWriter, code int, message string) {
	write(w, code, map[string]any{"error": map[string]any{"code": code, "message": message,
		"errors": []any{map[string]any{"message": message, "domain": "global", "reason": strings.ToLower(http.StatusText(code))}}}})
}
