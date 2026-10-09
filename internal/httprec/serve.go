package httprec

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
)

// TB is the part of testing.TB a Server reports to.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
	Cleanup(func())
}

// hopHeaders are left out of replayed answers: the server writes its own.
var hopHeaders = []string{"Content-Length", "Transfer-Encoding", "Connection", "Keep-Alive", "Date"}

// Server answers a client from a trace on a loopback port. Each request is
// answered with the first unused exchange with the same method and path
// (with its query), whatever the host: one server stands in for every host
// the session reached. PROPFIND and REPORT requests must also carry the
// recorded body, the queries a sync depends on; other bodies (writes,
// which hold timestamps) are not compared. The recorded origins
// (scheme://host) in answers' headers and text bodies become the server's
// URL. A request the trace has no exchange for fails the test, and so,
// when the test ends, does an exchange the client never asked for, unless
// AllowUnused was called. An exchange recorded with an error closes the
// connection.
type Server struct {
	URL string

	t           TB
	srv         *httptest.Server
	origins     []string
	mu          sync.Mutex
	xs          []Exchange
	used        []bool
	allowUnused bool
}

// Serve starts a Server for xs, closed when the test ends.
func Serve(t TB, xs []Exchange) *Server {
	t.Helper()
	s := &Server{t: t, xs: xs, used: make([]bool, len(xs))}
	for _, x := range xs {
		if u, err := url.Parse(x.URL); err == nil && u.Host != "" {
			if o := u.Scheme + "://" + u.Host; !slices.Contains(s.origins, o) {
				s.origins = append(s.origins, o)
			}
		}
	}
	s.srv = httptest.NewServer(s)
	s.URL = s.srv.URL
	t.Cleanup(func() {
		s.srv.Close()
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.allowUnused {
			return
		}
		for i, used := range s.used {
			if !used {
				t.Errorf("httprec: the client never sent %s %s", s.xs[i].Method, s.xs[i].URL)
			}
		}
	})
	return s
}

// AllowUnused lets the test end with exchanges the client never asked
// for.
func (s *Server) AllowUnused() {
	s.mu.Lock()
	s.allowUnused = true
	s.mu.Unlock()
}

// Rewrite returns u with the recorded origins replaced by the server's
// URL: what a test configures in place of a recorded URL.
func (s *Server) Rewrite(u string) string {
	for _, o := range s.origins {
		u = strings.ReplaceAll(u, o, s.URL)
	}
	return u
}

// Unused returns how many exchanges the client has not asked for yet.
func (s *Server) Unused() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, used := range s.used {
		if !used {
			n++
		}
	}
	return n
}

// compareBody says whether a method's request body must match.
func compareBody(method string) bool { return method == "PROPFIND" || method == "REPORT" }

// pathQuery is a URL's path with its query, as a server sees it.
func pathQuery(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return u.RequestURI()
}

func (s *Server) take(method, uri string, body []byte) (Exchange, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, x := range s.xs {
		if s.used[i] || x.Method != method || pathQuery(x.URL) != uri {
			continue
		}
		if compareBody(method) {
			want, err := x.RequestBody()
			if err != nil || !bytes.Equal([]byte(s.Rewrite(string(want))), body) {
				continue
			}
		}
		s.used[i] = true
		return x, true
	}
	return Exchange{}, false
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	x, ok := s.take(r.Method, r.URL.RequestURI(), body)
	if !ok {
		s.t.Errorf("%v: %s %s\n%s", ErrUnexpected, r.Method, r.URL.RequestURI(), body)
		http.Error(w, ErrUnexpected.Error(), http.StatusInternalServerError)
		return
	}
	if x.Err != "" || x.Status == 0 {
		panic(http.ErrAbortHandler)
	}
	resp, err := x.ResponseBody()
	if err != nil {
		s.t.Errorf("httprec: %s %s: %v", x.Method, x.URL, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !x.RespBase64 {
		resp = []byte(s.Rewrite(string(resp)))
	}
	for k, vs := range x.RespHeader {
		if slices.ContainsFunc(hopHeaders, func(h string) bool { return strings.EqualFold(h, k) }) {
			continue
		}
		for _, v := range vs {
			w.Header().Add(k, s.Rewrite(v))
		}
	}
	w.WriteHeader(x.Status)
	if _, err := w.Write(resp); err != nil {
		s.t.Logf("httprec: write %s %s: %v", x.Method, x.URL, fmt.Sprint(err))
	}
}
