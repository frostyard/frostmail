package rpcserver_test

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpcserver"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/store"
)

// rawConn speaks the wire format directly.
type rawConn struct {
	t  *testing.T
	nc net.Conn
	sc *bufio.Scanner
}

func dialRaw(t *testing.T, socket string) *rawConn {
	t.Helper()
	nc, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = nc.Close() })
	return &rawConn{t: t, nc: nc, sc: bufio.NewScanner(nc)}
}

func (r *rawConn) send(line string) {
	r.t.Helper()
	if _, err := r.nc.Write([]byte(line + "\n")); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rawConn) recv() string {
	r.t.Helper()
	_ = r.nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	if !r.sc.Scan() {
		r.t.Fatalf("no reply: %v", r.sc.Err())
	}
	return r.sc.Text()
}

func TestWireRules(t *testing.T) {
	srv := rpctest.Start(t)
	r := dialRaw(t, srv.Socket)

	r.send(`{"jsonrpc":"2.0","id":1,"method":"account.list"}`)
	if got := r.recv(); !strings.Contains(got, `"code":1004`) || !strings.Contains(got, `"id":1`) {
		t.Fatalf("call before hello = %s, want notReady", got)
	}
	r.send(`{"jsonrpc":"2.0","id":"h","method":"rpc.hello","params":{"protocol":999,"client":"t"}}`)
	if got := r.recv(); !strings.Contains(got, `"code":1003`) {
		t.Fatalf("wrong protocol = %s, want protocolMismatch", got)
	}
	// hello and a pipelined call: the call must see the connection ready.
	r.send(`{"jsonrpc":"2.0","id":2,"method":"rpc.hello","params":{"protocol":1,"client":"t"}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"mailbox.list"}`)
	if got := r.recv(); got != `{"jsonrpc":"2.0","id":2,"result":{"protocol":1,"server":"maild rpctest"}}` {
		t.Fatalf("hello = %s", got)
	}
	if got := r.recv(); got != `{"jsonrpc":"2.0","id":3,"result":[]}` {
		t.Fatalf("mailbox.list = %s", got)
	}
	for _, tc := range []struct{ req, want string }{
		{`not json`, `"code":-32700`},
		{`{"jsonrpc":"1.0","id":4,"method":"x"}`, `"code":-32600`},
		{`{"jsonrpc":"2.0","id":5,"method":"no.such"}`, `"code":-32601`},
		{`{"jsonrpc":"2.0","id":6,"method":"account.get","params":{}}`, `missing required param \"id\"`},
		{`{"jsonrpc":"2.0","id":7,"method":"account.get","params":{"id":1,"extra":true}}`, `"code":-32602`},
		{`{"jsonrpc":"2.0","id":8,"method":"account.get","params":[1]}`, `params must be an object`},
	} {
		r.send(tc.req)
		if got := r.recv(); !strings.Contains(got, tc.want) {
			t.Errorf("%s -> %s, want %s", tc.req, got, tc.want)
		}
	}
}

func TestEventsResumeAfterReconnect(t *testing.T) {
	srv := rpctest.Start(t)
	ctx := t.Context()
	emit := func(id int64) {
		t.Helper()
		if err := srv.DB.Tx(ctx, func(tx *store.Tx) error { return tx.Emit(ctx, api.AccountChanged{ID: id}) }); err != nil {
			t.Fatal(err)
		}
	}
	emit(1)
	emit(2)

	c := srv.Dial(t)
	since := int64(1)
	sub, err := c.Events().Subscribe(ctx, &api.EventsSubscribeParams{SinceSeq: &since})
	if err != nil || sub.Seq != 2 || sub.Resync {
		t.Fatalf("Subscribe = %+v, %v", sub, err)
	}
	emit(3)
	for _, wantSeq := range []int64{2, 3} {
		select {
		case ev := <-c.Notifications():
			e, err := api.DecodeEvent(ev.Event, ev.Data)
			if err != nil || ev.Seq != wantSeq || e.(api.AccountChanged).ID != wantSeq {
				t.Fatalf("event = %+v (%v), want seq %d", ev, err, wantSeq)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no event seq %d", wantSeq)
		}
	}
	if _, err := c.Events().Subscribe(ctx, nil); !errors.Is(err, api.ErrConflict) {
		t.Fatalf("second subscribe = %v, want conflict", err)
	}
}

func TestListenRejectsLiveSocketAndOpenDirectory(t *testing.T) {
	srv := rpctest.Start(t)
	if _, err := rpcserver.Listen(srv.Socket); err == nil || !strings.Contains(err.Error(), "already listening") {
		t.Fatalf("Listen on a live socket = %v", err)
	}
	dir, err := os.MkdirTemp("", "fm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := rpcserver.Listen(filepath.Join(dir, "maild.sock")); err == nil || !strings.Contains(err.Error(), "want 0700") {
		t.Fatalf("Listen in a 0755 directory = %v", err)
	}
}

func TestRepeatedHello(t *testing.T) {
	srv := rpctest.Start(t)
	c := srv.Dial(t)
	if _, err := c.RPC().Hello(context.Background(), &api.RPCHelloParams{Protocol: api.Protocol, Client: "again"}); err != nil {
		t.Fatalf("repeated hello: %v", err)
	}
}

// TestAttended: the app is attending while one of its connections that
// subscribed to events is open; other clients do not count.
func TestAttended(t *testing.T) {
	changes := make(chan bool, 4)
	srv := rpctest.StartWith(t, rpctest.Options{OnAttended: func(a bool) { changes <- a }})
	hello := func(client string) *rawConn {
		r := dialRaw(t, srv.Socket)
		r.send(`{"jsonrpc":"2.0","id":1,"method":"rpc.hello","params":{"protocol":1,"client":"` + client + `"}}`)
		if got := r.recv(); !strings.Contains(got, `"result"`) {
			t.Fatalf("hello = %s", got)
		}
		return r
	}
	subscribe := func(r *rawConn) {
		r.send(`{"jsonrpc":"2.0","id":2,"method":"events.subscribe","params":{}}`)
		if got := r.recv(); !strings.Contains(got, `"result"`) {
			t.Fatalf("subscribe = %s", got)
		}
	}
	cli := hello("mailctl dev")
	subscribe(cli)
	app := hello(rpcserver.AppClient + " 0.1.0")
	if srv.RPC.Attended() {
		t.Error("attended before the app subscribed")
	}
	subscribe(app)
	if got := <-changes; !got || !srv.RPC.Attended() {
		t.Errorf("after the app subscribed: change %v, attended %v", got, srv.RPC.Attended())
	}
	_ = app.nc.Close()
	select {
	case got := <-changes:
		if got || srv.RPC.Attended() {
			t.Errorf("after the app left: change %v, attended %v", got, srv.RPC.Attended())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no change when the app left")
	}
	_ = cli
}
