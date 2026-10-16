package graphtest

import (
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	ical "github.com/frostyard/frostmail/internal/calendar"
	"github.com/frostyard/frostmail/internal/graph"
)

// Graph's writes, as the probe and the write session found them (plan
// 0010, Phase 3b): POST creates (once per transactionId), PATCH merges
// top-level fields on an If-Match, DELETE moves the item away, and a
// series' occurrence becomes an exception when it is changed or
// answered, or a cancelled occurrence when deleted. Times come back in
// UTC, an all-day event's at midnight UTC.

// Write is a write the server took, for assertions: Body is the JSON sent,
// its keys sorted.
type Write struct {
	Method, Path, IfMatch, Body string
}

func (w Write) String() string {
	s := w.Method + " " + w.Path
	if w.IfMatch != "" {
		s += " [" + w.IfMatch + "]"
	}
	if w.Body != "" {
		s += " " + w.Body
	}
	return s
}

// WritesSince returns the writes after the first n.
func (s *Server) WritesSince(n int) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, w := range s.writes[min(n, len(s.writes)):] {
		out = append(out, w.String())
	}
	return out
}

// WriteCount is how many writes the server took.
func (s *Server) WriteCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.writes)
}

// Event returns an event's or a series' JSON as stored, or nil.
func (s *Server) Event(id string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it := s.findEvent(id); it != nil {
		return it.doc
	}
	return nil
}

// Contact returns a contact's JSON as stored and its photo, or nil.
func (s *Server) Contact(id string) (map[string]any, []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if it := s.findContact(id); it != nil {
		return it.doc, it.photo
	}
	return nil, nil
}

// serveWrite answers the writes and the reads that go with them; false
// for a request it does not serve.
func (s *Server) serveWrite(w http.ResponseWriter, r *http.Request, parts []string) bool {
	n := len(parts)
	switch {
	case r.Method == http.MethodGet && n == 3 && parts[1] == "contacts":
		s.getContact(w, parts[2])
	case r.Method == http.MethodGet && n == 4 && parts[1] == "events" && parts[3] == "instances":
		s.instances(w, parts[2], r.URL.Query())
	case r.Method == http.MethodGet && n == 4 && parts[1] == "calendars" && parts[3] == "events":
		s.eventsByUID(w, s.calendar(parts[2]), r.URL.Query())
	case r.Method == http.MethodPost && n == 4 && parts[1] == "calendars" && parts[3] == "events":
		s.createEvent(w, r, s.calendar(parts[2]))
	case r.Method == http.MethodPatch && n == 3 && parts[1] == "events":
		s.updateEvent(w, r, parts[2])
	case r.Method == http.MethodDelete && n == 3 && parts[1] == "events":
		s.deleteEvent(w, r, parts[2])
	case r.Method == http.MethodPost && n == 4 && parts[1] == "events":
		s.answer(w, r, parts[2], parts[3])
	case r.Method == http.MethodPost && strings.Join(parts, "/") == "me/contacts":
		s.createContact(w, r, s.folders[0])
	case r.Method == http.MethodPost && n == 4 && parts[1] == "contactFolders" && parts[3] == "contacts":
		s.createContact(w, r, s.folder(parts[2]))
	case r.Method == http.MethodPatch && n == 3 && parts[1] == "contacts":
		s.updateContact(w, r, parts[2])
	case r.Method == http.MethodDelete && n == 3 && parts[1] == "contacts":
		s.deleteContact(w, r, parts[2])
	case r.Method == http.MethodPut && n == 5 && parts[1] == "contacts" && parts[3] == "photo":
		s.setPhoto(w, r, parts[2])
	case r.Method == http.MethodDelete && n == 5 && parts[1] == "contacts" && parts[3] == "photo":
		fail(w, http.StatusMethodNotAllowed, "ErrorInvalidRequest", "The OData request is not supported.")
	default:
		return false
	}
	return true
}

