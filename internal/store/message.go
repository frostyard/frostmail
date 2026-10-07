package store

import (
	"context"
	"errors"
	"time"
)

// errNotImplemented marks stubs that task cards replace.
var errNotImplemented = errors.New("store: not implemented")

// Address is one mailbox address.
type Address struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

// Part is one MIME part from BODYSTRUCTURE.
type Part struct {
	Path        string // IMAP part specifier: "1", "1.2"
	ContentType string // lowercase type/subtype
	Charset     string
	Encoding    string // lowercase Content-Transfer-Encoding
	Disposition string // inline, attachment or ""
	Filename    string
	ContentID   string // without angle brackets
	Size        int64
}

// MessageHeader is what a header fetch learns about one message in one
// mailbox (docs/design/sync.md, the reconcile pass).
type MessageHeader struct {
	UID             uint32
	ModSeq          uint64
	Flags           Flags
	InternalDate    time.Time
	Size            int64
	MessageID       string // without angle brackets
	InReplyTo       string
	References      []string
	Subject         string
	From            Address
	To              []Address
	Cc              []Address
	Bcc             []Address
	ReplyTo         []Address
	Date            time.Time // zero when the Date header is missing or invalid
	ListID          string
	ListUnsubscribe string
	AuthResults     string
	HasAttachments  bool
	Preview         string
	Parts           []Part
}

// FlagUpdate is a message's flags as the server reports them.
type FlagUpdate struct {
	UID    uint32
	ModSeq uint64
	Flags  Flags
}

// InsertHeaders stores the messages in hs that mailboxID does not hold yet
// and returns their new IDs in the order of hs. A header whose UID is
// already in the mailbox is skipped, so a reconcile pass can be repeated
// after a crash. Each insert writes a messages row (body_state 'headers',
// thread_id NULL, subject_norm from mimex.NormalizeSubject, addresses and
// references as JSON arrays, date_hdr NULL for a zero Date), its
// message_mailbox row (uid, modseq) and its parts rows. It emits nothing:
// the sync engine threads, indexes and emits message.changed for the batch
// in the same transaction. Task T-0013 implements it.
func (t *Tx) InsertHeaders(ctx context.Context, accountID, mailboxID int64, hs []MessageHeader) ([]int64, error) {
	return nil, errNotImplemented
}

// MailboxUIDs returns the UIDs stored for mailboxID in ascending order; none
// is an empty slice. Task T-0013 implements it.
func (d *DB) MailboxUIDs(ctx context.Context, mailboxID int64) ([]uint32, error) {
	return nil, errNotImplemented
}

// UpdateFlags applies server-reported flags to the messages of mailboxID by
// UID and returns, in the order of ups, the IDs of messages whose stored
// flags changed. Every matched membership gets the update's modseq; UIDs not
// in the mailbox are skipped. It emits nothing. Task T-0013 implements it.
func (t *Tx) UpdateFlags(ctx context.Context, mailboxID int64, ups []FlagUpdate) ([]int64, error) {
	return nil, errNotImplemented
}

// RemoveUIDs removes the memberships of uids in mailboxID, deletes the
// messages left in no mailbox together with their parts and search entries,
// and returns the deleted message IDs in ascending order. UIDs not in the
// mailbox are skipped. It emits nothing. Task T-0013 implements it.
func (t *Tx) RemoveUIDs(ctx context.Context, mailboxID int64, uids []uint32) ([]int64, error) {
	return nil, errNotImplemented
}
