// Command uifixture builds a maild data directory for the app's tests: one
// account whose INBOX holds N mailgen messages with their bodies stored, and
// a Hostile mailbox holding the hostile-HTML corpus (docs/plans/0004, Phase 4).
// maild started on it needs no mail server: bodies are local and the account
// has no password, so sync never connects.
//
//	go run ./tools/uifixture -out DIR -n 100000 -hostile internal/render/testdata/hostile
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/mailgen"
	"github.com/frostyard/frostmail/internal/mimex"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

const (
	fixtureEmail = "test1@mailtest.test"
	batchSize    = 1000
)

// Options say what to build.
type Options struct {
	Out     string // the data directory to create (FROSTMAIL_DATA_DIR)
	N       int    // mailgen messages in INBOX
	Seed    uint64 // mailgen seed
	Hostile string // a directory of .html files for the Hostile mailbox; "" for none
	// SMTP is host:port of a plain submission server (tools/smtpsink) the
	// account sends through, with a stored password; "" leaves sending
	// pointed at nothing.
	SMTP string
	// Showcase, when set, replaces the fixture account with hand-written
	// mail for screenshots (buildShowcase).
	Showcase string
}

// Build creates the data directory o.Out and its blobs store, one account
// with the fixture mailboxes, o.N mailgen messages in INBOX committed in
// batches of at most 1,000, and, when o.Hostile names a directory, its
// .html files as messages in the Hostile mailbox. It fails if o.Out
// already exists.
func Build(ctx context.Context, o Options) error {
	if err := os.Mkdir(o.Out, 0o700); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(o.Out, "blobs"), 0o700); err != nil {
		return fmt.Errorf("create blob dir: %w", err)
	}
	db, err := store.Open(ctx, filepath.Join(o.Out, "frostmail.db"))
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	if o.Showcase != "" {
		return buildShowcase(ctx, db, blob.New(filepath.Join(o.Out, "blobs")), o.Out, o.Showcase, time.Now())
	}
	accountID, mbIDs, err := setupAccount(ctx, db)
	if err != nil {
		return err
	}
	if o.SMTP != "" {
		if err := useSMTP(ctx, db, accountID, o.Out, o.SMTP); err != nil {
			return err
		}
	}
	b := &builder{db: db, blobs: blob.New(filepath.Join(o.Out, "blobs")), accountID: accountID}

	if o.N > 0 {
		err = mailgen.Each(o.N, o.Seed, func(m mailgen.Message) error {
			return b.add(ctx, mbIDs["INBOX"], m.Raw, uint32(m.Index), m.Flags)
		})
		if err != nil {
			return fmt.Errorf("generate inbox: %w", err)
		}
	}
	if err := b.flush(ctx, mbIDs["INBOX"]); err != nil {
		return err
	}
	if o.Hostile != "" {
		return buildHostile(ctx, b, mbIDs["Hostile"], o.Hostile)
	}
	return nil
}

// setupAccount inserts the fixture account and mailboxes in one transaction
// and returns the account ID with the mailbox IDs by path.
func setupAccount(ctx context.Context, db *store.DB) (int64, map[string]int64, error) {
	return insertAccount(ctx, db, fixtureEmail, "Test One", "127.0.0.1", 1, 1, fixtureMailboxes())
}

// insertAccount inserts an IMAP account on host (IMAP on imapPort with TLS,
// submission on smtpPort with STARTTLS) with its mailboxes, and returns
// its ID with the mailbox IDs by path.
func insertAccount(ctx context.Context, db *store.DB, email, name, host string, imapPort, smtpPort int,
	mailboxes []store.ServerMailbox) (int64, map[string]int64, error) {
	var accountID int64
	err := db.Tx(ctx, func(tx *store.Tx) error {
		a, err := tx.InsertAccount(ctx, store.Account{
			Kind:        api.AccountKindIMAP,
			Email:       email,
			DisplayName: name,
			Auth:        api.AuthKindPassword,
			IMAP:        store.ServerConfig{Host: host, Port: imapPort, TLS: api.TLSModeTLS, Username: email},
			SMTP:        store.ServerConfig{Host: host, Port: smtpPort, TLS: api.TLSModeStartTLS, Username: email},
		})
		if err != nil {
			return err
		}
		accountID = a.ID
		_, err = tx.ReplaceMailboxes(ctx, accountID, mailboxes)
		return err
	})
	if err != nil {
		return 0, nil, err
	}
	list, err := db.ListMailboxes(ctx, accountID)
	if err != nil {
		return 0, nil, err
	}
	ids := make(map[string]int64, len(list))
	for _, mb := range list {
		ids[mb.Path] = mb.ID
	}
	return accountID, ids, nil
}

