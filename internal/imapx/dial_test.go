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
