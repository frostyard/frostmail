// Package graphtest is an in-process Microsoft Graph for tests of Graph
// sync (docs/design/pim.md, Microsoft Graph), as davtest is for DAV: the
// account's calendars and contact folders in memory, delta rounds with
// tokens and pages, series read with their exceptions, contact photos,
// and hooks that play another client's changes. It answers as Graph did
// the probe of an Outlook.com account (plan 0010, Phase 1).
package graphtest

import (
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// PageSize is how many items a page holds, small so tests page.
const PageSize = 2

// Server is the fake Graph.
type Server struct {
	URL   string // the base URL, as graph.BaseURL
	Token string // the bearer token it accepts

	mu        sync.Mutex
	version   int
	calendars []*calendar
	folders   []*folder
	throttle  int // requests left to answer 429
	// Requests counts requests by method and path, without the query.
	Requests map[string]int
}

type item struct {
	id      string
	version int // when it last changed
	removed bool
	doc     map[string]any
	photo   []byte
}

type calendar struct {
	id, name string
	canEdit  bool
	isDef    bool
	items    []*item // singles and masters; exceptions live in their master
}

type folder struct {
	id, name string
	items    []*item
}

// Start runs a server that accepts token, with a default calendar
// ("cal-1", "Calendar") and the default contact folder.
func Start(t testing.TB, token string) *Server {
	t.Helper()
	s := &Server{Token: token, Requests: map[string]int{}}
	s.calendars = []*calendar{{id: "cal-1", name: "Calendar", canEdit: true, isDef: true}}
	s.folders = []*folder{{id: "contacts", name: "Contacts"}}
	srv := httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(srv.Close)
	s.URL = srv.URL + "/v1.0/"
	return s
}

// AddCalendar adds a calendar.
func (s *Server) AddCalendar(id, name string, canEdit bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calendars = append(s.calendars, &calendar{id: id, name: name, canEdit: canEdit})
}

// Throttle answers the next n requests with 429 and Retry-After: 0.
func (s *Server) Throttle(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.throttle = n
}

func (s *Server) next() int {
	s.version++
	return s.version
}

// PutEvent stores an event (a single event, or a master with
// "exceptionOccurrences") in a calendar, as another client would. The
// event's "id" names it; its change key and ETag are set here.
func (s *Server) PutEvent(calendarID, event string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var doc map[string]any
	if err := json.Unmarshal([]byte(event), &doc); err != nil {
		panic(err)
	}
	cal := s.calendar(calendarID)
	v := s.next()
	key := "ck-" + strconv.Itoa(v)
	doc["changeKey"], doc["@odata.etag"] = key, `W/"`+key+`"`
	id, _ := doc["id"].(string)
	for _, it := range cal.items {
		if it.id == id {
			it.doc, it.version, it.removed = doc, v, false
			return
		}
	}
	cal.items = append(cal.items, &item{id: id, version: v, doc: doc})
}

// DeleteEvent deletes an event or a series.
func (s *Server) DeleteEvent(calendarID, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.calendar(calendarID).items {
		if it.id == id {
			it.removed, it.version = true, s.next()
		}
	}
}

// PutContact stores a contact in a folder ("contacts" is the default),
// with photo when it is not nil.
func (s *Server) PutContact(folderID, contact string, photo []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var doc map[string]any
	if err := json.Unmarshal([]byte(contact), &doc); err != nil {
		panic(err)
	}
	f := s.folder(folderID)
	v := s.next()
	key := "ck-" + strconv.Itoa(v)
	doc["changeKey"], doc["@odata.etag"] = key, `W/"`+key+`"`
	id, _ := doc["id"].(string)
	for _, it := range f.items {
		if it.id == id {
			it.doc, it.version, it.removed, it.photo = doc, v, false, photo
			return
		}
	}
	f.items = append(f.items, &item{id: id, version: v, doc: doc, photo: photo})
}

// DeleteContact deletes a contact.
func (s *Server) DeleteContact(folderID, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.folder(folderID).items {
		if it.id == id {
			it.removed, it.version = true, s.next()
		}
	}
}

func (s *Server) calendar(id string) *calendar {
	for _, c := range s.calendars {
		if c.id == id {
			return c
		}
	}
	panic("graphtest: no calendar " + id)
}

func (s *Server) folder(id string) *folder {
	for _, f := range s.folders {
		if f.id == id {
			return f
		}
	}
	panic("graphtest: no folder " + id)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := strings.TrimPrefix(r.URL.Path, "/v1.0/")
	s.Requests[r.Method+" "+path]++
	if r.Header.Get("Authorization") != "Bearer "+s.Token {
		fail(w, http.StatusUnauthorized, "InvalidAuthenticationToken", "the token is not valid")
		return
	}
	if s.throttle > 0 {
		s.throttle--
		w.Header().Set("Retry-After", "0")
		fail(w, http.StatusTooManyRequests, "TooManyRequests", "slow down")
		return
	}
	parts := strings.Split(path, "/")
	q := r.URL.Query()
	switch {
	case r.Method == http.MethodGet && path == "me/calendars":
		var out []any
		for _, c := range s.calendars {
			out = append(out, map[string]any{"id": c.id, "name": c.name, "canEdit": c.canEdit, "isDefaultCalendar": c.isDef,
				"color": "auto", "hexColor": ""})
		}
		page(w, out, "", "")
	case r.Method == http.MethodGet && path == "me/contactFolders":
		var out []any
		for _, f := range s.folders[1:] {
			out = append(out, map[string]any{"id": f.id, "displayName": f.name})
		}
		page(w, out, "", "")
	case r.Method == http.MethodGet && len(parts) == 5 && parts[1] == "calendars" && parts[3] == "calendarView" && parts[4] == "delta":
		s.calendarDelta(w, r, s.calendar(parts[2]), q)
	case r.Method == http.MethodGet && len(parts) == 4 && parts[1] == "calendars" && parts[3] == "calendarView":
		s.calendarView(w, s.calendar(parts[2]))
	case r.Method == http.MethodGet && path == "me/contacts":
		s.contacts(w, s.folders[0])
	case r.Method == http.MethodGet && len(parts) == 4 && parts[1] == "contactFolders" && parts[3] == "contacts":
		s.contacts(w, s.folder(parts[2]))
	case r.Method == http.MethodGet && len(parts) == 3 && parts[1] == "events":
		s.event(w, parts[2])
	case r.Method == http.MethodGet && path == "me/contacts/delta":
		s.contactsDelta(w, r, s.folders[0], q)
	case r.Method == http.MethodGet && len(parts) == 5 && parts[1] == "contactFolders" && parts[3] == "contacts" && parts[4] == "delta":
		s.contactsDelta(w, r, s.folder(parts[2]), q)
	case r.Method == http.MethodGet && len(parts) == 5 && parts[1] == "contacts" && parts[3] == "photo" && parts[4] == "$value":
		s.photo(w, parts[2])
	default:
		fail(w, http.StatusBadRequest, "BadRequest", "graphtest does not serve "+r.Method+" "+path)
	}
}

func fail(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	b, _ := json.Marshal(map[string]any{"error": map[string]any{"code": code, "message": message}})
	_, _ = w.Write(b)
}

func page(w http.ResponseWriter, values []any, next, delta string) {
	doc := map[string]any{"value": values}
	if values == nil {
		doc["value"] = []any{}
	}
	if next != "" {
		doc["@odata.nextLink"] = next
	}
	if delta != "" {
		doc["@odata.deltaLink"] = delta
	}
	w.Header().Set("Content-Type", "application/json")
	b, _ := json.Marshal(doc)
	_, _ = w.Write(b)
}

// round pages a delta round over items changed after since, its state
// in the query: $deltatoken (a finished round's version) or $skiptoken
// (the version the round reads from, and how many items it has sent).
func (s *Server) round(w http.ResponseWriter, r *http.Request, q url.Values, items func(since int) []any) {
	since, skip := 0, 0
	if t := q.Get("$deltatoken"); t != "" {
		v, err := strconv.Atoi(t)
		if err != nil || v > s.version {
			fail(w, http.StatusGone, "syncStateNotFound", "the delta token is not known")
			return
		}
		since = v
	}
	if t := q.Get("$skiptoken"); t != "" {
		a, b, _ := strings.Cut(t, ".")
		since, _ = strconv.Atoi(a)
		skip, _ = strconv.Atoi(b)
	}
	all := items(since)
	end := min(skip+PageSize, len(all))
	link := func(key, value string) string {
		u := *r.URL
		qq := u.Query()
		qq.Del("$deltatoken")
		qq.Del("$skiptoken")
		qq.Set(key, value)
		u.RawQuery = qq.Encode()
		return "http://" + r.Host + u.Path + "?" + u.RawQuery
	}
	if end < len(all) {
		page(w, all[skip:end], link("$skiptoken", strconv.Itoa(since)+"."+strconv.Itoa(end)), "")
		return
	}
	page(w, all[skip:end], "", link("$deltatoken", strconv.Itoa(s.version)))
}

func (s *Server) calendarDelta(w http.ResponseWriter, r *http.Request, cal *calendar, q url.Values) {
	s.round(w, r, q, func(since int) []any {
		var out []any
		for _, it := range cal.items {
			if it.version <= since {
				continue
			}
			if it.removed {
				if since > 0 {
					out = append(out, map[string]any{"@odata.type": "#microsoft.graph.event", "id": it.id,
						"@removed": map[string]any{"reason": "deleted"}})
				}
				continue
			}
			doc := without(it.doc, "exceptionOccurrences")
			out = append(out, doc)
			if doc["type"] == "seriesMaster" {
				// A bare occurrence, as Graph sends among a series' items.
				out = append(out, map[string]any{"id": it.id + "-occ", "type": "occurrence", "seriesMasterId": it.id})
				if exceptions, ok := it.doc["exceptionOccurrences"].([]any); ok {
					out = append(out, exceptions...)
				}
			}
		}
		return out
	})
}

// calendarView lists a calendar's live events as Graph's calendarView
// does: single events, and for a series one occurrence and its
// exceptions, all in one page.
func (s *Server) calendarView(w http.ResponseWriter, cal *calendar) {
	var out []any
	for _, it := range cal.items {
		if it.removed {
			continue
		}
		if it.doc["type"] != "seriesMaster" {
			out = append(out, map[string]any{"id": it.id, "type": it.doc["type"], "@odata.etag": it.doc["@odata.etag"]})
			continue
		}
		out = append(out, map[string]any{"id": it.id + "-occ", "type": "occurrence", "seriesMasterId": it.id})
		if exceptions, ok := it.doc["exceptionOccurrences"].([]any); ok {
			for _, ex := range exceptions {
				if m, ok := ex.(map[string]any); ok {
					out = append(out, map[string]any{"id": m["id"], "type": "exception", "seriesMasterId": it.id})
				}
			}
		}
	}
	page(w, out, "", "")
}

// contacts lists a folder's live contacts with their ETags.
func (s *Server) contacts(w http.ResponseWriter, f *folder) {
	var out []any
	for _, it := range f.items {
		if !it.removed {
			out = append(out, map[string]any{"id": it.id, "@odata.etag": it.doc["@odata.etag"]})
		}
	}
	page(w, out, "", "")
}

func without(doc map[string]any, key string) map[string]any {
	out := make(map[string]any, len(doc))
	for k, v := range doc {
		if k != key {
			out[k] = v
		}
	}
	return out
}

func (s *Server) event(w http.ResponseWriter, id string) {
	for _, cal := range s.calendars {
		for _, it := range cal.items {
			if it.id == id && !it.removed {
				w.Header().Set("Content-Type", "application/json")
				b, _ := json.Marshal(it.doc)
				_, _ = w.Write(b)
				return
			}
		}
	}
	fail(w, http.StatusNotFound, "ErrorItemNotFound", "The specified object was not found in the store.")
}

func (s *Server) contactsDelta(w http.ResponseWriter, r *http.Request, f *folder, q url.Values) {
	s.round(w, r, q, func(since int) []any {
		var out []any
		for _, it := range f.items {
			if it.version <= since {
				continue
			}
			if it.removed {
				if since > 0 {
					out = append(out, map[string]any{"@odata.type": "#microsoft.graph.contact", "id": it.id,
						"@removed": map[string]any{"reason": "deleted"}})
				}
				continue
			}
			out = append(out, it.doc)
		}
		return out
	})
}

func (s *Server) photo(w http.ResponseWriter, id string) {
	for _, f := range s.folders {
		for _, it := range f.items {
			if it.id == id && !it.removed && it.photo != nil {
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write(it.photo)
				return
			}
		}
	}
	fail(w, http.StatusNotFound, "ImageNotFound", "The photo wasn't found.")
}

// Events returns a calendar's live events' IDs, sorted, for assertions.
func (s *Server) Events(calendarID string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, it := range s.calendar(calendarID).items {
		if !it.removed {
			out = append(out, it.id)
		}
	}
	slices.Sort(out)
	return out
}
