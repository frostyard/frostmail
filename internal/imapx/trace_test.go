package imapx_test

import (
	"encoding/base64"
	"net"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
)

// closingTrace records whether the session closed its trace.
type closingTrace struct {
	lockedTrace
	closed bool
}

func (c *closingTrace) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func TestTraceMarksDirectionsAndRedactsLogin(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	seedMem(t, mem)
	opts := mem.DialOptions()
	tr := &closingTrace{}
	opts.Trace = tr
	s, err := imapx.Open(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Select(t.Context(), "INBOX"); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	got := tr.String()
	var login, selected, greeting bool
	for line := range strings.SplitSeq(strings.TrimSpace(got), "\n") {
		if !strings.HasPrefix(line, "C: ") && !strings.HasPrefix(line, "S: ") {
			t.Errorf("line without a direction: %q", line)
		}
		if strings.Contains(line, "LOGIN") && !strings.HasSuffix(line, " LOGIN [redacted]") {
			t.Errorf("an unredacted LOGIN line: %q", line)
		}
		login = login || strings.HasPrefix(line, "C: ") && strings.HasSuffix(line, " LOGIN [redacted]")
		selected = selected || strings.HasPrefix(line, "C: ") && strings.HasSuffix(line, " SELECT INBOX")
		greeting = greeting || strings.HasPrefix(line, "S: * OK")
	}
	if !login || !selected || !greeting {
		t.Errorf("trace lacks the redacted LOGIN, the SELECT or the greeting:\n%s", got)
	}
	tr.mu.Lock()
	closed := tr.closed
	tr.mu.Unlock()
	if !closed {
		t.Error("closing the session left its trace open")
	}
}

func TestTraceRedactsXOAuth2(t *testing.T) {
	const token = "ya29.token-should-not-appear"
	mem := imapxtest.StartMemFullOAuth(t, token)
	opts := mem.DialOptions()
	opts.Password, opts.OAuth = token, true
	tr := &lockedTrace{}
	opts.Trace = tr
	s, err := imapx.Open(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	got := tr.String()
	encoded := base64.StdEncoding.EncodeToString([]byte("user=" + opts.Username + "\x01auth=Bearer " + token + "\x01\x01"))
	if strings.Contains(got, token) || strings.Contains(got, encoded[:24]) {
		t.Fatalf("the token is in the trace:\n%s", got)
	}
	if !strings.Contains(got, " AUTHENTICATE XOAUTH2 [redacted]") {
		t.Errorf("no redacted AUTHENTICATE in the trace:\n%s", got)
	}
}

func TestFailedDialClosesItsTrace(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().(*net.TCPAddr)
	_ = ln.Close() // nothing listens there now
	tr := &closingTrace{}
	_, err = imapx.Open(t.Context(), imapx.DialOptions{Host: "127.0.0.1", Port: addr.Port, TLS: api.TLSModeInsecure, Trace: tr})
	if err == nil {
		t.Fatal("dialing a closed port succeeded")
	}
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if !tr.closed {
		t.Error("a failed dial left its trace open")
	}
}
