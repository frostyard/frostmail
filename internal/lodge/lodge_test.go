package lodge_test

import (
	"context"
	"encoding/base64"
	"encoding/json/v2"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"golang.org/x/net/websocket"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/lodge"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// pixel is a 1×1 PNG.
var pixel, _ = base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")

func message() string {
	img := base64.StdEncoding.EncodeToString(pixel)
	return strings.ReplaceAll(`From: Ann <ann@mailtest.test>
To: test@mailtest.test
Subject: Pictures
Message-ID: <pictures@mailtest.test>
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="outer"

--outer
Content-Type: multipart/related; boundary="rel"

--rel
Content-Type: text/html; charset=utf-8

<p>Look <img src="cid:dot@x"> and <a href="https://example.com/">read</a></p><script>alert(1)</script>
--rel
Content-Type: image/png
Content-ID: <dot@x>
Content-Transfer-Encoding: base64

`+img+`
--rel--
--outer
Content-Type: text/plain; name="notes.txt"
Content-Disposition: attachment; filename="notes.txt"

the attachment
--outer--
`, "\n", "\r\n")
}

// start runs maild with lodge's content route on a synced account whose
// INBOX holds message().
func start(t *testing.T) (*rpctest.Server, *api.Client, int64) {
	t.Helper()
	mem := imapxtest.StartMem(t)
	if _, err := mem.User.Append("INBOX", strings.NewReader(message()), &imap.AppendOptions{}); err != nil {
		t.Fatal(err)
	}
	cfg := mailsync.Config{PollInterval: time.Hour, IdleMax: time.Minute, MinBackoff: 50 * time.Millisecond, NoPrefetch: true}
	srv := rpctest.StartWith(t, rpctest.Options{Sync: &cfg, Lodge: true})
	c := srv.Dial(t)
	ctx := t.Context()
	inbox := api.MailboxRoleInbox
	v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Role: &inbox}})
	if err != nil {
		t.Fatal(err)
	}
	opts := mem.DialOptions()
	server := api.ServerConfig{Host: opts.Host, Port: int64(opts.Port), TLS: opts.TLS, Username: opts.Username}
	a, err := c.Account().Create(ctx, &api.AccountCreateParams{
		Kind: api.AccountKindIMAP, Email: "test@mailtest.test", Auth: api.AuthKindPassword, IMAP: &server, SMTP: &server,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Account().SetPassword(ctx, &api.AccountSetPasswordParams{ID: a.ID, Password: imapxtest.Password}); err != nil {
		t.Fatal(err)
	}
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev := <-c.Notifications():
			var d api.ViewDelta
			if ev.Event != "view.delta" || json.Unmarshal(ev.Data, &d) != nil || d.ID != v.ID || d.Count == 0 {
				continue
			}
			rows, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: 1})
			if err != nil {
				t.Fatal(err)
			}
			return srv, c, rows[0].ID
		case <-timeout:
			t.Fatal("the message never synced")
		}
	}
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func serve(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/content/"+path, nil))
	return rec
}

