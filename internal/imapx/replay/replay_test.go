package replay_test

import (
	"bufio"
	"net"
	"strconv"
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
	lines, err := replay.Parse(strings.NewReader("# a comment\n\nS: * OK ready\nC: a NOOP\nS:\n"))
	if err != nil || len(lines) != 3 || lines[0].Sent || !lines[1].Sent || lines[2] != (replay.Line{}) {
		t.Errorf("parse = %+v, %v", lines, err)
	}
}

// A literal's lines stay with the response or command that carries them,
// whatever they look like: a body line starting with "+" is no
// continuation request, and a draft line that reads like a command is no
// new command.
func TestSplitKeepsLiteralsWhole(t *testing.T) {
	body := []string{"Subject: x", "", "+1 555 0100", "* 2 EXISTS", ""}
	draft := []string{"Subject: y", "", "Hi THERE", ""}
	size := func(lines []string) string { return strconv.Itoa(len(strings.Join(lines, "\r\n"))) }
	var script []string
	add := func(prefix string, lines ...string) {
		for _, l := range lines {
			script = append(script, prefix+l)
		}
	}
	add("S: ", "* OK ready")
	add("C: ", "a1 UID FETCH 1 (BODY.PEEK[])")
	add("S: ", "* 1 FETCH (UID 1 BODY[] {"+size(body)+"}")
	add("S: ", body[:len(body)-1]...)
	add("S: ", ")", "a1 OK done")
	add("C: ", `a2 APPEND "Drafts" {`+size(draft)+"+}")
	add("C: ", draft...)
	add("S: ", "a2 OK [APPENDUID 1 2] done")
	lines, err := replay.Parse(strings.NewReader(strings.Join(script, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	_, xs := replay.Split(lines)
	var got []string
	for _, x := range xs {
		var texts []string
		for _, l := range x {
			texts = append(texts, l.Text)
		}
		got = append(got, strings.Join(texts, "|"))
	}
	want := []string{
		"a1 UID FETCH 1 (BODY.PEEK[])|* 1 FETCH (UID 1 BODY[] {39}|Subject: x||+1 555 0100|* 2 EXISTS|)|a1 OK done",
		`a2 APPEND "Drafts" {24+}|Subject: y||Hi THERE||a2 OK [APPENDUID 1 2] done`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("exchanges:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A command that acts on the selected mailbox gets the answer recorded
// with that mailbox selected, though the same command was recorded in
// another: the client here visits the mailboxes in the other order.
func TestReplayAnswersFromTheSelectedMailbox(t *testing.T) {
	script, err := replay.Parse(strings.NewReader(strings.Join([]string{
		"S: * OK ready",
		`C: a1 EXAMINE "Drafts"`,
		"S: a1 OK examined",
		"C: a2 UID SEARCH ALL",
		"S: * SEARCH 7",
		"S: a2 OK searched",
		"C: a3 EXAMINE INBOX",
		"S: a3 OK examined",
		"C: a4 UID SEARCH ALL",
		"S: * SEARCH 1 2",
		"S: a4 OK searched",
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
	_, _ = r.ReadString('\n')
	var got []string
	for _, cmd := range []string{"b1 EXAMINE INBOX", "b2 UID SEARCH ALL", `b3 EXAMINE "Drafts"`, "b4 UID SEARCH ALL"} {
		if _, err := conn.Write([]byte(cmd + "\r\n")); err != nil {
			t.Fatal(err)
		}
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				t.Fatalf("answer to %q: %v", cmd, err)
			}
			line = strings.TrimRight(line, "\r\n")
			if strings.HasPrefix(line, cmd[:3]) {
				break
			}
			got = append(got, line)
		}
	}
	if strings.Join(got, "|") != "* SEARCH 1 2|* SEARCH 7" {
		t.Errorf("answers = %q, want INBOX's then Drafts'", got)
	}
	_ = conn.Close()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// go-imap sends FETCH and STATUS items in an order that varies between
// runs; a FETCH may carry a modifier after its items, and a mailbox name
// may hold parentheses.
func TestReplayMatchesItemsInAnyOrder(t *testing.T) {
	script, err := replay.Parse(strings.NewReader(strings.Join([]string{
		"S: * OK ready",
		"C: a1 UID FETCH 1:2 (UID FLAGS MODSEQ) (CHANGEDSINCE 7)",
		"S: a1 OK fetched",
		`C: a2 STATUS "Old (2019)" (MESSAGES UIDNEXT HIGHESTMODSEQ)`,
		"S: a2 OK status",
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
	_, _ = r.ReadString('\n')
	for _, cmd := range []string{
		"b1 UID FETCH 1:2 (MODSEQ UID FLAGS) (CHANGEDSINCE 7)",
		`b2 STATUS "Old (2019)" (HIGHESTMODSEQ MESSAGES UIDNEXT)`,
	} {
		if _, err := conn.Write([]byte(cmd + "\r\n")); err != nil {
			t.Fatal(err)
		}
		if line, err := r.ReadString('\n'); err != nil || !strings.HasPrefix(line, cmd[:3]+"OK") {
			t.Fatalf("answer to %q = %q, %v", cmd, line, err)
		}
	}
	_ = conn.Close()
	if err := s.Close(); err != nil {
		t.Fatal(err)
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
		"C: a4 UID SEARCH RETURN (ALL) ALL",
		`S: * ESEARCH (TAG "a4") UID ALL 1:3`,
		"S: a4 OK search done",
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
	send("x4 UID SEARCH RETURN (ALL) ALL")
	if got := strings.Join(readUntil("x4 "), "|"); got != `* ESEARCH (TAG "x4") UID ALL 1:3|x4 OK search done` {
		t.Errorf("SEARCH answer = %q", got)
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
