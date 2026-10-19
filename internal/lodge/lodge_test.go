package lodge_test

import (
	"encoding/base64"
	"encoding/json/v2"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/lodge/kit/kittest"
	kitlodgeapp "github.com/frostyard/lodge/kit/lodgeapp"
	"github.com/frostyard/lodge/kit/manifest"

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
	// Through the socket, as lodge-content asks for it.
	l := lodgeSocket(t, srv)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/content/"+*r.ContentPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := l.Do(req, kittest.Content())
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Security-Policy") != lodge.PagePolicy {
		t.Fatalf("the page = %d (%s) %s", resp.StatusCode, resp.Header.Get("Content-Security-Policy"), body)
	}
	if code, _ := l.Get(t, "/content/"+*r.ContentPath, kittest.Session()); code != http.StatusForbidden {
		t.Errorf("the page as a session = %d, want 403", code)
	}
	page := string(body)
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
	if code, got := l.Get(t, "/content/"+*f.ContentPath, kittest.Content()); code != http.StatusOK || strings.TrimSpace(got) != "the attachment" {
		t.Errorf("the part = %d %q", code, got)
	}

	// A session renders the same way.
	sess, _ := srv.DialAs(t, api.Caller{Kind: api.CallerKindSession, Login: "e2e@example.com", NodeName: "e2e"})
	r, err = sess.Message().Render(ctx, &api.MessageRenderParams{ID: id, Content: &yes})
	if err != nil || r.ContentPath == nil {
		t.Fatalf("render as a session = %v, %v", r, err)
	}
}

// onLinux skips a test that drives lodge's app socket off Linux: lodge and
// its app sockets are Linux-only, and the kit's peer check refuses every
// connection elsewhere (ADR-0041).
func onLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("lodge's app sockets are Linux-only; maild serves its local socket on " + runtime.GOOS)
	}
}

// startLodge serves srv as mail's lodge socket, with kittest playing lodge.
func startLodge(t *testing.T, srv *kitlodgeapp.Server) *kittest.Lodge {
	t.Helper()
	onLinux(t)
	return kittest.Start(t, lodge.ID, srv)
}

// lodgeSocket serves maild's lodge socket for srv, with kittest playing lodge.
func lodgeSocket(t *testing.T, srv *rpctest.Server) *kittest.Lodge {
	t.Helper()
	return startLodge(t, lodge.NewServer(lodge.Options{
		Ctx: t.Context(), Version: "test", RPC: srv.RPC, Router: srv.Router, Content: srv.Content, Log: discard(),
	}))
}

func TestSocketRules(t *testing.T) {
	data, err := os.ReadFile("../../packaging/lodge/app.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Content || !m.ContentLinks || !m.MCP || m.Device || m.RemoteImages || m.Key != "m" || m.User != "frostmail" {
		t.Errorf("manifest %+v, want the spec's: content, contentLinks, mcp, key m, user frostmail, and nothing else", m)
	}
	kittest.CheckRules(t, lodgeSocket(t, rpctest.StartWith(t, rpctest.Options{Lodge: true})), m)
}

func TestSessionOverAPI(t *testing.T) {
	l := lodgeSocket(t, rpctest.StartWith(t, rpctest.Options{Lodge: true}))
	ctx := t.Context()
	nc, err := l.DialAPI(ctx, kittest.Session())
	if err != nil {
		t.Fatal(err)
	}
	c := api.NewClient(nc)
	t.Cleanup(func() { _ = c.Close() })
	h, err := c.RPC().Hello(ctx, &api.RPCHelloParams{Protocol: api.Protocol, Client: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if h.Caller == nil || *h.Caller != api.CallerKindSession || h.Login == nil || *h.Login != kittest.Login ||
		h.NodeName == nil || *h.NodeName != kittest.NodeName {
		t.Fatalf("hello = %+v, want lodge's session", h)
	}
	if err := c.RPC().Ping(ctx, &api.RPCPingParams{}); err != nil {
		t.Fatalf("rpc.ping: %v", err)
	}
	if _, err := c.Account().List(ctx, &api.AccountListParams{}); err != nil {
		t.Fatalf("account.list as a session: %v", err)
	}
	if err := c.Message().Save(ctx, &api.MessageSaveParams{ID: 1, Path: "/tmp/x.eml"}); !errors.Is(err, api.ErrMethodNotFound) {
		t.Fatalf("message.save as a session = %v, want methodNotFound", err)
	}
}

// TestListenRefusesOffLinux: maild --lodge says why it cannot serve lodge
// where lodge does not run.
func TestListenRefusesOffLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("lodge's socket opens on Linux; this checks the refusal elsewhere")
	}
	if _, err := lodge.Listen(); err == nil || !strings.Contains(err.Error(), "lodge runs on Linux") {
		t.Fatalf("Listen on %s = %v, want a refusal naming Linux", runtime.GOOS, err)
	}
}