// record reads a write's JSON body and records the write.
func (s *Server) record(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		fail(w, http.StatusBadRequest, "BadRequest", err.Error())
		return nil, false
	}
	wr := Write{Method: r.Method, Path: strings.TrimPrefix(r.URL.Path, "/v1.0/"), IfMatch: r.Header.Get("If-Match")}
	var body map[string]any
	if len(data) > 0 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(data, &body); err != nil {
			fail(w, http.StatusBadRequest, "BadRequest", err.Error())
			return nil, false
		}
		b, _ := json.Marshal(body, json.Deterministic(true))
		wr.Body = string(b)
	} else if len(data) > 0 {
		wr.Body = strconv.Itoa(len(data)) + " bytes of " + r.Header.Get("Content-Type")
		body = map[string]any{"$value": data}
	}
	s.writes = append(s.writes, wr)
	return body, true
}

func (s *Server) touch(it *item) {
	v := s.next()
	key := "ck-" + strconv.Itoa(v)
	it.doc["changeKey"], it.doc["@odata.etag"] = key, `W/"`+key+`"`
	it.version = v
}

// stale answers 412 when If-Match names another version.
func stale(w http.ResponseWriter, r *http.Request, doc map[string]any) bool {
	if m := r.Header.Get("If-Match"); m != "" && m != doc["@odata.etag"] {
		fail(w, http.StatusPreconditionFailed, "ErrorIrresolvableConflict",
			"The send or update operation could not be performed because the change key passed in the request does not match the current change key for the item.")
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, doc any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	b, _ := json.Marshal(doc)
	_, _ = w.Write(b)
}

func (s *Server) findEvent(id string) *item {
	for _, cal := range s.calendars {
		for _, it := range cal.items {
			if it.id == id && !it.removed {
				return it
			}
		}
	}
	return nil
}

func (s *Server) findContact(id string) *item {
	for _, f := range s.folders {
		for _, it := range f.items {
			if it.id == id && !it.removed {
				return it
			}
		}
	}
	return nil
}

// occurrence finds what an event ID names inside a series: one of its
// exceptions, or one of its occurrences ("<master>-occ-<key>", as
// instances lists them).
func (s *Server) occurrence(id string) (master *item, exception map[string]any, key string) {
	if m, k, ok := strings.Cut(id, "-occ-"); ok {
		return s.findEvent(m), nil, k
	}
	for _, cal := range s.calendars {
		for _, it := range cal.items {
			for _, ex := range exceptions(it) {
				if ex["id"] == id && !it.removed {
					return it, ex, ""
				}
			}
		}
	}
	return nil, nil, ""
}

func exceptions(it *item) []map[string]any {
	list, _ := it.doc["exceptionOccurrences"].([]any)
	var out []map[string]any
	for _, ex := range list {
		if m, ok := ex.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// zoneOf reads Graph's zone name, Windows or IANA.
func zoneOf(name string) *time.Location {
	if iana, ok := ical.WindowsZone(name); ok {
		name = iana
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// toUTC turns a written start or end into what Graph gives back, and
// names its zone; an all-day event's must be midnight.
func toUTC(v any, allDay bool) (map[string]any, string, bool) {
	m, _ := v.(map[string]any)
	dt, _ := m["dateTime"].(string)
	zone, _ := m["timeZone"].(string)
	t, err := time.ParseInLocation("2006-01-02T15:04:05", strings.SplitN(dt, ".", 2)[0], zoneOf(zone))
	if err != nil || allDay && (t.Hour() != 0 || t.Minute() != 0 || t.Second() != 0) {
		return nil, "", false
	}
	return map[string]any{"dateTime": t.UTC().Format("2006-01-02T15:04:05.0000000"), "timeZone": "UTC"}, zone, true
}

// apply merges written fields into an event as Graph stores them. An
// all-day event is kept at midnight UTC, its zone UTC, whatever zone it
// was written in (Outlook.com, 2026-10-16).
func apply(w http.ResponseWriter, doc, fields map[string]any) bool {
	allDay, _ := doc["isAllDay"].(bool)
	if v, ok := fields["isAllDay"].(bool); ok {
		allDay = v
	}
	for k, v := range fields {
		switch k {
		case "start", "end":
			t, zone, ok := toUTC(v, allDay)
			if !ok {
				fail(w, http.StatusBadRequest, "ErrorInvalidRequest", "Your request can't be completed. "+k+" is not valid.")
				return false
			}
			if allDay {
				m, _ := v.(map[string]any)
				dt, _ := m["dateTime"].(string)
				t, zone = map[string]any{"dateTime": strings.SplitN(dt, ".", 2)[0] + ".0000000", "timeZone": "UTC"}, "UTC"
			}
			doc[k] = t
			if k == "start" {
				doc["originalStartTimeZone"] = zone
			} else {
				doc["originalEndTimeZone"] = zone
			}
		case "recurrence":
			doc[k] = v
			if v != nil {
				doc["type"] = "seriesMaster"
			}
		case "transactionId", "$value":
		default:
			doc[k] = v
		}
	}
	return true
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request, cal *calendar) {
	fields, ok := s.record(w, r)
	if !ok {
		return
	}
	if tx, _ := fields["transactionId"].(string); tx != "" {
		for _, it := range cal.items {
			if it.doc["transactionId"] == tx && !it.removed {
				writeJSON(w, http.StatusCreated, it.doc)
				return
			}
		}
	}
	v := s.next()
	id := "AAMk-w" + strconv.Itoa(v)
	doc := map[string]any{"id": id, "iCalUId": "040000008200E00074C5B7101A82E008-" + strconv.Itoa(v),
		"type": "singleInstance", "isOrganizer": true, "transactionId": fields["transactionId"],
		"responseStatus": map[string]any{"response": "organizer"}}
	if s.User != "" {
		doc["organizer"] = map[string]any{"emailAddress": map[string]any{"name": "", "address": s.User}}
	}
	if !apply(w, doc, fields) {
		return
	}
	it := &item{id: id, doc: doc}
	s.touch(it)
	cal.items = append(cal.items, it)
	writeJSON(w, http.StatusCreated, doc)
}

func (s *Server) updateEvent(w http.ResponseWriter, r *http.Request, id string) {
	fields, ok := s.record(w, r)
	if !ok {
		return
	}
	if it := s.findEvent(id); it != nil {
		before, _ := json.Marshal(it.doc)
		if stale(w, r, it.doc) || !apply(w, it.doc, fields) {
			return
		}
		if reshapes(before, fields) {
			// Outlook drops a series' exceptions and cancellations with its
			// time or pattern, and keeps them when only its end changes.
			delete(it.doc, "exceptionOccurrences")
			delete(it.doc, "cancelledOccurrences")
		}
		s.touch(it)
		writeJSON(w, http.StatusOK, it.doc)
		return
	}
	master, ex, key := s.occurrence(id)
	if master == nil {
		fail(w, http.StatusNotFound, "ErrorItemNotFound", "The specified object was not found in the store.")
		return
	}
	if ex == nil {
		if ex = s.except(master, key); ex == nil {
			fail(w, http.StatusNotFound, "ErrorItemNotFound", "The specified object was not found in the store.")
			return
		}
	} else if stale(w, r, ex) {
		return
	}
	if !apply(w, ex, fields) {
		return
	}
	s.touch(master)
	writeJSON(w, http.StatusOK, ex)
}

// reshapes reports whether a write moves a series' occurrences: a new
// start, end or pattern.
func reshapes(before []byte, fields map[string]any) bool {
	var was map[string]any
	_ = json.Unmarshal(before, &was)
	if was["type"] != "seriesMaster" {
		return false
	}
	_, start := fields["start"]
	_, end := fields["end"]
	rec, ok := fields["recurrence"].(map[string]any)
	if !ok {
		return start || end
	}
	old, _ := was["recurrence"].(map[string]any)
	a, _ := json.Marshal(old["pattern"], json.Deterministic(true))
	b, _ := json.Marshal(rec["pattern"], json.Deterministic(true))
	return start || end || string(a) != string(b)
}

// except makes an occurrence of a series an exception, as it first is.
func (s *Server) except(master *item, key string) map[string]any {
	at, err := graph.KeyTime(key)
	if err != nil {
		return nil
	}
	var occ *ical.Occurrence
	for _, o := range s.expand(master, at.Add(-time.Hour), at.Add(time.Hour)) {
		if o.RecurrenceID == key {
			occ = &o
		}
	}
	if occ == nil {
		return nil
	}
	ex := map[string]any{}
	for k, v := range master.doc {
		switch k {
		case "recurrence", "exceptionOccurrences", "cancelledOccurrences", "transactionId":
		default:
			ex[k] = v
		}
	}
	v := s.next()
	ex["id"], ex["type"], ex["seriesMasterId"] = "AAMk-x"+strconv.Itoa(v), "exception", master.id
	// Outlook's exception has an iCalUId of its own, with its day in it.
	ex["iCalUId"] = fmt.Sprint(master.doc["iCalUId"], "-", at.UTC().Format("20060102"))
	ex["occurrenceId"] = "OID." + master.id + "." + s.day(master, at)
	ex["originalStart"] = at.UTC().Format(time.RFC3339)
	ex["start"], ex["end"] = s.instantDoc(master, occ.Start), s.instantDoc(master, occ.End)
	key2 := "ck-" + strconv.Itoa(v)
	ex["changeKey"], ex["@odata.etag"] = key2, `W/"`+key2+`"`
	list, _ := master.doc["exceptionOccurrences"].([]any)
	master.doc["exceptionOccurrences"] = append(list, ex)
	return ex
}

// day is the day an occurrence starts on in the series' zone, as
// occurrence IDs name it.
func (s *Server) day(master *item, at time.Time) string {
	if allDay, _ := master.doc["isAllDay"].(bool); allDay {
		return at.UTC().Format(time.DateOnly)
	}
	zone, _ := master.doc["originalStartTimeZone"].(string)
	return at.In(zoneOf(zone)).Format(time.DateOnly)
}

// instantDoc is an occurrence's start or end as Graph gives it, in UTC.
func (s *Server) instantDoc(_ *item, t time.Time) map[string]any {
	return map[string]any{"dateTime": t.UTC().Format("2006-01-02T15:04:05.0000000"), "timeZone": "UTC"}
}

// expand lists a series' occurrences in a window, as the derived source
// has them.
func (s *Server) expand(master *item, from, to time.Time) []ical.Occurrence {
	b, _ := json.Marshal(master.doc)
	var e graph.Event
	if json.Unmarshal(b, &e) != nil {
		return nil
	}
	src, err := graph.EventSource(e, e.ExceptionOccurrences, s.User)
	if err != nil {
		return nil
	}
	events, err := ical.Parse(src, ical.Options{Local: time.UTC})
	if err != nil {
		return nil
	}
	occ, _ := ical.Expand(events, from, to)
	return occ
}

func (s *Server) instances(w http.ResponseWriter, id string, q url.Values) {
	master := s.findEvent(id)
	if master == nil {
		fail(w, http.StatusNotFound, "ErrorItemNotFound", "The specified object was not found in the store.")
		return
	}
	from, err1 := time.Parse(time.RFC3339, q.Get("startDateTime"))
	to, err2 := time.Parse(time.RFC3339, q.Get("endDateTime"))
	if err1 != nil || err2 != nil {
		fail(w, http.StatusBadRequest, "ErrorInvalidParameter", "startDateTime and endDateTime are required")
		return
	}
	b, _ := json.Marshal(master.doc)
	var e graph.Event
	_ = json.Unmarshal(b, &e)
	series, err := graph.SeriesOf(e)
	if err != nil {
		fail(w, http.StatusInternalServerError, "ErrorInternalServerError", err.Error())
		return
	}
	var out []any
	for _, o := range s.expand(master, from, to) {
		doc := map[string]any{"id": master.id + "-occ-" + o.RecurrenceID, "type": "occurrence",
			"start": s.instantDoc(master, o.Start), "end": s.instantDoc(master, o.End), "isAllDay": o.AllDay}
		if exID, ok := series.Exception(o.RecurrenceID); ok {
			doc["id"], doc["type"] = exID, "exception"
		}
		out = append(out, doc)
	}
	page(w, out, "", "")
}

func (s *Server) eventsByUID(w http.ResponseWriter, cal *calendar, q url.Values) {
	filter := q.Get("$filter")
	uid, ok := strings.CutPrefix(filter, "iCalUId eq '")
	if !ok || !strings.HasSuffix(uid, "'") {
		fail(w, http.StatusBadRequest, "BadRequest", "graphtest filters only by iCalUId")
		return
	}
	uid = strings.ReplaceAll(strings.TrimSuffix(uid, "'"), "''", "'")
	var out []any
	for _, it := range cal.items {
		if !it.removed && it.doc["iCalUId"] == uid {
			out = append(out, it.doc)
		}
	}
	page(w, out, "", "")
}

func (s *Server) deleteEvent(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.record(w, r); !ok {
		return
	}
	if it := s.findEvent(id); it != nil {
		if stale(w, r, it.doc) {
			return
		}
		it.removed, it.version = true, s.next()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	master, ex, key := s.occurrence(id)
	if master == nil {
		fail(w, http.StatusNotFound, "ErrorItemNotFound", "The specified object was not found in the store.")
		return
	}
	at, err := graph.KeyTime(key)
	if ex != nil {
		if stale(w, r, ex) {
			return
		}
		at, err = time.Parse(time.RFC3339, ex["originalStart"].(string))
		list, _ := master.doc["exceptionOccurrences"].([]any)
		master.doc["exceptionOccurrences"] = slices.DeleteFunc(list, func(v any) bool {
			m, _ := v.(map[string]any)
			return m["id"] == id
		})
	}
	if err != nil {
		fail(w, http.StatusBadRequest, "ErrorInvalidIdMalformed", "Id is malformed.")
		return
	}
	cancelled, _ := master.doc["cancelledOccurrences"].([]any)
	master.doc["cancelledOccurrences"] = append(cancelled, "OID."+master.id+"."+s.day(master, at))
	s.touch(master)
	w.WriteHeader(http.StatusNoContent)
}

var responses = map[string]string{"accept": "accepted", "tentativelyAccept": "tentativelyAccepted", "decline": "declined"}

func (s *Server) answer(w http.ResponseWriter, r *http.Request, id, action string) {
	response, ok := responses[action]
	if !ok {
		fail(w, http.StatusBadRequest, "BadRequest", "graphtest does not serve "+action)
		return
	}
	if _, ok := s.record(w, r); !ok {
		return
	}
	status := map[string]any{"response": response, "time": "2026-10-16T12:00:00Z"}
	if it := s.findEvent(id); it != nil {
		it.doc["responseStatus"] = status
		s.touch(it)
		if action == "decline" {
			it.removed = true // Outlook takes a declined invitation off the calendar
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	master, ex, key := s.occurrence(id)
	if master != nil && ex == nil {
		ex = s.except(master, key)
	}
	if ex == nil {
		fail(w, http.StatusNotFound, "ErrorItemNotFound", "The specified object was not found in the store.")
		return
	}
	ex["responseStatus"] = status
	s.touch(master)
	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) getContact(w http.ResponseWriter, id string) {
	if it := s.findContact(id); it != nil {
		writeJSON(w, http.StatusOK, it.doc)
		return
	}
	fail(w, http.StatusNotFound, "ErrorItemNotFound", "The specified object was not found in the store.")
}

func (s *Server) createContact(w http.ResponseWriter, r *http.Request, f *folder) {
	fields, ok := s.record(w, r)
	if !ok {
		return
	}
	v := s.next()
	id := "AAMk-c" + strconv.Itoa(v)
	doc := map[string]any{"id": id, "parentFolderId": f.id}
	for k, val := range fields {
		doc[k] = val
	}
	it := &item{id: id, doc: doc}
	s.touch(it)
	f.items = append(f.items, it)
	writeJSON(w, http.StatusCreated, doc)
}

func (s *Server) updateContact(w http.ResponseWriter, r *http.Request, id string) {
	fields, ok := s.record(w, r)
	if !ok {
		return
	}
	it := s.findContact(id)
	if it == nil {
		fail(w, http.StatusNotFound, "ErrorItemNotFound", "The specified object was not found in the store.")
		return
	}
	if stale(w, r, it.doc) {
		return
	}
	for k, v := range fields {
		it.doc[k] = v
	}
	s.touch(it)
	writeJSON(w, http.StatusOK, it.doc)
}

func (s *Server) deleteContact(w http.ResponseWriter, r *http.Request, id string) {
	if _, ok := s.record(w, r); !ok {
		return
	}
	it := s.findContact(id)
	if it == nil {
		fail(w, http.StatusNotFound, "ErrorItemNotFound", "The specified object was not found in the store.")
		return
	}
	if stale(w, r, it.doc) {
		return
	}
	it.removed, it.version = true, s.next()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) setPhoto(w http.ResponseWriter, r *http.Request, id string) {
	body, ok := s.record(w, r)
	if !ok {
		return
	}
	it := s.findContact(id)
	if it == nil {
		fail(w, http.StatusNotFound, "ErrorItemNotFound", "The specified object was not found in the store.")
		return
	}
	it.photo, _ = body["$value"].([]byte)
	s.touch(it)
	w.WriteHeader(http.StatusOK)
}
