// Package gtasks is a client of the Google Tasks API (v1) for pimsync
// (docs/design/pim.md, Tasks): task lists, tasks with their JSON kept as
// sent, and the writes Frostmail makes. It is tested against gtaskstest.
package gtasks

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Errors the API answers with; other failures are *APIError.
var (
	// ErrUnauthorized is a 401: the access token was refused.
	ErrUnauthorized = errors.New("gtasks: unauthorized")
	// ErrNotFound is a 404: the list or task is gone.
	ErrNotFound = errors.New("gtasks: not found")
)

// APIError is any other answer that is not 2xx.
type APIError struct {
	Status  int
	Message string // the error's message from Google's JSON, when there is one
}

func (e *APIError) Error() string { return "gtasks: " + http.StatusText(e.Status) + ": " + e.Message }

// Options configure a Client.
type Options struct {
	// HTTP sends requests; nil uses a client with a 30-second timeout.
	HTTP *http.Client
	// Token gives the bearer token for each request.
	Token func(ctx context.Context) (string, error)
	// UserAgent is sent with every request when set.
	UserAgent string
}

// List is a task list.
type List struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Updated time.Time `json:"updated"`
}

// Link is a task's link; a task made from a Gmail message links to it.
type Link struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Link        string `json:"link"`
}

// Task is a task as the API sent it.
type Task struct {
	ID    string
	Title string
	Notes string
	// Completed is set when the status is "completed".
	Completed   bool
	CompletedAt time.Time // zero unless completed
	// Due is the due date, YYYY-MM-DD, or "": Google keeps only the date.
	Due         string
	Parent      string
	Position    string
	Updated     time.Time
	Deleted     bool
	Hidden      bool
	Links       []Link
	WebViewLink string
	// Raw is the task's JSON as the API sent it.
	Raw []byte
}

// Fields are what Insert and Patch send; nil fields are left out, so a
// Patch keeps them.
type Fields struct {
	Title *string
	Notes *string
	// Due is YYYY-MM-DD, or "" to clear it.
	Due       *string
	Completed *bool
}

// Client speaks to the API at a base URL (…/tasks/v1, with or without a
// trailing slash).
type Client struct {
	base string
	opts Options
}

// New returns a Client.
func New(base string, opts Options) *Client {
	if opts.HTTP == nil {
		opts.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{base: strings.TrimRight(base, "/"), opts: opts}
}

// Lists returns every task list, following pages.
func (c *Client) Lists(ctx context.Context) ([]List, error) {
	q := url.Values{"maxResults": {"100"}}
	var lists []List
	for {
		var page struct {
			Items []List `json:"items"`
			Next  string `json:"nextPageToken"`
		}
		if err := c.readPage(ctx, "/users/@me/lists", q, &page); err != nil {
			return nil, fmt.Errorf("list task lists: %w", err)
		}
		lists = append(lists, page.Items...)
		if page.Next == "" {
			return lists, nil
		}
		q.Set("pageToken", page.Next)
	}
}

// Tasks returns a list's tasks updated at or after updatedMin (every task
// when it is zero), completed, hidden and deleted ones included, following
// pages, in the API's order.
func (c *Client) Tasks(ctx context.Context, list string, updatedMin time.Time) ([]Task, error) {
	q := url.Values{"maxResults": {"100"}, "showCompleted": {"true"},
		"showHidden": {"true"}, "showDeleted": {"true"}}
	if !updatedMin.IsZero() {
		q.Set("updatedMin", updatedMin.UTC().Format(time.RFC3339Nano))
	}
	var tasks []Task
	for {
		var page struct {
			Items []jsontext.Value `json:"items"`
			Next  string           `json:"nextPageToken"`
		}
		if err := c.readPage(ctx, tasksPath(list), q, &page); err != nil {
			return nil, fmt.Errorf("list tasks: %w", err)
		}
		for _, raw := range page.Items {
			task, err := decodeTask(raw)
			if err != nil {
				return nil, err
			}
			tasks = append(tasks, task)
		}
		if page.Next == "" {
			return tasks, nil
		}
		q.Set("pageToken", page.Next)
	}
}

// Insert adds a task under parent ("" for the top level) after previous
// ("" for the first place).
func (c *Client) Insert(ctx context.Context, list string, f Fields, parent, previous string) (Task, error) {
	return c.writeTask(ctx, http.MethodPost, tasksPath(list), placement(parent, previous), fieldsJSON(f))
}

// Patch changes the fields f sets.
func (c *Client) Patch(ctx context.Context, list, id string, f Fields) (Task, error) {
	return c.writeTask(ctx, http.MethodPatch, tasksPath(list)+"/"+url.PathEscape(id), nil, fieldsJSON(f))
}

// Delete deletes a task (and, on Google's side, its subtasks).
func (c *Client) Delete(ctx context.Context, list, id string) error {
	_, status, err := c.request(ctx, http.MethodDelete, tasksPath(list)+"/"+url.PathEscape(id), nil, nil)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("delete task: expected 204, got %d", status)
	}
	return nil
}

