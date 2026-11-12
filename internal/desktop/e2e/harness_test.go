//go:build e2e

// Package e2e drives the built app in real windows on fixture mail, through
// MyGo's automation plugin (plan 0015, Phase 9): what ui.Tester cannot show,
// a real page, a real pointer and more than one window. Every test has its
// own maild, data, config and cache in a temporary directory, and stops
// before acting unless the window says it is on that maild.
//
// Run it with make desktop-e2e; it needs a display.
package e2e

import (
	"bufio"
	"context"
	"encoding/json/v2"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// settle bounds a wait for the UI; the longest is a page or a window opening
// on a loaded runner.
const settle = 60 * time.Second

var (
	binDir   string
	repoRoot string
	pipeSeq  atomic.Int64
)

func exe(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(binDir, name)
}

func TestMain(m *testing.M) {
	code, err := setup(m)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

func setup(m *testing.M) (int, error) {
	var err error
	if repoRoot, err = filepath.Abs("../../.."); err != nil {
		return 0, err
	}
	if binDir, err = os.MkdirTemp("", "fmbin"); err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(binDir) }()
	for _, b := range [][]string{
		{"-tags", "automation", "-o", exe("frostmail"), "./cmd/frostmail"},
		{"-o", exe("maild"), "./cmd/maild"},
		{"-o", exe("mailctl"), "./cmd/mailctl"},
		{"-o", exe("uifixture"), "./tools/uifixture"},
		{"-o", exe("smtpsink"), "./tools/smtpsink"},
	} {
		cmd := exec.Command("go", append([]string{"build"}, b...)...)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			return 0, fmt.Errorf("go build %v: %w\n%s", b, err, out)
		}
	}
	return m.Run(), nil
}

type options struct {
	server  *rpctest.Options // an in-process maild for held/failing RPCs in real windows
	fixture []string         // uifixture's arguments, after -out
	env     []string
	path    string // a directory put first on PATH
}

// session is one maild and one app on their own directories.
type session struct {
	t      *testing.T
	root   string
	socket string
	env    []string
	maild  *exec.Cmd
	app    *exec.Cmd
	conn   net.Conn
	reader *bufio.Reader
	nextID int
	client *api.Client
}

