package mailsync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/imapx"
	"github.com/frostyard/frostmail/internal/store"
)

// No local server speaks Gmail's extensions, so the Gmail path is tested
// against gmailModel: All Mail, Spam and Trash hold messages under their own
// UIDs, labels live on the messages, and STORE, MOVE and EXPUNGE behave as
// Gmail's IMAP does in the cases Frostmail uses. Recorded sessions from a
// real account check the model (docs/plans/0006-m4-daily-driver.md).

const (
	gAll   = "[Gmail]/All Mail"
	gSpam  = "[Gmail]/Spam"
	gTrash = "[Gmail]/Trash"
)

type gmailModel struct {
	folders  map[string]*gmFolder
	labels   map[string]string // label folder path → X-GM-LABELS name
	msgs     []*gmMsg
	modseq   uint64
	nextID   uint64
	selected string
	log      []string        // commands run, "SELECT path" and so on
	refuse   map[string]bool // commands answered NO, by name ("STORE labels")
}

type gmFolder struct{ validity, next uint32 }

type gmMsg struct {
	id, thread uint64
	folder     string
	uid        uint32
	labels     []string
	flags      []string
	modseq     uint64
	subject    string
}

func newGmailModel() *gmailModel {
	g := &gmailModel{
		folders: map[string]*gmFolder{},
		labels: map[string]string{
			"INBOX": `\Inbox`, "[Gmail]/Sent Mail": `\Sent`, "[Gmail]/Drafts": `\Draft`,
			"[Gmail]/Starred": `\Starred`, "[Gmail]/Important": `\Important`, "Work": "Work",
		},
		modseq: 100, nextID: 1000,
	}
	for _, f := range []string{gAll, gSpam, gTrash} {
		g.folders[f] = &gmFolder{validity: 7, next: 1}
	}
	return g
}

// add puts a new message in folder with labels; thread 0 starts a thread.
func (g *gmailModel) add(folder, subject string, thread uint64, labels ...string) *gmMsg {
	g.nextID++
	if thread == 0 {
		thread = g.nextID
	}
	m := &gmMsg{id: g.nextID, thread: thread, subject: subject, labels: labels}
	g.msgs = append(g.msgs, m)
	g.place(m, folder)
	return m
}

func (g *gmailModel) place(m *gmMsg, folder string) {
	f := g.folders[folder]
	m.folder, m.uid = folder, f.next
	f.next++
	g.touch(m)
}

func (g *gmailModel) touch(m *gmMsg) {
	g.modseq++
	m.modseq = g.modseq
}

// labelBase offsets a message's All Mail UID to its UID in a label folder.
const labelBase = 5000

// uidIn is a message's UID in folder: its own in its synced folder,
// labelBase more than its All Mail UID in the folder of a label it has, and
// 0 elsewhere.
func (g *gmailModel) uidIn(m *gmMsg, folder string) uint32 {
	if g.folders[folder] != nil {
		if m.folder == folder {
			return m.uid
		}
		return 0
	}
	if l := g.labels[folder]; l != "" && m.folder == gAll && slices.Contains(m.labels, l) {
		return labelBase + m.uid
	}
	return 0
}

func (g *gmailModel) in(folder string) []*gmMsg {
	var out []*gmMsg
	for _, m := range g.msgs {
		if g.uidIn(m, folder) != 0 {
			out = append(out, m)
		}
	}
	slices.SortFunc(out, func(a, b *gmMsg) int { return int(g.uidIn(a, folder)) - int(g.uidIn(b, folder)) })
	return out
}

func (g *gmailModel) at(uids []uint32) []*gmMsg {
	var out []*gmMsg
	for _, m := range g.in(g.selected) {
		if slices.Contains(uids, g.uidIn(m, g.selected)) {
			out = append(out, m)
		}
	}
	return out
}

func (g *gmailModel) Capabilities() imapx.Capabilities {
	return imapx.Capabilities{CondStore: true, Move: true, UIDPlus: true, Idle: true, SpecialUse: true, Gmail: true}
}

