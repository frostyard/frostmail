package davx

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// xmlns declares the namespaces request bodies use: DAV, CalDAV, CardDAV,
// Apple's calendarserver.org (getctag) and Apple's iCal (calendar-color).
const xmlns = `xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav" ` +
	`xmlns:cr="urn:ietf:params:xml:ns:carddav" xmlns:cs="http://calendarserver.org/ns/" ` +
	`xmlns:a="http://apple.com/ns/ical/"`

func xmlBody(inner string) []byte {
	return []byte(`<?xml version="1.0" encoding="utf-8"?>` + "\n" + inner)
}

// propfind is a PROPFIND body asking for props, written as "d:getetag".
func propfind(props ...string) []byte {
	var b strings.Builder
	b.WriteString(`<d:propfind ` + xmlns + `><d:prop>`)
	for _, p := range props {
		b.WriteString("<" + p + "/>")
	}
	b.WriteString(`</d:prop></d:propfind>`)
	return xmlBody(b.String())
}

func escape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

type multistatus struct {
	Responses []response `xml:"DAV: response"`
	SyncToken string     `xml:"DAV: sync-token"`
}

type response struct {
	Hrefs     []string   `xml:"DAV: href"`
	Status    string     `xml:"DAV: status"`
	Propstats []propstat `xml:"DAV: propstat"`
}

type propstat struct {
	Prop   prop   `xml:"DAV: prop"`
	Status string `xml:"DAV: status"`
}

// prop holds every property davx asks for; a pointer is nil when the
// propstat does not carry the property.
type prop struct {
	ResourceType    *resourceType `xml:"DAV: resourcetype"`
	DisplayName     *string       `xml:"DAV: displayname"`
	ETag            *string       `xml:"DAV: getetag"`
	SyncToken       *string       `xml:"DAV: sync-token"`
	CTag            *string       `xml:"http://calendarserver.org/ns/ getctag"`
	Principal       *hrefs        `xml:"DAV: current-user-principal"`
	CalendarHome    *hrefs        `xml:"urn:ietf:params:xml:ns:caldav calendar-home-set"`
	AddressHome     *hrefs        `xml:"urn:ietf:params:xml:ns:carddav addressbook-home-set"`
	Color           *string       `xml:"http://apple.com/ns/ical/ calendar-color"`
	CalDescription  *string       `xml:"urn:ietf:params:xml:ns:caldav calendar-description"`
	CardDescription *string       `xml:"urn:ietf:params:xml:ns:carddav addressbook-description"`
	Components      *compSet      `xml:"urn:ietf:params:xml:ns:caldav supported-calendar-component-set"`
	Privileges      *privSet      `xml:"DAV: current-user-privilege-set"`
	Reports         *reportSet    `xml:"DAV: supported-report-set"`
	CalendarData    *string       `xml:"urn:ietf:params:xml:ns:caldav calendar-data"`
	AddressData     *string       `xml:"urn:ietf:params:xml:ns:carddav address-data"`
}

type marker struct{}

type resourceType struct {
	Collection  *marker `xml:"DAV: collection"`
	Calendar    *marker `xml:"urn:ietf:params:xml:ns:caldav calendar"`
	AddressBook *marker `xml:"urn:ietf:params:xml:ns:carddav addressbook"`
}

type hrefs struct {
	Hrefs []string `xml:"DAV: href"`
}

type compSet struct {
	Comps []struct {
		Name string `xml:"name,attr"`
	} `xml:"urn:ietf:params:xml:ns:caldav comp"`
}

type privSet struct {
	Privileges []struct {
		All          *marker `xml:"DAV: all"`
		Write        *marker `xml:"DAV: write"`
		WriteContent *marker `xml:"DAV: write-content"`
	} `xml:"DAV: privilege"`
}

// writable reports whether the privileges allow changing the collection's
// objects: DAV:all, DAV:write or DAV:write-content.
func (p *privSet) writable() bool {
	for _, priv := range p.Privileges {
		if priv.All != nil || priv.Write != nil || priv.WriteContent != nil {
			return true
		}
	}
	return false
}

type reportSet struct {
	Reports []struct {
		Report struct {
			SyncCollection *marker `xml:"DAV: sync-collection"`
		} `xml:"DAV: report"`
	} `xml:"DAV: supported-report"`
}

func (r *reportSet) syncCollection() bool {
	for _, rep := range r.Reports {
		if rep.Report.SyncCollection != nil {
			return true
		}
	}
	return false
}

func parseMultistatus(body []byte) (*multistatus, error) {
	var ms multistatus
	if err := xml.Unmarshal(body, &ms); err != nil {
		return nil, fmt.Errorf("davx: multistatus: %w", err)
	}
	return &ms, nil
}

// statusCode reads "HTTP/1.1 404 Not Found"; 0 when there is none.
func statusCode(s string) int {
	f := strings.Fields(s)
	if len(f) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(f[1])
	return n
}

// found merges the properties of the response's 200 propstats.
func (r *response) found() prop {
	var out prop
	for _, ps := range r.Propstats {
		if c := statusCode(ps.Status); c != 0 && c/100 != 2 {
			continue
		}
		p := ps.Prop
		merge(&out.ResourceType, p.ResourceType)
		merge(&out.DisplayName, p.DisplayName)
		merge(&out.ETag, p.ETag)
		merge(&out.SyncToken, p.SyncToken)
		merge(&out.CTag, p.CTag)
		merge(&out.Principal, p.Principal)
		merge(&out.CalendarHome, p.CalendarHome)
		merge(&out.AddressHome, p.AddressHome)
		merge(&out.Color, p.Color)
		merge(&out.CalDescription, p.CalDescription)
		merge(&out.CardDescription, p.CardDescription)
		merge(&out.Components, p.Components)
		merge(&out.Privileges, p.Privileges)
		merge(&out.Reports, p.Reports)
		merge(&out.CalendarData, p.CalendarData)
		merge(&out.AddressData, p.AddressData)
	}
	return out
}

func merge[T any](dst **T, src *T) {
	if *dst == nil {
		*dst = src
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// hrefPath returns an href as a path on base's origin: the server may
// write hrefs as paths or as URLs. ok is false for another origin.
func hrefPath(base *url.URL, href string) (string, bool) {
	href = strings.TrimSpace(href)
	u, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	if u.Host == "" {
		return u.EscapedPath(), true
	}
	if !sameOrigin(u, base) {
		return "", false
	}
	return u.EscapedPath(), true
}

// samePath compares two hrefs as decoded paths, ignoring a trailing
// slash: servers differ in how they escape.
func samePath(a, b string) bool {
	da, err1 := url.PathUnescape(a)
	db, err2 := url.PathUnescape(b)
	if err1 != nil || err2 != nil {
		return strings.TrimSuffix(a, "/") == strings.TrimSuffix(b, "/")
	}
	return strings.TrimSuffix(da, "/") == strings.TrimSuffix(db, "/")
}
