package davx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Discover finds the home set of kind for the user that opts signs in as.
// It asks start for the user's principal (first at /.well-known/carddav or
// caldav when start has no path), then the principal for its home set,
// following redirects. start's origin is trusted; every other origin it is
// sent to must pass opts.Trusted. It returns the home set's absolute URL.
func Discover(ctx context.Context, start string, kind Kind, opts Options) (string, error) {
	u, err := url.Parse(start)
	if err != nil {
		return "", fmt.Errorf("davx: %w", err)
	}
	if err := checkScheme(u, opts.AllowHTTP); err != nil {
		return "", err
	}
	d := discovery{opts: opts, start: u, kind: kind}
	var tries []*url.URL
	if u.Path == "" || u.Path == "/" {
		wk := *u
		wk.Path = "/.well-known/" + kind.String()
		tries = append(tries, &wk)
	}
	tries = append(tries, u)
	var last error
	for _, try := range tries {
		home, err := d.from(ctx, try)
		if err == nil {
			return home, nil
		}
		if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrUntrusted) || ctx.Err() != nil {
			return "", err
		}
		last = err
	}
	return "", last
}

type discovery struct {
	opts  Options
	start *url.URL
	kind  Kind
}

func (d discovery) homeProp() string {
	if d.kind == Calendars {
		return "c:calendar-home-set"
	}
	return "cr:addressbook-home-set"
}

func (d discovery) home(p prop) *hrefs {
	if d.kind == Calendars {
		return p.CalendarHome
	}
	return p.AddressHome
}

// from asks u for the principal and home set, then the principal.
func (d discovery) from(ctx context.Context, u *url.URL) (string, error) {
	p, at, err := d.propfind(ctx, u, "d:current-user-principal", d.homeProp())
	if err != nil {
		return "", err
	}
	if h := d.home(p); h != nil && len(h.Hrefs) > 0 {
		return d.resolve(at, h.Hrefs[0])
	}
	if p.Principal == nil || len(p.Principal.Hrefs) == 0 {
		return "", fmt.Errorf("davx: %s names no principal", at.Redacted())
	}
	principal, err := d.resolve(at, p.Principal.Hrefs[0])
	if err != nil {
		return "", err
	}
	pu, _ := url.Parse(principal)
	p, at, err = d.propfind(ctx, pu, d.homeProp())
	if err != nil {
		return "", err
	}
	if h := d.home(p); h != nil && len(h.Hrefs) > 0 {
		return d.resolve(at, h.Hrefs[0])
	}
	return "", fmt.Errorf("davx: principal %s has no %s home set", at.Redacted(), d.kind)
}

// resolve makes an href from a response at "at" absolute and checks that
// credentials may go there.
func (d discovery) resolve(at *url.URL, href string) (string, error) {
	ref, err := url.Parse(strings.TrimSpace(href))
	if err != nil {
		return "", fmt.Errorf("davx: href %q: %w", href, err)
	}
	u := at.ResolveReference(ref)
	if err := d.trust(u); err != nil {
		return "", err
	}
	return u.String(), nil
}

func (d discovery) trust(u *url.URL) error {
	if err := checkScheme(u, d.opts.AllowHTTP); err != nil {
		return err
	}
	if sameOrigin(u, d.start) || d.opts.Trusted != nil && d.opts.Trusted(u) {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrUntrusted, u.Redacted())
}

// propfind asks u for props at depth 0, following up to five redirects,
// and returns the properties found and the URL that answered.
func (d discovery) propfind(ctx context.Context, u *url.URL, props ...string) (prop, *url.URL, error) {
	for range 5 {
		if err := d.trust(u); err != nil {
			return prop{}, nil, err
		}
		c, err := New(u.String(), d.opts)
		if err != nil {
			return prop{}, nil, err
		}
		r := request{method: "PROPFIND", url: u, body: propfind(props...), ctype: xmlType, headers: map[string]string{"Depth": "0"}}
		resp, body, err := c.do(ctx, r)
		if err != nil {
			return prop{}, nil, err
		}
		switch resp.StatusCode {
		case http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther,
			http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
			loc, err := url.Parse(resp.Header.Get("Location"))
			if err != nil || resp.Header.Get("Location") == "" {
				return prop{}, nil, fmt.Errorf("davx: %s redirects nowhere", u.Redacted())
			}
			u = u.ResolveReference(loc)
			continue
		case http.StatusMultiStatus:
			ms, err := parseMultistatus(body)
			if err != nil {
				return prop{}, nil, err
			}
			if len(ms.Responses) == 0 {
				return prop{}, nil, fmt.Errorf("davx: %s answered PROPFIND with no response", u.Redacted())
			}
			return ms.Responses[0].found(), u, nil
		}
		return prop{}, nil, statusError(r, resp, body)
	}
	return prop{}, nil, fmt.Errorf("davx: too many redirects from %s", d.start.Redacted())
}