func (g *gmailModel) Select(_ context.Context, path string) (imapx.Selected, error) {
	g.log = append(g.log, "SELECT "+path)
	f := g.folders[path]
	if f == nil && g.labels[path] != "" {
		f = &gmFolder{validity: 9, next: labelBase + g.folders[gAll].next}
	}
	if f == nil {
		return imapx.Selected{}, fmt.Errorf("select %s: no such folder", path)
	}
	g.selected = path
	return imapx.Selected{UIDValidity: f.validity, UIDNext: f.next, HighestModSeq: g.modseq, Messages: uint32(len(g.in(path)))}, nil
}

func (g *gmailModel) UIDs(context.Context) ([]uint32, error) {
	g.log = append(g.log, "UID SEARCH")
	uids := []uint32{}
	for _, m := range g.in(g.selected) {
		uids = append(uids, g.uidIn(m, g.selected))
	}
	return uids, nil
}

func (g *gmailModel) FetchGmailHeaders(_ context.Context, uids []uint32) ([]store.MessageHeader, error) {
	g.log = append(g.log, fmt.Sprintf("FETCH headers %v", uids))
	var out []store.MessageHeader
	for _, m := range g.at(uids) {
		out = append(out, store.MessageHeader{
			UID: g.uidIn(m, g.selected), ModSeq: m.modseq, Flags: store.FlagsFromIMAP(m.flags),
			InternalDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Date: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
			MessageID: fmt.Sprintf("m%d@mail.gmail.com", m.id), Subject: m.subject,
			From:    store.Address{Name: "Ann", Addr: "ann@example.com"},
			GmMsgID: m.id, GmThrID: m.thread, Labels: slices.Clone(m.labels),
		})
	}
	return out, nil
}

func (g *gmailModel) FetchGmailChanges(_ context.Context, uids []uint32, since uint64) ([]store.FlagUpdate, error) {
	g.log = append(g.log, fmt.Sprintf("FETCH changes %v since %d", uids, since))
	var out []store.FlagUpdate
	for _, m := range g.at(uids) {
		if m.modseq > since {
			out = append(out, store.FlagUpdate{UID: g.uidIn(m, g.selected), ModSeq: m.modseq, Flags: store.FlagsFromIMAP(m.flags), Labels: append([]string{}, m.labels...)})
		}
	}
	return out, nil
}

func (g *gmailModel) StoreLabels(_ context.Context, uids []uint32, add, remove []string) error {
	g.log = append(g.log, fmt.Sprintf("STORE %v +labels %v -labels %v", uids, add, remove))
	if g.refuse["STORE labels"] {
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "refused by the test"}
	}
	for _, m := range g.at(uids) {
		g.label(m, add, remove)
	}
	return nil
}

func (g *gmailModel) label(m *gmMsg, add, remove []string) {
	m.labels = slices.DeleteFunc(m.labels, func(l string) bool { return slices.Contains(remove, l) })
	for _, l := range add {
		if !slices.Contains(m.labels, l) {
			m.labels = append(m.labels, l)
		}
	}
	g.touch(m)
}

func (g *gmailModel) StoreFlags(_ context.Context, uids []uint32, add, remove []string) error {
	g.log = append(g.log, fmt.Sprintf("STORE %v +flags %v -flags %v", uids, add, remove))
	for _, m := range g.at(uids) {
		m.flags = slices.DeleteFunc(m.flags, func(f string) bool { return slices.Contains(remove, f) })
		for _, f := range add {
			if !slices.Contains(m.flags, f) {
				m.flags = append(m.flags, f)
			}
		}
		// Gmail keeps \Starred and \Flagged together.
		switch {
		case slices.Contains(add, `\Flagged`):
			g.label(m, []string{`\Starred`}, nil)
		case slices.Contains(remove, `\Flagged`):
			g.label(m, nil, []string{`\Starred`})
		default:
			g.touch(m)
		}
	}
	return nil
}

