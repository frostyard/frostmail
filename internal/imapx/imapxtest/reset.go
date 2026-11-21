package imapxtest

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
)

// ExamineReset resets the connections of a Mem server that examine a
// mailbox (read-only selects, as maild's body connection C3 makes) while
// it is armed, as a network that drops a connection does.
type ExamineReset struct {
	armed atomic.Bool
	// Resets counts the connections reset.
	Resets atomic.Int64
}

// Arm makes later examines reset their connection.
func (r *ExamineReset) Arm() { r.armed.Store(true) }

// Disarm lets later examines through.
func (r *ExamineReset) Disarm() { r.armed.Store(false) }

// StartMemExamineReset is StartMem whose connections are reset when they
// examine a mailbox while the returned ExamineReset is armed. It starts
// armed.
func StartMemExamineReset(t testing.TB) (*Mem, *ExamineReset) {
	t.Helper()
	r := &ExamineReset{}
	r.Arm()
	m := startMemConn(t, imap.CapSet{imap.CapIMAP4rev1: {}}, func(conn *imapserver.Conn, s imapserver.Session) imapserver.Session {
		return &resetSession{Session: s, conn: conn, reset: r}
	})
	return m, r
}

type resetSession struct {
	imapserver.Session
	conn  *imapserver.Conn
	reset *ExamineReset
}

func (s *resetSession) Select(mailbox string, options *imap.SelectOptions) (*imap.SelectData, error) {
	if options != nil && options.ReadOnly && s.reset.armed.Load() {
		s.reset.Resets.Add(1)
		_ = s.conn.NetConn().Close()
		return nil, errors.New("imapxtest: connection reset")
	}
	return s.Session.Select(mailbox, options)
}

func (s *resetSession) Namespace() (*imap.NamespaceData, error) {
	return s.Session.(imapserver.SessionNamespace).Namespace()
}
