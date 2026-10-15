package imapxtest

import (
	"sync/atomic"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
)

// Counts counts the commands of a Mem server that show how bodies are
// fetched.
type Counts struct {
	// Examines counts read-only selects (EXAMINE).
	Examines atomic.Int64
	// Whole counts fetches of whole messages (BODY[]).
	Whole atomic.Int64
}

// StartMemCounted is StartMem that counts examines and whole-message
// fetches in the returned Counts.
func StartMemCounted(t testing.TB) (*Mem, *Counts) {
	t.Helper()
	c := &Counts{}
	m := startMemWith(t, imap.CapSet{imap.CapIMAP4rev1: {}}, func(s imapserver.Session) imapserver.Session {
		return &countedSession{Session: s, counts: c}
	})
	return m, c
}

type countedSession struct {
	imapserver.Session
	counts *Counts
}

func (s *countedSession) Select(mailbox string, options *imap.SelectOptions) (*imap.SelectData, error) {
	if options != nil && options.ReadOnly {
		s.counts.Examines.Add(1)
	}
	return s.Session.Select(mailbox, options)
}

func (s *countedSession) Fetch(w *imapserver.FetchWriter, numSet imap.NumSet, options *imap.FetchOptions) error {
	for _, sec := range options.BodySection {
		if sec.Specifier == imap.PartSpecifierNone && len(sec.Part) == 0 && sec.Partial == nil {
			s.counts.Whole.Add(1)
		}
	}
	return s.Session.Fetch(w, numSet, options)
}

func (s *countedSession) Namespace() (*imap.NamespaceData, error) {
	return s.Session.(imapserver.SessionNamespace).Namespace()
}