const xmlType = `application/xml; charset=utf-8`

// Collection is an address book or calendar.
type Collection struct {
	Href        string // its path, as the server wrote it
	Name        string
	Description string
	Color       string   // #rrggbb, or "" when the server has none
	Components  []string // a calendar's component types (VEVENT, VTODO); nil when the server does not say
	SyncToken   string
	CTag        string
	ReadOnly    bool // the user may not change its objects
	// Sync is whether the collection answers sync-collection: it says so in
	// supported-report-set or, when it lists no reports, has a sync token.
	Sync bool
}

// Collections lists the address books or calendars in the client's home
// set.
func (c *Client) Collections(ctx context.Context, kind Kind) ([]Collection, error) {
	props := []string{"d:resourcetype", "d:displayname", "d:sync-token", "cs:getctag",
		"d:current-user-privilege-set", "d:supported-report-set"}
	if kind == Calendars {
		props = append(props, "c:supported-calendar-component-set", "a:calendar-color", "c:calendar-description")
	} else {
		props = append(props, "cr:addressbook-description")
	}
	ms, err := c.multistatus(ctx, "PROPFIND", c.base.EscapedPath(), "1", propfind(props...))
	if err != nil {
		return nil, err
	}
	var out []Collection
	for _, r := range ms.Responses {
		if len(r.Hrefs) == 0 {
			continue
		}
		path, ok := hrefPath(c.base, r.Hrefs[0])
		p := r.found()
		if !ok || p.ResourceType == nil || samePath(path, c.base.EscapedPath()) {
			continue
		}
		if kind == Calendars && p.ResourceType.Calendar == nil || kind == AddressBooks && p.ResourceType.AddressBook == nil {
			continue
		}
		col := Collection{
			Href: path, Name: deref(p.DisplayName), SyncToken: deref(p.SyncToken), CTag: deref(p.CTag),
			Color: color(deref(p.Color)), ReadOnly: p.Privileges != nil && !p.Privileges.writable(),
		}
		col.Description = deref(p.CardDescription)
		if kind == Calendars {
			col.Description = deref(p.CalDescription)
		}
		if p.Components != nil {
			for _, comp := range p.Components.Comps {
				col.Components = append(col.Components, strings.ToUpper(comp.Name))
			}
		}
		col.Sync = p.Reports != nil && p.Reports.syncCollection() || p.Reports == nil && col.SyncToken != ""
		out = append(out, col)
	}
	return out, nil
}

// color turns Apple's #RRGGBBAA or #RRGGBB into #rrggbb; anything else
// becomes "".
func color(s string) string {
	s = strings.ToLower(s)
	if len(s) != 7 && len(s) != 9 || s[0] != '#' {
		return ""
	}
	for _, ch := range s[1:] {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return ""
		}
	}
	return s[:7]
}

// multistatus sends a PROPFIND or REPORT and parses its 207 answer.
func (c *Client) multistatus(ctx context.Context, method, href, depth string, body []byte) (*multistatus, error) {
	u, err := c.resolve(href)
	if err != nil {
		return nil, err
	}
	var h map[string]string
	if depth != "" {
		h = map[string]string{"Depth": depth}
	}
	r := request{method: method, url: u, body: body, ctype: xmlType, headers: h}
	resp, rbody, err := c.do(ctx, r)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusMultiStatus {
		if bytes.Contains(rbody, []byte("valid-sync-token")) || resp.StatusCode == http.StatusGone && method == "REPORT" {
			return nil, ErrInvalidToken
		}
		return nil, statusError(r, resp, rbody)
	}
	return parseMultistatus(rbody)
}

// Change is an object's href and its current ETag.
type Change struct {
	Href string
	ETag string // "" when the server did not say
}

// Delta is what changed in a collection since a sync token.
type Delta struct {
	Changed []Change // new or changed objects
	Deleted []string // hrefs of removed objects
	Token   string   // the token to ask from next time
	// More means the server cut the answer short (507): ask again from
	// Token for the rest.
	More bool
}

