// Package davtest is an in-process CardDAV and CalDAV server for tests, as
// imapxtest is for IMAP (docs/design/pim.md): memory collections, ETags,
// sync tokens over a change log, conditional writes, and hooks to change
// objects as another client would and to make requests fail.
//
// It serves one user. The principal is PrincipalPath; address books live
// under ContactsHome and calendars under CalendarsHome, and
// /.well-known/carddav and caldav redirect to /dav/.
package davtest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"
)

// The server's fixed names.
const (
	User          = "user"
	Password      = "secret"
	PrincipalPath = "/dav/principals/user/"
	ContactsHome  = "/dav/user/contacts/"
	CalendarsHome = "/dav/user/calendars/"
	// InboxPath is the scheduling inbox, named with ScheduleDefault.
	InboxPath = "/dav/user/inbox/"
)

// Options tune the server.
type Options struct {
	// Bearer, when set, is a token accepted as "Authorization: Bearer …"
	// besides User and Password.
	Bearer string
	// NoSync makes collections neither list nor answer sync-collection.
	NoSync bool
	// NoWellKnown answers /.well-known/ with 404.
	NoWellKnown bool
	// Truncate cuts sync-collection answers after this many changes and
	// marks them 507, as servers with a result limit do; 0 never cuts.
	Truncate int
	// CDATA writes object data in CDATA sections with literal line
	// endings, which XML parsers turn from CRLF into LF.
	CDATA bool
	// NoETagOnPut leaves the ETag header out of PUT responses.
	NoETagOnPut bool
	// ScheduleDefault is the decoded path of the user's default calendar,
	// named by the scheduling inbox (RFC 6638 §9.2); "" names no inbox.
	ScheduleDefault string
	// RelocateCreates stores a new object at a name of the server's
	// choosing, from its UID, and answers 201 with a Location and no ETag,
	// as Google's CalDAV does.
	RelocateCreates bool
	// NoInitialSync answers a sync-collection without a token with 400,
	// as Google's CardDAV does, while still listing the report.
	NoInitialSync bool
}

// Request is one request the server received.
type Request struct {
	Method string
	Path   string // escaped, as sent
	Depth  string
	Report string // REPORTs: sync-collection, addressbook-multiget or calendar-multiget
	Token  string // sync-collection: the token sent
	Hrefs  int    // multiget: how many hrefs
	Header http.Header
}

// Server is a running DAV server.
type Server struct {
	// URL is the server's https:// origin.
	URL string
	// Client trusts the server's certificate.
	Client *http.Client

	opts Options
	ts   *httptest.Server

	mu    sync.Mutex
	epoch int // bumped by InvalidateTokens
	seq   int // the server-wide change counter
	colls map[string]*collection
	reqs  []Request
	fail  func(*http.Request) int
}

type collection struct {
	path, name, color string
	calendar          bool
	comps             []string
	readOnly          bool
	objects           map[string]*object // by decoded path
	changes           []change
	// phantoms are listed by a PROPFIND of the collection, with an ETag,
	// and nowhere else (Phantom).
	phantoms map[string]string
}

type object struct {
	data []byte
	etag string
}

type change struct {
	seq  int
	path string
}

// New starts a server, stopped when the test ends.
func New(t testing.TB, opts Options) *Server {
	s := &Server{opts: opts, colls: map[string]*collection{}}
	s.ts = httptest.NewTLSServer(s)
	s.URL, s.Client = s.ts.URL, s.ts.Client()
	t.Cleanup(s.ts.Close)
	return s
}

// AddressBook adds an address book named name (a path segment) and
// returns its path.
func (s *Server) AddressBook(name, display string) string {
	return s.add(&collection{path: ContactsHome + name + "/", name: display})
}

// Calendar adds a calendar with the components (VEVENT, VTODO; VEVENT
// when none) and returns its path. color is #rrggbb, or "".
func (s *Server) Calendar(name, display, color string, comps ...string) string {
	if len(comps) == 0 {
		comps = []string{"VEVENT"}
	}
	return s.add(&collection{path: CalendarsHome + name + "/", name: display, color: color, calendar: true, comps: comps})
}

func (s *Server) add(c *collection) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	c.objects = map[string]*object{}
	s.colls[c.path] = c
	return c.path
}