func fixtureMailboxes() []store.ServerMailbox {
	mk := func(path string, role api.MailboxRole) store.ServerMailbox {
		return store.ServerMailbox{Path: path, Delimiter: "/", Role: role, Selectable: true, Subscribed: true}
	}
	return []store.ServerMailbox{
		mk("INBOX", api.MailboxRoleInbox),
		mk("Drafts", api.MailboxRoleDrafts),
		mk("Sent", api.MailboxRoleSent),
		mk("Junk", api.MailboxRoleJunk),
		mk("Trash", api.MailboxRoleTrash),
		mk("Archive", api.MailboxRoleArchive),
		mk("Hostile", api.MailboxRoleNone),
	}
}

// pending is one message whose blob is stored but whose row is not yet committed.
type pending struct {
	header store.MessageHeader
	blobID string
}

// builder accumulates messages and commits them in batches.
type builder struct {
	db        *store.DB
	blobs     *blob.Store
	accountID int64
	batch     []pending
}

// add stores the raw message as a blob, parses its header and flushes the
// batch when it reaches batchSize.
func (b *builder) add(ctx context.Context, mailboxID int64, raw []byte, uid uint32, flags string) error {
	blobID, err := b.blobs.Put(ctx, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("put blob uid %d: %w", uid, err)
	}
	h, err := headerFromRaw(raw, uid, flags)
	if err != nil {
		return err
	}
	b.batch = append(b.batch, pending{header: h, blobID: blobID})
	if len(b.batch) >= batchSize {
		return b.flush(ctx, mailboxID)
	}
	return nil
}

// flush commits the accumulated batch into mailboxID: headers, bodies and
// search entries, then threads for the batch's IDs.
func (b *builder) flush(ctx context.Context, mailboxID int64) error {
	if len(b.batch) == 0 {
		return nil
	}
	batch := b.batch
	b.batch = make([]pending, 0, batchSize)
	hs := make([]store.MessageHeader, len(batch))
	for i, p := range batch {
		hs[i] = p.header
	}
	return b.db.Tx(ctx, func(tx *store.Tx) error {
		ids, err := tx.InsertHeaders(ctx, b.accountID, mailboxID, hs)
		if err != nil {
			return err
		}
		for i, id := range ids {
			if err := tx.SetBody(ctx, id, batch[i].blobID); err != nil {
				return err
			}
			if err := tx.IndexMessage(ctx, id, store.SearchDocFor(hs[i])); err != nil {
				return err
			}
		}
		_, err = tx.AssignThreads(ctx, b.accountID, ids)
		return err
	})
}

// buildHostile turns every .html file in dir, in name order, into a
// message with UID from 1 in mailboxID.
func buildHostile(ctx context.Context, b *builder, mailboxID int64, dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil {
		return fmt.Errorf("list hostile corpus: %w", err)
	}
	for i, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return fmt.Errorf("read hostile %s: %w", f, err)
		}
		name := strings.TrimSuffix(filepath.Base(f), ".html")
		if err := b.add(ctx, mailboxID, hostileMessage(name, raw), uint32(i+1), ""); err != nil {
			return err
		}
	}
	return b.flush(ctx, mailboxID)
}

func hostileMessage(name string, html []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("From: Hostile <hostile@mailtest.test>\r\n")
	buf.WriteString("To: " + fixtureEmail + "\r\n")
	buf.WriteString("Subject: Hostile: " + name + "\r\n")
	buf.WriteString("Date: Mon, 05 Oct 2026 12:00:00 +0000\r\n")
	buf.WriteString("Message-ID: <hostile-" + name + "@uifixture.test>\r\n")
	buf.WriteString("MIME-Version: 1.0\r\n")
	buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	buf.WriteString("\r\n")
	buf.Write(html)
	return buf.Bytes()
}

