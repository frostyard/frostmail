//go:build integration

package pimsync_test

// Contact edits against Radicale in the frostmail-mailtest container
// (make engine-it; plan 0009, Phase 2), as test2@mailtest.test, whose
// address book no other test uses. The manager is not started, so every
// pass is the test's own: a kick runs nothing behind its back. Edits,
// photos, deletes and restores reach the server; another client's change
// to other fields is kept when an edit is applied again, and one to the
// same field stands; verify finds both sides the same.

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/davx"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

func TestRadicaleContactEdits(t *testing.T) {
	host := os.Getenv("FROSTMAIL_IT_HOST")
	if host == "" {
		t.Skip("FROSTMAIL_IT_HOST is not set; run make engine-it")
	}
	const user, password = "test2@mailtest.test", "frostmail-test"
	ctx := t.Context()
	insecure := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // the test container's certificate
	}}
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	start := "https://" + host + ":5232/"
	var acct store.Account
	err = db.Tx(ctx, func(tx *store.Tx) error {
		server := store.ServerConfig{Host: host, Port: 993, TLS: api.TLSModeTLS, Username: user}
		if acct, err = tx.InsertAccount(ctx, store.Account{Kind: api.AccountKindIMAP, Email: user,
			Auth: api.AuthKindPassword, IMAP: server, SMTP: server}); err != nil {
			return err
		}
		return tx.SetService(ctx, acct.ID, api.ServiceKindContacts, true, start)
	})
	if err != nil {
		t.Fatal(err)
	}
	sec := secrets.NewFile(filepath.Join(t.TempDir(), "secrets.json"))
	if err := sec.Set(ctx, secrets.AccountPassword(acct.ID), password); err != nil {
		t.Fatal(err)
	}
	m := pimsync.New(db, sec, slog.New(slog.NewTextHandler(io.Discard, nil)), pimsync.Config{HTTP: insecure})
	people := engine.New(engine.Deps{DB: db, PIM: m}).People()
	pass := func() {
		t.Helper()
		if err := m.Pass(ctx, acct.ID); err != nil {
			t.Fatalf("Pass: %v", err)
		}
	}
	pass() // discovery makes Radicale create the user's address book

	opts := davx.Options{HTTP: insecure, Authorization: func(context.Context) (string, error) {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+password)), nil
	}}
	home, err := davx.Discover(ctx, start, davx.AddressBooks, opts)
	if err != nil {
		t.Fatal(err)
	}
	other, err := davx.New(home, opts)
	if err != nil {
		t.Fatal(err)
	}
	href := "/" + strings.ReplaceAll(user, "@", "%40") + "/contacts/radicale-ann.vcf"
	card := func(title, note string) []byte {
		return []byte("BEGIN:VCARD\r\nVERSION:3.0\r\nUID:radicale-ann\r\nN:Smith;Ann;;;\r\nFN:Ann Smith\r\n" +
			"TITLE:" + title + "\r\nNOTE:" + note + "\r\nX-CUSTOM:keep me\r\nEND:VCARD\r\n")
	}
	server := func() string {
		t.Helper()
		obj, err := other.Get(ctx, href)
		if err != nil {
			t.Fatal(err)
		}
		return string(obj.Data)
	}
	otherPut := func(data []byte) {
		t.Helper()
		obj, err := other.Get(ctx, href)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := other.Put(ctx, href, data, davx.AddressBooks, obj.ETag); err != nil {
			t.Fatal(err)
		}
	}
	// The container's reset keeps Radicale's data: start from no card.
	if err := other.Delete(ctx, href, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Put(ctx, href, card("Head of Operations", "Runs the offsite"), davx.AddressBooks, ""); err != nil {
		t.Fatal(err)
	}
	pass()
	rows, err := db.People(ctx, "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("people = %+v, %v", rows, err)
	}
	ann := rows[0].ContactIDs[0]
	title := func(v string) api.ContactFields { return api.ContactFields{Title: &v} }

	// An edit while another client changes another field: both survive.
	if _, err := people.Update(ctx, &api.PeopleUpdateParams{ID: ann, Fields: title("COO")}); err != nil {
		t.Fatal(err)
	}
	otherPut(card("Head of Operations", "Moved to Lisbon"))
	pass()
	if got := server(); !strings.Contains(got, "TITLE:COO") || !strings.Contains(got, "NOTE:Moved to Lisbon") ||
		!strings.Contains(got, "X-CUSTOM:keep me") {
		t.Errorf("after the edit the server has %s", got)
	}

	// An edit while another client changes the same field: theirs stands.
	if _, err := people.Update(ctx, &api.PeopleUpdateParams{ID: ann, Fields: title("Chief")}); err != nil {
		t.Fatal(err)
	}
	otherPut(card("Chief of Staff", "Moved to Lisbon"))
	pass()
	if got := server(); !strings.Contains(got, "TITLE:Chief of Staff") {
		t.Errorf("after the refused edit the server has %s", got)
	}
	if obj, err := db.GetObject(ctx, ann); err != nil || !strings.Contains(string(obj.Raw), "TITLE:Chief of Staff") {
		t.Errorf("after the refused edit the local card is %s, %v", obj.Raw, err)
	}

	// A photo, and its undo.
	jpeg := base64.StdEncoding.EncodeToString([]byte("\xff\xd8\xff\xe0 a jpeg"))
	w, err := people.SetPhoto(ctx, &api.PeopleSetPhotoParams{ID: ann, Photo: &api.Photo{ContentType: "image/jpeg", Data: jpeg}})
	if err != nil {
		t.Fatal(err)
	}
	pass()
	if !strings.Contains(server(), "PHOTO;ENCODING=b;TYPE=JPEG:") {
		t.Error("the photo did not reach Radicale")
	}
	if err := people.Restore(ctx, &api.PeopleRestoreParams{RestoreID: w.RestoreID}); err != nil {
		t.Fatal(err)
	}
	pass()
	if strings.Contains(server(), "PHOTO") {
		t.Error("the photo's undo did not reach Radicale")
	}

	// A delete, and its undo.
	gone, err := people.Delete(ctx, &api.PeopleDeleteParams{ID: ann})
	if err != nil {
		t.Fatal(err)
	}
	pass()
	if _, err := other.Get(ctx, href); err == nil {
		t.Error("the deleted contact is still on Radicale")
	}
	if err := people.Restore(ctx, &api.PeopleRestoreParams{RestoreID: gone.RestoreID}); err != nil {
		t.Fatal(err)
	}
	pass()
	if got := server(); !strings.Contains(got, "TITLE:Chief of Staff") || !strings.Contains(got, "X-CUSTOM:keep me") {
		t.Errorf("after the delete's undo the server has %s", got)
	}
	pass()
	checks, err := m.Verify(ctx, acct.ID)
	if err != nil || len(checks) != 1 || !pimsync.Clean(checks[0]) || checks[0].Server != 1 || checks[0].Local != 1 {
		t.Errorf("verify = %+v, %v", checks, err)
	}
}