// Move moves messages to All Mail, Spam or Trash from any folder, or out of
// Spam or Trash into a label folder (which returns that folder's UIDs).
func (g *gmailModel) Move(_ context.Context, uids []uint32, dest string) (map[uint32]uint32, error) {
	g.log = append(g.log, fmt.Sprintf("MOVE %v %s", uids, dest))
	if g.refuse["MOVE"] {
		return nil, &imap.Error{Type: imap.StatusResponseTypeNo, Text: "refused by the test"}
	}
	out := map[uint32]uint32{}
	for _, m := range g.at(uids) {
		old := g.uidIn(m, g.selected)
		switch {
		case g.folders[dest] != nil:
			g.place(m, dest)
			out[old] = m.uid
		case g.labels[dest] != "" && (g.selected == gSpam || g.selected == gTrash):
			g.place(m, gAll)
			g.label(m, []string{g.labels[dest]}, nil)
			out[old] = g.uidIn(m, dest)
		default:
			return nil, fmt.Errorf("move %s to %s: not modeled", g.selected, dest)
		}
	}
	return out, nil
}

func (g *gmailModel) SearchMessageID(_ context.Context, msgid string) ([]uint32, error) {
	var out []uint32
	for _, m := range g.in(g.selected) {
		if fmt.Sprintf("m%d@mail.gmail.com", m.id) == msgid {
			out = append(out, g.uidIn(m, g.selected))
		}
	}
	return out, nil
}

func (g *gmailModel) Append(context.Context, string, []byte, []string) (uint32, error) {
	return 0, errors.New("append: not modeled")
}

func (g *gmailModel) Expunge(_ context.Context, uids []uint32) error {
	g.log = append(g.log, fmt.Sprintf("EXPUNGE %v", uids))
	if g.selected != gTrash {
		return fmt.Errorf("expunge in %s: not modeled", g.selected)
	}
	for _, m := range g.at(uids) {
		if slices.Contains(m.flags, `\Deleted`) {
			g.msgs = slices.DeleteFunc(g.msgs, func(x *gmMsg) bool { return x == m })
		}
	}
	return nil
}

// gmailEnv is an actor on the Gmail path over a fresh store and a model.
type gmailEnv struct {
	t   *testing.T
	db  *store.DB
	a   *actor
	g   *gmailModel
	mbs map[string]int64 // path → mailbox ID
	seq int64            // events read so far
}

