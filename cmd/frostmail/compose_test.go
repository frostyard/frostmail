package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/compose"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// M8: a part's path in the cache names its message and specifier, and
// nothing else is asked of maild.
func TestPartOf(t *testing.T) {
	for rel, want := range map[string]struct {
		message int64
		part    string
	}{
		"m/12/2.png":         {12, "2"},
		"m/12/1.2.jpeg":      {12, "1.2"},
		"m/7/1.2.3/name.pdf": {7, "1.2.3"},
	} {
		message, part, ok := partOf(rel)
		if !ok || message != want.message || part != want.part {
			t.Errorf("%s = %d, %q, %v", rel, message, part, ok)
		}
	}
	for _, rel := range []string{"", "r/abc.png", "m/x/2.png", "m/1/../../etc/passwd", "m/1/2", "/m/1/2.png", "m/1/2/a/b"} {
		if _, _, ok := partOf(rel); ok {
			t.Errorf("%q read as a part", rel)
		}
	}
}

// partMessages names every part on lodge's content route, as maild
// serving lodge does, and records what was asked.
type partMessages struct {
	api.MessageService
	asked *atomic.Pointer[api.MessagePartParams]
}

const partContentPath = "p/0123abcd/m/12/2/photo.png"

func (m partMessages) Part(_ context.Context, p *api.MessagePartParams) (*api.PartFile, error) {
	m.asked.Store(p)
	path := partContentPath
	return &api.PartFile{Path: "m/12/2.png", ContentType: "image/png", Filename: "photo.png", ContentPath: &path}, nil
}

// M8: behind lodge, a draft's inline image comes from lodge's content
// route, which maild names, and not from a parts cache on this computer,
// which lodge's maild does not fill.
func TestLoadPartReadsLodgesContentRoute(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	picture := b.Bytes()
	var asked atomic.Pointer[api.MessagePartParams]
	srv := rpctest.StartWith(t, rpctest.Options{Services: func(s *api.Services) { s.Message = partMessages{s.Message, &asked} }})
	c := srv.Dial(t)
	var lodge *httptest.Server
	lodge = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lodge/apps":
			_, _ = w.Write([]byte(`{"contentOrigin":"` + lodge.URL + `"}`))
		case "/mail/" + partContentPath:
			_, _ = w.Write(picture)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(lodge.Close)
	l, err := conn.ParseLodge(lodge.URL)
	if err != nil {
		t.Fatal(err)
	}
	// What a read of this computer's cache would find instead.
	cache := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cache, "parts", "m", "12"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "parts", "m", "12", "2.png"), []byte("not this"), 0o600); err != nil {
		t.Fatal(err)
	}
	load := func(src string) ([]byte, error) { return loadPart(c, conn.Target{Lodge: &l}, cache, src) }
	got, err := load("mailpart://localhost/m/12/2.png")
	if err != nil || !bytes.Equal(got, picture) {
		t.Fatalf("read %q, %v", got, err)
	}
	if p := asked.Load(); p == nil || p.ID != 12 || p.Path != "2" {
		t.Errorf("maild was asked %+v", p)
	}
	// The editor shows it.
	text := compose.ToText(`<p><img src="mailpart://localhost/m/12/2.png"></p>`, load)
	var shown bool
	for _, p := range text.Paragraphs() {
		for _, r := range p.Runs {
			if a := r.Attributes.Attachment; a != nil && a.Image != nil {
				shown = true
			}
		}
	}
	if !shown {
		t.Error("the editor has no picture")
	}
	// The download leaves nothing behind.
	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "compose-part-") {
			t.Errorf("left %s", e.Name())
		}
	}
}

// M7: a window's client ends with the target it opened on, and follows
// reconnects to it.
func TestBoundClientEndsWithItsTarget(t *testing.T) {
	first, second := &api.Client{}, &api.Client{}
	target, current := conn.Target{Socket: "/run/a"}, first
	client := boundClient(func() conn.Target { return target }, func() *api.Client { return current })
	if got := client(); got != first {
		t.Errorf("on its target: %p", got)
	}
	current = second
	if got := client(); got != second {
		t.Errorf("after a reconnect: %p", got)
	}
	lodge, err := conn.ParseLodge("https://lodge.test")
	if err != nil {
		t.Fatal(err)
	}
	target = conn.Target{Lodge: &lodge}
	if got := client(); got != nil {
		t.Errorf("after a change of target: %p", got)
	}
}

func testComposeWindows(drafts ...int64) (*composeWindows, *int) {
	quits := 0
	s := &composeWindows{windows: map[int64]*mygo.Window{}, leaving: map[int64]bool{}, gone: map[int64][]func(){},
		quitWaits: map[int64]bool{}, quit: func() { quits++ }}
	for _, d := range drafts {
		s.windows[d] = nil
	}
	return s, &quits
}

// askedByQuit is a quit's pass over the windows reaching draft's, which
// turns it down to save: the pass stops there.
func askedByQuit(s *composeWindows, draft int64) {
	s.quitting = true
	s.refused(draft)
	s.quitting = false
}

// RR1-N1: a quit stops at the first compose window still saving, before it
// asks the windows after it; it goes on each time such a window closes.
func TestQuitGoesOnAfterEachComposerCloses(t *testing.T) {
	s, quits := testComposeWindows(1, 2, 3)
	askedByQuit(s, 1)
	s.closed(1)
	if *quits != 1 {
		t.Fatalf("quits after the first window closed: %d", *quits)
	}
	askedByQuit(s, 2)
	s.closed(2)
	if *quits != 2 {
		t.Fatalf("quits after the second window closed: %d", *quits)
	}
	// A window the user closed, with no quit asking, quits nothing.
	s.refused(3)
	s.closed(3)
	if *quits != 2 {
		t.Errorf("quits after a window the user closed: %d", *quits)
	}
}

// RR1-N1, M28: a window whose save failed stays open, saying so: what
// waited for it goes on, and the quit it stopped ends there.
func TestAKeptComposerEndsTheQuit(t *testing.T) {
	s, quits := testComposeWindows(1)
	askedByQuit(s, 1)
	waited := false
	s.gone[1] = []func(){func() { waited = true }}
	s.kept(1)
	if !waited || s.leaving[1] {
		t.Errorf("waited %v, leaving %v", waited, s.leaving[1])
	}
	s.refused(1)
	s.closed(1)
	if *quits != 0 {
		t.Errorf("quits after a kept window closed later: %d", *quits)
	}
}
