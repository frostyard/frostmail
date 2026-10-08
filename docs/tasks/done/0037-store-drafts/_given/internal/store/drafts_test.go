package store

// CONTRACT TEST for task card T-0037 (docs/tasks). Do not edit.

import (
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"
)

func draftFixture(t *testing.T) (*DB, int64, int64) {
	t.Helper()
	d, _ := openTest(t)
	acct, inbox, _ := mailboxFixture(t, d)
	ids := insertHeaders(t, d, acct, inbox, header(1, "Plan"))
	return d, acct, ids[0]
}

func createDraft(t *testing.T, d *DB, in Draft) Draft {
	t.Helper()
	var out Draft
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		var err error
		out, err = tx.CreateDraft(t.Context(), in)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func sampleDraft(acct, source int64) Draft {
	return Draft{
		AccountID: acct,
		Kind:      "reply",
		SourceID:  source,
		Content: DraftContent{
			IdentityID: 0,
			To:         []Address{{Name: "Ann", Addr: "ann@x.test"}},
			Cc:         []Address{{Addr: "cc@x.test"}},
			Bcc:        []Address{{Name: "Hidden", Addr: "bcc@x.test"}},
			Subject:    "Re: Plan",
			HTML:       "<p>Sounds good</p>",
		},
		MessageID:  "d1@frostmail.test",
		InReplyTo:  "a@x.test",
		References: []string{"root@x.test", "a@x.test"},
	}
}

func TestCreateAndGetDraft(t *testing.T) {
	d, acct, source := draftFixture(t)
	in := sampleDraft(acct, source)
	got := createDraft(t, d, in)
	if got.ID == 0 || !got.CreatedAt.Equal(testNow) || !got.UpdatedAt.Equal(testNow) {
		t.Fatalf("created = %+v; want an ID and both times = the clock", got)
	}
	if got.ServerUID != 0 || !got.SavedAt.IsZero() || len(got.Attachments) != 0 {
		t.Fatalf("created = %+v; want no server copy and no attachments", got)
	}
	read, err := d.GetDraft(t.Context(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.AccountID != acct || read.Kind != "reply" || read.SourceID != source ||
		read.MessageID != "d1@frostmail.test" || read.InReplyTo != "a@x.test" ||
		!slices.Equal(read.References, in.References) || !read.CreatedAt.Equal(testNow) {
		t.Fatalf("read = %+v", read)
	}
	if !reflect.DeepEqual(read.Content, in.Content) {
		t.Fatalf("content = %+v, want %+v", read.Content, in.Content)
	}
	if _, err := d.GetDraft(t.Context(), 999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetDraft(999) = %v, want ErrNotFound", err)
	}
}

func TestEmptyDraftLists(t *testing.T) {
	d, acct, _ := draftFixture(t)
	got := createDraft(t, d, Draft{AccountID: acct, Kind: "new", MessageID: "d2@frostmail.test"})
	read, err := d.GetDraft(t.Context(), got.ID)
	if err != nil {
		t.Fatal(err)
	}
	if read.SourceID != 0 || len(read.Content.To) != 0 || len(read.Content.Cc) != 0 || len(read.Content.Bcc) != 0 ||
		len(read.References) != 0 || read.Content.Subject != "" {
		t.Fatalf("read = %+v; want empty fields", read)
	}
}

func TestUpdateDraftContent(t *testing.T) {
	d, acct, source := draftFixture(t)
	created := createDraft(t, d, sampleDraft(acct, source))
	later := testNow.Add(time.Minute)
	d.Now = func() time.Time { return later }
	content := DraftContent{To: []Address{{Addr: "bob@x.test"}}, Subject: "Changed", HTML: "<p>new</p>"}
	var got Draft
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		var err error
		got, err = tx.UpdateDraftContent(t.Context(), created.ID, content)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got.Content.Subject != "Changed" || !got.UpdatedAt.Equal(later) || !got.CreatedAt.Equal(testNow) {
		t.Fatalf("updated = %+v", got)
	}
	read, _ := d.GetDraft(t.Context(), created.ID)
	if read.Content.HTML != "<p>new</p>" || len(read.Content.To) != 1 || read.Content.To[0].Addr != "bob@x.test" {
		t.Fatalf("read after update = %+v", read.Content)
	}
	err := d.Tx(t.Context(), func(tx *Tx) error {
		_, err := tx.UpdateDraftContent(t.Context(), 999, content)
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("update of a missing draft = %v, want ErrNotFound", err)
	}
}

func TestListDrafts(t *testing.T) {
	d, acct, _ := draftFixture(t)
	if _, err := d.db.ExecContext(t.Context(), `INSERT INTO accounts (id, kind, email, auth, imap_host, imap_port, imap_tls,
		imap_username, smtp_host, smtp_port, smtp_tls, smtp_username, created_at)
		VALUES (77, 'imap', 'b@mailtest.test', 'password', 'h', 993, 'tls', 'u', 'h', 587, 'starttls', 'u', 'now')`); err != nil {
		t.Fatal(err)
	}
	first := createDraft(t, d, Draft{AccountID: acct, Kind: "new", MessageID: "1@x"})
	d.Now = func() time.Time { return testNow.Add(time.Minute) }
	second := createDraft(t, d, Draft{AccountID: acct, Kind: "new", MessageID: "2@x"})
	other := createDraft(t, d, Draft{AccountID: 77, Kind: "new", MessageID: "3@x"})
	ids := func(ds []Draft) []int64 {
		var out []int64
		for _, x := range ds {
			out = append(out, x.ID)
		}
		return out
	}
	got, err := d.ListDrafts(t.Context(), acct)
	if err != nil || !slices.Equal(ids(got), []int64{second.ID, first.ID}) {
		t.Fatalf("ListDrafts(acct) = %v, %v; want newest first", ids(got), err)
	}
	all, err := d.ListDrafts(t.Context(), 0)
	if err != nil || !slices.Equal(ids(all), []int64{other.ID, second.ID, first.ID}) {
		t.Fatalf("ListDrafts(0) = %v, %v; want every account, newest first, then higher ID", ids(all), err)
	}
}

func TestDraftAttachments(t *testing.T) {
	d, acct, _ := draftFixture(t)
	dr := createDraft(t, d, Draft{AccountID: acct, Kind: "new", MessageID: "1@x"})
	other := createDraft(t, d, Draft{AccountID: acct, Kind: "new", MessageID: "2@x"})
	var a, b DraftAttachment
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		var err error
		if a, err = tx.AddDraftAttachment(t.Context(), dr.ID, DraftAttachment{BlobID: "aa", Filename: "a.pdf", ContentType: "application/pdf", Size: 10}); err != nil {
			return err
		}
		b, err = tx.AddDraftAttachment(t.Context(), dr.ID, DraftAttachment{BlobID: "bb", Filename: "b.txt", ContentType: "text/plain", Size: 3})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if a.ID == 0 || b.ID <= a.ID || a.Filename != "a.pdf" || b.BlobID != "bb" {
		t.Fatalf("added %+v %+v", a, b)
	}
	read, _ := d.GetDraft(t.Context(), dr.ID)
	if !reflect.DeepEqual(read.Attachments, []DraftAttachment{a, b}) {
		t.Fatalf("attachments = %+v, want %+v", read.Attachments, []DraftAttachment{a, b})
	}
	err := d.Tx(t.Context(), func(tx *Tx) error { return tx.RemoveDraftAttachment(t.Context(), other.ID, a.ID) })
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("removing another draft's attachment = %v, want ErrNotFound", err)
	}
	if err := d.Tx(t.Context(), func(tx *Tx) error { return tx.RemoveDraftAttachment(t.Context(), dr.ID, a.ID) }); err != nil {
		t.Fatal(err)
	}
	read, _ = d.GetDraft(t.Context(), dr.ID)
	if !reflect.DeepEqual(read.Attachments, []DraftAttachment{b}) {
		t.Fatalf("after removal = %+v", read.Attachments)
	}
}

func TestDeleteDraft(t *testing.T) {
	d, acct, source := draftFixture(t)
	dr := createDraft(t, d, sampleDraft(acct, source))
	var deleted Draft
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		if _, err := tx.AddDraftAttachment(t.Context(), dr.ID, DraftAttachment{BlobID: "aa", Filename: "a", ContentType: "x/y", Size: 1}); err != nil {
			return err
		}
		if err := tx.SetDraftServerCopy(t.Context(), dr.ID, 42, testNow); err != nil {
			return err
		}
		var err error
		deleted, err = tx.DeleteDraft(t.Context(), dr.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if deleted.ID != dr.ID || deleted.ServerUID != 42 || len(deleted.Attachments) != 1 {
		t.Fatalf("deleted = %+v; want the draft as it was, server copy and attachments included", deleted)
	}
	if _, err := d.GetDraft(t.Context(), dr.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetDraft after delete = %v", err)
	}
	var n int
	if err := d.db.QueryRowContext(t.Context(), `SELECT count(*) FROM draft_attachments`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("attachments left: %d, %v", n, err)
	}
	err := d.Tx(t.Context(), func(tx *Tx) error {
		_, err := tx.DeleteDraft(t.Context(), dr.ID)
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
}

func TestDraftsToSave(t *testing.T) {
	d, acct, _ := draftFixture(t)
	early := createDraft(t, d, Draft{AccountID: acct, Kind: "new", MessageID: "1@x"})
	d.Now = func() time.Time { return testNow.Add(10 * time.Second) }
	recent := createDraft(t, d, Draft{AccountID: acct, Kind: "new", MessageID: "2@x"})
	ids := func(ds []Draft) []int64 {
		var out []int64
		for _, x := range ds {
			out = append(out, x.ID)
		}
		return out
	}
	quiet := testNow.Add(5 * time.Second) // quiet since: only early qualifies
	got, err := d.DraftsToSave(t.Context(), acct, quiet)
	if err != nil || !slices.Equal(ids(got), []int64{early.ID}) {
		t.Fatalf("DraftsToSave = %v, %v; want only the draft quiet long enough", ids(got), err)
	}
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		return tx.SetDraftServerCopy(t.Context(), early.ID, 42, testNow.Add(6*time.Second))
	}); err != nil {
		t.Fatal(err)
	}
	read, _ := d.GetDraft(t.Context(), early.ID)
	if read.ServerUID != 42 || !read.SavedAt.Equal(testNow.Add(6*time.Second)) {
		t.Fatalf("server copy = %d at %v", read.ServerUID, read.SavedAt)
	}
	if got, _ := d.DraftsToSave(t.Context(), acct, testNow.Add(time.Hour)); !slices.Equal(ids(got), []int64{recent.ID}) {
		t.Fatalf("after saving: %v; want only the unsaved draft", ids(got))
	}
	d.Now = func() time.Time { return testNow.Add(20 * time.Second) }
	if err := d.Tx(t.Context(), func(tx *Tx) error {
		_, err := tx.UpdateDraftContent(t.Context(), early.ID, DraftContent{Subject: "edited"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = d.DraftsToSave(t.Context(), acct, testNow.Add(time.Hour))
	// Oldest update first: recent (+10 s) before the re-edited early (+20 s).
	if !slices.Equal(ids(got), []int64{recent.ID, early.ID}) {
		t.Fatalf("after editing a saved draft: %v; want it listed again, oldest update first", ids(got))
	}
	if got[1].ServerUID != 42 {
		t.Fatalf("listed draft lost its server UID: %+v", got[1])
	}
	if got, _ := d.DraftsToSave(t.Context(), 999, testNow.Add(time.Hour)); len(got) != 0 {
		t.Fatalf("another account's drafts listed: %v", ids(got))
	}
	if err := d.Tx(t.Context(), func(tx *Tx) error { return tx.SetDraftServerCopy(t.Context(), 999, 1, testNow) }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetDraftServerCopy(999) = %v, want ErrNotFound", err)
	}
}