func newGmailEnv(t *testing.T) *gmailEnv {
	t.Helper()
	ctx := t.Context()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "mail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := New(db, nil, blob.New(filepath.Join(dir, "blobs")), slog.New(slog.DiscardHandler), Config{Chunk: 2}, func([]api.EventEnvelope) {})
	e := &gmailEnv{t: t, db: db, g: newGmailModel(), mbs: map[string]int64{}}
	server := store.ServerConfig{Host: "imap.gmail.com", Port: 993, TLS: api.TLSModeTLS, Username: "ann@gmail.com"}
	var acct store.Account
	var mbs []store.Mailbox
	err = db.Tx(ctx, func(tx *store.Tx) error {
		var err error
		acct, err = tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindGmail, Email: "ann@gmail.com", Auth: api.AuthKindOAuth2, IMAP: server, SMTP: server})
		if err != nil {
			return err
		}
		list := []store.ServerMailbox{
			{Path: "INBOX", Delimiter: "/", Role: api.MailboxRoleInbox, Selectable: true},
			{Path: gAll, Delimiter: "/", Role: api.MailboxRoleAll, Attrs: []string{`\All`}, Selectable: true},
			{Path: gSpam, Delimiter: "/", Role: api.MailboxRoleJunk, Attrs: []string{`\Junk`}, Selectable: true},
			{Path: gTrash, Delimiter: "/", Role: api.MailboxRoleTrash, Attrs: []string{`\Trash`}, Selectable: true},
			{Path: "[Gmail]/Sent Mail", Delimiter: "/", Role: api.MailboxRoleSent, Attrs: []string{`\Sent`}, Selectable: true},
			{Path: "[Gmail]/Drafts", Delimiter: "/", Role: api.MailboxRoleDrafts, Attrs: []string{`\Drafts`}, Selectable: true},
			{Path: "[Gmail]/Starred", Delimiter: "/", Role: api.MailboxRoleFlagged, Attrs: []string{`\Flagged`}, Selectable: true},
			{Path: "[Gmail]/Important", Delimiter: "/", Role: api.MailboxRoleNone, Attrs: []string{`\Important`}, Selectable: true},
			{Path: "Work", Delimiter: "/", Role: api.MailboxRoleNone, Selectable: true},
		}
		if mbs, err = tx.ReplaceMailboxes(ctx, acct.ID, list); err != nil {
			return err
		}
		return tx.MarkGmailLabels(ctx, acct.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, mb := range mbs {
		e.mbs[mb.Path] = mb.ID
	}
	e.a = &actor{m: m, acct: acct, gmail: true, unsaved: map[int64]time.Time{}}
	e.drain()
	return e
}

// pass runs a Gmail pass and returns the commands it sent.
func (e *gmailEnv) pass() []string {
	e.t.Helper()
	e.g.log = nil
	if err := e.a.gmailPass(e.t.Context(), e.g); err != nil {
		e.t.Fatal(err)
	}
	e.invariants()
	return e.g.log
}

// invariants checks what every step must leave: labels without UIDs, and
// synced folders with UIDs unless a move there is pending.
func (e *gmailEnv) invariants() {
	e.t.Helper()
	if n := e.count(`SELECT COUNT(*) FROM message_mailbox mm JOIN mailboxes mb ON mb.id = mm.mailbox_id
		WHERE mb.is_gmail_label = 1 AND mm.uid IS NOT NULL`); n != 0 {
		e.t.Errorf("%d label memberships have a UID", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM message_mailbox mm JOIN mailboxes mb ON mb.id = mm.mailbox_id
		WHERE mb.is_gmail_label = 0 AND mm.uid IS NULL AND mm.pending = 0`); n != 0 {
		e.t.Errorf("%d synced memberships have no UID and no pending move", n)
	}
}

// subjects lists the subjects stored in a mailbox, sorted.
func (e *gmailEnv) subjects(path string) []string {
	e.t.Helper()
	out := []string{}
	e.query(func(tx *store.Tx) error {
		rows, err := tx.QueryContext(e.t.Context(), `SELECT m.subject FROM message_mailbox mm
			JOIN messages m ON m.id = mm.message_id WHERE mm.mailbox_id = ? ORDER BY m.subject`, e.mbs[path])
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				return err
			}
			out = append(out, s)
		}
		return rows.Err()
	})
	return out
}

// id returns the stored message with subject.
func (e *gmailEnv) id(subject string) int64 {
	e.t.Helper()
	var id int64
	e.query(func(tx *store.Tx) error {
		return tx.QueryRowContext(e.t.Context(), `SELECT id FROM messages WHERE subject = ?`, subject).Scan(&id)
	})
	return id
}

func (e *gmailEnv) count(query string, args ...any) int {
	e.t.Helper()
	var n int
	e.query(func(tx *store.Tx) error { return tx.QueryRowContext(e.t.Context(), query, args...).Scan(&n) })
	return n
}

func (e *gmailEnv) query(f func(tx *store.Tx) error) {
	e.t.Helper()
	if err := e.db.Tx(e.t.Context(), f); err != nil {
		e.t.Fatal(err)
	}
}

// drain returns the events emitted since the last drain.
func (e *gmailEnv) drain() []api.Event {
	e.t.Helper()
	envs, err := e.db.ChangesSince(e.t.Context(), e.seq)
	if err != nil {
		e.t.Fatal(err)
	}
	var out []api.Event
	for _, env := range envs {
		e.seq = max(e.seq, env.Seq)
		ev, err := api.DecodeEvent(env.Event, env.Data)
		if err != nil {
			e.t.Fatal(err)
		}
		out = append(out, ev)
	}
	return out
}

// changedMailboxes lists the mailbox paths of mailbox.changed events.
func (e *gmailEnv) changedMailboxes(evs []api.Event) []string {
	var out []string
	for _, ev := range evs {
		if c, ok := ev.(api.MailboxChanged); ok {
			for path, id := range e.mbs {
				if id == c.ID && !slices.Contains(out, path) {
					out = append(out, path)
				}
			}
		}
	}
	slices.Sort(out)
	return out
}

func eq(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %q, want %q", what, got, want)
	}
}

func TestGmailInitialPass(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	hello := g.add(gAll, "Hello", 0, `\Inbox`, `\Important`)
	g.add(gAll, "Re: Hello", hello.thread, `\Inbox`, "Work")
	g.add(gAll, "Report", 0, `\Sent`, "Work", "Unknown label")
	g.add(gAll, "Archived", 0)
	g.add(gSpam, "Win big", 0)
	g.add(gTrash, "Old", 0)

	e.pass()
	eq(t, "INBOX", e.subjects("INBOX"), []string{"Hello", "Re: Hello"})
	eq(t, "Work", e.subjects("Work"), []string{"Re: Hello", "Report"})
	eq(t, "Sent", e.subjects("[Gmail]/Sent Mail"), []string{"Report"})
	eq(t, "Important", e.subjects("[Gmail]/Important"), []string{"Hello"})
	eq(t, "All Mail", e.subjects(gAll), []string{"Archived", "Hello", "Re: Hello", "Report"})
	eq(t, "Spam", e.subjects(gSpam), []string{"Win big"})
	eq(t, "Trash", e.subjects(gTrash), []string{"Old"})
	if n := e.count(`SELECT COUNT(*) FROM messages`); n != 6 {
		t.Errorf("stored %d messages, want each once (6)", n)
	}
	// Labels are memberships without UIDs; synced folders have UIDs.
	if n := e.count(`SELECT COUNT(*) FROM message_mailbox WHERE uid IS NULL AND mailbox_id IN (?, ?, ?)`,
		e.mbs[gAll], e.mbs[gSpam], e.mbs[gTrash]); n != 0 {
		t.Errorf("%d synced memberships without a UID", n)
	}
	if n := e.count(`SELECT COUNT(DISTINCT thread_id) FROM messages WHERE subject IN ('Hello', 'Re: Hello')`); n != 1 {
		t.Errorf("X-GM-THRID threads: %d threads, want 1", n)
	}

	// Nothing changed: each folder is one SELECT.
	eq(t, "commands of an idle pass", e.pass(), []string{"SELECT " + gAll, "SELECT " + gSpam, "SELECT " + gTrash})
}

func TestGmailLabelChangesArrive(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	a := g.add(gAll, "A", 0, `\Inbox`)
	b := g.add(gAll, "B", 0, `\Inbox`)
	e.pass()
	e.drain()
	idA := e.id("A")

	// In Gmail's web UI: A is archived and labeled Work, B is starred.
	g.label(a, []string{"Work"}, []string{`\Inbox`})
	b.flags = append(b.flags, `\Flagged`)
	g.label(b, []string{`\Starred`}, nil)
	log := e.pass()
	if !slices.ContainsFunc(log, func(c string) bool { return strings.HasPrefix(c, "FETCH changes") }) || slices.ContainsFunc(log, func(c string) bool { return strings.HasPrefix(c, "FETCH headers") }) {
		t.Errorf("label changes should be fetched as changes only: %q", log)
	}
	eq(t, "INBOX", e.subjects("INBOX"), []string{"B"})
	eq(t, "Work", e.subjects("Work"), []string{"A"})
	eq(t, "Starred", e.subjects("[Gmail]/Starred"), []string{"B"})
	if e.id("A") != idA {
		t.Error("a label change replaced the message row")
	}
	eq(t, "mailboxes announced", e.changedMailboxes(e.drain()), []string{"INBOX", "Work", gAll, "[Gmail]/Starred"})
}

func TestGmailTrashKeepsTheMessage(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	m := g.add(gAll, "Doomed", 0, `\Inbox`, "Work")
	g.add(gAll, "Kept", 0, `\Inbox`)
	e.pass()
	id := e.id("Doomed")
	e.drain()

	// Deleted in the web UI: it leaves All Mail and every label for Trash.
	g.place(m, gTrash)
	e.pass()
	eq(t, "INBOX", e.subjects("INBOX"), []string{"Kept"})
	eq(t, "Work", e.subjects("Work"), nil)
	eq(t, "Trash", e.subjects(gTrash), []string{"Doomed"})
	if e.id("Doomed") != id {
		t.Error("moving to Trash replaced the message row; its body cache and selection are lost")
	}
	for _, ev := range e.drain() {
		if r, ok := ev.(api.MessageRemoved); ok {
			t.Errorf("message.removed %v for a message that only moved", r.IDs)
		}
	}

	// Emptied from Trash: gone after the pass, and announced.
	g.msgs = slices.DeleteFunc(g.msgs, func(x *gmMsg) bool { return x == m })
	e.pass()
	if n := e.count(`SELECT COUNT(*) FROM messages WHERE id = ?`, id); n != 0 {
		t.Error("a message no folder holds is still stored")
	}
	removed := false
	for _, ev := range e.drain() {
		if r, ok := ev.(api.MessageRemoved); ok && slices.Contains(r.IDs, id) {
			removed = true
		}
	}
	if !removed {
		t.Error("no message.removed for the deleted message")
	}
}

func TestGmailRestoreFromSpam(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	m := g.add(gSpam, "Not spam", 0)
	e.pass()
	id := e.id("Not spam")
	g.place(m, gAll)
	g.label(m, []string{`\Inbox`}, nil)
	e.pass()
	eq(t, "INBOX", e.subjects("INBOX"), []string{"Not spam"})
	eq(t, "Spam", e.subjects(gSpam), nil)
	if e.id("Not spam") != id {
		t.Error("restoring from Spam replaced the message row")
	}
}

func TestGmailUIDValidityChange(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	g.add(gAll, "One", 0, `\Inbox`)
	g.add(gAll, "Two", 0, "Work")
	e.pass()
	one, two := e.id("One"), e.id("Two")

	f := g.folders[gAll]
	f.validity, f.next = 8, 1
	for _, m := range g.in(gAll) {
		g.place(m, gAll)
	}
	e.pass()
	if e.id("One") != one || e.id("Two") != two {
		t.Error("a new UIDVALIDITY should re-attach messages by X-GM-MSGID")
	}
	eq(t, "INBOX", e.subjects("INBOX"), []string{"One"})
	eq(t, "Work", e.subjects("Work"), []string{"Two"})
}

// TestGmailModelRefusesUnmodeled keeps the model honest: a command it
// does not model fails instead of passing silently.
func TestGmailModelRefusesUnmodeled(t *testing.T) {
	g := newGmailModel()
	if _, err := g.Select(t.Context(), "Nonexistent"); err == nil {
		t.Error("selecting a folder Gmail does not have should fail")
	}
	g.add(gAll, "x", 0, "Work")
	for _, from := range []string{gAll, "Work"} {
		if _, err := g.Select(t.Context(), from); err != nil {
			t.Fatal(err)
		}
		uids, _ := g.UIDs(t.Context())
		if _, err := g.Move(t.Context(), uids, "INBOX"); err == nil {
			t.Errorf("moving from %s to a label folder should fail in the model", from)
		}
	}
}

// replay sends the queued actions to the model.
func (e *gmailEnv) replay() []string {
	e.t.Helper()
	e.g.log = nil
	if err := e.a.replay(e.t.Context(), e.g); err != nil {
		e.t.Fatal(err)
	}
	e.invariants()
	return e.g.log
}

func (e *gmailEnv) move(ids []int64, from, to string) {
	e.t.Helper()
	if err := e.a.m.Move(e.t.Context(), ids, e.mbs[from], e.mbs[to]); err != nil {
		e.t.Fatal(err)
	}
}

func (g *gmailModel) find(subject string) *gmMsg {
	for _, m := range g.msgs {
		if m.subject == subject {
			return m
		}
	}
	return nil
}

func sorted(l []string) []string {
	out := slices.Clone(l)
	slices.Sort(out)
	return out
}

func TestGmailArchiveAndMoveBetweenLabels(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	g.add(gAll, "A", 0, `\Inbox`, "Work")
	g.add(gAll, "B", 0, `\Inbox`)
	e.pass()
	a, b := e.id("A"), e.id("B")
	e.drain()

	// Archive A from INBOX: it keeps Work. Move B from INBOX to Work.
	e.move([]int64{a}, "INBOX", gAll)
	e.move([]int64{b}, "INBOX", "Work")
	eq(t, "INBOX at once", e.subjects("INBOX"), nil)
	eq(t, "Work at once", e.subjects("Work"), []string{"A", "B"})
	eq(t, "All Mail at once", e.subjects(gAll), []string{"A", "B"})
	eq(t, "mailboxes announced", e.changedMailboxes(e.drain()), []string{"INBOX", "Work"})

	// A pass before the replay leaves the local change alone.
	e.pass()
	eq(t, "INBOX before the replay", e.subjects("INBOX"), nil)

	eq(t, "replay", e.replay(), []string{
		"SELECT " + gAll, `STORE [1] +labels [] -labels [\Inbox]`,
		"SELECT " + gAll, `STORE [2] +labels [Work] -labels [\Inbox]`,
	})
	eq(t, "A's labels", sorted(g.find("A").labels), []string{"Work"})
	eq(t, "B's labels", sorted(g.find("B").labels), []string{"Work"})
	e.pass()
	eq(t, "INBOX after", e.subjects("INBOX"), nil)
	eq(t, "Work after", e.subjects("Work"), []string{"A", "B"})

	// Without a source, a message leaves INBOX if it is there: A is not,
	// so moving it to INBOX only adds the label; C is, so moving it to Work
	// takes it out of INBOX.
	g.add(gAll, "C", 0, `\Inbox`)
	e.pass()
	c := e.id("C")
	if err := e.a.m.Move(t.Context(), []int64{a}, 0, e.mbs["INBOX"]); err != nil {
		t.Fatal(err)
	}
	if err := e.a.m.Move(t.Context(), []int64{c}, 0, e.mbs["Work"]); err != nil {
		t.Fatal(err)
	}
	e.replay()
	eq(t, "A's labels after a move without a source", sorted(g.find("A").labels), []string{"Work", `\Inbox`})
	eq(t, "C's labels after a move without a source", sorted(g.find("C").labels), []string{"Work"})
}

func TestGmailDeleteMovesToTrashThenExpunges(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	g.add(gAll, "Doomed", 0, `\Inbox`, "Work")
	e.pass()
	id := e.id("Doomed")

	if err := e.a.m.Delete(t.Context(), []int64{id}); err != nil {
		t.Fatal(err)
	}
	eq(t, "INBOX", e.subjects("INBOX"), nil)
	eq(t, "Work", e.subjects("Work"), nil)
	eq(t, "Trash", e.subjects(gTrash), []string{"Doomed"})
	eq(t, "replay", e.replay(), []string{"SELECT " + gAll, "MOVE [1] " + gTrash})
	if m := g.find("Doomed"); m.folder != gTrash {
		t.Fatalf("on the server the message is in %s", m.folder)
	}
	e.pass()
	if e.id("Doomed") != id {
		t.Error("the trashed message was stored again")
	}
	for _, ev := range e.drain() {
		if r, ok := ev.(api.MessageRemoved); ok {
			t.Errorf("message.removed %v for a message moved to Trash", r.IDs)
		}
	}

	// Deleting in Trash deletes it from the server.
	if err := e.a.m.Delete(t.Context(), []int64{id}); err != nil {
		t.Fatal(err)
	}
	eq(t, "replay in Trash", e.replay(), []string{"SELECT " + gTrash, `STORE [1] +flags [\Deleted] -flags []`, "EXPUNGE [1]"})
	if g.find("Doomed") != nil {
		t.Error("the message is still on the server")
	}
	e.pass()
	if n := e.count(`SELECT COUNT(*) FROM messages`); n != 0 {
		t.Errorf("%d messages stored after the delete", n)
	}
}

func TestGmailJunkAndBack(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	g.add(gAll, "Offer", 0, `\Inbox`)
	e.pass()
	id := e.id("Offer")

	e.move([]int64{id}, "INBOX", gSpam)
	eq(t, "INBOX", e.subjects("INBOX"), nil)
	eq(t, "Spam", e.subjects(gSpam), []string{"Offer"})
	eq(t, "replay to Spam", e.replay(), []string{"SELECT " + gAll, "MOVE [1] " + gSpam})
	e.pass()

	// Not junk: back to INBOX. Gmail returns it to All Mail with \Inbox.
	e.move([]int64{id}, gSpam, "INBOX")
	eq(t, "INBOX at once", e.subjects("INBOX"), []string{"Offer"})
	eq(t, "Spam at once", e.subjects(gSpam), nil)
	eq(t, "replay to INBOX", e.replay(), []string{"SELECT " + gSpam, "MOVE [1] INBOX"})
	if n := e.count(`SELECT COUNT(*) FROM message_mailbox WHERE pending = 1`); n != 0 {
		t.Errorf("%d memberships still pending after the replay", n)
	}
	e.pass()
	eq(t, "INBOX after", e.subjects("INBOX"), []string{"Offer"})
	eq(t, "All Mail after", e.subjects(gAll), []string{"Offer"})
	if e.id("Offer") != id {
		t.Error("the message came back as a new row")
	}
}

func TestGmailStarFollowsFlag(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	g.add(gAll, "Star me", 0, `\Inbox`)
	e.pass()
	id := e.id("Star me")
	e.drain()

	if err := e.a.m.SetFlags(t.Context(), []int64{id}, store.FlagChange{Flagged: new(true)}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Starred at once", e.subjects("[Gmail]/Starred"), []string{"Star me"})
	if !slices.Contains(e.changedMailboxes(e.drain()), "[Gmail]/Starred") {
		t.Error("Starred's counts changed without mailbox.changed")
	}
	e.replay()
	if m := g.find("Star me"); !slices.Contains(m.labels, `\Starred`) {
		t.Errorf("server labels = %q", m.labels)
	}
	e.pass()
	eq(t, "Starred after", e.subjects("[Gmail]/Starred"), []string{"Star me"})

	if err := e.a.m.SetFlags(t.Context(), []int64{id}, store.FlagChange{Flagged: new(false)}); err != nil {
		t.Fatal(err)
	}
	eq(t, "Starred after unflagging", e.subjects("[Gmail]/Starred"), nil)
}

func TestGmailRefusedLabelEditIsUndone(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	g.add(gAll, "Stuck", 0, `\Inbox`)
	e.pass()
	id := e.id("Stuck")

	e.move([]int64{id}, "INBOX", "Work")
	g.refuse = map[string]bool{"STORE labels": true}
	e.replay()
	e.pass()
	eq(t, "INBOX after the refusal", e.subjects("INBOX"), []string{"Stuck"})
	eq(t, "Work after the refusal", e.subjects("Work"), nil)
}

func TestGmailRefusedTrashIsUndone(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	g.add(gAll, "Keep", 0, `\Inbox`, "Work")
	e.pass()
	id := e.id("Keep")

	if err := e.a.m.Delete(t.Context(), []int64{id}); err != nil {
		t.Fatal(err)
	}
	g.refuse = map[string]bool{"MOVE": true}
	e.replay()
	e.pass()
	eq(t, "Trash after the refusal", e.subjects(gTrash), nil)
	eq(t, "INBOX after the refusal", e.subjects("INBOX"), []string{"Keep"})
	eq(t, "Work after the refusal", e.subjects("Work"), []string{"Keep"})
}

func TestGmailDraftCopyIsDeletedThroughTrash(t *testing.T) {
	e := newGmailEnv(t)
	g := e.g
	old := g.add(gAll, "Draft v1", 0, `\Draft`)
	g.add(gAll, "Draft v2", old.thread, `\Draft`)
	e.pass()
	eq(t, "Drafts", e.subjects("[Gmail]/Drafts"), []string{"Draft v1", "Draft v2"})

	// The saved copy is known by its UID in the Drafts folder.
	err := e.db.Tx(t.Context(), func(tx *store.Tx) error {
		return QueueRemoveDraftCopy(t.Context(), tx, e.a.acct.ID, g.uidIn(old, "[Gmail]/Drafts"))
	})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "replay", e.replay(), []string{
		"SELECT [Gmail]/Drafts", "MOVE [5001] " + gTrash,
		"SELECT " + gTrash, `STORE [1] +flags [\Deleted] -flags []`, "EXPUNGE [1]",
	})
	if g.find("Draft v1") != nil {
		t.Error("the old copy is still on the server")
	}
	e.pass()
	eq(t, "Drafts after", e.subjects("[Gmail]/Drafts"), []string{"Draft v2"})
	eq(t, "All Mail after", e.subjects(gAll), []string{"Draft v2"})
}