// Move puts a task under parent after previous.
func (c *Client) Move(ctx context.Context, list, id, parent, previous string) (Task, error) {
	return c.writeTask(ctx, http.MethodPost, tasksPath(list)+"/"+url.PathEscape(id)+"/move", placement(parent, previous), nil)
}

func tasksPath(list string) string { return "/lists/" + url.PathEscape(list) + "/tasks" }

func placement(parent, previous string) url.Values {
	q := url.Values{}
	if parent != "" {
		q.Set("parent", parent)
	}
	if previous != "" {
		q.Set("previous", previous)
	}
	return q
}

func fieldsJSON(f Fields) map[string]any {
	out := map[string]any{}
	if f.Title != nil {
		out["title"] = *f.Title
	}
	if f.Notes != nil {
		out["notes"] = *f.Notes
	}
	if f.Due != nil {
		out["due"] = nil
		if *f.Due != "" {
			out["due"] = *f.Due + "T00:00:00.000Z"
		}
	}
	if f.Completed != nil {
		out["status"] = "completed"
		if !*f.Completed {
			out["status"] = "needsAction"
			out["completed"] = nil
		}
	}
	return out
}

func decodeTask(raw []byte) (Task, error) {
	var wire struct {
		ID          string    `json:"id"`
		Title       string    `json:"title"`
		Notes       string    `json:"notes"`
		Status      string    `json:"status"`
		Completed   time.Time `json:"completed"`
		Due         string    `json:"due"`
		Parent      string    `json:"parent"`
		Position    string    `json:"position"`
		Updated     time.Time `json:"updated"`
		Deleted     bool      `json:"deleted"`
		Hidden      bool      `json:"hidden"`
		Links       []Link    `json:"links"`
		WebViewLink string    `json:"webViewLink"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return Task{}, fmt.Errorf("decode task: %w", err)
	}
	if len(wire.Due) >= 10 {
		wire.Due = wire.Due[:10]
	}
	return Task{ID: wire.ID, Title: wire.Title, Notes: wire.Notes,
		Completed: wire.Status == "completed", CompletedAt: wire.Completed,
		Due: wire.Due, Parent: wire.Parent, Position: wire.Position,
		Updated: wire.Updated, Deleted: wire.Deleted, Hidden: wire.Hidden,
		Links: wire.Links, WebViewLink: wire.WebViewLink, Raw: raw}, nil
}

func (c *Client) readPage(ctx context.Context, path string, q url.Values, page any) error {
	raw, _, err := c.request(ctx, http.MethodGet, path, q, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, page); err != nil {
		return fmt.Errorf("decode page: %w", err)
	}
	return nil
}

func (c *Client) writeTask(ctx context.Context, method, path string, q url.Values, fields map[string]any) (Task, error) {
	var body []byte
	if fields != nil {
		var err error
		body, err = json.Marshal(fields)
		if err != nil {
			return Task{}, fmt.Errorf("encode task fields: %w", err)
		}
	}
	raw, _, err := c.request(ctx, method, path, q, body)
	if err != nil {
		return Task{}, fmt.Errorf("write task: %w", err)
	}
	return decodeTask(raw)
}

func (c *Client) request(ctx context.Context, method, path string, q url.Values, body []byte) ([]byte, int, error) {
	if c.opts.Token == nil {
		return nil, 0, errors.New("get tasks token: no token source")
	}
	token, err := c.opts.Token(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("get tasks token: %w", err)
	}
	endpoint := c.base + path
	if len(q) > 0 {
		endpoint += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("create tasks request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if c.opts.UserAgent != "" {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.opts.HTTP.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("send tasks request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, responseError(resp)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read tasks response: %w", err)
	}
	return raw, resp.StatusCode, nil
}

func responseError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusNotFound:
		return ErrNotFound
	}
	var answer struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	// The status remains useful even when the error body is not Google's JSON.
	_ = json.UnmarshalRead(resp.Body, &answer)
	return &APIError{Status: resp.StatusCode, Message: answer.Error.Message}
}
