package davtest

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

const (
	nsDAV     = "DAV:"
	nsCalDAV  = "urn:ietf:params:xml:ns:caldav"
	nsCardDAV = "urn:ietf:params:xml:ns:carddav"
	nsCS      = "http://calendarserver.org/ns/"
	nsApple   = "http://apple.com/ns/ical/"
)

var prefixes = map[string]string{nsDAV: "d", nsCalDAV: "c", nsCardDAV: "cr", nsCS: "cs", nsApple: "a"}

// props maps a property to its inner XML for one resource.
type props map[xml.Name]string

func dav(local string) xml.Name     { return xml.Name{Space: nsDAV, Local: local} }
func caldav(local string) xml.Name  { return xml.Name{Space: nsCalDAV, Local: local} }
func carddav(local string) xml.Name { return xml.Name{Space: nsCardDAV, Local: local} }

// msResponse is one response of a multistatus: a status, or the
// properties asked for split into found and not found.
type msResponse struct {
	href    string
	status  int
	found   props
	missing []xml.Name
}

func writeMultistatus(w http.ResponseWriter, rs []msResponse, syncToken string) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n<d:multistatus")
	for _, ns := range []string{nsDAV, nsCalDAV, nsCardDAV, nsCS, nsApple} {
		fmt.Fprintf(&b, ` xmlns:%s="%s"`, prefixes[ns], ns)
	}
	b.WriteString(">")
	for _, r := range rs {
		b.WriteString("<d:response><d:href>" + escape(escapePath(r.href)) + "</d:href>")
		if r.status != 0 {
			fmt.Fprintf(&b, "<d:status>HTTP/1.1 %d %s</d:status>", r.status, http.StatusText(r.status))
		}
		if len(r.found) > 0 {
			b.WriteString("<d:propstat><d:prop>")
			for _, n := range sortedNames(r.found) {
				b.WriteString(element(n, r.found[n]))
			}
			b.WriteString("</d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat>")
		}
		if len(r.missing) > 0 {
			b.WriteString("<d:propstat><d:prop>")
			for _, n := range r.missing {
				b.WriteString(element(n, ""))
			}
			b.WriteString("</d:prop><d:status>HTTP/1.1 404 Not Found</d:status></d:propstat>")
		}
		b.WriteString("</d:response>")
	}
	if syncToken != "" {
		b.WriteString("<d:sync-token>" + escape(syncToken) + "</d:sync-token>")
	}
	b.WriteString("</d:multistatus>")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write(b.Bytes())
}

func sortedNames(p props) []xml.Name {
	names := make([]xml.Name, 0, len(p))
	for n := range p {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b xml.Name) int { return strings.Compare(a.Space+a.Local, b.Space+b.Local) })
	return names
}

func element(n xml.Name, inner string) string {
	tag, decl := n.Local, ""
	if p, ok := prefixes[n.Space]; ok {
		tag = p + ":" + n.Local
	} else {
		decl = ` xmlns="` + escape(n.Space) + `"`
	}
	if inner == "" {
		return "<" + tag + decl + "/>"
	}
	return "<" + tag + decl + ">" + inner + "</" + tag + ">"
}

func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func escapePath(p string) string { return (&url.URL{Path: p}).EscapedPath() }

func hrefXML(p string) string { return "<d:href>" + escape(escapePath(p)) + "</d:href>" }

// requested reads the property names a PROPFIND or REPORT body's d:prop
// asks for; all is true for an empty body or d:allprop.
func requested(body []byte) (names []xml.Name, all bool) {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, true
	}
	d := xml.NewDecoder(bytes.NewReader(body))
	depth, inProp := 0, -1
	for {
		tok, err := d.Token()
		if err != nil {
			return names, false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch {
			case t.Name == dav("allprop"):
				return nil, true
			case t.Name == dav("prop") && inProp < 0:
				inProp = depth
			case inProp >= 0 && depth == inProp+1:
				names = append(names, t.Name)
			}
		case xml.EndElement:
			if depth == inProp {
				inProp = -1
			}
			depth--
		}
	}
}

