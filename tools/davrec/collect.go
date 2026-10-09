package main

import (
	"encoding/base64"
	"encoding/json/v2"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/frostyard/frostmail/internal/contentline"
	"github.com/frostyard/frostmail/internal/httprec"
)

const onePixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII="

var emailPattern = regexp.MustCompile(`[\pL\pN.!#$&'*+?^_` + "`" + `{|}~-]+@[\pL\pN-]+(?:\.[\pL\pN-]+)+`)
var folds = regexp.MustCompile("\r?\n[ \t]")

func (s *scrubber) prepare(xs []httprec.Exchange) error {
	for i := range xs {
		x := &xs[i]
		var err error
		x.ReqBody, err = s.prepareBody(x.ReqBody, x.ReqBase64, x.ReqHeader)
		if err != nil {
			return err
		}
		x.RespBody, err = s.prepareBody(x.RespBody, x.RespBase64, x.RespHeader)
		if err != nil {
			return err
		}
		for _, text := range exchangeText(*x) {
			s.collectEmails(text)
		}
	}
	return nil
}

func exchangeText(x httprec.Exchange) []string {
	out := []string{x.URL, x.Err}
	if !x.ReqBase64 {
		out = append(out, x.ReqBody)
	}
	if !x.RespBase64 {
		out = append(out, x.RespBody)
	}
	for _, h := range []http.Header{x.ReqHeader, x.RespHeader} {
		for k, vs := range h {
			out = append(out, k)
			out = append(out, vs...)
		}
	}
	return out
}

func (s *scrubber) collectEmails(text string) {
	for _, email := range emailPattern.FindAllString(decoded(text), -1) {
		// A resource extension is not part of the address in a DAV href.
		email = strings.TrimSuffix(strings.TrimSuffix(email, ".vcf"), ".ics")
		dot := strings.LastIndexByte(email, '.')
		s.excluded[strings.ToLower(email[dot+1:])] = true
		s.collectValue(email)
	}
}

func (s *scrubber) collectValue(text string) {
	text = decoded(text)
	eligible := false
	for _, w := range words(text) {
		lower := strings.ToLower(w)
		if len([]rune(w)) >= 3 && !vocabulary[lower] {
			s.secrets[lower] = true
			eligible = true
		}
	}
	// The leak check looks for a value without the dates and zones
	// scrubbing keeps.
	if v := strings.ToLower(maskKept(text)); eligible && len([]rune(text)) >= 6 && len(strings.TrimSpace(v)) >= 3 {
		s.values[v] = true
	}
}

func (s *scrubber) prepareBody(body string, binary bool, h http.Header) (string, error) {
	if binary {
		if !strings.HasPrefix(strings.ToLower(h.Get("Content-Type")), "image/") {
			return "", errors.New("cannot scrub a non-image binary body")
		}
		return onePixel, nil
	}
	trimmed := strings.TrimSpace(body)
	switch {
	case strings.HasPrefix(strings.ToUpper(trimmed), "BEGIN:"):
		return s.content(body)
	case strings.HasPrefix(trimmed, "<"):
		return s.xmlBody(body)
	case strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "["):
		var v any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			return "", errors.New("cannot parse a JSON body")
		}
		s.jsonValues(v, false)
	}
	return body, nil
}

func (s *scrubber) jsonValues(v any, links bool) {
	switch v := v.(type) {
	case []any:
		for _, item := range v {
			s.jsonValues(item, links)
		}
	case map[string]any:
		// A task list's ID encodes a number of the account's.
		list := v["kind"] == "tasks#taskList"
		for k, item := range v {
			s.syntax(k)
			if kind, ok := item.(string); ok && k == "kind" {
				s.syntax(kind)
			}
			if text, ok := item.(string); ok && (k == "title" || k == "notes" || links && (k == "description" || k == "link") ||
				list && k == "id") {
				s.collectValue(text)
			}
			s.jsonValues(item, k == "links" || links)
		}
	}
}

// XML token offsets let us patch embedded content without reserializing
// the envelope: request bodies must retain the client's exact formatting.
func (s *scrubber) xmlBody(body string) (string, error) {
	s.xmlNames(body)
	d := xml.NewDecoder(strings.NewReader(body))
	var stack []string
	var edits []contentline.Edit
	for {
		start := int(d.InputOffset())
		token, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", errors.New("cannot parse an XML body")
		}
		switch t := token.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name.Local)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			name := stack[len(stack)-1]
			text := string(t)
			if name == "displayname" || name == "calendar-description" || name == "addressbook-description" {
				s.collectValue(text)
			}
			if name != "address-data" && name != "calendar-data" {
				continue
			}
			changed, err := s.content(text)
			if err != nil {
				return "", err
			}
			if changed == text {
				continue
			}
			var b strings.Builder
			if err := xml.EscapeText(&b, []byte(changed)); err != nil {
				return "", err
			}
			edits = append(edits, contentline.Edit{Start: start, End: int(d.InputOffset()), Text: b.String()})
		}
	}
	out, err := contentline.Apply([]byte(body), edits...)
	return string(out), err
}

