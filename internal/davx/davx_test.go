package davx_test

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/davtest"
	"github.com/frostyard/frostmail/internal/davx"
)

func basic(user, pass string) func(context.Context) (string, error) {
	return func(context.Context) (string, error) {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)), nil
	}
}

func opts(s *davtest.Server) davx.Options {
	return davx.Options{HTTP: s.Client, Authorization: basic(davtest.User, davtest.Password)}
}

func card(uid, name string) []byte {
	return []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:" + uid + "\r\nFN:" + name + "\r\nEND:VCARD\r\n")
}

func client(t *testing.T, s *davtest.Server, home string) *davx.Client {
	t.Helper()
	c, err := davx.New(s.URL+home, opts(s))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDiscover(t *testing.T) {
	s := davtest.New(t, davtest.Options{})
	for _, tc := range []struct {
		start string
		kind  davx.Kind
		want  string
	}{
		{s.URL, davx.AddressBooks, davtest.ContactsHome},
		{s.URL + "/", davx.Calendars, davtest.CalendarsHome},
		{s.URL + "/dav/", davx.Calendars, davtest.CalendarsHome},
		{s.URL + davtest.PrincipalPath, davx.AddressBooks, davtest.ContactsHome},
	} {
		got, err := davx.Discover(t.Context(), tc.start, tc.kind, opts(s))
		if err != nil || got != s.URL+tc.want {
			t.Errorf("Discover(%s, %v) = %q, %v; want %q", tc.start, tc.kind, got, err, s.URL+tc.want)
		}
	}
	noWK := davtest.New(t, davtest.Options{NoWellKnown: true})
	if got, err := davx.Discover(t.Context(), noWK.URL, davx.Calendars, opts(noWK)); err != nil || got != noWK.URL+davtest.CalendarsHome {
		t.Errorf("without well-known: %q, %v", got, err)
	}
	bad := opts(s)
	bad.Authorization = basic(davtest.User, "wrong")
	if _, err := davx.Discover(t.Context(), s.URL, davx.AddressBooks, bad); !errors.Is(err, davx.ErrUnauthorized) {
		t.Errorf("wrong password: %v", err)
	}
	bearer := davtest.New(t, davtest.Options{Bearer: "tok"})
	o := opts(bearer)
	o.Authorization = func(context.Context) (string, error) { return "Bearer tok", nil }
	if _, err := davx.Discover(t.Context(), bearer.URL, davx.AddressBooks, o); err != nil {
		t.Errorf("bearer: %v", err)
	}
}

// TestDiscoverTrust: credentials follow a redirect to another origin only
// when Options.Trusted accepts it.
func TestDiscoverTrust(t *testing.T) {
	s := davtest.New(t, davtest.Options{})
	var sawAuth bool
	front := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = sawAuth || r.Header.Get("Authorization") != ""
		http.Redirect(w, r, s.URL+"/dav/", http.StatusMovedPermanently)
	}))
	t.Cleanup(front.Close)
	o := opts(s)
	if _, err := davx.Discover(t.Context(), front.URL, davx.AddressBooks, o); !errors.Is(err, davx.ErrUntrusted) {
		t.Errorf("untrusted redirect: %v", err)
	}
	target, _ := url.Parse(s.URL)
	o.Trusted = func(u *url.URL) bool { return u.Host == target.Host }
	got, err := davx.Discover(t.Context(), front.URL, davx.AddressBooks, o)
	if err != nil || got != s.URL+davtest.ContactsHome {
		t.Errorf("trusted redirect: %q, %v", got, err)
	}
	if !sawAuth {
		t.Error("the starting origin got no credentials")
	}
	if _, err := davx.New("http://example.com/", davx.Options{}); !errors.Is(err, davx.ErrUntrusted) {
		t.Errorf("plain HTTP: %v", err)
	}
	if _, err := davx.New("http://example.com/", davx.Options{AllowHTTP: true}); err != nil {
		t.Errorf("plain HTTP allowed: %v", err)
	}
}

