//go:build !windows

package main

// These tests need a real data lock, which is a no-op on Windows
// (internal/datalock), or a Unix socket listener of their own.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/frostyard/frostmail/internal/datalock"
	"github.com/frostyard/frostmail/internal/rpctest"
)

// After the socket stops answering, maild can still hold the data lock.
// shutdown waits until it can take that lock, and fails when it cannot.
func TestShutdownWaitsForTheDataLock(t *testing.T) {
	data := t.TempDir()
	t.Setenv("FROSTMAIL_DATA_DIR", data)
	release, err := datalock.Acquire(data) // stands for the stopping maild's lock
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// The socket stops, the lock stays: shutdown fails at its limit.
	first := rpctest.Start(t)
	var out bytes.Buffer
	err = runShutdown(t.Context(), &out, first.Socket, data, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "still holds the lock") || out.Len() != 0 {
		t.Fatalf("runShutdown = %q, %v; want an error that maild still holds the lock", out.String(), err)
	}
	<-first.Stopped()

	// The lock goes while shutdown waits for it: shutdown then returns 0.
	second := rpctest.Start(t)
	done := make(chan error, 1)
	go func() { done <- runShutdown(t.Context(), &bytes.Buffer{}, second.Socket, data, shutdownWait) }()
	<-second.Stopped() // the socket answers no more, the lock is still held
	select {
	case err := <-done:
		t.Fatalf("shutdown returned %v while the lock was held", err)
	default:
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("shutdown after the lock went = %v; want success", err)
	}
	// shutdown took the lock only to let it go.
	again, err := datalock.Acquire(data)
	if err != nil {
		t.Fatalf("the lock after shutdown = %v; want it free", err)
	}
	again()
}

// No socket answers, but the lock is held: a maild that is starting (it
// locks before it listens) or stopping. shutdown waits for the lock, and
// exits 0 once it is free; when it is never free, it fails by its deadline
// with the lock error.
func TestShutdownWithNoSocketWaitsForTheLock(t *testing.T) {
	data := t.TempDir()
	socket := filepath.Join(t.TempDir(), "maild.sock") // nothing listens
	release, err := datalock.Acquire(data)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	var out bytes.Buffer
	err = runShutdown(t.Context(), &out, socket, data, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "still holds the lock") || !errors.Is(err, context.DeadlineExceeded) || out.Len() != 0 {
		t.Fatalf("runShutdown = %q, %v; want the lock error at the deadline", out.String(), err)
	}

	out.Reset()
	done := make(chan error, 1)
	go func() { done <- runShutdown(t.Context(), &out, socket, data, shutdownWait) }()
	select {
	case err := <-done:
		t.Fatalf("shutdown returned %v while the lock was held", err) // the lock is still ours
	default:
	}
	release()
	if err := <-done; err != nil {
		t.Fatalf("shutdown after the lock went = %v; want success", err)
	}
	if out.String() == "" {
		t.Fatal("shutdown printed nothing")
	}
	again, err := datalock.Acquire(data)
	if err != nil {
		t.Fatalf("the lock after shutdown = %v; want it free", err)
	}
	again()

	// Nothing answers and the lock is free at once: the plain message.
	out.Reset()
	if err := runShutdown(t.Context(), &out, socket, data, shutdownWait); err != nil || out.String() != "maild is not running\n" {
		t.Fatalf("runShutdown = %q, %v; want %q", out.String(), err, "maild is not running\n")
	}
}

