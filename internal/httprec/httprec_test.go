package httprec_test

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/frostyard/frostmail/internal/httprec"
)

// origin is a stand-in for a DAV server: PROPFIND answers a multistatus
// naming itself by absolute URL, GET a binary photo, PUT an ETag and a
// cookie, and /moved redirects.
func origin(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == "PROPFIND":
			w.WriteHeader(207)
			fmt.Fprintf(w, "<multistatus><href>%s/cal/</href><echo>%s</echo></multistatus>", srv.URL, body)
		case r.URL.Path == "/moved":
			http.Redirect(w, r, srv.URL+"/cal/", http.StatusMovedPermanently)
		case r.Method == "GET":
			_, _ = w.Write([]byte{0x89, 'P', 'N', 'G', 0xff, 0x00})
		case r.Method == "PUT":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "secret-cookie"})
			w.Header().Set("ETag", `"2"`)
			w.WriteHeader(http.StatusCreated)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, c *http.Client, method, url, body string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Depth", "1")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

// session makes the requests a test replays.
func session(t *testing.T, c *http.Client, base string) []string {
	t.Helper()
	var out []string
	for _, r := range []struct{ method, path, body string }{
		{"PROPFIND", "/cal/", "<propfind/>"},
		{"GET", "/photo.png", ""},
		{"PUT", "/cal/a.ics", "BEGIN:VCALENDAR"},
		{"GET", "/photo.png", ""},
	} {
		resp, body := do(t, c, r.method, base+r.path, r.body)
		out = append(out, fmt.Sprintf("%d %q %s", resp.StatusCode, body, resp.Header.Get("ETag")))
	}
	return out
}

func record(t *testing.T) (*httptest.Server, []string, []httprec.Exchange, string) {
	t.Helper()
	srv := origin(t)
	var trace bytes.Buffer
	c := &http.Client{Transport: &httprec.Transport{W: &trace}}
	answers := session(t, c, srv.URL)
	xs, err := httprec.Parse(&trace)
	if err != nil {
		t.Fatal(err)
	}
	return srv, answers, xs, trace.String()
}

func TestRecordRedactsCredentials(t *testing.T) {
	_, _, xs, raw := record(t)
	if strings.Contains(raw, "secret-token") || strings.Contains(raw, "secret-cookie") {
		t.Fatalf("a credential is in the trace:\n%s", raw)
	}
	if len(xs) != 4 {
		t.Fatalf("exchanges = %d", len(xs))
	}
	if got := xs[0].ReqHeader.Get("Authorization"); got != httprec.Redacted {
		t.Errorf("Authorization = %q", got)
	}
	if got := xs[2].RespHeader.Get("Set-Cookie"); got != httprec.Redacted {
		t.Errorf("Set-Cookie = %q", got)
	}
	if xs[0].ReqHeader.Get("Depth") != "1" || xs[0].ReqBody != "<propfind/>" || xs[0].Status != 207 {
		t.Errorf("PROPFIND = %+v", xs[0])
	}
	photo, _ := xs[1].ResponseBody()
	if !xs[1].RespBase64 || !bytes.Equal(photo, []byte{0x89, 'P', 'N', 'G', 0xff, 0x00}) {
		t.Errorf("binary body = %+v", xs[1])
	}
}

func TestRecordsFailures(t *testing.T) {
	var trace bytes.Buffer
	c := &http.Client{Transport: &httprec.Transport{W: &trace}}
	if _, err := c.Get("http://127.0.0.1:1/nothing"); err == nil {
		t.Fatal("connected to port 1")
	}
	xs, _ := httprec.Parse(&trace)
	if len(xs) != 1 || xs[0].Err == "" || xs[0].Status != 0 {
		t.Errorf("failure = %+v", xs)
	}
}

func TestReplayAnswersAsRecorded(t *testing.T) {
	srv, answers, xs, _ := record(t)
	rs := httprec.Serve(t, xs)
	got := session(t, http.DefaultClient, rs.URL)
	for i := range answers {
		// The recorded server named itself; the replay names itself.
		if want := strings.ReplaceAll(answers[i], srv.URL, rs.URL); got[i] != want {
			t.Errorf("answer %d = %s, want %s", i, got[i], want)
		}
	}
	if rs.Unused() != 0 {
		t.Errorf("unused = %d", rs.Unused())
	}
}

func TestReplayRewritesRedirects(t *testing.T) {
	srv := origin(t)
	var trace bytes.Buffer
	c := &http.Client{Transport: &httprec.Transport{W: &trace}, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, _ := do(t, c, "GET", srv.URL+"/moved", "")
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	xs, _ := httprec.Parse(&trace)
	rs := httprec.Serve(t, xs)
	resp, _ = do(t, c, "GET", rs.URL+"/moved", "")
	if loc := resp.Header.Get("Location"); loc != rs.URL+"/cal/" {
		t.Errorf("Location = %q, want %q", loc, rs.URL+"/cal/")
	}
	if rs.Rewrite(srv.URL+"/x") != rs.URL+"/x" {
		t.Errorf("Rewrite = %q", rs.Rewrite(srv.URL+"/x"))
	}
}

// tb records what a Server reports.
type tb struct {
	mu       sync.Mutex
	errors   []string
	cleanups []func()
}

func (f *tb) Helper()             {}
func (f *tb) Logf(string, ...any) {}
func (f *tb) Cleanup(fn func())   { f.cleanups = append(f.cleanups, fn) }
func (f *tb) Errorf(s string, a ...any) {
	f.mu.Lock()
	f.errors = append(f.errors, fmt.Sprintf(s, a...))
	f.mu.Unlock()
}
func (f *tb) end() {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

func TestReplayRefusesTheUnexpected(t *testing.T) {
	_, _, xs, _ := record(t)
	f := &tb{}
	rs := httprec.Serve(f, xs)
	resp, _ := do(t, http.DefaultClient, "PROPFIND", rs.URL+"/cal/", "<propfind><other/></propfind>")
	if resp.StatusCode != http.StatusInternalServerError || len(f.errors) != 1 ||
		!strings.Contains(f.errors[0], "unexpected request") {
		t.Errorf("a different body: %d, %q", resp.StatusCode, f.errors)
	}
	f.end()
	if len(f.errors) != 1+4 {
		t.Errorf("unused exchanges reported = %q", f.errors)
	}

	f = &tb{}
	rs = httprec.Serve(f, xs)
	rs.AllowUnused()
	f.end()
	if len(f.errors) != 0 {
		t.Errorf("with AllowUnused: %q", f.errors)
	}
}

func TestReplayClosesOnARecordedFailure(t *testing.T) {
	rs := httprec.Serve(t, []httprec.Exchange{{Method: "GET", URL: "https://dav.example/x", Err: "connection reset"}})
	if _, err := http.Get(rs.URL + "/x"); err == nil {
		t.Error("a recorded failure answered")
	}
}

func TestWriteAndParse(t *testing.T) {
	xs := []httprec.Exchange{{Method: "GET", URL: "https://dav.example/a", Status: 200, RespBody: "hi"}}
	var b bytes.Buffer
	if err := httprec.Write(&b, "Google, first sync\nscrubbed", xs); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.String(), "# Google, first sync\n# scrubbed\n{") {
		t.Errorf("trace = %q", b.String())
	}
	got, err := httprec.Parse(&b)
	if err != nil || len(got) != 1 || got[0].RespBody != "hi" {
		t.Errorf("parsed = %+v, %v", got, err)
	}
	if _, err := httprec.Parse(strings.NewReader("{\"url\":\"x\"}\n")); err == nil {
		t.Error("an exchange without a method parsed")
	}
}
