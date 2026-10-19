// Command lodgeprobe checks a maild serving lodge's mail app socket the way
// lodge reaches it, for smoke tests of maild --lodge (dev/lodge/run.sh, a
// deployment; docs/specs/lodge-app.md): GET /health, a session over /api
// that lists the mailboxes and renders the newest inbox message for the
// content route, and that page from /content/ as caller content. The
// socket admits only lodge's uid, so run it as lodge's user (or as the
// LODGE_UID a development root names).
//
//	lodgeprobe -socket ROOT/run/lodge/apps/mail.sock
package main

import (
	"context"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/frostyard/lodge/kit/lodgeapp"

	"github.com/frostyard/frostmail/api"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lodgeprobe:", err)
		os.Exit(1)
	}
}

// The identity lodge's development build gives every peer.
var (
	session = lodgeapp.Caller{Kind: lodgeapp.Session, Login: "e2e@example.com", DisplayName: "E2E",
		NodeID: "e2e-node", NodeName: "e2e", Session: "0123456789abcdef"}
	content = lodgeapp.Caller{Kind: lodgeapp.Content, Login: "e2e@example.com", NodeID: "e2e-node", NodeName: "e2e"}
)

func run(args []string) error {
	fs := flag.NewFlagSet("lodgeprobe", flag.ContinueOnError)
	socket := fs.String("socket", "/run/lodge/apps/mail.sock", "maild's lodge app socket")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", *socket)
		},
		Protocols: new(http.Protocols),
	}
	tr.Protocols.SetHTTP1(true)
	hc := &http.Client{Transport: tr}

	var h lodgeapp.Health
	if err := get(ctx, hc, "/health", lodgeapp.Caller{}, &h); err != nil {
		return err
	}
	if !h.OK {
		return fmt.Errorf("health: not OK: %s", h.Reason)
	}
	fmt.Printf("health: ok, maild %s, protocol %d\n", h.Version, h.Protocol)

	hdr := http.Header{}
	session.SetHeaders(hdr)
	ws, _, err := websocket.Dial(ctx, "ws://lodge/api", &websocket.DialOptions{HTTPClient: hc, HTTPHeader: hdr, Host: "lodge"})
	if err != nil {
		return fmt.Errorf("/api: %w", err)
	}
	ws.SetReadLimit(api.MaxMessageSize + 1)
	c := api.NewClient(websocket.NetConn(ctx, ws, websocket.MessageText))
	defer func() { _ = c.Close() }()
	hello, err := c.RPC().Hello(ctx, &api.RPCHelloParams{Protocol: api.Protocol, Client: "lodgeprobe"})
	if err != nil {
		return fmt.Errorf("rpc.hello: %w", err)
	}
	if hello.Caller == nil || *hello.Caller != api.CallerKindSession {
		return fmt.Errorf("rpc.hello: caller %v, want session", hello.Caller)
	}
	if err := c.RPC().Ping(ctx, &api.RPCPingParams{}); err != nil {
		return fmt.Errorf("rpc.ping: %w", err)
	}
	mbs, err := c.Mailbox().List(ctx, &api.MailboxListParams{})
	if err != nil {
		return fmt.Errorf("mailbox.list: %w", err)
	}
	fmt.Printf("session: %s, as %s; %d mailboxes\n", hello.Server, *hello.Caller, len(mbs))

	inbox := api.MailboxRoleInbox
	v, err := c.View().Open(ctx, &api.ViewOpenParams{Query: api.ViewQuery{Role: &inbox}})
	if err != nil {
		return fmt.Errorf("view.open: %w", err)
	}
	rows, err := c.View().Range(ctx, &api.ViewRangeParams{ID: v.ID, Start: 0, End: min(v.Count, 50)})
	if err != nil {
		return fmt.Errorf("view.range: %w", err)
	}
	yes := true
	for _, row := range rows {
		r, err := c.Message().Render(ctx, &api.MessageRenderParams{ID: row.ID, Content: &yes})
		if err != nil {
			return fmt.Errorf("message.render %d: %w", row.ID, err)
		}
		if r.ContentPath == nil {
			continue // a text-only message
		}
		page, policy, err := fetch(ctx, hc, "/content/"+*r.ContentPath, content)
		if err != nil {
			return err
		}
		if !strings.Contains(policy, "sandbox") || strings.Contains(policy, "allow-scripts") || strings.Contains(page, "<script") {
			return fmt.Errorf("the page of %d: policy %q, or a script in it", row.ID, policy)
		}
		fmt.Printf("content: %q renders at %s, %d bytes, sandboxed\n", row.Subject, *r.ContentPath, len(page))
		fmt.Println("lodgeprobe: ok")
		return nil
	}
	return errors.New("no HTML message in the first 50 of the inbox")
}

func get(ctx context.Context, hc *http.Client, path string, c lodgeapp.Caller, v any) error {
	body, _, err := fetch(ctx, hc, path, c)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(body), v)
}

func fetch(ctx context.Context, hc *http.Client, path string, c lodgeapp.Caller) (body, policy string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://lodge"+path, nil)
	if err != nil {
		return "", "", err
	}
	if c.Kind != "" {
		c.SetHeaders(req.Header)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("%s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return string(b), resp.Header.Get("Content-Security-Policy"), nil
}
