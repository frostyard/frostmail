package imapx_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
	"github.com/frostyard/frostmail/internal/imapx/replay"
)

// gmailScript records a session that selects INBOX and lists its UIDs,
// then makes the server Gmail: X-GM-EXT-1 among its capabilities, and the
// listing replaced by the given exchanges.
func gmailScript(t *testing.T, exchanges string) []replay.Line {
	t.Helper()
	mem := imapxtest.StartMemFull(t)
	opts := mem.DialOptions()
	var tr lockedBuffer
	opts.Trace = &tr
	s, err := imapx.Open(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Select(t.Context(), "INBOX"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UIDs(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	lines, err := replay.Parse(strings.NewReader(tr.String()))
	if err != nil {
		t.Fatal(err)
	}
	gmail := func(l replay.Line) replay.Line {
		if !l.Sent {
			l.Text = strings.Replace(l.Text, "CAPABILITY ", "CAPABILITY X-GM-EXT-1 ", 1)
		}
		return l
	}
	greeting, xs := replay.Split(lines)
	var script []replay.Line
	for _, l := range greeting {
		script = append(script, gmail(l))
	}
	for _, x := range xs {
		if strings.Contains(x[0].Text, "UID SEARCH") {
			continue
		}
		for _, l := range x {
			script = append(script, gmail(l))
		}
	}
	extra, err := replay.Parse(strings.NewReader(exchanges))
	if err != nil {
		t.Fatal(err)
	}
	return append(script, extra...)
}

func replayDial(srv *replay.Server) imapx.DialOptions {
	host, port, _ := strings.Cut(srv.Addr(), ":")
	opts := imapx.DialOptions{Host: host, TLS: api.TLSModeInsecure, Username: "anyone", Password: "anything"}
	for _, r := range port {
		opts.Port = opts.Port*10 + int(r-'0')
	}
	return opts
}

// A Gmail search goes out as X-GM-RAW, with the UID floor and the day it
// starts from when given, and comes back as UIDs.
func TestSearchGmailRaw(t *testing.T) {
	srv := replay.Start(t, gmailScript(t, `
C: G1 UID SEARCH RETURN (ALL) X-GM-RAW "category:promotions"
S: * ESEARCH (TAG "G1") UID ALL 9,3:4
S: G1 OK SEARCH completed
C: G2 UID SEARCH RETURN (ALL) UID 5:* SINCE "1-Oct-2026" X-GM-RAW "category:updates"
S: * ESEARCH (TAG "G2") UID ALL 9
S: G2 OK SEARCH completed
`))
	s, err := imapx.Open(t.Context(), replayDial(srv))
	if err != nil {
		t.Fatal(err)
	}
	if !s.Caps.Gmail {
		t.Fatal("the script's server is not Gmail")
	}
	if _, err := s.Select(t.Context(), "INBOX"); err != nil {
		t.Fatal(err)
	}
	got, err := s.SearchGmailRaw(t.Context(), "category:promotions", time.Time{}, 0)
	if err != nil || !slices.Equal(got, []uint32{3, 4, 9}) {
		t.Errorf("promotions = %v, %v; want [3 4 9]", got, err)
	}
	since := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	got, err = s.SearchGmailRaw(t.Context(), "category:updates", since, 5)
	if err != nil || !slices.Equal(got, []uint32{9}) {
		t.Errorf("new updates = %v, %v; want [9]", got, err)
	}
	_ = s.Close()
	if err := srv.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSearchGmailRawNeedsGmail(t *testing.T) {
	mem := imapxtest.StartMemFull(t)
	s, err := imapx.Open(t.Context(), mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.Select(t.Context(), "INBOX"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SearchGmailRaw(t.Context(), "category:social", time.Time{}, 0); !errors.Is(err, imapx.ErrNotGmail) {
		t.Errorf("search on a server without X-GM-EXT-1 = %v, want ErrNotGmail", err)
	}
}
