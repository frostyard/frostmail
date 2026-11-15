package store

import (
	"testing"

	"github.com/frostyard/frostmail/api"
)

// The people mark lasts until RelinkPeople, and removing an address book
// sets it, since its contacts go with it.
func TestPeopleStaleMark(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	stale := func() bool {
		t.Helper()
		s, err := d.PeopleStale(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	tx := func(f func(*Tx) error) {
		t.Helper()
		if err := d.Tx(ctx, f); err != nil {
			t.Fatal(err)
		}
	}
	if stale() {
		t.Fatal("a new store's people are stale")
	}
	var acct Account
	books := []RemoteCollection{{Href: "/a/", Name: "A"}, {Href: "/b/", Name: "B"}}
	tx(func(tx *Tx) error {
		var err error
		acct, err = tx.InsertAccount(ctx, Account{Kind: api.AccountKindIMAP, Email: "u@example.com", Auth: api.AuthKindPassword,
			IMAP: ServerConfig{Host: "h", Port: 993, TLS: api.TLSModeTLS, Username: "u"}, SMTP: ServerConfig{Host: "h", Port: 587, TLS: api.TLSModeStartTLS, Username: "u"}})
		if err != nil {
			return err
		}
		_, err = tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindAddressbook, books)
		return err
	})
	if stale() {
		t.Error("adding address books marked people stale")
	}
	tx(func(tx *Tx) error { return tx.MarkPeopleStale(ctx) })
	tx(func(tx *Tx) error { return tx.MarkPeopleStale(ctx) })
	if !stale() {
		t.Error("MarkPeopleStale left no mark")
	}
	tx(func(tx *Tx) error { return tx.RelinkPeople(ctx) })
	if stale() {
		t.Error("RelinkPeople left the mark")
	}
	tx(func(tx *Tx) error {
		_, err := tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindCalendar, []RemoteCollection{{Href: "/c/", Name: "C"}})
		if err != nil {
			return err
		}
		_, err = tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindCalendar, nil)
		return err
	})
	if stale() {
		t.Error("removing a calendar marked people stale")
	}
	tx(func(tx *Tx) error {
		_, err := tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindAddressbook, books[:1])
		return err
	})
	if !stale() {
		t.Error("removing an address book left people unmarked")
	}
}

// ObjectRaws reads a collection's sources by href.
func TestObjectRaws(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	var col, other int64
	if err := d.Tx(ctx, func(tx *Tx) error {
		acct, err := tx.InsertAccount(ctx, Account{Kind: api.AccountKindGmail, Email: "u@example.com", Auth: api.AuthKindOAuth2,
			IMAP: ServerConfig{Host: "h", Port: 993, TLS: api.TLSModeTLS, Username: "u"}, SMTP: ServerConfig{Host: "h", Port: 587, TLS: api.TLSModeStartTLS, Username: "u"}})
		if err != nil {
			return err
		}
		cols, err := tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindTasklist, []RemoteCollection{{Href: "l1", Name: "1"}, {Href: "l2", Name: "2"}})
		if err != nil {
			return err
		}
		col, other = cols[0].ID, cols[1].ID
		for _, o := range []Object{
			{CollectionID: col, Href: "t1", Kind: ObjectGTask, UID: "t1", Raw: []byte(`{"id":"t1"}`)},
			{CollectionID: col, Href: "t2", Kind: ObjectGTask, UID: "t2", Raw: []byte(`{"id":"t2"}`)},
			{CollectionID: other, Href: "t3", Kind: ObjectGTask, UID: "t3", Raw: []byte(`{"id":"t3"}`)},
		} {
			if _, err := tx.PutObject(ctx, o); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	raws, err := d.ObjectRaws(ctx, col)
	if err != nil {
		t.Fatal(err)
	}
	if len(raws) != 2 || string(raws["t1"]) != `{"id":"t1"}` || string(raws["t2"]) != `{"id":"t2"}` {
		t.Errorf("raws = %q", raws)
	}
}