// SetReadOnly makes a collection refuse (or accept again) PUT and DELETE.
func (s *Server) SetReadOnly(coll string, ro bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.colls[coll].readOnly = ro
}

// Rename changes a collection's display name.
func (s *Server) Rename(coll, display string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.colls[coll].name = display
}

// RemoveCollection deletes a collection and its objects.
func (s *Server) RemoveCollection(coll string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.colls, coll)
}

// Phantom makes a PROPFIND of path's collection list path with an ETag,
// while a sync-collection, a multiget and a GET do not have it: Google
// lists events its sync reports as deleted and does not serve.
func (s *Server) Phantom(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.parent(path)
	if c == nil {
		panic("davtest: no collection for " + path)
	}
	if c.phantoms == nil {
		c.phantoms = map[string]string{}
	}
	c.phantoms[path] = fmt.Sprintf(`"phantom-%d"`, len(c.phantoms)+1)
}

// Put stores data at path, a decoded path inside a collection, as another
// client would, and returns the new ETag.
func (s *Server) Put(path string, data []byte) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.parent(path)
	if c == nil {
		panic("davtest: no collection for " + path)
	}
	return s.store(c, path, data)
}

// Delete removes the object at path as another client would.
func (s *Server) Delete(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.parent(path); c != nil {
		s.remove(c, path)
	}
}

// Object returns the object at path.
func (s *Server) Object(path string) (data []byte, etag string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.parent(path)
	if c == nil || c.objects[path] == nil {
		return nil, "", false
	}
	o := c.objects[path]
	return slices.Clone(o.data), o.etag, true
}

// Paths returns the paths of a collection's objects, sorted.
func (s *Server) Paths(coll string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Sorted(maps.Keys(s.colls[coll].objects))
}

// InvalidateTokens makes every sync token handed out so far invalid, as a
// server that pruned its change log.
func (s *Server) InvalidateTokens() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.epoch++
}

// Requests returns the requests received so far.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.reqs)
}

// ResetRequests forgets the requests received so far.
func (s *Server) ResetRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reqs = nil
}

// FailWith makes each request for which f returns a status other than 0
// fail with that status; nil stops failing. f runs without the server's
// lock and must not block.
func (s *Server) FailWith(f func(*http.Request) int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = f
}

func (s *Server) store(c *collection, path string, data []byte) string {
	sum := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	c.objects[path] = &object{data: slices.Clone(data), etag: etag}
	s.seq++
	c.changes = append(c.changes, change{s.seq, path})
	return etag
}

func (s *Server) remove(c *collection, path string) {
	delete(c.objects, path)
	s.seq++
	c.changes = append(c.changes, change{s.seq, path})
}

// parent returns the collection holding the object at path.
func (s *Server) parent(path string) *collection {
	i := strings.LastIndexByte(path, '/')
	if i < 0 || strings.HasSuffix(path, "/") {
		return nil
	}
	return s.colls[path[:i+1]]
}

func (s *Server) token() string { return fmt.Sprintf("http://davtest.test/sync/%d/%d", s.epoch, s.seq) }

func (c *collection) ctag() string {
	if len(c.changes) == 0 {
		return "ctag-0"
	}
	return fmt.Sprintf("ctag-%d", c.changes[len(c.changes)-1].seq)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	rec := Request{Method: r.Method, Path: r.URL.EscapedPath(), Depth: r.Header.Get("Depth"), Header: r.Header.Clone()}
	if r.Method == "REPORT" {
		rec.Report, rec.Token, rec.Hrefs = describeReport(body)
	}
	s.mu.Lock()
	s.reqs = append(s.reqs, rec)
	fail := s.fail
	s.mu.Unlock()
	if fail != nil {
		if code := fail(r); code != 0 {
			http.Error(w, "davtest: failing on purpose", code)
			return
		}
	}
	if !s.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Basic realm="davtest"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/.well-known/") {
		if s.opts.NoWellKnown || (r.URL.Path != "/.well-known/carddav" && r.URL.Path != "/.well-known/caldav") {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/dav/", http.StatusMovedPermanently)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch r.Method {
	case "PROPFIND":
		s.propfind(w, r, body)
	case "REPORT":
		s.report(w, r, body)
	case http.MethodGet:
		s.get(w, r)
	case http.MethodPut:
		s.put(w, r, body)
	case http.MethodDelete:
		s.delete(w, r)
	case http.MethodOptions:
		w.Header().Set("DAV", "1, 3, addressbook, calendar-access")
		w.Header().Set("Allow", "OPTIONS, GET, PUT, DELETE, PROPFIND, REPORT")
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) authorized(r *http.Request) bool {
	if u, p, ok := r.BasicAuth(); ok && u == User && p == Password {
		return true
	}
	return s.opts.Bearer != "" && r.Header.Get("Authorization") == "Bearer "+s.opts.Bearer
}

func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	c := s.parent(r.URL.Path)
	if c == nil || c.objects[r.URL.Path] == nil {
		http.NotFound(w, r)
		return
	}
	o := c.objects[r.URL.Path]
	w.Header().Set("ETag", o.etag)
	w.Header().Set("Content-Type", c.contentType())
	_, _ = w.Write(o.data)
}

