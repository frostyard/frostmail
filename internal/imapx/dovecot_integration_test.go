//go:build integration

package imapx_test

// Runs against the frostmail-mailtest container (dev/incus/mailtest.sh):
// make engine-it sets FROSTMAIL_IT_HOST and restores the clean snapshot.

import (
	"os"
	"testing"

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