// A connection that ends before rpc.shutdown answers is not a failure:
// maild closes its connections as it stops, so the reply can be lost. The
// wait then checks the socket and the lock, as it does after any ask.
func TestShutdownTakesAClosedConnectionAsAnAsk(t *testing.T) {
	// A maild that stops on rpc.shutdown, closing the connection before it
	// replies: the lock it held goes, and shutdown exits 0.
	t.Run("a maild that stops", func(t *testing.T) {
		data := t.TempDir()
		release, err := datalock.Acquire(data) // stands for the stopping maild's lock
		if err != nil {
			t.Fatal(err)
		}
		letGo := sync.OnceFunc(release)
		t.Cleanup(letGo)
		socket, stopped := standIn(t, true)

		var out bytes.Buffer
		done := make(chan error, 1)
		go func() {
			done <- runShutdown(t.Context(), &out, socket, data, shutdownWait)
		}()
		<-stopped // the stand-in answered rpc.hello, got rpc.shutdown and stopped
		select {
		case err := <-done:
			t.Fatalf("shutdown returned %v after a lost reply; want it to wait for the lock", err)
		default:
		}
		letGo()
		if err := <-done; err != nil {
			t.Fatalf("shutdown after the lock went = %v; want success", err)
		}
		if got := out.String(); got != "maild stopped\n" {
			t.Fatalf("output = %q; want %q", got, "maild stopped\n")
		}
		again, err := datalock.Acquire(data)
		if err != nil {
			t.Fatalf("the lock after shutdown = %v; want it free", err)
		}
		again()
	})

	// A maild that drops the connection on rpc.shutdown but keeps answering
	// rpc.hello does not stop: shutdown asks again, and ends at its deadline.
	t.Run("a maild that still answers", func(t *testing.T) {
		socket, _ := standIn(t, false)
		var out bytes.Buffer
		err := runShutdown(t.Context(), &out, socket, t.TempDir(), 300*time.Millisecond)
		if err == nil || !strings.Contains(err.Error(), "still answers") || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("runShutdown = %v; want an error that maild still answers at the deadline", err)
		}
		if out.Len() != 0 {
			t.Fatalf("output = %q; want none", out.String())
		}
	})
}

// standIn is a maild that closes its connection on rpc.shutdown without a
// reply: with stop, it also stops, closing its listener. It answers
// rpc.hello as maild does. stopped is closed once it has stopped, and nil
// when it does not stop.
func standIn(t *testing.T, stop bool) (socket string, stopped <-chan struct{}) {
	t.Helper()
	socket, cleanup, err := rpctest.SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sync.OnceFunc(func() { _ = ln.Close() }))
	gone := make(chan struct{})
	var goneOnce sync.Once
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			closeConn := sync.OnceFunc(func() { _ = nc.Close() })
			t.Cleanup(closeConn)
			go func() {
				sc := bufio.NewScanner(nc)
				for sc.Scan() {
					var req struct {
						ID     json.RawMessage `json:"id"`
						Method string          `json:"method"`
					}
					if json.Unmarshal(sc.Bytes(), &req) != nil {
						continue
					}
					switch req.Method {
					case "rpc.hello":
						_, _ = fmt.Fprintf(nc, `{"jsonrpc":"2.0","id":%s,"result":{"protocol":1,"server":"stand-in"}}`+"\n", req.ID)
					case "rpc.shutdown":
						closeConn() // no reply: maild closes its connections as it stops
						if stop {
							_ = ln.Close() // it stops: no new connection, no new ask
							goneOnce.Do(func() { close(gone) })
						}
					}
				}
			}()
		}
	}()
	if !stop {
		return socket, nil
	}
	return socket, gone
}

// listen runs a stand-in on a new socket: it answers rpc.hello when hello is
// set, never answers rpc.shutdown, and never answers anything else.
func listen(t *testing.T, hello bool) string {
	t.Helper()
	socket, cleanup, err := rpctest.SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = nc.Close() })
			go func() {
				sc := bufio.NewScanner(nc)
				for sc.Scan() {
					var req struct {
						ID     json.RawMessage `json:"id"`
						Method string          `json:"method"`
					}
					if json.Unmarshal(sc.Bytes(), &req) != nil || req.Method != "rpc.hello" || !hello {
						continue // no answer
					}
					_, _ = fmt.Fprintf(nc, `{"jsonrpc":"2.0","id":%s,"result":{"protocol":1,"server":"stand-in"}}`+"\n", req.ID)
				}
			}()
		}
	}()
	return socket
}

// One deadline covers every call: a listener that accepts and never answers
// rpc.hello, or that answers it and never answers rpc.shutdown, ends
// shutdown at its deadline, not never.
func TestShutdownEndsAtItsDeadlineWhenMaildNeverAnswers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		hello bool
		want  string
	}{
		{"no answer to rpc.hello", false, "did not answer on"},
		{"no answer to rpc.shutdown", true, "did not answer rpc.shutdown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := runShutdown(t.Context(), &out, listen(t, tc.hello), t.TempDir(), 300*time.Millisecond)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("runShutdown = %v; want an error with %q at the deadline", err, tc.want)
			}
			if out.Len() != 0 {
				t.Fatalf("output = %q; want none", out.String())
			}
		})
	}
}
