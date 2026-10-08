package imapx

import (
	"bytes"
	"io"
	"net"
	"strings"
	"sync"
)

// Session traces (MAILD_IMAP_TRACE, docs/design/testing.md): every line
// the client sends is written as "C: <line>" and every line it receives as
// "S: <line>", after TLS, so recordings can become replay tests. LOGIN and
// AUTHENTICATE arguments, and any SASL lines up to the authentication's
// tagged reply, are replaced by [redacted]. Message contents are kept: a
// trace holds the account's mail and stays private until tools/imaprec
// rewrites it.

// traceConn copies a connection's traffic to a trace.
type traceConn struct {
	net.Conn
	t *trace
}

func (c *traceConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.t.write(false, p[:n])
	return n, err
}

func (c *traceConn) Write(p []byte) (int, error) {
	c.t.write(true, p)
	return c.Conn.Write(p)
}

func (c *traceConn) Close() error {
	err := c.Conn.Close()
	c.t.close()
	return err
}

// trace splits both directions into lines and writes them, redacted, to w.
type trace struct {
	mu       sync.Mutex
	w        io.Writer
	sent     []byte // partial lines
	received []byte
	authTag  string // the AUTHENTICATE command awaiting its tagged reply
	closed   bool
}

func newTrace(w io.Writer) *trace { return &trace{w: w} }

func (t *trace) write(sent bool, p []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	buf := &t.received
	if sent {
		buf = &t.sent
	}
	*buf = append(*buf, p...)
	for {
		i := bytes.IndexByte(*buf, '\n')
		if i < 0 {
			return
		}
		line := strings.TrimRight(string((*buf)[:i]), "\r")
		*buf = (*buf)[i+1:]
		if sent {
			t.emit("C: " + t.clientLine(line))
		} else {
			t.serverLine(line)
			t.emit("S: " + line)
		}
	}
}

// clientLine redacts credentials from a line the client sent.
func (t *trace) clientLine(line string) string {
	if t.authTag != "" {
		return "[redacted]"
	}
	tag, rest, _ := strings.Cut(line, " ")
	cmd, args, _ := strings.Cut(rest, " ")
	switch strings.ToUpper(cmd) {
	case "LOGIN":
		return tag + " LOGIN [redacted]"
	case "AUTHENTICATE":
		t.authTag = tag
		mech, _, _ := strings.Cut(args, " ")
		return tag + " AUTHENTICATE " + mech + " [redacted]"
	}
	return line
}

// serverLine ends the redaction of SASL lines at the tagged reply.
func (t *trace) serverLine(line string) {
	if t.authTag != "" && strings.HasPrefix(line, t.authTag+" ") {
		t.authTag = ""
	}
}

func (t *trace) emit(line string) {
	_, _ = io.WriteString(t.w, line+"\n")
}

// close writes what is left of partial lines and closes the trace's
// writer when it can be closed.
func (t *trace) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return
	}
	t.closed = true
	if len(t.sent) > 0 {
		t.emit("C: " + t.clientLine(string(t.sent)))
	}
	if len(t.received) > 0 {
		t.emit("S: " + string(t.received))
	}
	if c, ok := t.w.(io.Closer); ok {
		_ = c.Close()
	}
}
