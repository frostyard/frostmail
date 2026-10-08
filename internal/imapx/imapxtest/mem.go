// Package imapxtest runs go-imap's in-memory IMAP server for unit tests.
// It has no CONDSTORE, QRESYNC or Gmail extensions: test those against the
// Dovecot container (make engine-it) or recorded transcripts.
package imapxtest

import (
	"errors"
	"net"
	"strconv"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/emersion/go-sasl"
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
	return startMemWith(t, caps, nil)
}

func startMemWith(t testing.TB, caps imap.CapSet, wrap func(imapserver.Session) imapserver.Session) *Mem {
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
			s := mem.NewSession()
			if wrap != nil {
				s = wrap(s)
			}
			return s, nil, nil
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

// StartMemFullOAuth is StartMemFull that also accepts AUTHENTICATE XOAUTH2
// with the given access token (as the user Username).
func StartMemFullOAuth(t testing.TB, token string) *Mem {
	t.Helper()
	caps := imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapMove: {}, imap.CapUIDPlus: {}, imap.CapESearch: {}}
	return startMemWith(t, caps, func(s imapserver.Session) imapserver.Session { return &oauthSession{Session: s, token: token} })
}

// oauthSession adds XOAUTH2 to a memory session; MOVE and NAMESPACE pass
// through.
type oauthSession struct {
	imapserver.Session
	token string
}

func (s *oauthSession) AuthenticateMechanisms() []string { return []string{"XOAUTH2"} }

func (s *oauthSession) Authenticate(mech string) (sasl.Server, error) {
	if mech != "XOAUTH2" {
		return nil, errors.New("unsupported mechanism")
	}
	return &xoauth2Server{s: s}, nil
}

func (s *oauthSession) Move(w *imapserver.MoveWriter, numSet imap.NumSet, dest string) error {
	return s.Session.(imapserver.SessionMove).Move(w, numSet, dest)
}

func (s *oauthSession) Namespace() (*imap.NamespaceData, error) {
	return s.Session.(imapserver.SessionNamespace).Namespace()
}

// xoauth2Server checks the initial response; a wrong token gets Gmail's
// failure challenge, then NO.
type xoauth2Server struct {
	s       *oauthSession
	refused bool
}

func (x *xoauth2Server) Next(response []byte) ([]byte, bool, error) {
	if x.refused {
		return nil, true, errors.New("invalid credentials")
	}
	want := "user=" + Username + "\x01auth=Bearer " + x.s.token + "\x01\x01"
	if string(response) != want {
		x.refused = true
		return []byte(`{"status":"401","schemes":"Bearer","scope":"https://mail.google.com/"}`), false, nil
	}
	if err := x.s.Login(Username, Password); err != nil {
		return nil, true, err
	}
	return nil, true, nil
}