func (c *collection) contentType() string {
	if c.calendar {
		return "text/calendar; charset=utf-8"
	}
	return "text/vcard; charset=utf-8"
}

func (s *Server) put(w http.ResponseWriter, r *http.Request, body []byte) {
	c := s.parent(r.URL.Path)
	switch {
	case c == nil:
		http.Error(w, "no such collection", http.StatusConflict)
		return
	case c.readOnly:
		http.Error(w, "read-only", http.StatusForbidden)
		return
	case !strings.HasPrefix(r.Header.Get("Content-Type"), strings.Split(c.contentType(), ";")[0]):
		http.Error(w, "wrong content type", http.StatusUnsupportedMediaType)
		return
	case !bytes.Contains(body, []byte("BEGIN:VCARD")) && !c.calendar || !bytes.Contains(body, []byte("BEGIN:VCALENDAR")) && c.calendar:
		http.Error(w, "not a vCard or iCalendar object", http.StatusBadRequest)
		return
	}
	old := c.objects[r.URL.Path]
	if !preconditions(r, old) {
		http.Error(w, "precondition failed", http.StatusPreconditionFailed)
		return
	}
	if old == nil && s.opts.RelocateCreates {
		if uid := objectUID(body); uid != "" {
			ext := path.Ext(r.URL.Path)
			at := c.path + uid + ext
			s.store(c, at, body)
			w.Header().Set("Location", s.URL+escapePath(at))
			w.WriteHeader(http.StatusCreated)
			return
		}
	}
	etag := s.store(c, r.URL.Path, body)
	if !s.opts.NoETagOnPut {
		w.Header().Set("ETag", etag)
	}
	if old == nil {
		w.WriteHeader(http.StatusCreated)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) delete(w http.ResponseWriter, r *http.Request) {
	c := s.parent(r.URL.Path)
	if c == nil || c.objects[r.URL.Path] == nil {
		http.NotFound(w, r)
		return
	}
	if c.readOnly {
		http.Error(w, "read-only", http.StatusForbidden)
		return
	}
	if !preconditions(r, c.objects[r.URL.Path]) {
		http.Error(w, "precondition failed", http.StatusPreconditionFailed)
		return
	}
	s.remove(c, r.URL.Path)
	w.WriteHeader(http.StatusNoContent)
}

// preconditions checks If-Match and If-None-Match against the object
// (nil when there is none).
func preconditions(r *http.Request, o *object) bool {
	if m := r.Header.Get("If-Match"); m != "" && (o == nil || m != "*" && m != o.etag) {
		return false
	}
	if m := r.Header.Get("If-None-Match"); m != "" && o != nil && (m == "*" || m == o.etag) {
		return false
	}
	return true
}

// describeReport reads a REPORT body's root element, sync token and href
// count, for Requests.
func describeReport(body []byte) (report, token string, hrefs int) {
	d := xml.NewDecoder(bytes.NewReader(body))
	var in string
	for {
		tok, err := d.Token()
		if err != nil {
			return report, token, hrefs
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if report == "" {
				report = t.Name.Local
			}
			in = t.Name.Local
			if in == "href" {
				hrefs++
			}
		case xml.CharData:
			if in == "sync-token" {
				token += string(t)
			}
		case xml.EndElement:
			in = ""
		}
	}
}

// objectUID is the first UID in a vCard or iCalendar object.
func objectUID(data []byte) string {
	for line := range strings.SplitSeq(string(data), "\n") {
		if uid, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), "UID:"); ok {
			return strings.TrimSpace(uid)
		}
	}
	return ""
}
