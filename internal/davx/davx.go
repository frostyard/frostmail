// Package davx is Frostmail's WebDAV client for CardDAV (RFC 6352) and
// CalDAV (RFC 4791): discovery, collections, sync-collection (RFC 6578),
// multiget, and conditional writes of raw objects (ADR-0017). Objects are
// bytes with ETags; parsing them is the caller's business (ADR-0018).
//
// A Client talks to one origin, the home set's. Credentials go only there,
// to origins Options.Trusted accepts, and never over plain HTTP unless
// Options.AllowHTTP is set.
package davx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Kind is the kind of collection a client works with.
type Kind int

const (
	// AddressBooks are CardDAV address books.
	AddressBooks Kind = iota + 1
	// Calendars are CalDAV calendars.
	Calendars
)

// ContentType is the media type of the kind's objects.
func (k Kind) ContentType() string {
	if k == Calendars {
		return "text/calendar; charset=utf-8"
	}
	return "text/vcard; charset=utf-8"
}

func (k Kind) String() string {
	if k == Calendars {
		return "caldav"
	}
	return "carddav"
}

var (
	// ErrUnauthorized means the server refused the credentials (401).
	ErrUnauthorized = errors.New("davx: the server refused the credentials")
	// ErrPrecondition means a conditional write failed (412): the object
	// changed on the server, or one exists where a new one was to go.
	ErrPrecondition = errors.New("davx: the object on the server is not the one expected")
	// ErrInvalidToken means the server no longer accepts a sync token; the
	// collection must be compared in full.
	ErrInvalidToken = errors.New("davx: the sync token is no longer valid")
	// ErrNotFound means the resource does not exist (404, 410).
	ErrNotFound = errors.New("davx: not found")
	// ErrUntrusted means the server sent the client to an origin that
	// Options.Trusted does not accept, or to plain HTTP.
	ErrUntrusted = errors.New("davx: the server points to an untrusted location")
)

// StatusError is an HTTP status the client did not expect.
type StatusError struct {
	Method, URL string
	Code        int
	Body        string // the start of the response body
}

func (e *StatusError) Error() string {
	s := fmt.Sprintf("davx: %s %s: HTTP %d", e.Method, e.URL, e.Code)
	if e.Body != "" {
		s += ": " + e.Body
	}
	return s
}

// Options configure a client.
type Options struct {
	// Authorization returns the Authorization header for a request, such
	// as "Basic …" or "Bearer …"; nil sends none.
	Authorization func(ctx context.Context) (string, error)
	// HTTP sends the requests; nil uses one with a 60-second timeout. The
	// client never lets it follow redirects.
	HTTP *http.Client
	// Trusted reports whether credentials may go to u, an origin other
	// than the one a client or discovery started from (a redirect, a
	// principal or a home set on another host). Nil trusts none.
	Trusted func(u *url.URL) bool
	// AllowHTTP permits plain http:// URLs, for local test servers only.
	AllowHTTP bool
	// UserAgent is sent with every request.
	UserAgent string
}

// Client talks to one DAV origin.
type Client struct {
	base *url.URL
	opts Options
	http *http.Client
}

// New returns a client for base, an absolute URL whose origin every
// request goes to (normally a home set from Discover).
func New(base string, opts Options) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("davx: %w", err)
	}
	if err := checkScheme(u, opts.AllowHTTP); err != nil {
		return nil, err
	}
	hc := &http.Client{Timeout: time.Minute}
	if opts.HTTP != nil {
		copied := *opts.HTTP
		hc = &copied
	}
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if u.Path == "" {
		u.Path = "/"
	}
	return &Client{base: u, opts: opts, http: hc}, nil
}

func checkScheme(u *url.URL, allowHTTP bool) error {
	switch {
	case u.Scheme == "https" && u.Host != "":
		return nil
	case u.Scheme == "http" && u.Host != "" && allowHTTP:
		return nil
	case u.Scheme == "http" && u.Host != "":
		return fmt.Errorf("%w: %s is not HTTPS", ErrUntrusted, u.Redacted())
	}
	return fmt.Errorf("davx: %q is not an absolute HTTPS URL", u.Redacted())
}

// URL is the client's base URL.
func (c *Client) URL() string { return c.base.String() }

