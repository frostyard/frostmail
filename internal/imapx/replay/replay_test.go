package replay_test

import (
	"bufio"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/imapx/replay"
)

type buffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (w *buffer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *buffer) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

// session opens a session, selects a mailbox, lists and fetches its
// messages, and closes; it returns the subjects it fetched.
func session(t *testing.T, opts imapx.DialOptions, mailbox string) []string {
	t.Helper()
	s, err := imapx.Open(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Select(t.Context(), mailbox); err != nil {
		return nil
	}
	uids, err := s.UIDs(t.Context())
	if err != nil || len(uids) == 0 {
		return nil
	}
	hs, err := s.FetchHeaders(t.Context(), uids)
	if err != nil {
		return nil
	}
	var out []string
	for _, h := range hs {
		out = append(out, h.Subject)
	}
	return out
}

// record runs session against a memory server holding two messages and
// returns its trace.
func record(t *testing.T) ([]replay.Line, []string) {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	for _, subject := range []string{"First", "Second"} {
		raw := "From: Ann <ann@x.test>\r\nSubject: " + subject + "\r\nMessage-ID: <" + subject + "@x.test>\r\n\r\nHello.\r\n"
		if _, err := mem.User.Append("INBOX", strings.NewReader(raw), &imap.AppendOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	opts := mem.DialOptions()
	var tr buffer
	opts.Trace = &tr
	subjects := session(t, opts, "INBOX")
	script, err := replay.Parse(strings.NewReader(tr.String()))
	if err != nil {
		t.Fatal(err)
	}
	return script, subjects
}

func replayOptions(s *replay.Server) imapx.DialOptions {
	host, port, _ := strings.Cut(s.Addr(), ":")
	var p int
	for _, r := range port {
		p = p*10 + int(r-'0')
	}
	return imapx.DialOptions{Host: host, Port: p, TLS: api.TLSModeInsecure, Username: "anyone", Password: "anything"}
}

func TestReplayPlaysARecordedSession(t *testing.T) {
	script, want := record(t)
	if len(want) != 2 {
		t.Fatalf("recorded subjects = %v", want)
	}
	s := replay.Start(t, script)
	got := session(t, replayOptions(s), "INBOX")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("replayed subjects = %v, want %v", got, want)
	}
}

func TestReplayFailsOnAnUnexpectedCommand(t *testing.T) {
	script, _ := record(t)
	s := replay.Start(t, script)
	session(t, replayOptions(s), "Archive")
	err := s.Close()
	if err == nil || !strings.Contains(err.Error(), "Archive") {
		t.Errorf("replay of a different session = %v, want a mismatch naming the SELECT of Archive", err)
	}
}

func TestParseRejectsUnmarkedLines(t *testing.T) {
	if _, err := replay.Parse(strings.NewReader("C: a LOGIN [redacted]\nhello\n")); err == nil {
		t.Error("a line without C: or S: parsed")
	}
	lines, err := replay.Parse(strings.NewReader("# a comment\n\nS: * OK ready\nC: a NOOP\n"))
	if err != nil || len(lines) != 2 || lines[0].Sent || !lines[1].Sent {
		t.Errorf("parse = %+v, %v", lines, err)
	}
}

// A client may send pipelined commands in another order than recorded;
// each still gets its own recorded answer, under the client's tags.
func TestReplayAnswersCommandsInAnyOrder(t *testing.T) {
	script, err := replay.Parse(strings.NewReader(strings.Join([]string{
		"S: * OK ready",
		"C: a1 NOOP",
		"C: a2 CAPABILITY",
		"S: * CAPABILITY IMAP4rev1 IDLE",
		"S: a2 OK capability done",
		"S: * 3 EXISTS",
		"S: a1 OK noop done",
		"C: a3 IDLE",
		"S: + idling",
		"S: * 4 EXISTS",
		"C: DONE",
		"S: a3 OK idle done",
	}, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	s := replay.Start(t, script)
	conn, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(conn)
	readUntil := func(prefix string) []string {
		t.Helper()
		var lines []string
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				t.Fatalf("read: %v (so far %q)", err, lines)
			}
			line = strings.TrimRight(line, "\r\n")
			lines = append(lines, line)
			if strings.HasPrefix(line, prefix) {
				return lines
			}
		}
	}
	readUntil("* OK")
	send := func(line string) {
		t.Helper()
		if _, err := conn.Write([]byte(line + "\r\n")); err != nil {
			t.Fatal(err)
		}
	}
	send("x1 CAPABILITY")
	if got := strings.Join(readUntil("x1 "), "|"); got != "* CAPABILITY IMAP4rev1 IDLE|x1 OK capability done" {
		t.Errorf("CAPABILITY answer = %q", got)
	}
	send("x2 NOOP")
	if got := strings.Join(readUntil("x2 "), "|"); got != "* 3 EXISTS|x2 OK noop done" {
		t.Errorf("NOOP answer = %q", got)
	}
	send("x3 IDLE")
	if got := strings.Join(readUntil("* 4"), "|"); got != "+ idling|* 4 EXISTS" {
		t.Errorf("IDLE answer = %q", got)
	}
	send("DONE")
	readUntil("x3 OK")
	_ = conn.Close()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReplayReportsCommandsNeverSent(t *testing.T) {
	script, _ := replay.Parse(strings.NewReader("S: * OK ready\nC: a1 NOOP\nS: a1 OK\nC: a2 CAPABILITY\nS: a2 OK\n"))
	s := replay.Start(t, script)
	conn, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(conn)
	_, _ = r.ReadString('\n')
	_, _ = conn.Write([]byte("b1 NOOP\r\n"))
	_, _ = r.ReadString('\n')
	_ = conn.Close()
	if err := s.Close(); err == nil || !strings.Contains(err.Error(), "CAPABILITY") {
		t.Errorf("close = %v, want the unsent CAPABILITY named", err)
	}
}
