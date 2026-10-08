package store

import (
	"slices"
	"testing"

	"github.com/frostyard/frostmail/api"
)

type gmailFixture struct {
	d                                 *DB
	acct                              int64
	all, spam, trash, inbox, sent, wk int64
	labels                            map[string]int64
}

func newGmail(t *testing.T) *gmailFixture {
	t.Helper()
	d, _ := openTest(t)
	ctx := t.Context()
	res, err := d.db.ExecContext(ctx, `INSERT INTO accounts (kind, email, auth, imap_host, imap_port, imap_tls,
		imap_username, smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
		VALUES ('gmail', 'ann@gmail.com', 'oauth2', 'h', 993, 'tls', 'u', 'h', 465, 'tls', 'u', 'now')`)
	if err != nil {
		t.Fatal(err)
	}
	f := &gmailFixture{d: d}
	f.acct, _ = res.LastInsertId()
	var mbs []Mailbox
	err = d.Tx(ctx, func(tx *Tx) error {
		mbs, err = tx.ReplaceMailboxes(ctx, f.acct, []ServerMailbox{
			{Path: "INBOX", Delimiter: "/", Role: api.MailboxRoleInbox, Selectable: true},
			{Path: "[Gmail]/All Mail", Delimiter: "/", Role: api.MailboxRoleAll, Attrs: []string{`\All`}, Selectable: true},
			{Path: "[Gmail]/Spam", Delimiter: "/", Role: api.MailboxRoleJunk, Attrs: []string{`\Junk`}, Selectable: true},
			{Path: "[Gmail]/Trash", Delimiter: "/", Role: api.MailboxRoleTrash, Attrs: []string{`\Trash`}, Selectable: true},
			{Path: "[Gmail]/Sent Mail", Delimiter: "/", Role: api.MailboxRoleSent, Attrs: []string{`\Sent`}, Selectable: true},
			{Path: "[Gmail]/Important", Delimiter: "/", Role: api.MailboxRoleNone, Attrs: []string{`\Important`}, Selectable: true},
			{Path: "Work", Delimiter: "/", Role: api.MailboxRoleNone, Selectable: true},
		})
		if err != nil {
			return err
		}
		if err := tx.MarkGmailLabels(ctx, f.acct); err != nil {
			return err
		}
		f.labels, err = tx.GmailLabels(ctx, f.acct)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, mb := range mbs {
		switch mb.Path {
		case "INBOX":
			f.inbox = mb.ID
		case "[Gmail]/All Mail":
			f.all = mb.ID
		case "[Gmail]/Spam":
			f.spam = mb.ID
		case "[Gmail]/Trash":
			f.trash = mb.ID
		case "[Gmail]/Sent Mail":
			f.sent = mb.ID
		case "Work":
			f.wk = mb.ID
		}
	}
	return f
}

func (f *gmailFixture) insert(t *testing.T, mailbox int64, hs ...MessageHeader) GmailResult {
	t.Helper()
	var r GmailResult
	err := f.d.Tx(t.Context(), func(tx *Tx) error {
		var err error
		r, err = tx.InsertGmail(t.Context(), f.acct, mailbox, mailbox == f.all, f.labels, hs)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// memberships lists a message's mailboxes and UIDs ("id:uid", uid 0 for a
// label).
func (f *gmailFixture) memberships(t *testing.T, msg int64) map[int64]uint32 {
	t.Helper()
	rows, err := f.d.db.QueryContext(t.Context(), `SELECT mailbox_id, COALESCE(uid, 0) FROM message_mailbox WHERE message_id = ?`, msg)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[int64]uint32{}
	for rows.Next() {
		var mb int64
		var uid uint32
		if err := rows.Scan(&mb, &uid); err != nil {
			t.Fatal(err)
		}
		out[mb] = uid
	}
	return out
}

func gmailHeader(uid uint32, msgid, thrid uint64, labels ...string) MessageHeader {
	h := header(uid, "G")
	h.MessageID = "g" + string(rune('a'+uid)) + "@mail.gmail.com"
	h.GmMsgID, h.GmThrID, h.Labels = msgid, thrid, labels
	return h
}

func TestGmailLabelsMap(t *testing.T) {
	f := newGmail(t)
	want := map[string]int64{
		`\Inbox`: f.inbox, "INBOX": f.inbox, `\Sent`: f.sent, "[Gmail]/Sent Mail": f.sent,
		`\Important`: f.labels["[Gmail]/Important"], "[Gmail]/Important": f.labels["[Gmail]/Important"], "Work": f.wk,
	}
	for k, v := range want {
		if f.labels[k] != v || v == 0 {
			t.Errorf("label %q = %d, want %d", k, f.labels[k], v)
		}
	}
	for _, synced := range []string{"[Gmail]/All Mail", "[Gmail]/Spam", "[Gmail]/Trash"} {
		if _, ok := f.labels[synced]; ok {
			t.Errorf("%s is a label", synced)
		}
	}
}

func TestInsertGmailMovesAndLabels(t *testing.T) {
	f := newGmail(t)
	ctx := t.Context()
	r := f.insert(t, f.all, gmailHeader(10, 111, 900, `\Inbox`, "Work", "Unknown"), gmailHeader(11, 112, 900))
	if len(r.Added) != 2 || len(r.Changed) != 0 {
		t.Fatalf("insert = %+v", r)
	}
	if !slices.Equal(r.Mailboxes, []int64{f.inbox, f.wk}) {
		t.Errorf("touched mailboxes = %v", r.Mailboxes)
	}
	m := r.Added[0]
	if got := f.memberships(t, m); len(got) != 3 || got[f.all] != 10 || got[f.inbox] != 0 || got[f.wk] != 0 {
		t.Errorf("memberships = %v", got)
	}
	// Repeating the pass changes nothing.
	if again := f.insert(t, f.all, gmailHeader(10, 111, 900, `\Inbox`)); len(again.Added)+len(again.Changed) != 0 {
		t.Errorf("repeat = %+v", again)
	}

	// Archive in Gmail's web UI: the label edit arrives with the flags.
	err := f.d.Tx(ctx, func(tx *Tx) error {
		id, mbs, found, err := tx.SetGmailLabels(ctx, f.all, 10, []string{"Work"}, f.labels)
		if err != nil || !found || id != m || !slices.Equal(mbs, []int64{f.inbox}) {
			t.Errorf("set labels = %d %v %v %v", id, mbs, found, err)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Deleted in Gmail: All Mail loses it, Trash gains it, same message row.
	err = f.d.Tx(ctx, func(tx *Tx) error {
		msgs, mbs, err := tx.RemoveGmailUIDs(ctx, f.all, []uint32{10})
		if !slices.Equal(msgs, []int64{m}) || !slices.Equal(mbs, []int64{f.wk}) {
			t.Errorf("remove = %v %v", msgs, mbs)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	r = f.insert(t, f.trash, gmailHeader(5, 111, 900, `\Inbox`))
	if len(r.Added) != 0 || !slices.Equal(r.Changed, []int64{m}) {
		t.Fatalf("trash = %+v", r)
	}
	if got := f.memberships(t, m); len(got) != 1 || got[f.trash] != 5 {
		t.Errorf("after trash = %v (labels must not follow into Trash)", got)
	}
	var pruned []int64
	err = f.d.Tx(ctx, func(tx *Tx) error {
		pruned, err = tx.PruneGmail(ctx, f.acct)
		return err
	})
	if err != nil || len(pruned) != 0 {
		t.Fatalf("prune with nothing orphaned = %v, %v", pruned, err)
	}

	// Expunged from Trash: pruned at the end of the pass.
	err = f.d.Tx(ctx, func(tx *Tx) error {
		if _, _, err := tx.RemoveGmailUIDs(ctx, f.trash, []uint32{5}); err != nil {
			return err
		}
		pruned, err = tx.PruneGmail(ctx, f.acct)
		return err
	})
	if err != nil || !slices.Equal(pruned, []int64{m}) {
		t.Fatalf("prune = %v, %v", pruned, err)
	}
	if _, err := f.d.GetMessage(ctx, m); err == nil {
		t.Error("the pruned message is still stored")
	}
}

func TestAssignGmailThreads(t *testing.T) {
	f := newGmail(t)
	ctx := t.Context()
	r := f.insert(t, f.all, gmailHeader(1, 201, 777), gmailHeader(2, 202, 777), gmailHeader(3, 203, 888))
	var touched []int64
	err := f.d.Tx(ctx, func(tx *Tx) error {
		var err error
		touched, err = tx.AssignGmailThreads(ctx, f.acct, r.Added)
		return err
	})
	if err != nil || len(touched) != 2 {
		t.Fatalf("threads = %v, %v", touched, err)
	}
	var a, b, c int64
	for i, dst := range []*int64{&a, &b, &c} {
		if err := f.d.db.QueryRowContext(ctx, `SELECT thread_id FROM messages WHERE id = ?`, r.Added[i]).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	if a != b || a == c {
		t.Errorf("thread ids = %d %d %d", a, b, c)
	}
	var count int
	if err := f.d.db.QueryRowContext(ctx, `SELECT msg_count FROM threads WHERE id = ?`, a).Scan(&count); err != nil || count != 2 {
		t.Errorf("msg_count = %d, %v", count, err)
	}
}

func TestSetGmailLabelsKeepsQueuedChanges(t *testing.T) {
	f := newGmail(t)
	ctx := t.Context()
	r := f.insert(t, f.all, gmailHeader(10, 111, 900, `\Inbox`))
	m := r.Added[0]
	// Moved to Work locally; the label edit waits in the queue.
	err := f.d.Tx(ctx, func(tx *Tx) error {
		if _, err := tx.setGmailLabels(ctx, m, []string{"Work"}, f.labels); err != nil {
			return err
		}
		_, err := tx.QueueOp(ctx, f.acct, "labels", map[string]any{}, []int64{m})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	// A pass that ran before the replay still reports the old labels.
	err = f.d.Tx(ctx, func(tx *Tx) error {
		id, mbs, found, err := tx.SetGmailLabels(ctx, f.all, 10, []string{`\Inbox`}, f.labels)
		if id != m || !found || len(mbs) != 0 {
			t.Errorf("set labels on a queued message = %d %v %v", id, mbs, found)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got := f.memberships(t, m)
	if _, inWork := got[f.wk]; len(got) != 2 || !inWork || got[f.all] != 10 {
		t.Errorf("memberships = %v, want All Mail and the queued Work", got)
	}
}
