package rpcserver_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/frostyard/frostmail/internal/rpcserver"
	"github.com/frostyard/frostmail/internal/rpctest"
)

func dialConn(path string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return winio.DialPipeAccessImpLevel(ctx, path, windows.GENERIC_READ|windows.GENERIC_WRITE, winio.PipeImpLevelIdentification)
}

// maild's pipe grants only its user: its DACL has one entry, for the
// process's own SID, and no inherited ones.
func TestPipeGrantsOnlyTheUser(t *testing.T) {
	path, _, err := rpctest.SocketPath()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := rpcserver.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		if c, err := ln.Accept(); err == nil {
			defer func() { _ = c.Close() }()
			_, _ = c.Read(make([]byte, 1))
		}
	}()
	c, err := dialConn(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	h := windows.Handle(c.(interface{ Fd() uintptr }).Fd())
	sd, err := windows.GetSecurityInfo(h, windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	want := "D:P(A;;FA;;;" + user.User.Sid.String() + ")"
	if got := sd.String(); got != want {
		t.Errorf("pipe DACL = %s, want %s", got, want)
	}
}

// A client that connects anonymously gives maild no token to check: it is
// refused before it can call anything.
func TestAnonymousClientRefused(t *testing.T) {
	srv := rpctest.Start(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	nc, err := winio.DialPipeAccessImpLevel(ctx, srv.Socket, windows.GENERIC_READ|windows.GENERIC_WRITE, winio.PipeImpLevelAnonymous)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = nc.Close() }()
	_, _ = nc.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"rpc.hello","params":{"protocol":1,"client":"anon"}}` + "\n"))
	_ = nc.SetReadDeadline(time.Now().Add(5 * time.Second))
	if n, err := nc.Read(make([]byte, 64)); err == nil {
		t.Fatalf("an anonymous client got %d bytes back", n)
	}
}
