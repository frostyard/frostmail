package main

// CONTRACT TEST for task card T-0021 (docs/tasks). Do not edit.

import (
	"bytes"
	"crypto/sha256"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/emersion/go-message/mail"
)

func generate(t *testing.T, n int, seed uint64) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Maildir")
	if err := Generate(Options{N: n, Seed: seed, Out: dir}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func files(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, "cur"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	slices.Sort(out)
	return out
}

func digest(t *testing.T, dir string) [32]byte {
	t.Helper()
	h := sha256.New()
	for _, name := range files(t, dir) {
		data, err := os.ReadFile(filepath.Join(dir, "cur", name))
		if err != nil {
			t.Fatal(err)
		}
		h.Write([]byte(name))
		h.Write(data)
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

var nameRE = regexp.MustCompile(`^(\d{7})\.mailgen:2,([DFRS]*)$`)

func TestMaildirLayout(t *testing.T) {
	dir := generate(t, 300, 1)
	for _, sub := range []string{"cur", "new", "tmp"} {
		if fi, err := os.Stat(filepath.Join(dir, sub)); err != nil || !fi.IsDir() {
			t.Fatalf("%s: %v", sub, err)
		}
	}
	names := files(t, dir)
	if len(names) != 300 {
		t.Fatalf("%d files, want 300", len(names))
	}
	seen, flagged := 0, 0
	for i, name := range names {
		m := nameRE.FindStringSubmatch(name)
		if m == nil {
			t.Fatalf("file name %q does not match %s", name, nameRE)
		}
		if m[1] != strings.Repeat("0", 7-len(itoa(i+1)))+itoa(i+1) {
			t.Fatalf("file %d is named %q; want sequence numbers from 0000001", i, name)
		}
		if strings.Contains(m[2], "S") {
			seen++
		}
		if strings.Contains(m[2], "F") {
			flagged++
		}
	}
	if seen < 120 || seen > 240 || flagged < 3 || flagged > 40 {
		t.Fatalf("seen %d flagged %d of 300; want about 60%% seen and 5%% flagged", seen, flagged)
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

func TestDeterministic(t *testing.T) {
	a, b, c := generate(t, 200, 7), generate(t, 200, 7), generate(t, 200, 8)
	if digest(t, a) != digest(t, b) {
		t.Fatal("the same seed produced different Maildirs")
	}
	if digest(t, a) == digest(t, c) {
		t.Fatal("different seeds produced the same Maildir")
	}
}

func TestMessagesAreValidAndVaried(t *testing.T) {
	dir := generate(t, 400, 3)
	ids := map[string]bool{}
	senders := map[string]bool{}
	var replies, multipart, attachments int
	for _, name := range files(t, dir) {
		raw, err := os.ReadFile(filepath.Join(dir, "cur", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(raw, []byte("\r\n")) || bytes.Contains(bytes.ReplaceAll(raw, []byte("\r\n"), nil), []byte("\n")) {
			t.Fatalf("%s: lines must end in CRLF", name)
		}
		r, err := mail.CreateReader(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		h := r.Header
		subject, _ := h.Subject()
		from, _ := h.AddressList("From")
		to, _ := h.AddressList("To")
		date, _ := h.Date()
		id, _ := h.MessageID()
		if subject == "" || len(from) != 1 || len(to) == 0 || date.IsZero() || id == "" {
			t.Fatalf("%s: subject %q from %v to %v date %v id %q", name, subject, from, to, date, id)
		}
		if ids[id] {
			t.Fatalf("%s: duplicate Message-ID %s", name, id)
		}
		ids[id] = true
		senders[from[0].Address] = true
		if refs, _ := h.MsgIDList("References"); len(refs) > 0 {
			replies++
			for _, ref := range refs {
				if !ids[ref] {
					t.Fatalf("%s references %s, which no earlier message has", name, ref)
				}
			}
			if !strings.HasPrefix(subject, "Re: ") {
				t.Fatalf("%s: reply subject %q lacks Re:", name, subject)
			}
		}
		mt, _, _ := h.ContentType()
		if strings.HasPrefix(mt, "multipart/") {
			multipart++
		}
		for {
			p, err := r.NextPart()
			if err != nil {
				break
			}
			if _, ok := p.Header.(*mail.AttachmentHeader); ok {
				attachments++
			}
		}
		_ = r.Close()
	}
	if replies < 40 || multipart < 60 || attachments < 15 || len(senders) < 30 {
		t.Fatalf("replies %d multipart %d attachments %d senders %d in 400; want variety", replies, multipart, attachments, len(senders))
	}
}

func TestRunFlags(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Maildir")
	if err := run([]string{"-n", "5", "-seed", "2", "-out", dir}); err != nil {
		t.Fatal(err)
	}
	if got := len(files(t, dir)); got != 5 {
		t.Fatalf("%d files", got)
	}
	if err := run([]string{"-n", "0", "-out", dir}); err == nil {
		t.Fatal("-n 0 succeeded")
	}
	if err := run([]string{"-n", "5"}); err == nil {
		t.Fatal("missing -out succeeded")
	}
}
