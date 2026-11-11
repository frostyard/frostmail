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
	path := filepath.Join(t.TempDir(), "message.eml")
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
}
