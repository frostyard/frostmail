package imapx_test

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/imapx/replay"
)

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *lockedBuffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *lockedBuffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// fetchOne opens a session, selects INBOX and fetches UID 1's headers.
func fetchOne(t *testing.T, opts imapx.DialOptions) []imapx.Part {
	t.Helper()
	s, err := imapx.Open(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Select(t.Context(), "INBOX"); err != nil {
		t.Fatal(err)
	}
	hs, err := s.FetchHeaders(t.Context(), []uint32{1})
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 1 || hs[0].Subject != "Lighthouse invoice" {
		t.Fatalf("headers = %+v", hs)
	}
	return hs[0].Parts
}

// A BODYSTRUCTURE the parser rejects costs the message its parts, not the
// fetch: the message is stored without them, and the log shows the
// structure's shape with its strings masked.
func TestUnparseableBodyStructure(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	raw := "From: Ann <ann@x.test>\r\nSubject: Lighthouse invoice\r\nMessage-ID: <a@x.test>\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\nHello.\r\n"
	if _, err := mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	opts := mem.DialOptions()
	var tr lockedBuffer
	opts.Trace = &tr
	if parts := fetchOne(t, opts); len(parts) != 1 {
		t.Fatalf("recorded parts = %+v", parts)
	}

	// Break the recorded structure (a list where the size belongs), and
	// drop the preview fetch the client will no longer send.
	lines, err := replay.Parse(strings.NewReader(tr.String()))
	if err != nil {
		t.Fatal(err)
	}
	greeting, xs := replay.Split(lines)
	size := regexp.MustCompile(`("7bit" )(\d+)`)
	script := greeting
	broken := false
	for _, x := range xs {
		if strings.Contains(x[0].Text, "BODY.PEEK[1]") {
			continue
		}
		for _, l := range x {
			if !l.Sent && strings.Contains(l.Text, "BODYSTRUCTURE") && !broken {
				l.Text, broken = size.ReplaceAllString(l.Text, "$1($2)"), true
			}
			script = append(script, l)
		}
	}
	if !broken {
		t.Fatalf("no BODYSTRUCTURE in the recording:\n%s", tr.String())
	}
	srv := replay.Start(t, script)
	var logged lockedBuffer
	host, port, _ := strings.Cut(srv.Addr(), ":")
	ropts := imapx.DialOptions{Host: host, TLS: api.TLSModeInsecure, Username: "anyone", Password: "anything",
		Log: slog.New(slog.NewTextHandler(&logged, nil))}
	for _, r := range port {
		ropts.Port = ropts.Port*10 + int(r-'0')
	}
	if parts := fetchOne(t, ropts); len(parts) != 0 {
		t.Errorf("parts of an unparseable structure = %+v", parts)
	}
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
	out := logged.String()
	if !strings.Contains(out, "unparseable BODYSTRUCTURE") || !strings.Contains(out, "uid=1") ||
		!strings.Contains(out, `\"xxxx\" \"xxxxx\" (\"xxxxxxx\" \"xxx-0\") NIL NIL \"0xxx\" (8) 1`) {
		t.Errorf("log = %s", out)
	}
	if strings.Contains(out, "plain") || strings.Contains(out, "utf-8") {
		t.Errorf("the log shows the structure's strings: %s", out)
	}
}
