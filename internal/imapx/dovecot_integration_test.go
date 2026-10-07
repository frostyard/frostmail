//go:build integration

package imapx_test

// Runs against the frostmail-mailtest container (dev/incus/mailtest.sh):
// make engine-it sets FROSTMAIL_IT_HOST and restores the clean snapshot.

import (
	"os"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
)

func dovecot(t *testing.T, mode api.TLSMode, port int) imapx.DialOptions {
	host := os.Getenv("FROSTMAIL_IT_HOST")
	if host == "" {
		t.Skip("FROSTMAIL_IT_HOST is not set; run make engine-it")
	}
	return imapx.DialOptions{
		Host: host, Port: port, TLS: mode, Username: "test1@mailtest.test", Password: "frostmail-test",
		InsecureSkipVerify: true,
	}
}

func TestDovecotCapabilitiesAndSeed(t *testing.T) {
	for _, tc := range []struct {
		mode api.TLSMode
		port int
	}{{api.TLSModeTLS, 993}, {api.TLSModeStartTLS, 143}} {
		t.Run(string(tc.mode), func(t *testing.T) {
			c, err := imapx.Dial(t.Context(), dovecot(t, tc.mode, tc.port))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			caps := c.Caps()
			for _, want := range []imap.Cap{imap.CapCondStore, imap.CapQResync, imap.CapIdle, imap.CapMove, imap.CapUIDPlus, imap.CapSpecialUse, imap.CapESearch} {
				if !caps.Has(want) {
					t.Errorf("Dovecot lacks %s", want)
				}
			}
			sel, err := c.Select("INBOX", &imap.SelectOptions{CondStore: true}).Wait()
			if err != nil {
				t.Fatal(err)
			}
			if sel.NumMessages != 5 || sel.HighestModSeq == 0 || sel.UIDValidity == 0 {
				t.Fatalf("INBOX = %+v; want the 5 seeded messages with a modseq", sel)
			}
			msgs, err := c.Fetch(imap.UIDSet{imap.UIDRange{Start: 1}}, &imap.FetchOptions{
				UID: true, Envelope: true, ModSeq: true, Flags: true,
			}).Collect()
			if err != nil {
				t.Fatal(err)
			}
			if len(msgs) != 5 || msgs[0].Envelope.Subject != "Welcome to the frostmail test server" || msgs[0].ModSeq == 0 {
				t.Fatalf("first message = %+v", msgs[0])
			}
			if got := msgs[3].Envelope.Subject; got != "October deals — up to 40% off" {
				t.Errorf("RFC 2047 subject = %q", got)
			}
		})
	}
}

func TestDovecotSession(t *testing.T) {
	opts := dovecot(t, api.TLSModeTLS, 993)
	ctx := t.Context()
	s, err := imapx.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !s.Caps.CondStore || !s.Caps.ESearch || !s.Caps.Binary || !s.Caps.Move || !s.Caps.UIDPlus || !s.Caps.ListExtended {
		t.Fatalf("caps = %+v", s.Caps)
	}
	list, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	roles := map[string]api.MailboxRole{}
	for _, mb := range list {
		roles[mb.Path] = mb.Role
		if !mb.Subscribed {
			t.Errorf("%s not subscribed", mb.Path)
		}
	}
	if roles["Sent"] != api.MailboxRoleSent || roles["Archive"] != api.MailboxRoleArchive || roles["INBOX"] != api.MailboxRoleInbox {
		t.Fatalf("roles = %v", roles)
	}

	sel, err := s.Select(ctx, "INBOX")
	if err != nil || sel.Messages != 5 || sel.HighestModSeq == 0 {
		t.Fatalf("select = %+v, %v", sel, err)
	}
	hs, err := s.FetchHeaders(ctx, []uint32{4})
	if err != nil || len(hs) != 1 {
		t.Fatalf("headers = %v, %v", hs, err)
	}
	if hs[0].Preview != "October deals: up to 40% off. View in a browser: https://shop.mailtest.test/oct" {
		t.Errorf("BINARY preview = %q", hs[0].Preview)
	}

	// CHANGEDSINCE: nothing changed yet, then one flag change.
	if ups, err := s.FetchFlags(ctx, []uint32{1, 2, 3, 4, 5}, sel.HighestModSeq); err != nil || len(ups) != 0 {
		t.Fatalf("unchanged flags = %v, %v", ups, err)
	}
	if err := s.StoreFlags(ctx, []uint32{2}, []string{`\Flagged`, "$MailFlagBit2"}, nil); err != nil {
		t.Fatal(err)
	}
	ups, err := s.FetchFlags(ctx, []uint32{1, 2, 3, 4, 5}, sel.HighestModSeq)
	if err != nil || len(ups) != 1 || ups[0].UID != 2 || ups[0].Flags.Color != 5 || ups[0].ModSeq <= sel.HighestModSeq {
		t.Fatalf("changed flags = %+v, %v", ups, err)
	}

	// MOVE reports the new UID (COPYUID).
	moved, err := s.Move(ctx, []uint32{1}, "Archive")
	if err != nil || len(moved) != 1 || moved[1] == 0 {
		t.Fatalf("move = %v, %v", moved, err)
	}
	// UID EXPUNGE removes only the given UID.
	if err := s.StoreFlags(ctx, []uint32{3}, []string{`\Deleted`}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Expunge(ctx, []uint32{3}); err != nil {
		t.Fatal(err)
	}
	uids, err := s.UIDs(ctx)
	if err != nil || len(uids) != 3 || uids[0] != 2 {
		t.Fatalf("UIDs after move and expunge = %v, %v", uids, err)
	}
}

func TestDovecotIdleWakesOnDelivery(t *testing.T) {
	opts := dovecot(t, api.TLSModeTLS, 993)
	ctx := t.Context()
	woke := make(chan struct{}, 8)
	opts.OnUpdate = func() {
		select {
		case woke <- struct{}{}:
		default:
		}
	}
	idler, err := imapx.Open(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer idler.Close()
	if _, err := idler.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}
	other, err := imapx.Open(ctx, dovecot(t, api.TLSModeTLS, 993))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if _, err := other.Select(ctx, "INBOX"); err != nil {
		t.Fatal(err)
	}

	idling := make(chan struct{})
	done := make(chan error, 1)
	start := time.Now()
	go func() { done <- idler.Idle(ctx, woke, time.Minute, func() { close(idling) }) }()
	<-idling
	if err := other.StoreFlags(ctx, []uint32{4}, []string{`\Seen`}, nil); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Fatalf("woke after %v", d)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("IDLE did not wake on another client's flag change")
	}
}
