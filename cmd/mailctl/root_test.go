package main

import (
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/rpctest"
)

func TestDialUsesSocketFlag(t *testing.T) {
	srv := rpctest.Start(t)
	opts := &rootOptions{socket: srv.Socket}
	c, h, err := opts.dial(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if h.Server != rpctest.Name {
		t.Fatalf("hello = %+v", h)
	}
}

func TestDialDefaultNeedsRuntimeDir(t *testing.T) {
	t.Setenv("FROSTMAIL_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	_, _, err := (&rootOptions{}).dial(t.Context())
	if err == nil || !strings.Contains(err.Error(), "XDG_RUNTIME_DIR") {
		t.Fatalf("dial without a socket = %v", err)
	}
}