// xmlNames marks the words of body's element and attribute names, their
// prefixes, and attribute values (namespaces, a comp's name) as syntax.
func (s *scrubber) xmlNames(body string) {
	d := xml.NewDecoder(strings.NewReader(body))
	for {
		token, err := d.RawToken()
		if err != nil {
			return
		}
		switch t := token.(type) {
		case xml.StartElement:
			s.syntax(t.Name.Space, t.Name.Local)
			for _, a := range t.Attr {
				s.syntax(a.Name.Space, a.Name.Local, a.Value)
			}
		case xml.EndElement:
			s.syntax(t.Name.Space, t.Name.Local)
		}
	}
}

// syntaxParams are the parameters whose values are enumerations or
// references, never a person's text. TZID is not one: a zone is kept by
// its own rule (zones), and its city may be a personal place elsewhere.
var syntaxParams = fieldSet("TYPE VALUE ENCODING CHARSET PREF PARTSTAT ROLE CUTYPE RSVP RELTYPE RELATED FBTYPE FMTTYPE " +
	"MEDIATYPE RANGE SCHEDULE-AGENT SCHEDULE-STATUS SCHEDULE-FORCE-SEND LANGUAGE CALSCALE")

// syntax marks every word of texts as never personal: it is part of a
// format, and replacing it would change what parses or what the replay
// matches. A personal word that is also syntax is kept everywhere.
func (s *scrubber) syntax(texts ...string) {
	for _, text := range texts {
		for _, w := range words(text) {
			s.excluded[strings.ToLower(w)] = true
		}
	}
}

func (s *scrubber) content(body string) (string, error) {
	body = folds.ReplaceAllString(body, "")
	cs, err := contentline.Parse([]byte(body))
	if err != nil {
		return "", errors.New("cannot parse vCard or iCalendar content")
	}
	var edits []contentline.Edit
	for _, c := range cs {
		s.component(c, body, &edits)
	}
	out, err := contentline.Apply([]byte(body), edits...)
	return string(out), err
}

func (s *scrubber) component(c *contentline.Component, body string, edits *[]contentline.Edit) {
	s.syntax(c.Name)
	for _, p := range c.Props {
		s.syntax(p.Group, p.Name)
		for _, q := range p.Params {
			s.syntax(q.Name)
			if syntaxParams[q.Name] {
				s.syntax(q.Values...)
			}
		}
		personal := calendarFields[p.Name]
		if c.Name == "VCARD" {
			personal = cardFields[p.Name]
		}
		if personal || strings.HasPrefix(p.Name, "X-") {
			s.collectValue(p.Text())
		}
		if p.Name == "ORGANIZER" || p.Name == "ATTENDEE" {
			for _, q := range p.Params {
				if q.Name == "CN" || q.Name == "EMAIL" {
					for _, v := range q.Values {
						s.collectValue(v)
					}
				}
			}
		}
		if c.Name != "VCARD" || p.Name != "PHOTO" && p.Name != "LOGO" {
			continue
		}
		value := photoValue(p)
		if value == "" {
			continue
		}
		line := body[p.Start:p.End]
		at := strings.LastIndex(line, p.Value)
		if at >= 0 {
			*edits = append(*edits, contentline.Edit{Start: p.Start + at, End: p.Start + at + len(p.Value), Text: value})
		}
	}
	for _, child := range c.Children {
		s.component(child, body, edits)
	}
}

func photoValue(p contentline.Prop) string {
	if strings.HasPrefix(strings.ToLower(p.Value), "data:") {
		if strings.Contains(strings.ToLower(p.Value), ";base64,") {
			return "data:image/png;base64," + onePixel
		}
		png, _ := base64.StdEncoding.DecodeString(onePixel)
		var b strings.Builder
		b.WriteString("data:image/png,")
		for _, v := range png {
			b.WriteString(percentByte(v))
		}
		return b.String()
	}
	if p.HasParam("ENCODING", "b") || p.HasParam("ENCODING", "base64") {
		return onePixel
	}
	return ""
}
