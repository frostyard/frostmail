package unsubscribe

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	const pass = "mx.example; dkim=pass header.d=list.example; spf=pass"
	for _, tc := range []struct {
		name, header, post, auth string
		want                     []Method
		host, address, subject   string
	}{
		{"one-click first", "<mailto:leave@list.example?subject=bye>, <https://list.example/u/1>",
			"List-Unsubscribe=One-Click", pass, []Method{OneClick, Mail, Web}, "list.example", "leave@list.example", "bye"},
		{"no dkim pass", "<https://list.example/u/1>", "List-Unsubscribe=One-Click",
			"mx.example; dkim=fail", []Method{Web}, "list.example", "", ""},
		{"dkim=passed is not pass", "<https://list.example/u/1>", "List-Unsubscribe=One-Click",
			"mx.example; dkim=passed", []Method{Web}, "list.example", "", ""},
		{"no post header", "<https://list.example/u/1>", "", pass, []Method{Web}, "list.example", "", ""},
		{"mail alone, default subject", "<mailto:leave@list.example>", "", "", []Method{Mail}, "", "leave@list.example", "unsubscribe"},
		{"http is not taken", "<http://list.example/u>", "List-Unsubscribe=One-Click", pass, nil, "", "", ""},
		{"nothing", "", "", "", nil, "", "", ""},
	} {
		got := Parse(tc.header, tc.post, tc.auth)
		if !slices.Equal(got.Methods, tc.want) || got.Host != tc.host || got.Address != tc.address || got.Subject != tc.subject {
			t.Errorf("%s: %+v", tc.name, got)
		}
	}
}

func TestPublicRefusesInsideAddresses(t *testing.T) {
	for _, a := range []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254",
		"fe80::1", "fd00::1", "0.0.0.0", "::", "224.0.0.1", "100.64.0.1", "::ffff:127.0.0.1", "::ffff:10.0.0.1"} {
		if err := public(netip.MustParseAddr(a)); !errors.Is(err, errInside) {
			t.Errorf("%s: %v, want refused", a, err)
		}
	}
	for _, a := range []string{"93.184.216.34", "2606:2800:220:1::1"} {
		if err := public(netip.MustParseAddr(a)); err != nil {
			t.Errorf("%s: %v, want allowed", a, err)
		}
	}
}

func TestPostRefusesTheLocalServer(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("the request reached a local server")
	}))
	defer srv.Close()
	err := New().Post(t.Context(), srv.URL+"/u")
	if !errors.Is(err, errInside) {
		t.Errorf("post to %s: %v, want refused", srv.URL, err)
	}
	if err := New().Post(t.Context(), "http://list.example/u"); err == nil || !strings.Contains(err.Error(), "https") {
		t.Errorf("an http: URI: %v", err)
	}
}

func TestPostOneClick(t *testing.T) {
	status := http.StatusOK
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.Method != http.MethodPost || string(body) != "List-Unsubscribe=One-Click" ||
			r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" || r.Header.Get("User-Agent") != "Frostmail" ||
			r.Header.Get("Cookie") != "" || r.Header.Get("Referer") != "" {
			t.Errorf("request %s %q %v", r.Method, body, r.Header)
		}
		if status == http.StatusFound {
			http.Redirect(w, r, "https://elsewhere.example/", status)
			return
		}
		w.WriteHeader(status)
	}))
	defer srv.Close()
	p := newPoster(func(netip.Addr) error { return nil }, srv.Client().Transport.(*http.Transport).TLSClientConfig)
	if err := p.Post(t.Context(), srv.URL+"/u?list=1"); err != nil {
		t.Fatalf("one-click: %v", err)
	}
	for _, s := range []int{http.StatusInternalServerError, http.StatusFound} {
		status = s
		if err := p.Post(t.Context(), srv.URL+"/u"); err == nil {
			t.Errorf("status %d: no error", s)
		}
	}
}
