package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/internal/imapx/replay"
)

// trace parses a trace's lines.
func trace(t *testing.T, lines ...string) []replay.Line {
	t.Helper()
	text := strings.Join(lines, "\n")
	parsed, err := replay.Parse(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

// literalLines returns the S: lines of a literal's data, and its size:
// lines joined with CRLF; the last line carries what follows the literal.
func literalLines(data []string, after string) (lines []string, size string) {
	for i, l := range data {
		if i == len(data)-1 {
			l += after
		}
		lines = append(lines, "S: "+l)
	}
	return lines, strconv.Itoa(len(strings.Join(data, "\r\n")))
}

var key = []byte("a fixed key for tests")

func run1(t *testing.T, tr []replay.Line, opts options) string {
	t.Helper()
	opts.key = key
	out, err := convert(tr, opts)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// gmailTrace is a small Gmail first sync about made-up people.
func gmailTrace(t *testing.T) []replay.Line {
	headers, hsize := literalLines([]string{
		"References: <ledger-41@whitcombe.example>",
		"Authentication-Results: mx.google.com; dkim=pass header.i=@whitcombe.example",
		"", "",
	}, ")")
	preview, psize := literalLines([]string{
		"Caf=C3=A9 r=C3=A9sum=C3=A9 for Annabelle at Whitcombe Manor=",
		"+1 555 0100 lighthouse",
	}, ")")
	lines := []string{
		"S: * OK Gimap ready for requests from 203.0.113.77 zq81lighthousesession",
		"C: T1 LOGIN [redacted]",
		"S: T1 OK annabelle.whitcombe@gmail.com authenticated (Success)",
		`C: T2 LIST "" "*" RETURN (SUBSCRIBED SPECIAL-USE)`,
		`S: * LIST (\HasNoChildren) "/" "Lighthouse Keepers"`,
		`S: * LIST (\All \HasNoChildren) "/" "[Gmail]/All Mail"`,
		"S: T2 OK Success",
		`C: T3 SELECT "[Gmail]/All Mail" (CONDSTORE)`,
		`S: * FLAGS (\Seen $Whitcombefamily)`,
		"S: * 2 EXISTS",
		"S: T3 OK [READ-WRITE] [Gmail]/All Mail selected. (Success)",
		"C: T4 UID SEARCH RETURN (ALL) ALL",
		`S: * ESEARCH (TAG "T4") UID ALL 3:4`,
		"S: T4 OK SEARCH completed (Success)",
		`C: T5 UID FETCH 3:4 (UID ENVELOPE BODYSTRUCTURE X-GM-LABELS FLAGS INTERNALDATE BODY.PEEK[HEADER.FIELDS ("References")])`,
		`S: * 1 FETCH (X-GM-MSGID 1878505316514561155 X-GM-LABELS ("\\Inbox" "Lighthouse Keepers") UID 3 FLAGS (\Seen $Whitcombefamily) ` +
			`INTERNALDATE "08-Oct-2026 17:57:53 +0000" ENVELOPE ("Thu, 08 Oct 2026 10:57:52 -0700" "Re: Quarterly ledger" ` +
			`(("Annabelle Whitcombe" NIL "annabelle" "whitcombe.example")) NIL NIL ((NIL NIL "ledgers" "gmail.com")) NIL NIL ` +
			`"<ledger-41@whitcombe.example>" "<ledger-42@whitcombe.example>") BODYSTRUCTURE (("TEXT" "PLAIN" ("CHARSET" "UTF-8") NIL NIL ` +
			`"QUOTED-PRINTABLE" 120 3 NIL NIL NIL)("APPLICATION" "PDF" ("NAME" "Quarterly-Ledger.pdf") NIL NIL "BASE64" 900 NIL ` +
			`("ATTACHMENT" ("FILENAME" "Quarterly-Ledger.pdf")) NIL) "MIXED" ("BOUNDARY" "whitcombeboundary77") NIL NIL) ` +
			`BODY[HEADER.FIELDS (References)] {` + hsize + `}`,
	}
	lines = append(lines, headers...)
	lines = append(lines,
		`S: * 2 FETCH (X-GM-MSGID 1878505316514561156 X-GM-LABELS () UID 4 FLAGS () INTERNALDATE "08-Oct-2026 18:00:00 +0000" `+
			`ENVELOPE ("Thu, 08 Oct 2026 11:00:00 -0700" "Quarterly ledger" NIL NIL NIL NIL NIL NIL NIL "<ledger-41@whitcombe.example>") `+
			`BODYSTRUCTURE ("TEXT" "PLAIN" ("CHARSET" "ISO-8859-1") NIL NIL "7BIT" 10 1 NIL NIL NIL) BODY[HEADER.FIELDS (References)] {0}`,
		"S: )",
		"S: T5 OK Success",
		"C: T6 UID FETCH 3 (UID BODY.PEEK[1]<0.2048>)",
		"S: * 1 FETCH (UID 3 BODY[1]<0> {"+psize+"}",
	)
	lines = append(lines, preview...)
	lines = append(lines,
		"S: T6 OK Success",
		`C: T7 UID STORE 3 +X-GM-LABELS ("Lighthouse Keepers")`,
		`S: * 1 FETCH (X-GM-LABELS ("\\Inbox" "Lighthouse Keepers") UID 3)`,
		"S: T7 OK Success",
		"C: T8 IDLE",
		"S: + idling",
	)
	return trace(t, lines...)
}

func TestScrubReplacesTheMailAndKeepsTheProtocol(t *testing.T) {
	out := run1(t, gmailTrace(t), options{note: "a test"})
	for _, secret := range []string{"Whitcombe", "whitcombe", "Annabelle", "annabelle", "Lighthouse", "lighthouse",
		"Keepers", "Quarterly", "ledger", "Ledger", "Manor", "203", "113", "0100", "=C3", "=A9", "zq81"} {
		if strings.Contains(out, secret) {
			t.Errorf("the script still says %q", secret)
		}
	}
	for _, kept := range []string{
		"# a test\n", `"08-Oct-2026 17:57:53 +0000"`, `"Thu, 08 Oct 2026 10:57:52 -0700"`, `("CHARSET" "UTF-8")`,
		`("CHARSET" "ISO-8859-1")`, `"QUOTED-PRINTABLE" 120 3`, `"APPLICATION" "PDF"`, `"ATTACHMENT" ("FILENAME" "`,
		"X-GM-MSGID 1878505316514561155", `"\\Inbox"`, `"[Gmail]/All Mail"`, "FLAGS (\\Seen $", "Re: ",
		"@gmail.com authenticated (Success)", `* ESEARCH (TAG "T4") UID ALL 3:4`, "C: T1 LOGIN [redacted]",
	} {
		if !strings.Contains(out, kept) {
			t.Errorf("the script lost %q", kept)
		}
	}
	if strings.Contains(out, "T8") {
		t.Error("the IDLE the trace ends in, without its completion, was kept")
	}
	// The same words are the same fakes everywhere.
	label := regexp.MustCompile(`"/" "([^"]+)"`).FindStringSubmatch(out)
	if label == nil || strings.Count(out, `"`+label[1]+`"`) != 4 {
		t.Errorf("the label's fake %v is not in LIST, both FETCHes and the STORE", label)
	}
	cited := regexp.MustCompile(`References: <([^>]+)>`).FindStringSubmatch(out)
	if cited == nil || !strings.Contains(out, `"<`+cited[1]+`>")`) {
		t.Errorf("the References header %v does not cite the fake Message-ID of the ENVELOPE", cited)
	}
	if !strings.Contains(out, "=3D=3D") {
		t.Error("quoted-printable escapes were not replaced by =3D")
	}
	checkLiterals(t, out)
}

// checkLiterals parses a script's responses as imaprec does and fails
// when a literal's size does not match its data.
func checkLiterals(t *testing.T, script string) {
	t.Helper()
	lines, err := replay.Parse(strings.NewReader(script))
	if err != nil {
		t.Fatal(err)
	}
	_, xs := replay.Split(lines)
	for _, x := range xs {
		for i := range x {
			if x[i].Sent {
				continue
			}
			if _, want, ok := literalAt(x[i].Text); ok {
				text, _, err := joinLiterals(x[i:], 2)
				if err != nil || !strings.Contains(text, "{"+strconv.Itoa(want)+"}\r\n") {
					t.Errorf("literal of %d bytes at %q: %v", want, x[i].Text, err)
				}
			}
		}
	}
}

func TestScrubIsStableForOneKey(t *testing.T) {
	a := run1(t, gmailTrace(t), options{})
	b := run1(t, gmailTrace(t), options{})
	other, err := convert(gmailTrace(t), options{key: []byte("another key")})
	if err != nil {
		t.Fatal(err)
	}
	if a != b || a == string(other) {
		t.Error("fakes must depend on the key and only on it")
	}
}

func TestThroughCutsAfterTheCommand(t *testing.T) {
	out := run1(t, gmailTrace(t), options{through: "T4"})
	if !strings.Contains(out, "C: T4 ") || strings.Contains(out, "C: T5 ") {
		t.Errorf("-through T4 kept:\n%s", out)
	}
	if _, err := convert(gmailTrace(t), options{through: "T9", key: key}); err == nil {
		t.Error("an unknown -through tag was accepted")
	}
	if _, err := convert(gmailTrace(t), options{through: "T8", key: key}); err == nil {
		t.Error("-through an incomplete exchange was accepted")
	}
}

func TestLeakCheckRefusesUnscrubbedWords(t *testing.T) {
	// CAPABILITY is kept as the server sent it.
	tr := gmailTrace(t)
	tr = slices.Insert(tr, 1, replay.Line{Text: "* CAPABILITY IMAP4rev1 Whitcombe"})
	_, err := convert(tr, options{key: key})
	if err == nil || !strings.Contains(err.Error(), "Whitcombe") {
		t.Errorf("convert = %v, want the leak of Whitcombe reported", err)
	}
}

func TestClientLiteralsAreRefused(t *testing.T) {
	tr := trace(t, "S: * OK ready", `C: T1 APPEND "Drafts" {5+}`, "C: hello", "C: ", "S: T1 OK done")
	if _, err := convert(tr, options{key: key}); err == nil || !strings.Contains(err.Error(), "cut before it") {
		t.Errorf("convert = %v, want APPEND refused", err)
	}
}

// A trace drops each line's CR, so a literal whose lines ended in a bare
// LF is counted again that way when CRLF does not parse.
func TestLiteralWithBareLineFeeds(t *testing.T) {
	data := []string{"one", "two", "three"}
	size := strconv.Itoa(len(strings.Join(data, "\n")) + 1) // each line ends in LF
	tr := trace(t,
		"S: * OK ready",
		"C: T1 UID FETCH 1 (UID BODY.PEEK[1])",
		"S: * 1 FETCH (UID 1 BODY[1] {"+size+"}",
		"S: one", "S: two", "S: three",
		"S: )",
		"S: T1 OK done",
	)
	out := run1(t, tr, options{})
	checkLiterals(t, out)
	if !strings.Contains(out, "BODY[1] {17}\nS: ") { // three lines, now ending in CRLF
		t.Errorf("script:\n%s", out)
	}
}

// A partial fetch that ends between CR and LF leaves a bare CR in the
// trace's line; the script gets a space of the same size.
func TestLiteralEndingInCR(t *testing.T) {
	tr := trace(t,
		"S: * OK ready",
		"C: T1 UID FETCH 1 (UID BODY.PEEK[1]<0.4>)",
		"S: * 1 FETCH (UID 1 BODY[1]<0> {4}",
		"S: abc\r)",
		"S: T1 OK done",
	)
	out := run1(t, tr, options{})
	checkLiterals(t, out)
	if strings.Contains(out, "\r") || !regexp.MustCompile(`\nS: [a-z]{3} \)\n`).MatchString(out) {
		t.Errorf("script:\n%q", out)
	}
}

// A first sync of five messages trimmed to the two newest, with a preview
// fetch for an old message only: the mailbox shrinks, the header fetch
// asks for the two, the preview fetch is dropped, and responses are
// numbered as in a mailbox of two.
func TestTrimKeepsTheNewest(t *testing.T) {
	var lines []string
	add := func(l ...string) { lines = append(lines, l...) }
	add("S: * OK ready",
		`C: T1 EXAMINE "Archive" (CONDSTORE)`, "S: * 5 EXISTS", "S: * OK [UIDNEXT 60]", "S: T1 OK done",
		"C: T2 UID SEARCH RETURN (ALL) ALL", `S: * ESEARCH (TAG "T2") UID ALL 10:12,50:51`, "S: T2 OK done",
		"C: T3 UID FETCH 10:12,50:51 (UID ENVELOPE FLAGS)")
	for i, uid := range []int{10, 11, 12, 50, 51} {
		add("S: * " + strconv.Itoa(i+1) + " FETCH (UID " + strconv.Itoa(uid) + " FLAGS () ENVELOPE (NIL \"Hello\" NIL NIL NIL NIL NIL NIL NIL NIL))")
	}
	add("S: T3 OK done",
		"C: T4 UID FETCH 11 (UID BODY.PEEK[1]<0.2048>)", "S: * 2 FETCH (UID 11 BODY[1]<0> {2}", "S: hi)", "S: T4 OK done",
		"C: T5 UID FETCH 51 (UID BODY.PEEK[1.1]<0.2048>)", "S: * 5 FETCH (UID 51 BODY[1.1]<0> {2}", "S: yo)", "S: T5 OK done",
		`C: T6 EXAMINE "Junk" (CONDSTORE)`, "S: * 0 EXISTS", "S: T6 OK done",
		"C: T7 UID SEARCH RETURN (ALL) ALL", `S: * ESEARCH (TAG "T7") UID`, "S: T7 OK done")
	out := run1(t, trace(t, lines...), options{keep: 2})
	for _, want := range []string{
		"S: * 2 EXISTS", `S: * ESEARCH (TAG "T2") UID ALL 50:51`, "C: T3 UID FETCH 50:51 (UID ENVELOPE FLAGS)",
		"S: * 1 FETCH (UID 50 ", "S: * 2 FETCH (UID 51 ", "C: T5 UID FETCH 51 ", "S: * 2 FETCH (UID 51 BODY[1.1]<0> {2}",
		"S: * 0 EXISTS", `S: * ESEARCH (TAG "T7") UID`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the trimmed script lacks %q:\n%s", want, out)
		}
	}
	for _, gone := range []string{"UID 10 ", "UID 11 ", "UID 12 ", "C: T4 "} {
		if strings.Contains(out, gone) {
			t.Errorf("the trimmed script still has %q", gone)
		}
	}
	// A command that changes a trimmed mailbox ends the trim.
	lines = append(lines[:len(lines):len(lines)], `C: T8 UID STORE 51 +FLAGS (\Seen)`, "S: T8 OK done")
	if _, err := convert(trace(t, lines...), options{keep: 2, key: key}); err == nil {
		t.Error("trimming a session that stores flags was accepted")
	}
}

// Scripts scrubbed with one key file share their fakes; a key file too
// short to be secret is refused.
func TestKeyFileSharesFakes(t *testing.T) {
	dir := t.TempDir()
	tracePath, keyPath := filepath.Join(dir, "trace"), filepath.Join(dir, "key")
	var b strings.Builder
	for _, l := range gmailTrace(t) {
		prefix := "S: "
		if l.Sent {
			prefix = "C: "
		}
		b.WriteString(prefix + l.Text + "\n")
	}
	if err := os.WriteFile(tracePath, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("thirty-two bytes of private key!"), 0o600); err != nil {
		t.Fatal(err)
	}
	scrub := func() (string, error) {
		var out strings.Builder
		err := run([]string{"-keyfile", keyPath, tracePath}, &out)
		return out.String(), err
	}
	a, err := scrub()
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := scrub(); a != b {
		t.Error("two runs with one key file made different fakes")
	}
	if err := os.WriteFile(keyPath, []byte("short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := scrub(); err == nil {
		t.Error("a five-byte key file was accepted")
	}
}
