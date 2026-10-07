// Package imapxtest runs go-imap's in-memory IMAP server for unit tests.
// It has no CONDSTORE, QRESYNC or Gmail extensions: test those against the
// Dovecot container (make engine-it) or recorded transcripts.
package imapxtest

import (
	"net"
	"strconv"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/imapx"
)

// Username and Password log in to a Mem server.
const (
	Username = "test"
	Password = "test"
)

// Mem is a running in-memory server with one user and the standard
// mailboxes: INBOX, Drafts, Sent, Archive, Junk and Trash (special-use).
type Mem struct {
	User *imapmemserver.User
	Addr string
}

// StartMem listens on localhost with only IMAP4rev1 (and the extensions that
// need no backend support, such as IDLE), so sync's fallbacks are exercised.
// The server stops when the test ends.
func StartMem(t testing.TB) *Mem { return startMem(t, imap.CapSet{imap.CapIMAP4rev1: {}}) }

// StartMemFull is StartMem plus MOVE, UIDPLUS and ESEARCH, for tests of
// moves and deletes.
func StartMemFull(t testing.TB) *Mem {
	return startMem(t, imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapMove: {}, imap.CapUIDPlus: {}, imap.CapESearch: {}})
}

func startMem(t testing.TB, caps imap.CapSet) *Mem {
	t.Helper()
	user := imapmemserver.NewUser(Username, Password)
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatal(err)
	}
	for name, attr := range map[string]imap.MailboxAttr{
		"Drafts": imap.MailboxAttrDrafts, "Sent": imap.MailboxAttrSent, "Archive": imap.MailboxAttrArchive,
		"Junk": imap.MailboxAttrJunk, "Trash": imap.MailboxAttrTrash,
	} {
		if err := user.Create(name, &imap.CreateOptions{SpecialUse: []imap.MailboxAttr{attr}}); err != nil {
			t.Fatal(err)
		}
	}
	mem := imapmemserver.New()
	mem.AddUser(user)
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps:         caps,
		InsecureAuth: true,
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return &Mem{User: user, Addr: ln.Addr().String()}
}

// DialOptions logs in to the server without TLS.
func (m *Mem) DialOptions() imapx.DialOptions {
	host, port, _ := net.SplitHostPort(m.Addr)
	p, _ := strconv.Atoi(port)
	return imapx.DialOptions{Host: host, Port: p, TLS: api.TLSModeInsecure, Username: Username, Password: Password}
}