// resolve turns an href (a path, or a URL on the client's origin) into a
// URL on the client's origin.
func (c *Client) resolve(href string) (*url.URL, error) {
	ref, err := url.Parse(href)
	if err != nil {
		return nil, fmt.Errorf("davx: href %q: %w", href, err)
	}
	u := c.base.ResolveReference(ref)
	if !sameOrigin(u, c.base) {
		return nil, fmt.Errorf("%w: %s is not on %s", ErrUntrusted, u.Redacted(), c.base.Host)
	}
	return u, nil
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

// request is one HTTP exchange.
type request struct {
	method  string
	url     *url.URL
	body    []byte
	ctype   string
	headers map[string]string
}

// do sends r with the credentials and returns the response with its body
// read (at most 64 MiB).
func (c *Client) do(ctx context.Context, r request) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, r.method, r.url.String(), bytes.NewReader(r.body))
	if err != nil {
		return nil, nil, fmt.Errorf("davx: %w", err)
	}
	if r.body == nil {
		req.Body, req.ContentLength = http.NoBody, 0
	}
	if r.ctype != "" {
		req.Header.Set("Content-Type", r.ctype)
	}
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	if c.opts.UserAgent != "" {
		req.Header.Set("User-Agent", c.opts.UserAgent)
	}
	if c.opts.Authorization != nil {
		auth, err := c.opts.Authorization(ctx)
		if err != nil {
			return nil, nil, err
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("davx: %s %s: %w", r.method, r.url.Redacted(), err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("davx: %s %s: %w", r.method, r.url.Redacted(), err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, nil, ErrUnauthorized
	}
	return resp, body, nil
}

// statusError builds the error for an unexpected status.
func statusError(r request, resp *http.Response, body []byte) error {
	switch resp.StatusCode {
	case http.StatusPreconditionFailed:
		return ErrPrecondition
	case http.StatusNotFound, http.StatusGone:
		return fmt.Errorf("%w: %s", ErrNotFound, r.url.Redacted())
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 200 {
		text = text[:200] + "…"
	}
	return &StatusError{Method: r.method, URL: r.url.Redacted(), Code: resp.StatusCode, Body: text}
}

// Object is a contact or calendar object as the server holds it.
type Object struct {
	Href string // as the caller named it, or as the server wrote it
	ETag string
	Data []byte
}

// Get fetches one object.
func (c *Client) Get(ctx context.Context, href string) (Object, error) {
	u, err := c.resolve(href)
	if err != nil {
		return Object{}, err
	}
	r := request{method: http.MethodGet, url: u}
	resp, body, err := c.do(ctx, r)
	if err != nil {
		return Object{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Object{}, statusError(r, resp, body)
	}
	return Object{Href: href, ETag: resp.Header.Get("ETag"), Data: body}, nil
}

// Written is what a PUT left on the server.
type Written struct {
	// Href is where the object is: the href written, or for a new object
	// a server named itself (Google's CalDAV names it from its UID), the
	// Location it answered with.
	Href string
	// ETag is the new ETag, or "" when the server sent none (the caller
	// then fetches the object).
	ETag string
}

// Put writes an object. ifMatch is the ETag the change was made on; an
// empty ifMatch creates the object and fails with ErrPrecondition if one
// exists.
func (c *Client) Put(ctx context.Context, href string, data []byte, kind Kind, ifMatch string) (Written, error) {
	u, err := c.resolve(href)
	if err != nil {
		return Written{}, err
	}
	r := request{method: http.MethodPut, url: u, body: data, ctype: kind.ContentType(), headers: conditional(ifMatch)}
	resp, body, err := c.do(ctx, r)
	if err != nil {
		return Written{}, err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return Written{}, statusError(r, resp, body)
	}
	w := Written{Href: href, ETag: resp.Header.Get("ETag")}
	if loc := resp.Header.Get("Location"); loc != "" && resp.StatusCode == http.StatusCreated {
		if at, err := u.Parse(loc); err == nil {
			if path, ok := hrefPath(c.base, at.String()); ok && !samePath(path, href) {
				w.Href = path
			}
		}
	}
	return w, nil
}

// Delete removes an object if it still has the ETag ifMatch (any ETag when
// ifMatch is empty). An object already gone is not an error.
func (c *Client) Delete(ctx context.Context, href, ifMatch string) error {
	u, err := c.resolve(href)
	if err != nil {
		return err
	}
	var h map[string]string
	if ifMatch != "" {
		h = map[string]string{"If-Match": ifMatch}
	}
	r := request{method: http.MethodDelete, url: u, headers: h}
	resp, body, err := c.do(ctx, r)
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusAccepted, http.StatusNotFound, http.StatusGone:
		return nil
	}
	return statusError(r, resp, body)
}

func conditional(ifMatch string) map[string]string {
	if ifMatch == "" {
		return map[string]string{"If-None-Match": "*"}
	}
	return map[string]string{"If-Match": ifMatch}
}
