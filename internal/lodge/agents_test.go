package lodge

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// Agents refuses a connection once it is closed, and its serve never runs
// (issue #305).
func TestAgentsRefusesAfterClose(t *testing.T) {
	a := new(Agents)
	a.Close()
	ran := make(chan struct{})
	if a.start(t.Context(), func() { close(ran) }) {
		t.Fatal("start after Close = true; want it refused")
	}
	select {
	case <-ran:
		t.Fatal("the refused serve ran")
	default:
	}
}

// Agents refuses a connection once maild's context has ended (issue #305).
func TestAgentsRefusesAfterTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if new(Agents).start(ctx, func() {}) {
		t.Fatal("start with an ended context = true; want it refused")
	}
}

// An MCP session opened as maild stops gets an error and no client, so it
// never reaches the RPC server (issue #305).
func TestOpenAgentRefusesAsMaildStops(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// No RPC server: a refusal must come before it is used.
	o := Options{Ctx: ctx, Agents: new(Agents)}
	c, err := o.openAgent(t.Context(), "porter", api.Scope{All: true})
	if err == nil || c != nil {
		t.Fatalf("openAgent = %v, %v; want an error and no client", c, err)
	}
}

// Close waits for the connections that run (issue #305).
func TestAgentsCloseWaitsForTheConnectionsItTracks(t *testing.T) {
	a := new(Agents)
	started := make(chan struct{})
	release := make(chan struct{})
	letGo := sync.OnceFunc(func() { close(release) })
	t.Cleanup(letGo)
	if !a.start(t.Context(), func() {
		close(started)
		<-release
	}) {
		t.Fatal("start = false; want it tracked")
	}
	<-started
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		a.Close()
	}()
	select {
	case <-closed:
		t.Fatal("Close returned with a connection still running")
	case <-time.After(50 * time.Millisecond):
	}
	letGo()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return after the connection ended")
	}
}
