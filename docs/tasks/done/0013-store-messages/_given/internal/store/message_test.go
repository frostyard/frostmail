package store

// CONTRACT TEST for task card T-0013 (docs/tasks). Do not edit.

import (
	"context"
	"database/sql"
	"slices"
	"testing"
	"time"
)

// mailboxFixture inserts an account and two mailboxes and returns their IDs.
func mailboxFixture(t *testing.T, d *DB) (acct, inbox, archive int64) {
	t.Helper()
	ctx := t.Context()
	res, err := d.db.ExecContext(ctx, `INSERT INTO accounts (kind, email, auth, imap_host, imap_port, imap_tls,
		imap_username, smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
		VALUES ('imap', 'a@mailtest.test', 'password', 'h', 993, 'tls', 'u', 'h', 587, 'starttls', 'u', 'now')`)
	if err != nil {
		t.Fatal(err)
	}
	acct, _ = res.LastInsertId()
	for _, p := range []string{"INBOX", "Archive"} {
		res, err := d.db.ExecContext(ctx, `INSERT INTO mailboxes (account_id, path, name) VALUES (?, ?, ?)`, acct, p, p)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		if p == "INBOX" {
			inbox = id
		} else {
			archive = id
		}
	}
	return acct, inbox, archive
}

func header(uid uint32, subject string) MessageHeader {
	return MessageHeader{
		UID:          uid,
		ModSeq:       uint64(uid) * 10,
		Flags:        Flags{Seen: true, Flagged: true, Color: 3, Keywords: []string{"Work"}},
		InternalDate: time.Date(2026, 10, 7, 9, 0, int(uid), 0, time.UTC),
		Size:         int64(1000 + uid),
		MessageID:    "m" + subject + "@mailtest.test",
		InReplyTo:    "parent@mailtest.test",
		References:   []string{"root@mailtest.test", "parent@mailtest.test"},
		Subject:      "Re: [dev] " + subject,
		From:         Address{Name: "Bob Builder", Addr: "bob@mailtest.test"},
		To:           []Address{{Name: "", Addr: "a@mailtest.test"}},
		Cc:           []Address{{Name: "Carol", Addr: "carol@mailtest.test"}, {Name: "Dan", Addr: "dan@mailtest.test"}},
		Date:         time.Date(2026, 10, 7, 8, 59, 0, 0, time.UTC),
		ListID:       "dev.mailtest.test",
		Preview:      "Thursday works",
		Parts: []Part{
			{Path: "1", ContentType: "text/plain", Charset: "utf-8", Encoding: "7bit", Size: 120},
			{Path: "2", ContentType: "application/pdf", Encoding: "base64", Disposition: "attachment", Filename: "q3.pdf", Size: 5000},
		},
		HasAttachments: true,
	}
}

