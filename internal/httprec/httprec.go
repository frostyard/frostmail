// Package httprec records HTTP sessions and replays them in tests
// (docs/design/testing.md, DAV and Tasks recordings), as imapx's trace and
// replay do for IMAP. Transport appends every exchange a client makes to a
// trace, with credentials redacted; Serve answers a client from a trace.
// A trace holds the account's contacts, calendars and tasks: it never
// leaves the machine, and only copies scrubbed by tools/davrec are checked
// in.
package httprec

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"unicode/utf8"
)

// Redacted is what a credential header's value is replaced with.
const Redacted = "[redacted]"

// sensitive are the headers a trace never holds the value of.
var sensitive = []string{"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie"}

// Exchange is one request and the answer it got.
type Exchange struct {
	Method    string      `json:"method"`
	URL       string      `json:"url"`
	ReqHeader http.Header `json:"reqHeader,omitzero"`
	// ReqBody is the request's body: text as is, anything else base64
	// with ReqBase64 set.
	ReqBody   string `json:"reqBody,omitzero"`
	ReqBase64 bool   `json:"reqBase64,omitzero"`
	// Status is 0 when the request failed without an answer (Err).
	Status     int         `json:"status,omitzero"`
	RespHeader http.Header `json:"respHeader,omitzero"`
	RespBody   string      `json:"respBody,omitzero"`
	RespBase64 bool        `json:"respBase64,omitzero"`
	Err        string      `json:"err,omitzero"`
}

// RequestBody returns the request's body bytes.
func (x Exchange) RequestBody() ([]byte, error) { return decodeBody(x.ReqBody, x.ReqBase64) }

// ResponseBody returns the response's body bytes.
func (x Exchange) ResponseBody() ([]byte, error) { return decodeBody(x.RespBody, x.RespBase64) }

func decodeBody(s string, b64 bool) ([]byte, error) {
	if !b64 {
		return []byte(s), nil
	}
	return base64.StdEncoding.DecodeString(s)
}

func encodeBody(b []byte) (string, bool) {
	if utf8.Valid(b) {
		return string(b), false
	}
	return base64.StdEncoding.EncodeToString(b), true
}

// redact copies h with the credential headers' values replaced.
func redact(h http.Header) http.Header {
	if len(h) == 0 {
		return nil
	}
	out := h.Clone()
	for _, k := range sensitive {
		if vs := out.Values(k); len(vs) > 0 {
			out[http.CanonicalHeaderKey(k)] = []string{Redacted}
		}
	}
	return out
}

// Transport is an http.RoundTripper that sends each request through Base
// (nil: http.DefaultTransport) and appends the exchange to W as one JSON
// line. Bodies are read whole. Writes to W are serialized; a failure to
// write is reported to OnError, if set, and does not fail the request.
type Transport struct {
	Base    http.RoundTripper
	W       io.Writer
	OnError func(error)

	mu sync.Mutex
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	x := Exchange{Method: req.Method, URL: req.URL.String(), ReqHeader: redact(req.Header)}
	if req.Body != nil && req.Body != http.NoBody {
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("httprec: read request body: %w", err)
		}
		x.ReqBody, x.ReqBase64 = encodeBody(body)
		req = req.Clone(req.Context())
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		x.Err = err.Error()
		t.write(x)
		return nil, err
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil {
		x.Err = err.Error()
		t.write(x)
		return nil, fmt.Errorf("httprec: read response body: %w", err)
	}
	x.Status, x.RespHeader = resp.StatusCode, redact(resp.Header)
	x.RespBody, x.RespBase64 = encodeBody(body)
	t.write(x)
	return resp, nil
}

func (t *Transport) write(x Exchange) {
	line, err := json.Marshal(x)
	if err == nil {
		t.mu.Lock()
		_, err = t.W.Write(append(line, '\n'))
		t.mu.Unlock()
	}
	if err != nil && t.OnError != nil {
		t.OnError(fmt.Errorf("httprec: write trace: %w", err))
	}
}

// Parse reads a trace: one exchange per line; blank lines and lines
// starting with "#" are skipped.
func Parse(r io.Reader) ([]Exchange, error) {
	var out []Exchange
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 256*1024*1024)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var x Exchange
		if err := json.Unmarshal([]byte(line), &x); err != nil {
			return nil, fmt.Errorf("httprec: line %d: %w", n, err)
		}
		if x.Method == "" || x.URL == "" {
			return nil, fmt.Errorf("httprec: line %d: an exchange needs a method and a URL", n)
		}
		out = append(out, x)
	}
	return out, sc.Err()
}

// Load reads a trace file.
func Load(path string) ([]Exchange, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// Write writes exchanges as a trace, after note as "# " comment lines.
func Write(w io.Writer, note string, xs []Exchange) error {
	var b bytes.Buffer
	for line := range strings.SplitSeq(strings.TrimSpace(note), "\n") {
		if line != "" {
			b.WriteString("# " + line + "\n")
		}
	}
	for _, x := range xs {
		line, err := json.Marshal(x)
		if err != nil {
			return err
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	_, err := w.Write(b.Bytes())
	return err
}

// ErrUnexpected marks a request a replay has no exchange for.
var ErrUnexpected = errors.New("httprec: unexpected request")
