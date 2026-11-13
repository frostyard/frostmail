package conn_test

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/desktop/conn"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func TestParseLodge(t *testing.T) {
	l, err := conn.ParseLodge("https://lodge.example.ts.net")
	if err != nil {
		t.Fatal(err)
	}
	if l.Origin() != "https://lodge.example.ts.net" || l.APIURL() != "wss://lodge.example.ts.net/app/mail/api" {
		t.Errorf("origin %s, api %s", l.Origin(), l.APIURL())
	}
	if o, err := l.DerivedContentOrigin(); err != nil || o != "https://lodge-content.example.ts.net" {
		t.Errorf("derived content origin = %q, %v", o, err)
	}
	if l, err := conn.ParseLodge(" https://Lodge.example.ts.net/ "); err != nil || l.Origin() != "https://lodge.example.ts.net" {
		t.Errorf("a trailing slash and case = %v, %v", l, err)
	}
	dev, err := conn.ParseLodge("http://127.0.0.1:8480")
	if err != nil || dev.Origin() != "http://127.0.0.1:8480" || dev.APIURL() != "ws://127.0.0.1:8480/app/mail/api" {
		t.Errorf("loopback = %v, %v", dev, err)
	}
	if _, err := dev.DerivedContentOrigin(); err == nil {
		t.Error("loopback has a derived content origin")
	}
	if _, err := conn.ParseLodge("http://[::1]:8480"); err != nil {
		t.Errorf("IPv6 loopback: %v", err)
	}
	for _, bad := range []string{
		"http://lodge.example.ts.net", "ftp://lodge.example.ts.net", "https://lodge.example.ts.net/app",
		"https://u:p@lodge.example.ts.net", "https://lodge.example.ts.net/?x=1", "lodge.example.ts.net", "",
	} {
		if _, err := conn.ParseLodge(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

func TestAppConfigAndTarget(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"XDG_CONFIG_HOME": dir, "HOME": dir, "APPDATA": dir, "FROSTMAIL_SOCKET": "/run/test/maild.sock"}
	getenv := func(k string) string { return env[k] }

	tgt, err := conn.TargetFrom(getenv)
	if err != nil || tgt.Remote() || tgt.Socket != "/run/test/maild.sock" || tgt.String() != "Local" {
		t.Fatalf("no app.json: %+v, %v", tgt, err)
	}
	path := filepath.Join(dir, "frostmail", "app.json")
	if runtime.GOOS == "windows" {
		path = filepath.Join(dir, "Frostmail", "app.json")
	}
	if err := conn.WriteAppConfig(path, conn.AppConfig{Lodge: "https://lodge.example.ts.net/"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"lodge":"https://lodge.example.ts.net"}`+"\n" {
		t.Errorf("app.json = %q", data)
	}
	if fi, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Errorf("app.json's mode = %v, %v", fi.Mode(), err)
	}
	if tgt, err = conn.TargetFrom(getenv); err != nil || !tgt.Remote() || tgt.String() != "lodge.example.ts.net" {
		t.Errorf("lodge app.json: %+v, %v", tgt, err)
	}
	if err := conn.WriteAppConfig(path, conn.AppConfig{}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{}\n" {
		t.Errorf("local app.json = %q", data)
	}
	if err := conn.WriteAppConfig(path, conn.AppConfig{Lodge: "http://lodge.example.ts.net"}); err == nil {
		t.Error("an http lodge off loopback was written")
	}
	if err := os.WriteFile(path, []byte(`{"lodge":"ftp://x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.TargetFrom(getenv); err == nil {
		t.Error("an invalid lodge in app.json read without error")
	}
}

func TestDialLocal(t *testing.T) {
	srv := rpctest.Start(t)
	c, h, err := conn.Dial(t.Context(), conn.Target{Socket: srv.Socket}, "frostmail test")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if h.Protocol != api.Protocol {
		t.Errorf("hello = %+v", h)
	}
}

// fakeLodge serves /app/mail/api the way lodge does for the mail app: one
// JSON-RPC message per text frame, moved to and from maild's socket as
// lines. It records each upgrade's Origin and the frames the app sent, and
// drop ends every open WebSocket.
type fakeLodge struct {
	*httptest.Server
	mu      sync.Mutex
	origins []string
	frames  [][]byte
	open    []*websocket.Conn
	accepts atomic.Int32
}

func startFakeLodge(t *testing.T, socket string) *fakeLodge {
	t.Helper()
	f := &fakeLodge{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/app/mail/api" {
			http.NotFound(w, r)
			return
		}
		ws, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		f.accepts.Add(1)
		f.mu.Lock()
		f.origins = append(f.origins, r.Header.Get("Origin"))
		f.open = append(f.open, ws)
		f.mu.Unlock()
		nc, err := net.Dial("unix", socket)
		if err != nil {
			_ = ws.Close(websocket.StatusInternalError, "no maild")
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		defer nc.Close()
		go func() {
			defer cancel()
			sc := bufio.NewScanner(nc)
			sc.Buffer(nil, 16<<20)
			for sc.Scan() {
				if ws.Write(ctx, websocket.MessageText, sc.Bytes()) != nil {
					return
				}
			}
		}()
		for {
			_, data, err := ws.Read(ctx)
			if err != nil {
				return
			}
			f.mu.Lock()
			f.frames = append(f.frames, data)
			f.mu.Unlock()
			if _, err := nc.Write(append(data, '\n')); err != nil {
				return
			}
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeLodge) lodge(t *testing.T) conn.Lodge {
	t.Helper()
	l, err := conn.ParseLodge(f.URL)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func (f *fakeLodge) drop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ws := range f.open {
		_ = ws.Close(websocket.StatusGoingAway, "test drop")
	}
	f.open = nil
}

func TestDialLodgeMovesLinesAsFrames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake lodge dials maild's Unix socket")
	}
	srv := rpctest.Start(t)
	f := startFakeLodge(t, srv.Socket)
	l := f.lodge(t)
	ctx := t.Context()
	c, _, err := conn.Dial(ctx, conn.Target{Lodge: &l}, "frostmail test")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Snippet().Create(ctx, &api.SnippetCreateParams{Name: "Hi", HTML: "<p>Hi</p>"}); err != nil {
		t.Fatal(err)
	}
	select {
	case env := <-c.Notifications():
		if env.Event != "snippet.changed" {
			t.Errorf("event = %s", env.Event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no event through lodge")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.origins) != 1 || f.origins[0] != l.Origin() {
		t.Errorf("origins = %v, want %s", f.origins, l.Origin())
	}
	for _, fr := range f.frames {
		if bytes.ContainsRune(fr, '\n') {
			t.Errorf("a frame holds a newline: %q", fr)
		}
	}
}

func TestSessionReconnectsAndChangesTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake lodge dials maild's Unix socket")
	}
	srv := rpctest.Start(t)
	f := startFakeLodge(t, srv.Socket)
	l := f.lodge(t)
	states := make(chan conn.State, 64)
	var connects atomic.Int32
	s := conn.NewSession(conn.Target{Lodge: &l}, conn.Options{
		Client: "frostmail test", Retry: 20 * time.Millisecond,
		OnConnect: func(ctx context.Context, c *api.Client, _ *api.Hello) error {
			connects.Add(1)
			_, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{})
			return err
		},
		OnChange: func(st conn.State) { states <- st },
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()

	next := func(what string, ok func(conn.State) bool) conn.State {
		t.Helper()
		timeout := time.After(5 * time.Second)
		for {
			select {
			case st := <-states:
				if ok(st) {
					return st
				}
			case <-timeout:
				t.Fatalf("no state: %s", what)
			}
		}
	}
	next("connected through lodge", func(st conn.State) bool { return st.Connected && st.Target.Remote() })
	if s.Client() == nil {
		t.Fatal("no client while connected")
	}

	f.drop()
	lost := next("lost", func(st conn.State) bool { return !st.Connected })
	if lost.Err == nil {
		t.Error("a dropped connection reports no error")
	}
	next("connected again", func(st conn.State) bool { return st.Connected })
	if n := connects.Load(); n != 2 {
		t.Errorf("OnConnect ran %d times, want 2", n)
	}

	s.SetTarget(conn.Target{Socket: srv.Socket})
	st := next("connected locally", func(st conn.State) bool { return st.Connected && !st.Target.Remote() })
	if st.Target.String() != "Local" {
		t.Errorf("target = %s", st.Target)
	}
}

func TestSessionCountsFailedAttempts(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "maild.sock")
	if runtime.GOOS == "windows" {
		t.Skip("a missing pipe starts maild.exe on Windows")
	}
	states := make(chan conn.State, 64)
	s := conn.NewSession(conn.Target{Socket: missing}, conn.Options{Retry: 10 * time.Millisecond,
		OnChange: func(st conn.State) { states <- st }})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	for want := 1; want <= 3; want++ {
		select {
		case st := <-states:
			if st.Connected || st.Err == nil || st.Attempts != want {
				t.Fatalf("state %d = %+v", want, st)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no failed attempt")
		}
	}
}

func TestSafeNameStaysInItsDirectory(t *testing.T) {
	for name, want := range map[string]string{
		"a.pdf":        "a.pdf",
		"../../etc/pw": "_.._etc_pw",
		".hidden":      "hidden",
		"a\x00b:c\\d":  "a_b_c_d",
		"  ":           "attachment",
		"...":          "attachment",
	} {
		if got := conn.SafeName(name); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestDownloadRefusesPathsOutOfTheContentRoute(t *testing.T) {
	l, err := conn.ParseLodge("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/etc/passwd", "p/../../x", "p/a?x=1", "p/a#b"} {
		if _, err := l.Download(t.Context(), t.TempDir(), p, "x"); err == nil || !strings.Contains(err.Error(), "not a content path") {
			t.Errorf("Download(%q) = %v", p, err)
		}
	}
}

// A redirect could take the upgrade to plain http or to another origin, past
// ParseLodge's rule, so the dial never follows one.
func TestDialLodgeRefusesARedirect(t *testing.T) {
	var followed atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed.Add(1)
		http.NotFound(w, r)
	}))
	t.Cleanup(other.Close)
	from := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/app/mail/api", http.StatusFound)
	}))
	t.Cleanup(from.Close)
	l, err := conn.ParseLodge(from.URL)
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := conn.Dial(t.Context(), conn.Target{Lodge: &l}, "frostmail test")
	if err == nil {
		_ = c.Close()
		t.Fatal("a redirect connected")
	}
	if n := followed.Load(); n != 0 {
		t.Errorf("the dial followed the redirect %d times", n)
	}
}