// shortDir makes a directory whose path leaves room for a Unix socket.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "fme")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// isolated is the environment with every path of the user's replaced.
func isolated(t *testing.T, root, socket string) []string {
	t.Helper()
	set := map[string]string{
		"FROSTMAIL_DATA_DIR":      filepath.Join(root, "data"),
		"FROSTMAIL_CACHE_DIR":     filepath.Join(root, "cache"),
		"FROSTMAIL_SOCKET":        socket,
		"FROSTMAIL_SECRETS":       "file",
		"FROSTMAIL_SYNC":          "off",
		"FROSTMAIL_TEST_LINK_LOG": filepath.Join(root, "links.log"),
		"MYGO_AUTOMATION":         filepath.Join(root, "a.sock"),
	}
	if runtime.GOOS == "windows" {
		set["APPDATA"] = filepath.Join(root, "appdata")
		set["LOCALAPPDATA"] = filepath.Join(root, "localappdata")
	} else {
		if d := os.Getenv("WAYLAND_DISPLAY"); d != "" && !filepath.IsAbs(d) {
			set["WAYLAND_DISPLAY"] = filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), d)
		}
		set["HOME"] = filepath.Join(root, "home")
		set["XDG_CONFIG_HOME"] = filepath.Join(root, "config")
		set["XDG_DATA_HOME"] = filepath.Join(root, "share")
		set["XDG_CACHE_HOME"] = filepath.Join(root, "xdgcache")
		set["XDG_STATE_HOME"] = filepath.Join(root, "state")
		set["XDG_RUNTIME_DIR"] = filepath.Join(root, "run")
	}
	for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "XDG_STATE_HOME", "APPDATA", "LOCALAPPDATA"} {
		if v, ok := set[k]; ok {
			if err := os.MkdirAll(v, 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}
	if v, ok := set["XDG_RUNTIME_DIR"]; ok {
		if err := os.MkdirAll(filepath.Join(v, "app", "org.frostyard.Frostmail"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		_, replaced := set[k]
		return replaced || strings.HasPrefix(k, "FROSTMAIL_") || k == "FLATPAK_ID"
	})
	for k, v := range set {
		env = append(env, k+"="+v)
	}
	return env
}

// start builds the fixture, starts its maild and the app, and returns once
// the window says it is on this maild.
func start(t *testing.T, o options) *session {
	t.Helper()
	root := shortDir(t)
	socket := filepath.Join(root, "m.sock")
	if runtime.GOOS == "windows" {
		socket = fmt.Sprintf(`\\.\pipe\fm-e2e-%d-%d`, os.Getpid(), pipeSeq.Add(1))
	}
	s := &session{t: t, root: root, socket: socket, env: isolated(t, root, socket)}
	s.env = append(s.env, o.env...)
	if o.path != "" {
		s.env = append(s.env, "PATH="+o.path+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	fixture := exec.Command(exe("uifixture"), append([]string{"-out", filepath.Join(root, "data")}, o.fixture...)...)
	fixture.Dir = repoRoot
	if out, err := fixture.CombinedOutput(); err != nil {
		t.Fatalf("uifixture: %v\n%s", err, out)
	}
	if o.server == nil {
		s.startMaild()
	} else {
		opts := *o.server
		opts.DataDir = filepath.Join(root, "data")
		srv := rpctest.StartWith(t, opts)
		s.socket = srv.Socket
		s.env = append(s.env, "FROSTMAIL_SOCKET="+s.socket)
		s.client = srv.Dial(t)
	}
	s.app = s.spawn(exe("frostmail"), "app.log")
	t.Cleanup(func() {
		if s.conn != nil {
			_ = s.conn.Close()
		}
		stop(s.app)
		if s.client != nil {
			_ = s.client.Close()
		}
		stop(s.maild)
	})
	s.attach()
	s.waitFor(1, "the local fixture", func(tree []node) bool { return find(tree, "", "Connection: Local") != nil })
	return s
}

func stop(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}
}

func (s *session) spawn(path, log string, args ...string) *exec.Cmd {
	s.t.Helper()
	out, err := os.Create(filepath.Join(s.root, log))
	if err != nil {
		s.t.Fatal(err)
	}
	s.t.Cleanup(func() { _ = out.Close() })
	cmd := exec.Command(path, args...)
	cmd.Env, cmd.Stdout, cmd.Stderr = s.env, out, out
	if err := cmd.Start(); err != nil {
		s.t.Fatal(err)
	}
	return cmd
}

func (s *session) startMaild() {
	s.t.Helper()
	s.maild = s.spawn(exe("maild"), "maild.log")
	deadline := time.Now().Add(settle)
	for {
		c, _, err := api.Dial(s.t.Context(), s.socket, "e2e")
		if err == nil {
			s.client = c
			return
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("maild did not answer: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (s *session) attach() {
	s.t.Helper()
	path := filepath.Join(s.root, "a.sock")
	deadline := time.Now().Add(settle)
	for {
		conn, err := net.Dial("unix", path)
		if err == nil {
			s.conn, s.reader = conn, bufio.NewReaderSize(conn, 1<<20)
			return
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(filepath.Join(s.root, "app.log"))
			s.t.Fatalf("the app did not open its automation socket: %v\n%s", err, log)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// call asks the app; a method's error is returned for the caller to judge.
func (s *session) call(method string, params map[string]any, into any) error {
	s.t.Helper()
	s.nextID++
	line, err := json.Marshal(map[string]any{"id": s.nextID, "method": method, "params": params})
	if err != nil {
		return err
	}
	if err := s.conn.SetDeadline(time.Now().Add(settle)); err != nil {
		return err
	}
	if _, err := s.conn.Write(append(line, '\n')); err != nil {
		return err
	}
	answer, err := s.reader.ReadBytes('\n')
	if err != nil {
		return err
	}
	var res struct {
		Result map[string]any `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(answer, &res); err != nil {
		return err
	}
	if res.Error != nil {
		return fmt.Errorf("%s: %s: %s", method, res.Error.Code, res.Error.Message)
	}
	if into != nil {
		raw, err := json.Marshal(res.Result)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, into)
	}
	return nil
}

func (s *session) must(method string, params map[string]any, into any) {
	s.t.Helper()
	if err := s.call(method, params, into); err != nil {
		s.t.Fatal(err)
	}
}

type rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type node struct {
	ID     string   `json:"id"`
	Role   string   `json:"role"`
	Label  string   `json:"label"`
	Value  string   `json:"value"`
	Bounds rect     `json:"bounds"`
	States []string `json:"states"`
	Web    bool     `json:"webView"`
}

type window struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Content rect   `json:"contentBounds"`
}

func (s *session) windows() []window {
	s.t.Helper()
	var r struct {
		Windows []window `json:"windows"`
	}
	s.must("windows", nil, &r)
	return r.Windows
}

// tree reads a window's elements; a window that just closed has none.
func (s *session) tree(w int64) []node {
	s.t.Helper()
	var r struct {
		Nodes []node `json:"nodes"`
	}
	if err := s.call("tree", map[string]any{"window": w}, &r); err != nil {
		return nil
	}
	return r.Nodes
}

// find returns the first element of role (any, for "") whose label matches.
func find(tree []node, role, label string) *node {
	re := regexp.MustCompile(label)
	for i := range tree {
		if (role == "" || tree[i].Role == role) && re.MatchString(tree[i].Label) {
			return &tree[i]
		}
	}
	return nil
}

func (s *session) waitFor(w int64, what string, ok func([]node) bool) []node {
	s.t.Helper()
	deadline := time.Now().Add(settle)
	for {
		tree := s.tree(w)
		if ok(tree) {
			return tree
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("waiting for %s in window %d; it shows:\n%s", what, w, dump(tree))
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// within waits up to d for what ok wants and returns the tree that had it,
// or nil.
func (s *session) within(w int64, d time.Duration, ok func([]node) bool) []node {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if tree := s.tree(w); ok(tree) {
			return tree
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// clickUntil clicks the node until the window has an element labelled want,
// and returns that element. A click can land before the row is drawn, or be
// lost to a change of the list, so it is repeated only when a wait for its
// result runs out.
func (s *session) clickUntil(n node, want string) node {
	s.t.Helper()
	has := func(tree []node) bool { return find(tree, "", want) != nil }
	deadline := time.Now().Add(settle)
	for time.Now().Before(deadline) {
		s.center(1, n)
		if tree := s.within(1, 5*time.Second, has); tree != nil {
			return *find(tree, "", want)
		}
	}
	s.t.Fatalf("no %s after clicking %q; the window shows:\n%s", want, n.Label, dump(s.tree(1)))
	return node{}
}

func dump(tree []node) string {
	var b strings.Builder
	for _, n := range tree {
		fmt.Fprintf(&b, "%s %q %q\n", n.Role, n.Label, n.Value)
	}
	return b.String()
}

// waitNode waits for an element and returns it.
func (s *session) waitNode(w int64, role, label string) node {
	s.t.Helper()
	tree := s.waitFor(w, role+" "+label, func(tree []node) bool { return find(tree, role, label) != nil })
	return *find(tree, role, label)
}

// waitWindow waits for a window whose title matches.
func (s *session) waitWindow(title string) int64 {
	s.t.Helper()
	re := regexp.MustCompile(title)
	deadline := time.Now().Add(settle)
	for {
		ws := s.windows()
		for _, w := range ws {
			if re.MatchString(w.Title) {
				return w.ID
			}
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("no window titled %s among %+v", title, ws)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (s *session) idle(w int64) {
	s.t.Helper()
	_ = s.call("idle", map[string]any{"window": w, "timeout": 5000}, nil)
}

func (s *session) input(w int64, events ...map[string]any) {
	s.t.Helper()
	s.must("input", map[string]any{"window": w, "events": events}, nil)
	s.idle(w)
}

func (s *session) key(w int64, key string) {
	s.t.Helper()
	s.input(w, map[string]any{"type": "key", "key": key})
}

func (s *session) click(w int64, x, y float64) {
	s.t.Helper()
	s.input(w, map[string]any{"type": "click", "x": x, "y": y})
}

func (s *session) press(w int64, n node) {
	s.t.Helper()
	s.must("act", map[string]any{"window": w, "id": n.ID, "action": "press"}, nil)
	s.idle(w)
}

func (s *session) center(w int64, n node) {
	s.t.Helper()
	s.click(w, n.Bounds.X+n.Bounds.Width/2, n.Bounds.Y+n.Bounds.Height/2)
}

// eventually polls ok until settle.
func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), settle)
	defer cancel()
	for !ok() {
		select {
		case <-ctx.Done():
			t.Fatalf("waiting for %s", what)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