func insertHeaders(t *testing.T, d *DB, acct, mailbox int64, hs ...MessageHeader) []int64 {
	t.Helper()
	ctx := t.Context()
	var ids []int64
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		ids, err = tx.InsertHeaders(ctx, acct, mailbox, hs)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestInsertHeadersStoresEverything(t *testing.T) {
	d, events := openTest(t)
	ctx := t.Context()
	acct, inbox, _ := mailboxFixture(t, d)
	ids := insertHeaders(t, d, acct, inbox, header(7, "Lunch"))
	if len(ids) != 1 || ids[0] <= 0 {
		t.Fatalf("ids = %v", ids)
	}
	if len(*events) != 0 {
		t.Fatalf("InsertHeaders emitted %+v", *events)
	}
	var (
		msgid, irt, refs, subject, norm, fromName, fromAddr, to, cc, bcc, replyTo string
		dateHdr                                                                   sql.NullString
		internal, preview, listID, bodyState, keywords                            string
		size, seen, flagged, answered, draft, deleted, color, hasAtt              int64
		threadID                                                                  sql.NullInt64
	)
	err := d.db.QueryRowContext(ctx, `SELECT msgid_hdr, in_reply_to, refs_json, subject, subject_norm, from_name, from_addr,
		to_json, cc_json, bcc_json, reply_to_json, date_hdr, internal_date, size, preview, has_attachments, list_id,
		body_state, seen, flagged, answered, draft, deleted, flag_color, keywords_json, thread_id
		FROM messages WHERE id = ? AND account_id = ?`, ids[0], acct).Scan(
		&msgid, &irt, &refs, &subject, &norm, &fromName, &fromAddr, &to, &cc, &bcc, &replyTo, &dateHdr, &internal,
		&size, &preview, &hasAtt, &listID, &bodyState, &seen, &flagged, &answered, &draft, &deleted, &color, &keywords, &threadID)
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string][2]string{
		"msgid_hdr":     {msgid, "mLunch@mailtest.test"},
		"in_reply_to":   {irt, "parent@mailtest.test"},
		"refs_json":     {refs, `["root@mailtest.test","parent@mailtest.test"]`},
		"subject":       {subject, "Re: [dev] Lunch"},
		"subject_norm":  {norm, "Lunch"},
		"from":          {fromName + " <" + fromAddr + ">", "Bob Builder <bob@mailtest.test>"},
		"to_json":       {to, `[{"name":"","addr":"a@mailtest.test"}]`},
		"cc_json":       {cc, `[{"name":"Carol","addr":"carol@mailtest.test"},{"name":"Dan","addr":"dan@mailtest.test"}]`},
		"bcc_json":      {bcc, `[]`},
		"reply_to_json": {replyTo, `[]`},
		"date_hdr":      {dateHdr.String, "2026-10-07T08:59:00.000Z"},
		"internal_date": {internal, "2026-10-07T09:00:07.000Z"},
		"preview":       {preview, "Thursday works"},
		"list_id":       {listID, "dev.mailtest.test"},
		"body_state":    {bodyState, "headers"},
		"keywords_json": {keywords, `["Work"]`},
	} {
		if got[0] != got[1] {
			t.Errorf("%s = %q, want %q", name, got[0], got[1])
		}
	}
	if size != 1007 || hasAtt != 1 || seen != 1 || flagged != 1 || answered != 0 || draft != 0 || deleted != 0 || color != 3 || threadID.Valid {
		t.Errorf("size %d att %d seen %d flagged %d answered %d draft %d deleted %d color %d thread %v",
			size, hasAtt, seen, flagged, answered, draft, deleted, color, threadID)
	}
	var uid, modseq int64
	if err := d.db.QueryRowContext(ctx, `SELECT uid, modseq FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?`, ids[0], inbox).Scan(&uid, &modseq); err != nil || uid != 7 || modseq != 70 {
		t.Fatalf("membership uid %d modseq %d err %v", uid, modseq, err)
	}
	rows, err := d.db.QueryContext(ctx, `SELECT path, content_type, charset, encoding, disposition, filename, size FROM parts WHERE message_id = ? ORDER BY path`, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var parts []Part
	for rows.Next() {
		var p Part
		if err := rows.Scan(&p.Path, &p.ContentType, &p.Charset, &p.Encoding, &p.Disposition, &p.Filename, &p.Size); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, p)
	}
	if !slices.Equal(parts, header(7, "Lunch").Parts) {
		t.Fatalf("parts = %+v", parts)
	}
}

func TestInsertHeadersIsIdempotent(t *testing.T) {
	d, _ := openTest(t)
	acct, inbox, archive := mailboxFixture(t, d)
	first := insertHeaders(t, d, acct, inbox, header(1, "a"), header(2, "b"))
	again := insertHeaders(t, d, acct, inbox, header(2, "b"), header(3, "c"), header(1, "a"))
	if len(first) != 2 || len(again) != 1 {
		t.Fatalf("first %v again %v; the repeat must insert only UID 3", first, again)
	}
	// The same UID in another mailbox is a different message.
	if other := insertHeaders(t, d, acct, archive, header(1, "a")); len(other) != 1 {
		t.Fatalf("UID 1 in Archive = %v", other)
	}
	noDate := header(4, "d")
	noDate.Date = time.Time{}
	noDate.References = nil
	ids := insertHeaders(t, d, acct, inbox, noDate)
	var dateHdr sql.NullString
	var refs string
	if err := d.db.QueryRowContext(t.Context(), `SELECT date_hdr, refs_json FROM messages WHERE id = ?`, ids[0]).Scan(&dateHdr, &refs); err != nil {
		t.Fatal(err)
	}
	if dateHdr.Valid || refs != "[]" {
		t.Fatalf("zero Date stored as %v, nil References as %q", dateHdr, refs)
	}
}