// contentServer is a lodge whose content node serves body for every path.
func contentServer(t *testing.T, body string) conn.Lodge {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lodge/apps" {
			_, _ = w.Write([]byte(`{"contentOrigin":"` + srv.URL + `"}`))
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	l, err := conn.ParseLodge(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// A download's temporary file is its own: a file with the name a fixed one
// would take stays, and nothing is left behind.
func TestSaveToUsesATemporaryFileOfItsOwn(t *testing.T) {
	l := contentServer(t, "the message")
	dir := t.TempDir()
	path := filepath.Join(dir, "message.eml")
	beside := path + ".download"
	if err := os.WriteFile(beside, []byte("the user's"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.SaveTo(t.Context(), "m/1", path); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "the message" {
		t.Errorf("saved %q, %v", got, err)
	}
	if got, err := os.ReadFile(beside); err != nil || string(got) != "the user's" {
		t.Errorf("the file beside it holds %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 2 {
		t.Errorf("the directory holds %v, %v", entries, err)
	}
	if fi, err := os.Stat(path); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Errorf("the file's mode = %v, %v", fi.Mode(), err)
	}
}

func TestDownloadsToOnePathDoNotCollide(t *testing.T) {
	l := contentServer(t, strings.Repeat("x", 1<<20))
	dir := t.TempDir()
	path := filepath.Join(dir, "message.eml")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := l.SaveTo(t.Context(), "m/1", path); err != nil {
				t.Errorf("save: %v", err)
			}
		})
	}
	wg.Wait()
	if fi, err := os.Stat(path); err != nil || fi.Size() != 1<<20 {
		t.Errorf("the file = %v, %v", fi, err)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Errorf("the directory holds %v, %v: a temporary file was left", entries, err)
	}
}

// partServer is a lodge whose content node answers every path with n
// bytes, with or without saying how many, and counts the bytes it wrote.
func partServer(t *testing.T, n int, sayLength bool) (*conn.Lodge, *atomic.Int64) {
	t.Helper()
	var wrote atomic.Int64
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/lodge/apps" {
			_, _ = w.Write([]byte(`{"contentOrigin":"` + srv.URL + `"}`))
			return
		}
		if sayLength {
			w.Header().Set("Content-Length", strconv.Itoa(n))
		}
		chunk := bytes.Repeat([]byte("x"), 4096)
		for left := n; left > 0; {
			m := min(left, len(chunk))
			if _, err := w.Write(chunk[:m]); err != nil {
				return
			}
			wrote.Add(int64(m))
			left -= m
			if !sayLength {
				w.(http.Flusher).Flush() // chunked: the answer names no length
			}
		}
	}))
	t.Cleanup(srv.Close)
	l, err := conn.ParseLodge(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &l, &wrote
}

