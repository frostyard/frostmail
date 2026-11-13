package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/lodge/kit/kittest"
	kit "github.com/frostyard/lodge/kit/lodgeapp"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/frostyard/frostmail/api"
)

// heldAccounts holds the first account.list call before it reads the store,
// so that the agent's call is under way while maild stops.
type heldAccounts struct {
	api.AccountService
	entered chan struct{} // closed when the first call starts
	hold    chan struct{} // closed to let that call go on
	order   *callOrder
	once    sync.Once
}

// List holds its first call on the store, and records when it goes on.
func (a *heldAccounts) List(ctx context.Context, p *api.AccountListParams) ([]api.Account, error) {
	a.once.Do(func() {
		close(a.entered)
		<-a.hold
		a.order.add("call returned")
	})
	return a.AccountService.List(ctx, p)
}

// callOrder records, in order, what a test sees happen.
type callOrder struct {
	mu    sync.Mutex
	names []string
}

func (o *callOrder) add(name string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.names = append(o.names, name)
}

func (o *callOrder) seen() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.names)
}

// agentClient is an MCP client of maild's lodge socket, as an agent with
// scope account=*.
func agentClient(t *testing.T, l *kittest.Lodge) (*sdk.ClientSession, error) {
	t.Helper()
	tr := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", l.Socket)
	}}
	t.Cleanup(tr.CloseIdleConnections)
	hc := &http.Client{Transport: &asAgent{c: kittest.Agent("porter", "account=*"), base: tr}}
	return sdk.NewClient(&sdk.Implementation{Name: "porter", Version: "test"}, nil).
		Connect(t.Context(), &sdk.StreamableClientTransport{Endpoint: "http://lodge/mcp", HTTPClient: hc}, nil)
}

// asAgent sends every request with c's Lodge-* headers.
type asAgent struct {
	c    kit.Caller
	base http.RoundTripper
}

func (a *asAgent) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	a.c.SetHeaders(r.Header)
	return a.base.RoundTrip(r)
}

// An agent's call to /mcp reads and writes the store from a goroutine that
// outlives lodge's socket, so maild must wait for it before it closes the
// database (issue #305).
func TestMaildWaitsForAnAgentsCallBeforeClosingTheStore(t *testing.T) {
	newSandbox(t, false)
	root, err := os.MkdirTemp("", "lodge") // a socket path is at most 108 bytes
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	t.Setenv("LODGE_ROOT", root)
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan struct{})
	entered := make(chan struct{})
	hold := make(chan struct{})
	var once sync.Once
	letGo := func() { once.Do(func() { close(hold) }) }
	order := new(callOrder)
	var runErr error
	finished := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		letGo()
		<-finished
	})
	go func() {
		defer close(finished)
		runErr = runWith(ctx, []string{"-lodge"}, runOptions{
			serving: func() { close(ready) },
			services: func(s *api.Services) {
				s.Account = &heldAccounts{AccountService: s.Account, entered: entered, hold: hold, order: order}
			},
			closing: func() { order.add("closed") },
		})
	}()
	select {
	case <-ready:
	case <-finished:
		t.Fatalf("maild stopped before it served: %v", runErr)
	}
	cs, err := agentClient(t, kittest.Attach(t, "mail", root))
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		// The call returns when maild's context ends; the handler under it
		// does not.
		_, _ = cs.CallTool(t.Context(), &sdk.CallToolParams{Name: "list_mailboxes"})
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the agent's call never reached the store")
	}
	// The call is held in the store, so maild must not close the store yet.
	cancel()
	select {
	case <-finished:
		t.Fatalf("maild closed the store with the agent's call still running: %v", runErr)
	case <-time.After(100 * time.Millisecond):
	}
	if got := order.seen(); len(got) != 0 {
		t.Fatalf("maild closed the store with the agent's call still running: %v", got)
	}
	letGo()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("maild did not stop after the agent's call returned")
	}
	if runErr != nil {
		t.Fatalf("maild stopped with %v", runErr)
	}
	if got, want := order.seen(), []string{"call returned", "closed"}; !slices.Equal(got, want) {
		t.Fatalf("the agent's call and the close = %v; want %v", got, want)
	}
}