func TestMailboxUIDs(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct, inbox, archive := mailboxFixture(t, d)
	if uids, err := d.MailboxUIDs(ctx, inbox); err != nil || uids == nil || len(uids) != 0 {
		t.Fatalf("empty mailbox UIDs = %#v, %v", uids, err)
	}
	insertHeaders(t, d, acct, inbox, header(30, "c"), header(4, "a"), header(1000, "d"), header(17, "b"))
	insertHeaders(t, d, acct, archive, header(5, "x"))
	uids, err := d.MailboxUIDs(ctx, inbox)
	if err != nil || !slices.Equal(uids, []uint32{4, 17, 30, 1000}) {
		t.Fatalf("UIDs = %v, %v", uids, err)
	}
}

func TestUpdateFlags(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct, inbox, _ := mailboxFixture(t, d)
	ids := insertHeaders(t, d, acct, inbox, header(1, "a"), header(2, "b"))
	var changed []int64
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		changed, err = tx.UpdateFlags(ctx, inbox, []FlagUpdate{
			{UID: 2, ModSeq: 99, Flags: Flags{Answered: true, Deleted: true}},
			{UID: 1, ModSeq: 98, Flags: header(1, "a").Flags}, // unchanged flags, new modseq
			{UID: 55, ModSeq: 97, Flags: Flags{Seen: true}},   // not in the mailbox
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(changed, []int64{ids[1]}) {
		t.Fatalf("changed = %v, want [%d]", changed, ids[1])
	}
	var seen, answered, deleted, color int64
	var keywords string
	if err := d.db.QueryRowContext(ctx, `SELECT seen, answered, deleted, flag_color, keywords_json FROM messages WHERE id = ?`, ids[1]).Scan(&seen, &answered, &deleted, &color, &keywords); err != nil {
		t.Fatal(err)
	}
	if seen != 0 || answered != 1 || deleted != 1 || color != 0 || keywords != "[]" {
		t.Fatalf("flags seen %d answered %d deleted %d color %d keywords %s", seen, answered, deleted, color, keywords)
	}
	var modseq int64
	if err := d.db.QueryRowContext(ctx, `SELECT modseq FROM message_mailbox WHERE mailbox_id = ? AND uid = 1`, inbox).Scan(&modseq); err != nil || modseq != 98 {
		t.Fatalf("modseq of an unchanged message = %d, %v; want 98", modseq, err)
	}
}

func TestRemoveUIDs(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	acct, inbox, archive := mailboxFixture(t, d)
	ids := insertHeaders(t, d, acct, inbox, header(1, "a"), header(2, "b"), header(3, "c"))
	// Message 2 is also in Archive (as a label would be): only its INBOX
	// membership goes.
	if _, err := d.db.ExecContext(ctx, `INSERT INTO message_mailbox (message_id, mailbox_id, uid) VALUES (?, ?, 40)`, ids[1], archive); err != nil {
		t.Fatal(err)
	}
	if _, err := d.db.ExecContext(ctx, `INSERT INTO messages_fts (rowid, subject) VALUES (?, 'findme')`, ids[0]); err != nil {
		t.Fatal(err)
	}
	var removed []int64
	err := d.Tx(ctx, func(tx *Tx) error {
		var err error
		removed, err = tx.RemoveUIDs(ctx, inbox, []uint32{2, 1, 77})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(removed, []int64{ids[0]}) {
		t.Fatalf("removed = %v, want [%d]", removed, ids[0])
	}
	count := func(q string, args ...any) int {
		var n int
		if err := d.db.QueryRowContext(context.Background(), q, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM messages WHERE id = ?`, ids[0]); n != 0 {
		t.Fatal("message 1 survived")
	}
	if n := count(`SELECT COUNT(*) FROM parts WHERE message_id = ?`, ids[0]); n != 0 {
		t.Fatal("parts of message 1 survived")
	}
	if n := count(`SELECT COUNT(*) FROM messages_fts WHERE messages_fts MATCH 'findme'`); n != 0 {
		t.Fatal("search entry of message 1 survived")
	}
	if n := count(`SELECT COUNT(*) FROM messages WHERE id = ?`, ids[1]); n != 1 {
		t.Fatal("message 2, still in Archive, was deleted")
	}
	if n := count(`SELECT COUNT(*) FROM message_mailbox WHERE message_id = ? AND mailbox_id = ?`, ids[1], inbox); n != 0 {
		t.Fatal("message 2 is still in INBOX")
	}
	if uids, _ := d.MailboxUIDs(ctx, inbox); !slices.Equal(uids, []uint32{3}) {
		t.Fatalf("INBOX UIDs = %v, want [3]", uids)
	}
}
