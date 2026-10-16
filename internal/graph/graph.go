// Package graph is Frostmail's client for the part of Microsoft Graph that
// Microsoft accounts' contacts and calendars need (ADR-0039, ADR-0040;
// docs/design/pim.md, Microsoft Graph): listing calendars and contact
// folders, delta rounds, reading events, series and contacts, and turning
// them into the iCalendar and vCard sources the rest of maild reads.
package graph

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
	"strconv"
	"strings"
	"time"
)

// BaseURL is Microsoft Graph v1.0.
const BaseURL = "https://graph.microsoft.com/v1.0/"

// Errors a request ends with, besides StatusError.
var (
	// ErrUnauthorized is a 401: the access token was refused.
	ErrUnauthorized = errors.New("graph: unauthorized")
	// ErrNotFound is a 404.
	ErrNotFound = errors.New("graph: not found")
	// ErrPrecondition is a 412: the If-Match change key is stale.
	ErrPrecondition = errors.New("graph: the item changed on the server")
	// ErrDeltaExpired means Graph no longer knows a delta link (410, or
	// syncStateNotFound): the collection is listed again.
	ErrDeltaExpired = errors.New("graph: the delta link expired")
)

// StatusError is a request Graph refused for another reason.
type StatusError struct {
	Status  int
	Code    string // Graph's error code
	Message string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("graph: HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

// Client talks to Graph for one account.
type Client struct {
	// Base is Graph's URL, BaseURL unless a test server stands in.
	Base string
	HTTP *http.Client
	// Token gives the account's access token for Graph's audience.
	Token     func(ctx context.Context) (string, error)
	UserAgent string
	// Wait sleeps before a retry; nil waits for real.
	Wait func(ctx context.Context, d time.Duration) error
}

// maxRetries bounds the retries of a throttled or failing request, and
// maxWait one wait.
const (
	maxRetries = 3
	maxWait    = time.Minute
)

// prefer is every request's Prefer header: immutable IDs, times in UTC,
// pages of at most 100 items.
const prefer = `IdType="ImmutableId", outlook.timezone="UTC", odata.maxpagesize=100`

func (c *Client) url(path string) string {
	if strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://") {
		return path // a next or delta link
	}
	return strings.TrimSuffix(c.Base, "/") + "/" + strings.TrimPrefix(path, "/")
}

// Do sends one request and returns the response's body, retrying throttled
// and failing requests after Graph's Retry-After. headers may carry
// If-Match and Content-Type (JSON's when body is set).
func (c *Client) Do(ctx context.Context, method, path string, body []byte, headers map[string]string) ([]byte, http.Header, error) {
	for attempt := 0; ; attempt++ {
		tok, err := c.Token(ctx)
		if err != nil {
			return nil, nil, err
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.url(path), rd)
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Prefer", prefer)
		if c.UserAgent != "" {
			req.Header.Set("User-Agent", c.UserAgent)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, nil, fmt.Errorf("graph: %s %s: %w", method, path, err)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		_ = resp.Body.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("graph: %s %s: %w", method, path, err)
		}
		if resp.StatusCode < 300 {
			return data, resp.Header, nil
		}
		// A POST that failed may have been done: only a throttled one,
		// which Graph did not start, is sent again.
		retry := resp.StatusCode == http.StatusTooManyRequests || method != http.MethodPost &&
			(resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusGatewayTimeout)
		if retry && attempt < maxRetries {
			if err := c.wait(ctx, retryAfter(resp.Header, attempt)); err != nil {
				return nil, nil, err
			}
			continue
		}
		return nil, resp.Header, statusError(resp.StatusCode, data)
	}
}

func (c *Client) wait(ctx context.Context, d time.Duration) error {
	if c.Wait != nil {
		return c.Wait(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// retryAfter is Graph's Retry-After in seconds, else 2^attempt seconds.
func retryAfter(h http.Header, attempt int) time.Duration {
	if s, err := strconv.Atoi(strings.TrimSpace(h.Get("Retry-After"))); err == nil && s >= 0 {
		return min(time.Duration(s)*time.Second, maxWait)
	}
	return time.Duration(1<<attempt) * time.Second
}

func statusError(status int, body []byte) error {
	var reply struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &reply)
	code := reply.Error.Code
	switch {
	case status == http.StatusUnauthorized:
		return fmt.Errorf("%w: %s", ErrUnauthorized, code)
	case status == http.StatusNotFound || code == "ErrorItemNotFound":
		return fmt.Errorf("%w: %s", ErrNotFound, code)
	case status == http.StatusPreconditionFailed || code == "ErrorIrresolvableConflict":
		return fmt.Errorf("%w: %s", ErrPrecondition, reply.Error.Message)
	case status == http.StatusGone || strings.EqualFold(code, "syncStateNotFound") || strings.EqualFold(code, "resyncRequired"):
		return fmt.Errorf("%w: %s", ErrDeltaExpired, code)
	}
	return &StatusError{Status: status, Code: code, Message: reply.Error.Message}
}

// getJSON reads one path into out.
func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	data, _, err := c.Do(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("graph: GET %s: %w", path, err)
	}
	return nil
}

// page is one page of a collection or a delta round.
type page struct {
	Value     []jsontext.Value `json:"value"`
	NextLink  string           `json:"@odata.nextLink"`
	DeltaLink string           `json:"@odata.deltaLink"`
}

// list reads every page of a collection.
func (c *Client) list(ctx context.Context, path string) ([]jsontext.Value, error) {
	var out []jsontext.Value
	for i := 0; path != ""; i++ {
		if i == 10000 {
			return nil, errors.New("graph: a listing did not end")
		}
		var p page
		if err := c.getJSON(ctx, path, &p); err != nil {
			return nil, err
		}
		out = append(out, p.Value...)
		path = p.NextLink
	}
	return out, nil
}

// Delta runs a delta round from link, a delta link or a first request,
// to its end and returns the items and the delta link of the next round.
func (c *Client) Delta(ctx context.Context, link string) ([]jsontext.Value, string, error) {
	var out []jsontext.Value
	for i := 0; ; i++ {
		if i == 10000 {
			return nil, "", errors.New("graph: a delta round did not end")
		}
		var p page
		if err := c.getJSON(ctx, link, &p); err != nil {
			return nil, "", err
		}
		out = append(out, p.Value...)
		switch {
		case p.NextLink != "":
			link = p.NextLink
		case p.DeltaLink != "":
			return out, p.DeltaLink, nil
		default:
			return nil, "", errors.New("graph: a delta page has neither a next nor a delta link")
		}
	}
}

// Calendars lists the account's calendars.
func (c *Client) Calendars(ctx context.Context) ([]Calendar, error) {
	raw, err := c.list(ctx, "me/calendars?$top=100")
	if err != nil {
		return nil, err
	}
	return decodeAll[Calendar](raw)
}

// ContactFolders lists the account's contact folders besides the default.
func (c *Client) ContactFolders(ctx context.Context) ([]ContactFolder, error) {
	raw, err := c.list(ctx, "me/contactFolders?$top=100")
	if err != nil {
		return nil, err
	}
	return decodeAll[ContactFolder](raw)
}

// CalendarDelta is the first request of a calendar's delta round over a
// window.
func CalendarDelta(calendarID string, from, to time.Time) string {
	q := url.Values{"startDateTime": {from.UTC().Format(time.RFC3339)}, "endDateTime": {to.UTC().Format(time.RFC3339)}}
	return "me/calendars/" + url.PathEscape(calendarID) + "/calendarView/delta?" + q.Encode()
}

// ContactsDelta is the first request of a contact folder's delta round;
// DefaultFolder is the default folder's href.
func ContactsDelta(folder string) string {
	if folder == DefaultFolder {
		return "me/contacts/delta"
	}
	return "me/contactFolders/" + url.PathEscape(folder) + "/contacts/delta"
}

// DefaultFolder is the default contact folder's href.
const DefaultFolder = "contacts"

// CalendarView lists the events of a calendar that have occurrences in a
// window, as occurrences: single events, and a series' occurrences and
// exceptions with seriesMasterId, each with its ID, type and ETag.
func (c *Client) CalendarView(ctx context.Context, calendarID string, from, to time.Time) ([]Event, error) {
	q := url.Values{"startDateTime": {from.UTC().Format(time.RFC3339)}, "endDateTime": {to.UTC().Format(time.RFC3339)},
		"$select": {"id,type,seriesMasterId,changeKey"}, "$top": {"100"}}
	raw, err := c.list(ctx, "me/calendars/"+url.PathEscape(calendarID)+"/calendarView?"+q.Encode())
	if err != nil {
		return nil, err
	}
	return decodeAll[Event](raw)
}

// Contacts lists a contact folder's contacts, each with its ID and ETag.
func (c *Client) Contacts(ctx context.Context, folder string) ([]Contact, error) {
	path := "me/contacts"
	if folder != DefaultFolder {
		path = "me/contactFolders/" + url.PathEscape(folder) + "/contacts"
	}
	raw, err := c.list(ctx, path+"?$select=id,changeKey&$top=100")
	if err != nil {
		return nil, err
	}
	return decodeAll[Contact](raw)
}

// Event reads one event.
func (c *Client) Event(ctx context.Context, id string) (jsontext.Value, error) {
	var raw jsontext.Value
	err := c.getJSON(ctx, "me/events/"+url.PathEscape(id), &raw)
	return raw, err
}

// Series reads a series' master with its exceptions.
func (c *Client) Series(ctx context.Context, id string) (jsontext.Value, error) {
	var raw jsontext.Value
	err := c.getJSON(ctx, "me/events/"+url.PathEscape(id)+"?$expand=exceptionOccurrences", &raw)
	return raw, err
}

// Contact reads one contact.
func (c *Client) Contact(ctx context.Context, id string) (jsontext.Value, error) {
	var raw jsontext.Value
	err := c.getJSON(ctx, "me/contacts/"+url.PathEscape(id), &raw)
	return raw, err
}

// Photo reads a contact's photo; nil when it has none.
func (c *Client) Photo(ctx context.Context, contactID string) ([]byte, error) {
	data, _, err := c.Do(ctx, http.MethodGet, "me/contacts/"+url.PathEscape(contactID)+"/photo/$value", nil,
		map[string]string{"Accept": "image/*"})
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return data, err
}

func decodeAll[T any](raw []jsontext.Value) ([]T, error) {
	out := make([]T, 0, len(raw))
	for _, r := range raw {
		var v T
		if err := json.Unmarshal(r, &v); err != nil {
			return nil, fmt.Errorf("graph: %w", err)
		}
		out = append(out, v)
	}
	return out, nil
}