func TestRenderKeepsTheReaderPage(t *testing.T) {
	srv, c, id := start(t)
	ctx := t.Context()

	r, err := c.Message().Render(ctx, &api.MessageRenderParams{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	if r.ContentPath != nil {
		t.Errorf("render without content named a page: %s", *r.ContentPath)
	}
	yes := true
	r, err = c.Message().Render(ctx, &api.MessageRenderParams{ID: id, Content: &yes})
	if err != nil {
		t.Fatal(err)
	}
	if r.ContentPath == nil || !strings.HasPrefix(*r.ContentPath, "r/") {
		t.Fatalf("contentPath = %v, want r/<token>.html", r.ContentPath)
	}
	rec := serve(t, srv.Content, *r.ContentPath)
	if rec.Code != http.StatusOK {
		t.Fatalf("the page = %d %s", rec.Code, rec.Body)
	}
	page := rec.Body.String()
	for _, want := range []string{`<base target="_blank">`, `src="data:image/png;base64,`, `href="https://example.com/"`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s:\n%s", want, page)
		}
	}
	for _, never := range []string{"<script", "alert(1)", "cid:", "mailpart:"} {
		if strings.Contains(page, never) {
			t.Errorf("the page holds %s:\n%s", never, page)
		}
	}

	msg, err := c.Message().Get(ctx, &api.MessageGetParams{ID: id})
	if err != nil {
		t.Fatal(err)
	}
	var part string
	for _, p := range msg.Parts {
		if p.Filename == "notes.txt" {
			part = p.Path
		}
	}
	if part == "" {
		t.Fatalf("no attachment among %+v", msg.Parts)
	}
	f, err := c.Message().Part(ctx, &api.MessagePartParams{ID: id, Path: part})
	if err != nil {
		t.Fatal(err)
	}
	if f.ContentPath == nil || !strings.HasPrefix(*f.ContentPath, "p/") {
		t.Fatalf("part contentPath = %v, want p/<mac>/<path>", f.ContentPath)
	}
	rec = serve(t, srv.Content, *f.ContentPath)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "the attachment" ||
		rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Errorf("the part = %d %q (%s)", rec.Code, rec.Body, rec.Header().Get("Content-Type"))
	}

	// A session renders the same way.
	sess, _ := srv.DialAs(t, api.Caller{Kind: api.CallerKindSession, Login: "e2e@example.com", NodeName: "e2e"})
	r, err = sess.Message().Render(ctx, &api.MessageRenderParams{ID: id, Content: &yes})
	if err != nil || r.ContentPath == nil {
		t.Fatalf("render as a session = %v, %v", r, err)
	}
}

func TestAPIFrames(t *testing.T) {
	srv := rpctest.Start(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	a := &lodge.API{Ctx: ctx, RPC: srv.RPC, Router: srv.Router, Log: discard()}
	caller := api.Caller{Kind: api.CallerKindSession, Login: "e2e@example.com", NodeName: "e2e", Session: "0123456789abcdef"}
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { a.Serve(w, r, caller) }))
	t.Cleanup(hs.Close)

	dial := func() *websocket.Conn {
		t.Helper()
		ws, err := websocket.Dial("ws"+strings.TrimPrefix(hs.URL, "http")+"/api", "", "http://unchecked.example")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = ws.Close() })
		_ = ws.SetDeadline(time.Now().Add(5 * time.Second))
		return ws
	}
	recv := func(ws *websocket.Conn) map[string]any {
		t.Helper()
		var msg string
		if err := websocket.Message.Receive(ws, &msg); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(msg, "\n") || strings.Count(msg, "\n") != 1 {
			t.Errorf("frame %q: want one message ending in a newline", msg)
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(msg), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}

	ws := dial()
	// A frame without a newline is one message, and so is one with it.
	if err := websocket.Message.Send(ws, `{"jsonrpc":"2.0","id":1,"method":"rpc.hello","params":{"protocol":1,"client":"t"}}`); err != nil {
		t.Fatal(err)
	}
	hello := recv(ws)
	res, _ := hello["result"].(map[string]any)
	if res["caller"] != "session" || res["login"] != "e2e@example.com" || res["nodeName"] != "e2e" {
		t.Fatalf("hello = %v, want the session's caller", hello)
	}
	if err := websocket.Message.Send(ws, `{"jsonrpc":"2.0","id":2,"method":"rpc.ping"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	if m := recv(ws); m["id"] != float64(2) || m["error"] != nil {
		t.Fatalf("ping = %v", m)
	}
	if err := websocket.Message.Send(ws, `{"jsonrpc":"2.0","id":3,"method":"message.save","params":{"id":1,"path":"/tmp/x"}}`); err != nil {
		t.Fatal(err)
	}
	if e, _ := recv(ws)["error"].(map[string]any); e["code"] != float64(api.CodeMethodNotFound) {
		t.Fatalf("message.save as a session = %v, want methodNotFound", e)
	}

	// A frame holding two messages ends the connection.
	ws = dial()
	if err := websocket.Message.Send(ws, "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"rpc.ping\"}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"rpc.ping\"}\n"); err != nil {
		t.Fatal(err)
	}
	var msg string
	if err := websocket.Message.Receive(ws, &msg); err == nil {
		t.Fatalf("a frame of two messages answered %q, want the connection closed", msg)
	}
}
