//go:build e2e

package e2e

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// corpus copies the hostile mail with its canary's address replaced.
func corpus(t *testing.T, origin string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(repoRoot, "internal", "render", "testdata", "hostile")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b = bytes.ReplaceAll(b, []byte("http://canary.test"), []byte(origin))
		if err := os.WriteFile(filepath.Join(dir, e.Name()), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// openHostile shows the Hostile mailbox and opens its first message, and
// returns the reader's page. A click can land before the row is drawn, or
// be lost to a change of the list, so it is repeated until the page shows.
func openHostile(s *session) node {
	s.t.Helper()
	s.press(1, s.waitNode(1, "treeItem", "^Hostile"))
	row := s.waitNode(1, "listItem", "Hostile")
	deadline := time.Now().Add(settle)
	for {
		s.center(1, row)
		time.Sleep(time.Second)
		if body := find(s.tree(1), "", "Message body"); body != nil {
			return *body
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("the first message did not open; the window shows:\n%s", dump(s.tree(1)))
		}
	}
}

// Every hostile message, shown in the real page, asks for nothing.
func TestHostileMailMakesNoRequests(t *testing.T) {
	var hits atomic.Int64
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1) })}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	dir := corpus(t, "http://"+ln.Addr().String())
	s := start(t, options{fixture: []string{"-n", "2", "-hostile", dir}})
	openHostile(s)
	entries, _ := os.ReadDir(dir)
	for range entries {
		s.key(1, "Down")
		time.Sleep(400 * time.Millisecond) // a page's requests leave as it loads
	}
	time.Sleep(2 * time.Second)
	if n := hits.Load(); n != 0 {
		t.Errorf("the corpus made %d requests", n)
	}
}

// A real click on a link in the page reaches the handler, and no window opens.
func TestLinkClickReachesTheHandler(t *testing.T) {
	dir := t.TempDir()
	const url = "https://example.test/clicked"
	page := `<p><a href="` + url + `">` + strings.Repeat("click this link anywhere on these lines ", 120) + `</a></p>`
	if err := os.WriteFile(filepath.Join(dir, "01-link.html"), []byte(page), 0o600); err != nil {
		t.Fatal(err)
	}
	s := start(t, options{fixture: []string{"-n", "0", "-hostile", dir}})
	body := openHostile(s)
	time.Sleep(2 * time.Second) // the page lays out after the element exists
	log := filepath.Join(s.root, "links.log")
	origin := s.windows()[0].Content
	for _, dy := range []float64{40, 70, 100, 130} {
		realClick(t, int(origin.X+body.Bounds.X+body.Bounds.Width/2), int(origin.Y+body.Bounds.Y+dy))
		time.Sleep(500 * time.Millisecond)
		if _, err := os.Stat(log); err == nil {
			break
		}
	}
	eventually(t, "the link in the log", func() bool {
		b, _ := os.ReadFile(log)
		return strings.Contains(string(b), url)
	})
	if ws := s.windows(); len(ws) != 1 {
		t.Errorf("windows after the click: %+v", ws)
	}
}

// A forward from the native window keeps the message's inline image, as
// maild's quoted rendering gives it: a cid: part, or a data: URL for a small
// picture.
func TestForwardKeepsAnInlineImage(t *testing.T) {
	sinkDir := t.TempDir()
	sink := exec.Command(exe("smtpsink"), "-dir", sinkDir)
	out, err := sink.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stop(sink) })
	buf := make([]byte, 256)
	n, err := out.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	addr := strings.TrimSpace(strings.TrimPrefix(string(buf[:n]), "listening "))

	// Sending needs the sync manager, whose IMAP the fixture points nowhere.
	s := start(t, options{fixture: []string{"-showcase", filepath.Join(repoRoot, "app", "e2e", "showcase"), "-smtp", addr},
		env: []string{"FROSTMAIL_SYNC=on", "FROSTMAIL_UNDO_DELAY=0s"}})
	row := s.waitNode(1, "", "Your trip to Lisbon")
	deadline := time.Now().Add(settle)
	for find(s.tree(1), "", "Message body") == nil {
		if time.Now().After(deadline) {
			t.Fatalf("the message did not open; the window shows:\n%s", dump(s.tree(1)))
		}
		s.center(1, row)
		time.Sleep(time.Second)
	}
	s.key(1, "CmdOrCtrl+Shift+F")
	w := s.waitWindow("Fwd|Trip|trip|Lisbon")
	to := s.waitNode(w, "comboBox", "^To$")
	s.must("act", map[string]any{"window": w, "id": to.ID, "action": "focus"}, nil)
	s.input(w, map[string]any{"type": "text", "text": "someone@example.test,"})
	s.key(w, "CmdOrCtrl+Enter")

	var eml string
	eventually(t, "the sent message", func() bool {
		m, _ := filepath.Glob(filepath.Join(sinkDir, "*.eml"))
		if len(m) > 0 {
			eml = m[0]
		}
		return eml != ""
	})
	raw, err := os.ReadFile(eml)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	cid, html := inlineParts(t, msg.Header.Get("Content-Type"), msg.Body)
	if !strings.Contains(html, "data:image/png;base64,") && (cid == "" || !strings.Contains(html, "cid:"+strings.Trim(cid, "<>"))) {
		t.Errorf("the forward lost its picture (Content-ID %q):\n%s", cid, html)
	}
}

// inlineParts finds, under multipart/related, an image's Content-ID and the
// HTML body.
func inlineParts(t *testing.T, contentType string, r io.Reader) (cid, html string) {
	t.Helper()
	media, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(media, "multipart/") {
		return "", ""
	}
	mr := multipart.NewReader(r, params["boundary"])
	for {
		p, err := mr.NextRawPart()
		if err != nil {
			return cid, html
		}
		pt := p.Header.Get("Content-Type")
		body, _ := io.ReadAll(p)
		switch {
		case strings.HasPrefix(pt, "multipart/"):
			c, h := inlineParts(t, pt, bytes.NewReader(body))
			if c != "" {
				cid = c
			}
			if h != "" {
				html = h
			}
		case strings.HasPrefix(pt, "image/") && media == "multipart/related":
			cid = p.Header.Get("Content-Id")
		case strings.HasPrefix(pt, "text/html"):
			html = decode(p.Header.Get("Content-Transfer-Encoding"), body)
		}
	}
}
