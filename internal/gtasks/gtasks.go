// Package gtasks is a client of the Google Tasks API (v1) for pimsync
// (docs/design/pim.md, Tasks): task lists, tasks with their JSON kept as
// sent, and the writes Frostmail makes. It is tested against gtaskstest.
// Task T-0078 writes the client.
package gtasks

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// errNotYet marks what task card T-0078 has yet to write.
var errNotYet = errors.New("gtasks: not implemented yet")

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
	ID      string
	Title   string
	Updated time.Time
}

// Link is a task's link; a task made from a Gmail message links to it.
type Link struct {
	Type        string
	Description string
	Link        string
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
type Client struct{}

// New returns a Client.
func New(base string, opts Options) *Client { return &Client{} }

// Lists returns every task list, following pages.
func (c *Client) Lists(ctx context.Context) ([]List, error) { return nil, errNotYet }

// Tasks returns a list's tasks updated at or after updatedMin (every task
// when it is zero), completed, hidden and deleted ones included, following
// pages, in the API's order.
func (c *Client) Tasks(ctx context.Context, list string, updatedMin time.Time) ([]Task, error) {
	return nil, errNotYet
}

// Insert adds a task under parent ("" for the top level) after previous
// ("" for the first place).
func (c *Client) Insert(ctx context.Context, list string, f Fields, parent, previous string) (Task, error) {
	return Task{}, errNotYet
}

// Patch changes the fields f sets.
func (c *Client) Patch(ctx context.Context, list, id string, f Fields) (Task, error) {
	return Task{}, errNotYet
}

// Delete deletes a task (and, on Google's side, its subtasks).
func (c *Client) Delete(ctx context.Context, list, id string) error { return errNotYet }

// Move puts a task under parent after previous.
func (c *Client) Move(ctx context.Context, list, id, parent, previous string) (Task, error) {
	return Task{}, errNotYet
}