func TestCollections(t *testing.T) {
	s := davtest.New(t, davtest.Options{})
	s.AddressBook("default", "Contacts")
	ro := s.AddressBook("shared", "Shared")
	s.SetReadOnly(ro, true)
	s.Calendar("work", "Work", "#3366cc", "VEVENT", "VTODO")
	s.Calendar("home", "Home", "")

	books, err := client(t, s, davtest.ContactsHome).Collections(t.Context(), davx.AddressBooks)
	if err != nil {
		t.Fatal(err)
	}
	if len(books) != 2 || books[0].Href != davtest.ContactsHome+"default/" || books[0].Name != "Contacts" ||
		books[0].ReadOnly || !books[1].ReadOnly || !books[0].Sync || books[0].SyncToken == "" || books[0].CTag == "" {
		t.Errorf("address books = %+v", books)
	}
	cals, err := client(t, s, davtest.CalendarsHome).Collections(t.Context(), davx.Calendars)
	if err != nil {
		t.Fatal(err)
	}
	if len(cals) != 2 {
		t.Fatalf("calendars = %+v", cals)
	}
	home, work := cals[0], cals[1]
	if work.Color != "#3366cc" || !slices.Equal(work.Components, []string{"VEVENT", "VTODO"}) || home.Color != "" ||
		!slices.Equal(home.Components, []string{"VEVENT"}) {
		t.Errorf("calendars = %+v", cals)
	}

	noSync := davtest.New(t, davtest.Options{NoSync: true})
	noSync.AddressBook("default", "Contacts")
	books, err = client(t, noSync, davtest.ContactsHome).Collections(t.Context(), davx.AddressBooks)
	if err != nil || len(books) != 1 || books[0].Sync {
		t.Errorf("without sync-collection: %+v, %v", books, err)
	}
}

func TestSyncAndMultiget(t *testing.T) {
	for _, cdata := range []bool{false, true} {
		s := davtest.New(t, davtest.Options{CDATA: cdata})
		book := s.AddressBook("default", "Contacts")
		a, b, c := book+"a.vcf", book+"b c@x.vcf", book+"c.vcf"
		for _, p := range []string{a, b, c} {
			s.Put(p, card(p, "Person "+p))
		}
		cl := client(t, s, davtest.ContactsHome)
		d, err := cl.Sync(t.Context(), book, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Changed) != 3 || len(d.Deleted) != 0 || d.Token == "" || d.More {
			t.Fatalf("first sync = %+v", d)
		}
		var hrefs []string
		for _, ch := range d.Changed {
			hrefs = append(hrefs, ch.Href)
			if ch.ETag == "" {
				t.Errorf("%s has no ETag", ch.Href)
			}
		}
		if !slices.Contains(hrefs, strings.Replace(book+"b%20c@x.vcf", " ", "%20", 1)) {
			t.Errorf("hrefs = %q, want b c@x.vcf escaped", hrefs)
		}
		objs, missing, err := cl.Multiget(t.Context(), davx.AddressBooks, book, append(hrefs, book+"gone.vcf"))
		if err != nil {
			t.Fatal(err)
		}
		if len(objs) != 3 || !slices.Equal(missing, []string{book + "gone.vcf"}) {
			t.Fatalf("multiget = %d objects, missing %q", len(objs), missing)
		}
		for _, o := range objs {
			p, _ := url.PathUnescape(o.Href)
			want, etag, _ := s.Object(p)
			if string(o.Data) != string(want) || o.ETag != etag {
				t.Errorf("cdata %v: %s = %q (%s), want %q (%s)", cdata, o.Href, o.Data, o.ETag, want, etag)
			}
		}

		// Another client changes a, deletes c and adds d.
		s.Put(a, card(a, "Changed"))
		s.Delete(c)
		s.Put(book+"d.vcf", card("d", "New"))
		d2, err := cl.Sync(t.Context(), book, d.Token)
		if err != nil {
			t.Fatal(err)
		}
		if len(d2.Changed) != 2 || !slices.Equal(d2.Deleted, []string{c}) || d2.Token == d.Token {
			t.Errorf("second sync = %+v", d2)
		}
		d3, err := cl.Sync(t.Context(), book, d2.Token)
		if err != nil || len(d3.Changed)+len(d3.Deleted) != 0 {
			t.Errorf("quiet sync = %+v, %v", d3, err)
		}
	}
}

