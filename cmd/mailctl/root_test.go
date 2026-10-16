package main

import (
	"runtime"
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

func TestDialDefaultSocket(t *testing.T) {
	t.Setenv("FROSTMAIL_SOCKET", "")
	t.Setenv("XDG_RUNTIME_DIR", "")
	c, _, err := (&rootOptions{}).dial(t.Context())
	// Linux needs XDG_RUNTIME_DIR; Windows dials the user's own pipe,
	// where a maild of the user's may answer.
	want := "XDG_RUNTIME_DIR"
	if runtime.GOOS == "windows" {
		if err == nil {
			_ = c.Close()
			return
		}
		want = `\\.\pipe\frostmail-S-1-`
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("dial without a socket = %v, want an error naming %s", err, want)
	}
}
