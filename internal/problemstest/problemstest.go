//go:build linux

// Package problemstest is a journal socket for tests of problem records
// (ADR-0049): a unixgram socket in a temporary folder that a recorder made
// with problems.WithSocket writes to, and the checks of what it received.
package problemstest

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/frostyard/std/problems"

	"github.com/frostyard/frostmail/internal/config"
)

// AppID is the app ID of every Frostmail record.
const AppID = config.AppID

// Version is the release version the tests' recorders write.
const Version = "1.2.3"

// wait bounds a read of a record that the code under test has sent already:
// a recorder sends its datagram before the call returns, so the wait is
// only a guard against a test that hangs.
const wait = 10 * time.Second

// Journal is a test journal socket.
type Journal struct {
	// Path is the socket's path, for problems.WithSocket.
	Path string
	conn *net.UnixConn
}

// Listen binds a journal socket in a new temporary folder, which the test's
// end removes. The folder is short: a socket path has at most 107 bytes.
func Listen(t testing.TB) *Journal {
	t.Helper()
	dir, err := os.MkdirTemp("", "fm-journal-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "socket")
	conn, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &Journal{Path: path, conn: conn}
}

// Recorder is a recorder of component at Version that writes to j.
func (j *Journal) Recorder(t testing.TB, component problems.Component) *problems.Recorder {
	t.Helper()
	r := problems.New(AppID, Version, component, problems.WithSocket(j.Path))
	if err := r.Err(); err != nil {
		t.Fatal(err)
	}
	return r
}

// Record is 1 received record that passed problems.Validate.
type Record struct {
	Fields map[string]string
	problems.Detail
}

// Message is the record's MESSAGE.
func (r Record) Message() string { return r.Fields["MESSAGE"] }

// Next reads the next record, decodes it and validates it, or fails the test.
func (j *Journal) Next(t testing.TB) Record {
	t.Helper()
	buf := make([]byte, 1<<16)
	if err := j.conn.SetReadDeadline(time.Now().Add(wait)); err != nil {
		t.Fatal(err)
	}
	n, err := j.conn.Read(buf)
	if err != nil {
		t.Fatalf("no problem record: %v", err)
	}
	fields, err := problems.DecodeEntry(buf[:n])
	if err != nil {
		t.Fatalf("decode the record: %v", err)
	}
	d, err := problems.Validate(fields)
	if err != nil {
		t.Fatalf("the record is not valid: %v: %q", err, fields)
	}
	return Record{Fields: fields, Detail: d}
}

// NextError reads the next record and checks that it is the error code of
// component, from a recorder made by j.Recorder.
func (j *Journal) NextError(t testing.TB, component problems.Component, code string) Record {
	t.Helper()
	r := j.Next(t)
	r.check(t, component, "error")
	if r.Code != code {
		t.Fatalf("the record's code = %q, want %q", r.Code, code)
	}
	return r
}

// NextPanic reads the next record and checks that it is a panic of
// component, from a recorder made by j.Recorder or with the same identity.
func (j *Journal) NextPanic(t testing.TB, component problems.Component) Record {
	t.Helper()
	r := j.Next(t)
	r.check(t, component, "panic")
	return r
}

func (r Record) check(t testing.TB, component problems.Component, class string) {
	t.Helper()
	if r.AppID != AppID || r.AppVersion != Version || r.Runtime != "go" ||
		r.Component != string(component) || r.Class != class {
		t.Fatalf("the record = %+v, want app %s %s, runtime go, component %s, class %s",
			r.Detail, AppID, Version, component, class)
	}
}

// None fails the test when a record is waiting in the socket. Records are
// sent before the recording call returns, so a check right after the code
// under test sees every record it wrote.
func (j *Journal) None(t testing.TB) {
	t.Helper()
	b, err := j.pending()
	if err != nil {
		t.Fatal(err)
		return
	}
	if b != nil {
		fields, _ := problems.DecodeEntry(b)
		t.Fatalf("an unexpected problem record: %q", fields)
	}
}

// pending receives the datagram that waits in the socket, without blocking,
// or returns nil when none waits. A read deadline cannot do this: with a
// deadline in the past, Go returns a timeout before it tries the read.
func (j *Journal) pending() ([]byte, error) {
	raw, err := j.conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 1<<16)
	var n int
	var rerr error
	if err := raw.Read(func(fd uintptr) bool {
		n, _, rerr = syscall.Recvfrom(int(fd), buf, syscall.MSG_DONTWAIT)
		return true // never wait for the socket to become readable
	}); err != nil {
		return nil, err
	}
	if errors.Is(rerr, syscall.EAGAIN) || errors.Is(rerr, syscall.EWOULDBLOCK) {
		return nil, nil
	}
	if rerr != nil {
		return nil, rerr
	}
	return buf[:n], nil
}
