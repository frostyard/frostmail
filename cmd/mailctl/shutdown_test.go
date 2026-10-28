package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func TestHelloBuild(t *testing.T) {
	with := rpctest.StartWith(t, rpctest.Options{Build: "0a1b2c"})
	out, err := runMailctl(t, "--socket", with.Socket, "hello", "--build")
	if err != nil || out != "0a1b2c\n" {
		t.Fatalf("hello --build = %q, %v; want the build only", out, err)
	}
	// --build wins over --json, and a maild with no build gives an empty line.
	without := rpctest.Start(t)
	out, err = runMailctl(t, "--socket", without.Socket, "--json", "hello", "--build")
	if err != nil || out != "\n" {
		t.Fatalf("hello --build with no build = %q, %v; want an empty line", out, err)
	}
	// Plain hello keeps its output.
	out, err = runMailctl(t, "--socket", with.Socket, "hello")
	if err != nil || out != "maild rpctest (protocol 1)\n" {
		t.Fatalf("hello = %q, %v", out, err)
	}
	missing := filepath.Join(t.TempDir(), "maild.sock")
	if out, err := runMailctl(t, "--socket", missing, "hello", "--build"); err == nil || out != "" {
		t.Fatalf("hello --build without maild = %q, %v; want an error and no output", out, err)
	}
}

func TestShutdownStopsMaildAndWaits(t *testing.T) {
	t.Setenv("FROSTMAIL_DATA_DIR", t.TempDir())
	srv := rpctest.Start(t)
	out, err := runMailctl(t, "--socket", srv.Socket, "shutdown")
	if err != nil || out != "maild stopped\n" {
		t.Fatalf("shutdown = %q, %v", out, err)
	}
	select {
	case <-srv.Stopped(): // by the time mailctl returned, maild no longer answered
	default:
		t.Fatal("shutdown returned while maild was still serving")
	}
	if maildAnswers(t.Context(), srv.Socket) {
		t.Fatal("maild answers after shutdown")
	}
	// Nothing answers now: that is success.
	out, err = runMailctl(t, "--socket", srv.Socket, "shutdown")
	if err != nil || out != "maild is not running\n" {
		t.Fatalf("shutdown with no maild = %q, %v; want %q", out, err, "maild is not running\n")
	}
}

// stayingRPC accepts rpc.shutdown and does nothing, as a maild that does
// not stop.
type stayingRPC struct{ api.RPCService }

func (stayingRPC) Shutdown(context.Context, *api.RPCShutdownParams) error { return nil }

func TestShutdownFailsWhenMaildKeepsAnswering(t *testing.T) {
	srv := rpctest.StartWith(t, rpctest.Options{Services: func(s *api.Services) { s.RPC = stayingRPC{s.RPC} }})
	var out bytes.Buffer
	err := runShutdown(t.Context(), &out, srv.Socket, t.TempDir(), 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "still answers") {
		t.Fatalf("runShutdown = %v; want an error that maild still answers", err)
	}
	if out.Len() != 0 {
		t.Fatalf("output = %q; want none", out.String())
	}
	if !maildAnswers(t.Context(), srv.Socket) {
		t.Fatal("the stand-in maild stopped")
	}
}

// A data directory that does not exist holds no lock.
func TestShutdownWithNoDataDirectory(t *testing.T) {
	srv := rpctest.Start(t)
	missing := filepath.Join(t.TempDir(), "none")
	if err := runShutdown(t.Context(), &bytes.Buffer{}, srv.Socket, missing, shutdownWait); err != nil {
		t.Fatal(err)
	}
}
