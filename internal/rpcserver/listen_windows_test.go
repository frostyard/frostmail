package rpcserver_test

import (
	"context"
	"net"
	"testing"
	"time"
	"unsafe"

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
	// Compared as an ACE, not as SDDL text, which writes well-known SIDs as
	// aliases (the built-in Administrator, as CI runs, is LA).
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 || dacl == nil || dacl.AceCount != 1 {
		t.Fatalf("pipe DACL = %s: want one entry, protected from inheritance", sd)
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		t.Fatal(err)
	}
	sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE || !sid.Equals(user.User.Sid) ||
		ace.Mask&windows.FILE_GENERIC_READ != windows.FILE_GENERIC_READ || ace.Mask&windows.FILE_GENERIC_WRITE != windows.FILE_GENERIC_WRITE {
		t.Errorf("pipe DACL = %s: want full access for %s alone", sd, user.User.Sid)
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