// answer splits the resource's properties into those asked for and found,
// and those asked for and missing.
func answer(path string, have props, names []xml.Name, all bool) msResponse {
	r := msResponse{href: path, found: props{}}
	if all {
		r.found = have
		return r
	}
	for _, n := range names {
		if v, ok := have[n]; ok {
			r.found[n] = v
		} else {
			r.missing = append(r.missing, n)
		}
	}
	return r
}

func (s *Server) propfind(w http.ResponseWriter, r *http.Request, body []byte) {
	names, all := requested(body)
	path := r.URL.Path
	depth1 := r.Header.Get("Depth") == "1"
	var rs []msResponse
	switch {
	case path == "/" || path == "/dav/" || path == PrincipalPath:
		have := props{dav("current-user-principal"): hrefXML(PrincipalPath), dav("resourcetype"): "<d:collection/>"}
		if path == PrincipalPath {
			have[caldav("calendar-home-set")] = hrefXML(CalendarsHome)
			have[carddav("addressbook-home-set")] = hrefXML(ContactsHome)
			have[dav("resourcetype")] = "<d:principal/>"
		}
		rs = append(rs, answer(path, have, names, all))
	case path == ContactsHome || path == CalendarsHome:
		rs = append(rs, answer(path, props{dav("resourcetype"): "<d:collection/>"}, names, all))
		if depth1 {
			for _, p := range slices.Sorted(maps.Keys(s.colls)) {
				if c := s.colls[p]; c.calendar == (path == CalendarsHome) {
					rs = append(rs, answer(p, s.collectionProps(c), names, all))
				}
			}
		}
	case s.colls[path] != nil:
		c := s.colls[path]
		rs = append(rs, answer(path, s.collectionProps(c), names, all))
		if depth1 {
			for _, p := range slices.Sorted(maps.Keys(c.objects)) {
				rs = append(rs, answer(p, objectProps(c, c.objects[p]), names, all))
			}
		}
	case s.parent(path) != nil && s.parent(path).objects[path] != nil:
		c := s.parent(path)
		rs = append(rs, answer(path, objectProps(c, c.objects[path]), names, all))
	default:
		http.NotFound(w, r)
		return
	}
	writeMultistatus(w, rs, "")
}

func (s *Server) collectionProps(c *collection) props {
	p := props{
		dav("displayname"):  escape(c.name),
		cs("getctag"):       c.ctag(),
		dav("resourcetype"): "<d:collection/><cr:addressbook/>",
	}
	privs := "<d:privilege><d:read/></d:privilege>"
	if !c.readOnly {
		privs += "<d:privilege><d:write/></d:privilege>"
	}
	p[dav("current-user-privilege-set")] = privs
	reports := "<d:supported-report><d:report><cr:addressbook-multiget/></d:report></d:supported-report>"
	if c.calendar {
		p[dav("resourcetype")] = "<d:collection/><c:calendar/>"
		reports = "<d:supported-report><d:report><c:calendar-multiget/></d:report></d:supported-report>"
		var comps strings.Builder
		for _, comp := range c.comps {
			comps.WriteString(`<c:comp name="` + escape(comp) + `"/>`)
		}
		p[caldav("supported-calendar-component-set")] = comps.String()
		if c.color != "" {
			p[xml.Name{Space: nsApple, Local: "calendar-color"}] = escape(strings.ToUpper(c.color) + "FF")
		}
	}
	if !s.opts.NoSync {
		p[dav("sync-token")] = escape(s.token())
		reports += "<d:supported-report><d:report><d:sync-collection/></d:report></d:supported-report>"
	}
	p[dav("supported-report-set")] = reports
	return p
}

func cs(local string) xml.Name { return xml.Name{Space: nsCS, Local: local} }

func objectProps(c *collection, o *object) props {
	return props{
		dav("getetag"):        escape(o.etag),
		dav("getcontenttype"): escape(c.contentType()),
		dav("resourcetype"):   "",
	}
}

