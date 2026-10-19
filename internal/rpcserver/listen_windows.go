package rpcserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"runtime"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// Listen creates maild's named pipe at path (\\.\pipe\frostmail-<SID>,
// ADR-0038). Its DACL grants only the user maild runs as, and remote
// clients are refused. A pipe already there means maild is running: a
// named pipe disappears with the process that made it, so none is ever
// stale. Accept admits only clients running as that user, and closes the
// rest, logged to log: on Linux lodge's kit does the same (listen_unix.go).
func Listen(path string, log *slog.Logger) (net.Listener, error) {
	sid, err := ownSID()
	if err != nil {
		return nil, err
	}
	ln, err := winio.ListenPipe(path, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;" + sid.String() + ")",
		InputBufferSize:    64 << 10,
		OutputBufferSize:   64 << 10,
	})
	if err != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if c, derr := winio.DialPipeAccessImpLevel(ctx, path, windows.GENERIC_READ|windows.GENERIC_WRITE,
			winio.PipeImpLevelIdentification); derr == nil {
			_ = c.Close()
			return nil, fmt.Errorf("maild is already listening on %s", path)
		}
		return nil, fmt.Errorf("listen: %w", err)
	}
	return &peerChecked{Listener: ln, log: log}, nil
}

// peerChecked is the pipe's listener: Accept admits only the user's own
// clients (checkPeer), and closes the rest, logged to log.
type peerChecked struct {
	net.Listener
	log *slog.Logger
}

func (l *peerChecked) Accept() (net.Conn, error) {
	for {
		nc, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		if err := checkPeer(nc); err != nil {
			if l.log != nil {
				l.log.Warn("rejected connection", "err", err)
			}
			_ = nc.Close()
			continue
		}
		return nc, nil
	}
}

// checkPeer admits only processes of the user running maild: it
// impersonates the pipe's client and reads the user of its token, which
// needs no access to the client's process (one in another logon session,
// such as SSH's, grants none) and cannot race a reused process ID.
func checkPeer(nc net.Conn) error {
	f, ok := nc.(interface{ Fd() uintptr })
	if !ok {
		return fmt.Errorf("not a named pipe connection: %T", nc)
	}
	own, err := ownSID()
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		// The impersonation is the thread's: lock it, and should reverting
		// fail, leave it locked, so that the goroutine's end ends the
		// thread with it.
		runtime.LockOSThread()
		reverted, err := clientIs(windows.Handle(f.Fd()), own)
		if reverted {
			runtime.UnlockOSThread()
		}
		done <- err
	}()
	return <-done
}

var procImpersonateNamedPipeClient = windows.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateNamedPipeClient")

// clientIs reports whether the thread stopped impersonating the pipe's
// client, and an error unless the client runs as want.
func clientIs(h windows.Handle, want *windows.SID) (reverted bool, err error) {
	if r, _, err := procImpersonateNamedPipeClient.Call(uintptr(h)); r == 0 {
		return true, fmt.Errorf("impersonate the pipe client: %w", err)
	}
	var tok windows.Token
	err = windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &tok)
	if rerr := windows.RevertToSelf(); rerr != nil {
		if err == nil {
			_ = tok.Close()
		}
		return false, fmt.Errorf("stop impersonating the pipe client: %w", rerr)
	}
	if err != nil {
		return true, fmt.Errorf("the pipe client's token: %w", err)
	}
	defer func() { _ = tok.Close() }()
	user, err := tok.GetTokenUser()
	if err != nil {
		return true, fmt.Errorf("the pipe client's user: %w", err)
	}
	if !user.User.Sid.Equals(want) {
		return true, fmt.Errorf("the pipe client runs as %s, not %s", user.User.Sid, want)
	}
	return true, nil
}

func ownSID() (*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("the process's user: %w", err)
	}
	return user.User.Sid, nil
}
