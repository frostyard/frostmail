package store

import (
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
)

// Graph's JSON stays beside a derived source across local edits, which
// store no JSON, and is replaced by the next one Graph sends (ADR-0040).
func TestObjectRemoteKeptAcrossLocalEdits(t *testing.T) {
	d, _ := openTest(t)
	ctx := t.Context()
	var col int64
	if err := d.Tx(ctx, func(tx *Tx) error {
		acct, err := tx.InsertAccount(ctx, Account{Kind: api.AccountKindMicrosoft, Email: "cy@hotmail.com", Auth: api.AuthKindOAuth2,
			IMAP: ServerConfig{Host: "h", Port: 993, TLS: api.TLSModeTLS, Username: "u"}, SMTP: ServerConfig{Host: "h", Port: 587, TLS: api.TLSModeStartTLS, Username: "u"}})
		if err != nil {
			return err
		}
		cols, err := tx.ReplaceCollections(ctx, acct.ID, api.CollectionKindCalendar, []RemoteCollection{{Href: "AAMk-cal", Name: "Calendar"}})
		if err != nil {
			return err
		}
		col = cols[0].ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	put := func(o Object) int64 {
		var id int64
		if err := d.Tx(ctx, func(tx *Tx) error {
			var err error
			id, err = tx.PutObject(ctx, o)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	o := Object{CollectionID: col, Href: "AAMk-ev", ETag: `W/"1"`, Kind: ObjectVEvent, UID: "u", Raw: []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"), Remote: []byte(`{"id":"AAMk-ev"}`)}
	id := put(o)
	o.Raw, o.Remote = []byte("BEGIN:VCALENDAR\r\nX:edited\r\nEND:VCALENDAR\r\n"), nil
	put(o)
	got, err := d.GetObject(ctx, id)
	if err != nil || string(got.Remote) != `{"id":"AAMk-ev"}` || !strings.Contains(string(got.Raw), "edited") {
		t.Fatalf("after a local edit: remote %q raw %q, %v", got.Remote, got.Raw, err)
	}
	o.Remote = []byte(`{"id":"AAMk-ev","subject":"new"}`)
	put(o)
	if got, _ := d.GetObject(ctx, id); string(got.Remote) != `{"id":"AAMk-ev","subject":"new"}` {
		t.Fatalf("after a sync: remote %q", got.Remote)
	}
}