func TestSyncTruncatedAndInvalid(t *testing.T) {
	s := davtest.New(t, davtest.Options{Truncate: 2})
	book := s.AddressBook("default", "Contacts")
	for _, n := range []string{"1", "2", "3", "4", "5"} {
		s.Put(book+n+".vcf", card(n, n))
	}
	cl := client(t, s, davtest.ContactsHome)
	var all []string
	token := ""
	for range 10 {
		d, err := cl.Sync(t.Context(), book, token)
		if err != nil {
			t.Fatal(err)
		}
		for _, ch := range d.Changed {
			all = append(all, ch.Href)
		}
		token = d.Token
		if !d.More {
			break
		}
	}
	if len(all) != 5 {
		t.Errorf("truncated syncs found %q", all)
	}
	s.InvalidateTokens()
	if _, err := cl.Sync(t.Context(), book, token); !errors.Is(err, davx.ErrInvalidToken) {
		t.Errorf("invalidated token: %v", err)
	}
	list, err := cl.List(t.Context(), book)
	if err != nil || len(list) != 5 || list[0].ETag == "" {
		t.Errorf("List = %+v, %v", list, err)
	}
}

func TestWrites(t *testing.T) {
	s := davtest.New(t, davtest.Options{})
	book := s.AddressBook("default", "Contacts")
	cl := client(t, s, davtest.ContactsHome)
	ctx := t.Context()
	p := book + "new.vcf"

	etag, err := cl.Put(ctx, p, card("new", "One"), davx.AddressBooks, "")
	if err != nil || etag == "" {
		t.Fatalf("create = %q, %v", etag, err)
	}
	if _, err := cl.Put(ctx, p, card("new", "Two"), davx.AddressBooks, ""); !errors.Is(err, davx.ErrPrecondition) {
		t.Errorf("create over an existing object: %v", err)
	}
	s.Put(p, card("new", "Elsewhere"))
	if _, err := cl.Put(ctx, p, card("new", "Three"), davx.AddressBooks, etag); !errors.Is(err, davx.ErrPrecondition) {
		t.Errorf("update with a stale ETag: %v", err)
	}
	obj, err := cl.Get(ctx, p)
	if err != nil || !strings.Contains(string(obj.Data), "Elsewhere") {
		t.Fatalf("Get = %+v, %v", obj, err)
	}
	etag, err = cl.Put(ctx, p, card("new", "Four"), davx.AddressBooks, obj.ETag)
	if err != nil || etag == obj.ETag {
		t.Errorf("update = %q, %v", etag, err)
	}
	if err := cl.Delete(ctx, p, obj.ETag); !errors.Is(err, davx.ErrPrecondition) {
		t.Errorf("delete with a stale ETag: %v", err)
	}
	if err := cl.Delete(ctx, p, etag); err != nil {
		t.Errorf("delete: %v", err)
	}
	if err := cl.Delete(ctx, p, ""); err != nil {
		t.Errorf("delete again: %v", err)
	}
	if _, err := cl.Get(ctx, p); !errors.Is(err, davx.ErrNotFound) {
		t.Errorf("Get after delete: %v", err)
	}
	s.SetReadOnly(book, true)
	var se *davx.StatusError
	if _, err := cl.Put(ctx, p, card("new", "Five"), davx.AddressBooks, ""); !errors.As(err, &se) || se.Code != http.StatusForbidden {
		t.Errorf("read-only: %v", err)
	}
	if _, err := cl.Get(ctx, "https://elsewhere.example/x.vcf"); !errors.Is(err, davx.ErrUntrusted) {
		t.Errorf("another origin: %v", err)
	}

	quiet := davtest.New(t, davtest.Options{NoETagOnPut: true})
	qbook := quiet.AddressBook("default", "Contacts")
	if etag, err := client(t, quiet, davtest.ContactsHome).Put(ctx, qbook+"x.vcf", card("x", "X"), davx.AddressBooks, ""); err != nil || etag != "" {
		t.Errorf("PUT without an ETag header = %q, %v", etag, err)
	}
	var methods []string
	for _, r := range s.Requests() {
		methods = append(methods, r.Method)
		if r.Header.Get("Authorization") == "" {
			t.Errorf("%s %s without credentials", r.Method, r.Path)
		}
	}
	if !slices.Contains(methods, http.MethodPut) || !slices.Contains(methods, http.MethodDelete) {
		t.Errorf("requests = %q", methods)
	}
}