// Sync asks a collection what changed since token (sync-collection, RFC
// 6578); an empty token lists every object. A token the server no longer
// accepts gives ErrInvalidToken.
func (c *Client) Sync(ctx context.Context, coll, token string) (Delta, error) {
	body := xmlBody(`<d:sync-collection ` + xmlns + `><d:sync-token>` + escape(token) +
		`</d:sync-token><d:sync-level>1</d:sync-level><d:prop><d:getetag/></d:prop></d:sync-collection>`)
	ms, err := c.multistatus(ctx, "REPORT", coll, "0", body)
	if err != nil {
		return Delta{}, err
	}
	d := Delta{Token: strings.TrimSpace(ms.SyncToken)}
	for _, r := range ms.Responses {
		if len(r.Hrefs) == 0 {
			continue
		}
		path, ok := hrefPath(c.base, r.Hrefs[0])
		if !ok {
			continue
		}
		if samePath(path, coll) {
			if statusCode(r.Status) == http.StatusInsufficientStorage {
				d.More = true
			}
			continue
		}
		if code := statusCode(r.Status); code == http.StatusNotFound || code == http.StatusGone {
			d.Deleted = append(d.Deleted, path)
			continue
		}
		if strings.HasSuffix(path, "/") {
			continue // a member collection
		}
		d.Changed = append(d.Changed, Change{Href: path, ETag: deref(r.found().ETag)})
	}
	return d, nil
}

// List returns every object of a collection with its ETag (PROPFIND at
// depth 1), for servers without sync-collection and for verify.
func (c *Client) List(ctx context.Context, coll string) ([]Change, error) {
	ms, err := c.multistatus(ctx, "PROPFIND", coll, "1", propfind("d:getetag", "d:resourcetype"))
	if err != nil {
		return nil, err
	}
	var out []Change
	for _, r := range ms.Responses {
		if len(r.Hrefs) == 0 {
			continue
		}
		path, ok := hrefPath(c.base, r.Hrefs[0])
		p := r.found()
		if !ok || samePath(path, coll) || p.ResourceType != nil && p.ResourceType.Collection != nil {
			continue
		}
		out = append(out, Change{Href: path, ETag: deref(p.ETag)})
	}
	return out, nil
}

// Multiget fetches objects of a collection by href (addressbook-multiget
// or calendar-multiget). Objects come back under the hrefs the caller
// gave; hrefs the server does not have are returned in missing. An XML
// parser turns CRLF into LF (XML 1.0 §2.11), so data without any CR gets
// its CRLFs back.
func (c *Client) Multiget(ctx context.Context, kind Kind, coll string, hrefList []string) (objs []Object, missing []string, err error) {
	if len(hrefList) == 0 {
		return nil, nil, nil
	}
	var b strings.Builder
	root, data := "cr:addressbook-multiget", "cr:address-data"
	if kind == Calendars {
		root, data = "c:calendar-multiget", "c:calendar-data"
	}
	b.WriteString("<" + root + " " + xmlns + "><d:prop><d:getetag/><" + data + "/></d:prop>")
	for _, h := range hrefList {
		b.WriteString("<d:href>" + escape(h) + "</d:href>")
	}
	b.WriteString("</" + root + ">")
	ms, err := c.multistatus(ctx, "REPORT", coll, "1", xmlBody(b.String()))
	if err != nil {
		return nil, nil, err
	}
	got := make([]bool, len(hrefList))
	for _, r := range ms.Responses {
		for _, h := range r.Hrefs {
			path, ok := hrefPath(c.base, h)
			if !ok {
				continue
			}
			i := slices.IndexFunc(hrefList, func(want string) bool { return samePath(want, path) })
			if i < 0 || got[i] {
				continue
			}
			p := r.found()
			raw := p.AddressData
			if kind == Calendars {
				raw = p.CalendarData
			}
			if raw == nil {
				continue // a 404 propstat, or no data: missing below
			}
			got[i] = true
			objs = append(objs, Object{Href: hrefList[i], ETag: deref(p.ETag), Data: restoreCRLF(*raw)})
		}
	}
	for i, ok := range got {
		if !ok {
			missing = append(missing, hrefList[i])
		}
	}
	return objs, missing, nil
}

// restoreCRLF trims the whitespace a server may put around the data and,
// when the data holds no CR, ends its lines in CRLF again.
func restoreCRLF(s string) []byte {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "\r") {
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	return []byte(s + "\r\n")
}