// A part read for a thumbnail is held to the thumbnail's limit over
// lodge's content node, whatever size the message gave for it: an answer
// that says it is longer is refused unread, and one that does not say is
// read no further than the limit allows (frostyard/frostmail#312).
func TestReadPartHoldsLodgeToTheLimit(t *testing.T) {
	const limit = 64 << 10
	part := &api.PartFile{Filename: "photo.png", ContentPath: new("p/mac/photo.png")}
	for _, sayLength := range []bool{true, false} {
		l, _ := partServer(t, limit, sayLength)
		if data, err := conn.ReadPart(t.Context(), l, "", part, limit); err != nil || len(data) != limit {
			t.Errorf("length said %v: a part of the limit read %d bytes, %v", sayLength, len(data), err)
		}
		l, _ = partServer(t, limit+1, sayLength)
		if data, err := conn.ReadPart(t.Context(), l, "", part, limit); err == nil || !strings.Contains(err.Error(), "photo.png is over") {
			t.Errorf("length said %v: a part one byte over read %d bytes, %v", sayLength, len(data), err)
		}
	}
	// A far larger answer is not buffered: the client stops reading, so the
	// server cannot write all of it.
	const large = 64 << 20
	l, wrote := partServer(t, large, false)
	if _, err := conn.ReadPart(t.Context(), l, "", part, limit); err == nil {
		t.Fatal("a large part was read")
	}
	if n := wrote.Load(); n >= large/2 {
		t.Errorf("the server wrote %d of %d bytes to a reader with a limit of %d", n, large, limit)
	}
	if _, err := conn.ReadPart(t.Context(), l, "", &api.PartFile{Filename: "photo.png"}, limit); err == nil {
		t.Error("a part with no content path was read from lodge")
	}
}

// From maild's parts cache the same limit holds, and the path stays in it.
func TestReadPartHoldsTheCacheToTheLimit(t *testing.T) {
	const limit = 1000
	cache := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cache, "parts", "7"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, n := range map[string]int{"fits.png": limit, "over.png": limit + 1} {
		if err := os.WriteFile(filepath.Join(cache, "parts", "7", name), bytes.Repeat([]byte("x"), n), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cache, "outside.png"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := conn.ReadPart(t.Context(), nil, cache, &api.PartFile{Path: "7/fits.png", Filename: "fits.png"}, limit); err != nil || len(data) != limit {
		t.Errorf("a part of the limit read %d bytes, %v", len(data), err)
	}
	for _, path := range []string{"7/over.png", "../outside.png", "/etc/hostname", "7/missing.png"} {
		if data, err := conn.ReadPart(t.Context(), nil, cache, &api.PartFile{Path: path, Filename: "x.png"}, limit); err == nil {
			t.Errorf("%s read %d bytes", path, len(data))
		}
	}
}
