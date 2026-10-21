package imapx_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/imapx/imapxtest"
)

func TestDialMemServer(t *testing.T) {
	mem := imapxtest.StartMem(t)
	c, err := imapx.Dial(t.Context(), mem.DialOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	boxes, err := c.List("", "*", &imap.ListOptions{ReturnSpecialUse: true}).Collect()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, b := range boxes {
		names = append(names, b.Mailbox)
	}
	slices.Sort(names)
	if strings.Join(names, ",") != "Archive,Drafts,INBOX,Junk,Sent,Trash" {
		t.Fatalf("mailboxes = %v", names)
	}
}

func TestDialWrongPassword(t *testing.T) {
	mem := imapxtest.StartMem(t)
	opts := mem.DialOptions()
	opts.Password = "wrong"
	if _, err := imapx.Dial(t.Context(), opts); !errors.Is(err, imapx.ErrAuth) {
		t.Fatalf("Dial with a wrong password = %v, want ErrAuth", err)
	}
}

// TestDialOutlookNotConnected checks that Outlook's "User is authenticated
// but not connected." is ErrUnavailable, which sync retries, not ErrAuth.
func TestDialOutlookNotConnected(t *testing.T) {
	mem := imapxtest.StartMemFullOAuth(t, "tok")
	mem.Unavailable.Store(1)
	opts := mem.DialOptions()
	opts.Password, opts.OAuth = "tok", true
	_, err := imapx.Dial(t.Context(), opts)
	if !errors.Is(err, imapx.ErrUnavailable) || errors.Is(err, imapx.ErrAuth) {
		t.Fatalf("Dial = %v, want ErrUnavailable and not ErrAuth", err)
	}
	c, err := imapx.Dial(t.Context(), opts)
	if err != nil {
		t.Fatalf("the next Dial = %v", err)
	}
	_ = c.Close()
}
