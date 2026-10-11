package store

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
)

// conditionsFixture stores four messages around testNow (Wednesday
// 2026-10-07, 12:00 UTC) for the condition compiler:
//
//	lunch   from Ann <ann@x.test> (a VIP) to me, Cc Bob; today; unread;
//	        flagged red; q3.pdf; list dev.x.test; INBOX
//	invoice from Shop <billing@shop.test> to other@x.test, Cc me;
//	        yesterday; a pending reminder; Archive
//	old     from Carol <carol@y.test> (in People) to me; 2025-09-01 with
//	        no Date header; flagged purple; Trash
//	report  from me to ann@x.test; 2026-09-28; Sent
type conditionsFixture struct {
	d                           *DB
	acct                        int64
	inbox, archive, trash, sent int64
	lunch, invoice, old, report int64
}

func newConditionsFixture(t *testing.T) conditionsFixture {
	t.Helper()
	d, _ := openTest(t)
	ctx := t.Context()
	f := conditionsFixture{d: d}
	f.acct = insertAccount(t, d, sampleAccount("me@mailtest.test")).ID
	me := Address{Name: "Me", Addr: "me@mailtest.test"}
	ann := Address{Name: "Ann", Addr: "ann@x.test"}
	at := func(y int, m time.Month, day, h int) time.Time { return time.Date(y, m, day, h, 0, 0, 0, time.UTC) }
	err := d.Tx(ctx, func(tx *Tx) error {
		mbs, err := tx.ReplaceMailboxes(ctx, f.acct, []ServerMailbox{
			{Path: "INBOX", Role: api.MailboxRoleInbox, Selectable: true},
			{Path: "Archive", Role: api.MailboxRoleArchive, Selectable: true},
			{Path: "Trash", Role: api.MailboxRoleTrash, Selectable: true},
			{Path: "Sent", Role: api.MailboxRoleSent, Selectable: true},
		})
		if err != nil {
			return err
		}
		for _, mb := range mbs {
			switch mb.Role {
			case api.MailboxRoleInbox:
				f.inbox = mb.ID
			case api.MailboxRoleArchive:
				f.archive = mb.ID
			case api.MailboxRoleTrash:
				f.trash = mb.ID
			case api.MailboxRoleSent:
				f.sent = mb.ID
			}
		}
		for _, m := range []struct {
			mailbox int64
			h       MessageHeader
			id      *int64
		}{
			{f.inbox, MessageHeader{UID: 1, InternalDate: at(2026, 10, 7, 9), Date: at(2026, 10, 7, 9), MessageID: "lunch@x",
				Subject: "Lunch plans", From: ann, To: []Address{me}, Cc: []Address{{Name: "Bob", Addr: "bob@x.test"}},
				ListID: "dev.x.test", Preview: "Thursday works", HasAttachments: true,
				Parts: []Part{{Path: "2", ContentType: "application/pdf", Disposition: "attachment", Filename: "q3.pdf"}},
				Flags: Flags{Flagged: true, Color: 1}}, &f.lunch},
			{f.archive, MessageHeader{UID: 1, InternalDate: at(2026, 10, 6, 15), Date: at(2026, 10, 6, 15), MessageID: "invoice@x",
				Subject: "Invoice 42", From: Address{Name: "Shop", Addr: "billing@shop.test"},
				To: []Address{{Addr: "other@x.test"}}, Cc: []Address{me}, Preview: "Your order", Flags: Flags{Seen: true}}, &f.invoice},
			{f.trash, MessageHeader{UID: 1, InternalDate: at(2025, 9, 1, 8), MessageID: "old@x",
				Subject: "Old news", From: Address{Name: "Carol", Addr: "carol@y.test"}, To: []Address{me},
				Flags: Flags{Seen: true, Flagged: true, Color: 6}}, &f.old},
			{f.sent, MessageHeader{UID: 1, InternalDate: at(2026, 9, 28, 10), Date: at(2026, 9, 28, 10), MessageID: "report@x",
				Subject: "Report draft", From: me, To: []Address{ann}, Flags: Flags{Seen: true}}, &f.report},
		} {
			ids, err := tx.InsertHeaders(ctx, f.acct, m.mailbox, []MessageHeader{m.h})
			if err != nil {
				return err
			}
			*m.id = ids[0]
			if err := tx.IndexMessage(ctx, ids[0], SearchDocFor(m.h)); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO vips (address, added_at) VALUES ('ann@x.test', 'now')`); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO message_reminders (message_id, remind_at) VALUES (?, '2026-10-08T09:00:00.000Z')`, f.invoice)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Tx(ctx, func(tx *Tx) error {
		return tx.SetService(ctx, f.acct, api.ServiceKindContacts, true, "https://dav.test/")
	}); err != nil {
		t.Fatal(err)
	}
	col := replaceCols(t, d, f.acct, api.CollectionKindAddressbook, RemoteCollection{Href: "/ab/", Name: "Book"})[0].ID
	obj := putObject(t, d, Object{CollectionID: col, Href: "/ab/carol.vcf", Kind: ObjectVCard, Raw: []byte("C")})
	if err := d.Tx(ctx, func(tx *Tx) error {
		if err := tx.IndexContact(ctx, obj, ContactIndex{DisplayName: "Carol", Emails: []ContactEmail{{Email: "carol@y.test"}}}); err != nil {
			return err
		}
		return tx.RelinkPeople(ctx)
	}); err != nil {
		t.Fatal(err)
	}
	return f
}

// matching returns the IDs of the messages c matches, ascending.
func (f conditionsFixture) matching(t *testing.T, c api.Conditions, loc *time.Location) []int64 {
	t.Helper()
	p, err := CompileConditions(c, testNow, loc)
	if err != nil {
		t.Fatalf("CompileConditions(%+v): %v", c, err)
	}
	rows, err := f.d.db.QueryContext(t.Context(), `SELECT m.id FROM messages m WHERE `+p.SQL+` ORDER BY m.id`, p.Args...)
	if err != nil {
		t.Fatalf("%s %v: %v", p.SQL, p.Args, err)
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func one(field api.ConditionField, op api.ConditionOp, value string) api.Conditions {
	return api.Conditions{Match: api.ConditionMatchAll, Conditions: []api.Condition{{Field: field, Op: op, Value: value}}}
}

func TestCompileEveryCondition(t *testing.T) {
	f := newConditionsFixture(t)
	acct := strconv.FormatInt(f.acct, 10)
	id := func(n int64) string { return strconv.FormatInt(n, 10) }
	all := []int64{f.lunch, f.invoice, f.old, f.report}
	type F = api.ConditionField
	type O = api.ConditionOp
	cases := []struct {
		field F
		op    O
		value string
		want  []int64
	}{
		{"from", "contains", "ann", []int64{f.lunch}},
		{"from", "notcontains", "ann", []int64{f.invoice, f.old, f.report}},
		{"from", "is", "ANN@X.TEST", []int64{f.lunch}},
		{"from", "is", "shop", []int64{f.invoice}},
		{"from", "begins", "bill", []int64{f.invoice}},
		{"from", "ends", "@shop.test", []int64{f.invoice}},
		{"to", "contains", "other", []int64{f.invoice}},
		{"to", "is", "me@mailtest.test", []int64{f.lunch, f.old}},
		{"cc", "is", "me@mailtest.test", []int64{f.invoice}},
		{"cc", "begins", "bob", []int64{f.lunch}},
		{"recipient", "contains", "ann", []int64{f.report}},
		{"recipient", "notcontains", "me", []int64{f.report}},
		{"tome", "is", "true", []int64{f.lunch, f.old}},
		{"tome", "is", "false", []int64{f.invoice, f.report}},
		{"ccme", "is", "true", []int64{f.invoice}},
		{"subject", "contains", "lunch", []int64{f.lunch}},
		{"subject", "contains", "plans lunch", []int64{f.lunch}},
		{"subject", "contains", `"lunch plans"`, []int64{f.lunch}},
		{"subject", "contains", `"plans lunch"`, []int64{}},
		{"subject", "begins", "inv", []int64{f.invoice}},
		{"subject", "ends", "NEWS", []int64{f.old}},
		{"subject", "is", "report draft", []int64{f.report}},
		{"subject", "is", "report", []int64{}},
		{"content", "contains", "thursday", []int64{f.lunch}},
		{"content", "contains", "q3", []int64{f.lunch}},
		{"filename", "contains", "q3", []int64{f.lunch}},
		{"filename", "notcontains", "q3", []int64{f.invoice, f.old, f.report}},
		{"listid", "contains", "dev", []int64{f.lunch}},
		{"listid", "is", "dev.x.test", []int64{f.lunch}},
		{"account", "is", acct, all},
		{"account", "isnot", acct, []int64{}},
		{"account", "anyof", acct + ",999", all},
		{"mailbox", "is", id(f.inbox), []int64{f.lunch}},
		{"mailbox", "isnot", id(f.inbox), []int64{f.invoice, f.old, f.report}},
		{"mailbox", "anyof", id(f.inbox) + "," + id(f.archive), []int64{f.lunch, f.invoice}},
		{"mailbox", "is", "999", []int64{}},
		{"role", "is", "trash", []int64{f.old}},
		{"role", "anyof", "sent, trash", []int64{f.old, f.report}},
		{"role", "isnot", "trash", []int64{f.lunch, f.invoice, f.report}},
		{"received", "today", "", []int64{f.lunch}},
		{"received", "yesterday", "", []int64{f.invoice}},
		{"received", "thisweek", "", []int64{f.lunch, f.invoice}},
		{"received", "thismonth", "", []int64{f.lunch, f.invoice}},
		{"received", "thisyear", "", []int64{f.lunch, f.invoice, f.report}},
		{"received", "within", "2d", []int64{f.lunch, f.invoice}},
		{"received", "within", "10d", []int64{f.lunch, f.invoice, f.report}},
		{"received", "notwithin", "10d", []int64{f.old}},
		{"received", "on", "2026-10-06", []int64{f.invoice}},
		{"received", "since", "2026-10-06", []int64{f.lunch, f.invoice}},
		{"received", "before", "2026-10-06", []int64{f.old, f.report}},
		{"sent", "today", "", []int64{f.lunch}},
		{"sent", "thisyear", "", []int64{f.lunch, f.invoice, f.report}},
		{"sent", "notwithin", "1d", []int64{f.report}},
		{"unread", "is", "true", []int64{f.lunch}},
		{"unread", "is", "false", []int64{f.invoice, f.old, f.report}},
		{"flagged", "is", "true", []int64{f.lunch, f.old}},
		{"attachments", "is", "true", []int64{f.lunch}},
		{"color", "is", "1", []int64{f.lunch}},
		{"color", "is", "6", []int64{f.old}},
		{"color", "anyof", "1,6", []int64{f.lunch, f.old}},
		{"color", "isnot", "1", []int64{f.invoice, f.old, f.report}},
		{"vip", "is", "true", []int64{f.lunch}},
		{"vip", "is", "false", []int64{f.invoice, f.old, f.report}},
		{"contact", "is", "true", []int64{f.old}},
		{"reminder", "is", "true", []int64{f.invoice}},
	}
	for _, c := range cases {
		got := f.matching(t, one(c.field, c.op, c.value), time.UTC)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s %s %q = %v, want %v", c.field, c.op, c.value, got, c.want)
		}
	}
}

func TestCompileAnyAllAndEmpty(t *testing.T) {
	f := newConditionsFixture(t)
	two := []api.Condition{
		{Field: api.ConditionFieldFlagged, Op: api.ConditionOpIs, Value: "true"},
		{Field: api.ConditionFieldRole, Op: api.ConditionOpIs, Value: "trash"},
	}
	if got := f.matching(t, api.Conditions{Match: api.ConditionMatchAll, Conditions: two}, time.UTC); !slices.Equal(got, []int64{f.old}) {
		t.Errorf("all = %v, want [old]", got)
	}
	if got := f.matching(t, api.Conditions{Match: api.ConditionMatchAny, Conditions: two}, time.UTC); !slices.Equal(got, []int64{f.lunch, f.old}) {
		t.Errorf("any = %v, want [lunch old]", got)
	}
	for _, m := range []api.ConditionMatch{api.ConditionMatchAll, api.ConditionMatchAny} {
		if got := f.matching(t, api.Conditions{Match: m}, time.UTC); len(got) != 4 {
			t.Errorf("empty %s = %v, want all four", m, got)
		}
	}
}

// TestCompileDaysInLoc: relative days are the zone's. At 02:00 on the 7th
// in Honolulu (12:00 UTC), lunch (09:00 UTC) arrived yesterday.
func TestCompileDaysInLoc(t *testing.T) {
	f := newConditionsFixture(t)
	hnl := time.FixedZone("HST", -10*3600)
	if got := f.matching(t, one("received", "today", ""), hnl); len(got) != 0 {
		t.Errorf("today in HST = %v, want none", got)
	}
	if got := f.matching(t, one("received", "yesterday", ""), hnl); !slices.Equal(got, []int64{f.lunch, f.invoice}) {
		t.Errorf("yesterday in HST = %v, want [lunch invoice]", got)
	}
}

func TestCheckConditionsRefuses(t *testing.T) {
	type F = api.ConditionField
	type O = api.ConditionOp
	bad := []struct {
		field F
		op    O
		value string
	}{
		{"nope", "is", "x"},
		{"from", "nope", "x"},
		{"from", "today", ""},
		{"from", "contains", "!!!"},
		{"from", "is", ""},
		{"to", "notcontains", "x"},
		{"recipient", "is", "x"},
		{"content", "begins", "x"},
		{"listid", "begins", "x"},
		{"unread", "is", "yes"},
		{"unread", "isnot", "true"},
		{"color", "is", "8"},
		{"color", "is", "1,2"},
		{"account", "is", "abc"},
		{"account", "anyof", "1,,2"},
		{"mailbox", "is", "0"},
		{"role", "is", "nowhere"},
		{"role", "is", "none"},
		{"received", "today", "x"},
		{"received", "within", "7x"},
		{"received", "on", "10/06/2026"},
		{"received", "contains", "x"},
	}
	for _, b := range bad {
		if err := CheckConditions(one(b.field, b.op, b.value)); err == nil {
			t.Errorf("CheckConditions(%s %s %q) = nil, want an error", b.field, b.op, b.value)
		}
	}
	if err := CheckConditions(api.Conditions{Match: "", Conditions: []api.Condition{}}); err == nil {
		t.Error("match \"\" was accepted")
	}
	many := api.Conditions{Match: api.ConditionMatchAll}
	for range maxConditions + 1 {
		many.Conditions = append(many.Conditions, api.Condition{Field: "unread", Op: "is", Value: "true"})
	}
	if err := CheckConditions(many); err == nil || !strings.Contains(err.Error(), "more than") {
		t.Errorf("%d conditions: %v", len(many.Conditions), err)
	}
}

// TestViewIDsWithConditions: a view's conditions narrow it like its other
// fields.
func TestViewIDsWithConditions(t *testing.T) {
	f := newConditionsFixture(t)
	c := one("vip", "is", "true")
	got, err := f.d.ViewIDs(t.Context(), ViewFilter{Conditions: &c})
	if err != nil || !slices.Equal(got, []int64{f.lunch}) {
		t.Errorf("ViewIDs(vip) = %v, %v; want [lunch]", got, err)
	}
	c = one("flagged", "is", "true")
	got, err = f.d.ViewIDs(t.Context(), ViewFilter{MailboxID: f.trash, Conditions: &c})
	if err != nil || !slices.Equal(got, []int64{f.old}) {
		t.Errorf("ViewIDs(Trash, flagged) = %v, %v; want [old]", got, err)
	}
}

// TestCountView: a view filter's messages and unread ones, conditions
// included.
func TestCountView(t *testing.T) {
	f := newConditionsFixture(t)
	c := one("flagged", "is", "true")
	total, unread, err := f.d.CountView(t.Context(), ViewFilter{Conditions: &c})
	if err != nil || total != 2 || unread != 1 {
		t.Errorf("CountView(flagged) = %d, %d, %v; want 2, 1", total, unread, err)
	}
	total, unread, err = f.d.CountView(t.Context(), ViewFilter{MailboxID: f.sent})
	if err != nil || total != 1 || unread != 0 {
		t.Errorf("CountView(Sent) = %d, %d, %v; want 1, 0", total, unread, err)
	}
}