// headerFromRaw parses a raw message into a MessageHeader with the given
// UID and mailgen flag string.
func headerFromRaw(raw []byte, uid uint32, flags string) (store.MessageHeader, error) {
	root, err := message.Read(bytes.NewReader(raw))
	if err != nil && !message.IsUnknownCharset(err) && !message.IsUnknownEncoding(err) {
		return store.MessageHeader{}, fmt.Errorf("read message uid %d: %w", uid, err)
	}
	if root == nil {
		return store.MessageHeader{}, fmt.Errorf("read message uid %d: no entity", uid)
	}
	mh := mail.Header{Header: root.Header}
	h := store.MessageHeader{UID: uid, Size: int64(len(raw)), Flags: flagsFromString(flags)}
	h.Subject, _ = mh.Subject()
	h.MessageID, _ = mh.MessageID()
	h.From = firstAddress(mh, "From")
	h.To = addressList(mh, "To")
	h.Cc = addressList(mh, "Cc")
	if date, err := mh.Date(); err == nil {
		h.InternalDate, h.Date = date, date
	}
	if ids := mimex.ParseMessageIDs(mh.Get("In-Reply-To")); len(ids) > 0 {
		h.InReplyTo = ids[0]
	}
	h.References = mimex.ParseMessageIDs(mh.Get("References"))
	text, _, err := mimex.BodyText(raw)
	if err != nil {
		return store.MessageHeader{}, fmt.Errorf("body text uid %d: %w", uid, err)
	}
	h.Preview = mimex.Preview(text, 200)
	h.Parts, h.HasAttachments, err = partsOf(raw)
	if err != nil {
		return store.MessageHeader{}, fmt.Errorf("parts uid %d: %w", uid, err)
	}
	return h, nil
}

// addressList returns the addresses of a header; an absent or unparsable
// header is no addresses.
func addressList(mh mail.Header, key string) []store.Address {
	list, err := mh.AddressList(key)
	if err != nil {
		return nil
	}
	out := make([]store.Address, 0, len(list))
	for _, a := range list {
		out = append(out, store.Address{Name: a.Name, Addr: a.Address})
	}
	return out
}

func firstAddress(mh mail.Header, key string) store.Address {
	l := addressList(mh, key)
	if len(l) == 0 {
		return store.Address{}
	}
	return l[0]
}

// partsOf lists the message's leaf parts with the size of each decoded body.
func partsOf(raw []byte) ([]store.Part, bool, error) {
	var parts []store.Part
	hasAttachments := false
	err := mimex.WalkParts(raw, func(p mimex.PartInfo, body io.Reader) error {
		n, err := io.Copy(io.Discard, body)
		if err != nil {
			return fmt.Errorf("read part %s: %w", p.Path, err)
		}
		if p.Disposition == "attachment" || (p.Filename != "" && p.Disposition != "inline") {
			hasAttachments = true
		}
		parts = append(parts, store.Part{
			Path:        p.Path,
			ContentType: p.ContentType,
			Disposition: p.Disposition,
			Filename:    p.Filename,
			ContentID:   p.ContentID,
			Size:        n,
		})
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return parts, hasAttachments, nil
}

// flagsFromString maps a mailgen Maildir flag string to stored flags. A
// digit after F picks Mail.app's flag color (1 red to 7 gray).
func flagsFromString(s string) store.Flags {
	var f store.Flags
	for _, c := range s {
		switch {
		case c == 'S':
			f.Seen = true
		case c == 'R':
			f.Answered = true
		case c == 'F':
			f.Flagged = true
			f.Color = 1
		case c >= '1' && c <= '7' && f.Flagged:
			f.Color = int(c - '0')
		}
	}
	return f
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "uifixture:", err)
		os.Exit(1)
	}
}

// useSMTP points the account's submission server at addr (no TLS) and
// stores a password for it.
func useSMTP(ctx context.Context, db *store.DB, accountID int64, out, addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("-smtp: %w", err)
	}
	p, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("-smtp port: %w", err)
	}
	server := store.ServerConfig{Host: host, Port: p, TLS: api.TLSModeInsecure, Username: fixtureEmail}
	if err := db.Tx(ctx, func(tx *store.Tx) error {
		_, err := tx.UpdateAccount(ctx, accountID, store.AccountUpdate{SMTP: &server})
		return err
	}); err != nil {
		return err
	}
	return secrets.NewFile(filepath.Join(out, "secrets.json")).Set(ctx, secrets.AccountPassword(accountID), "fixture")
}

// run parses -out, -n, -seed, -hostile, -smtp and -showcase into Options
// and calls Build.
func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("uifixture", flag.ContinueOnError)
	out := fs.String("out", "", "data directory to create (required)")
	n := fs.Int("n", 1000, "mailgen messages in INBOX")
	seed := fs.Uint64("seed", 1, "mailgen seed")
	hostile := fs.String("hostile", "", "directory of .html files for the Hostile mailbox")
	smtpAddr := fs.String("smtp", "", "host:port of a plain SMTP server to send through (tools/smtpsink)")
	showcase := fs.String("showcase", "", "directory of hand-written mail for screenshots, instead of the fixture account")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return errors.New("-out is required")
	}
	if *n < 0 {
		return errors.New("-n must be at least 0")
	}
	return Build(ctx, Options{Out: *out, N: *n, Seed: *seed, Hostile: *hostile, SMTP: *smtpAddr, Showcase: *showcase})
}