func (s *Server) report(w http.ResponseWriter, r *http.Request, body []byte) {
	kind, _, _ := describeReport(body)
	switch kind {
	case "sync-collection":
		s.syncCollection(w, r, body)
	case "addressbook-multiget", "calendar-multiget":
		s.multiget(w, body, kind == "calendar-multiget")
	default:
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<d:error xmlns:d="DAV:"><d:supported-report/></d:error>`))
	}
}

func (s *Server) syncCollection(w http.ResponseWriter, r *http.Request, body []byte) {
	c := s.colls[r.URL.Path]
	if c == nil {
		http.NotFound(w, r)
		return
	}
	_, token, _ := describeReport(body)
	since, ok := s.parseToken(strings.TrimSpace(token))
	if s.opts.NoSync || !ok {
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		if s.opts.NoSync {
			_, _ = w.Write([]byte(`<d:error xmlns:d="DAV:"><d:supported-report/></d:error>`))
		} else {
			_, _ = w.Write([]byte(`<d:error xmlns:d="DAV:"><d:valid-sync-token/></d:error>`))
		}
		return
	}
	// The last change of each path after since, in change order.
	last := map[string]int{}
	for _, ch := range c.changes {
		if ch.seq > since {
			last[ch.path] = ch.seq
		}
	}
	paths := slices.Collect(maps.Keys(last))
	slices.SortFunc(paths, func(a, b string) int { return last[a] - last[b] })
	upTo, more := s.seq, false
	if s.opts.Truncate > 0 && len(paths) > s.opts.Truncate {
		paths, more = paths[:s.opts.Truncate], true
		upTo = last[paths[len(paths)-1]]
	}
	var rs []msResponse
	for _, p := range paths {
		o := c.objects[p]
		if o == nil {
			if since > 0 {
				rs = append(rs, msResponse{href: p, status: http.StatusNotFound})
			}
			continue
		}
		rs = append(rs, msResponse{href: p, found: props{dav("getetag"): escape(o.etag)}})
	}
	if more {
		rs = append(rs, msResponse{href: c.path, status: http.StatusInsufficientStorage})
	}
	writeMultistatus(w, rs, fmt.Sprintf("http://davtest.test/sync/%d/%d", s.epoch, upTo))
}

// parseToken reads a token this server handed out in its current epoch;
// "" is the start of time.
func (s *Server) parseToken(t string) (int, bool) {
	if t == "" {
		return 0, true
	}
	rest, ok := strings.CutPrefix(t, "http://davtest.test/sync/")
	if !ok {
		return 0, false
	}
	e, n, ok := strings.Cut(rest, "/")
	epoch, err1 := strconv.Atoi(e)
	seq, err2 := strconv.Atoi(n)
	if !ok || err1 != nil || err2 != nil || epoch != s.epoch || seq > s.seq {
		return 0, false
	}
	return seq, true
}

func (s *Server) multiget(w http.ResponseWriter, body []byte, calendar bool) {
	var req struct {
		Hrefs []string `xml:"DAV: href"`
	}
	if err := xml.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad multiget", http.StatusBadRequest)
		return
	}
	data := carddav("address-data")
	if calendar {
		data = caldav("calendar-data")
	}
	var rs []msResponse
	for _, h := range req.Hrefs {
		p, err := url.PathUnescape(strings.TrimSpace(h))
		c := s.parent(p)
		if err != nil || c == nil || c.objects[p] == nil || c.calendar != calendar {
			rs = append(rs, msResponse{href: p, status: http.StatusNotFound})
			continue
		}
		o := c.objects[p]
		text := escape(string(o.data))
		if s.opts.CDATA {
			text = "<![CDATA[" + string(o.data) + "]]>"
		}
		rs = append(rs, msResponse{href: p, found: props{dav("getetag"): escape(o.etag), data: text}})
	}
	writeMultistatus(w, rs, "")
}
