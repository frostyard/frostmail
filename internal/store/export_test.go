package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/frostyard/frostmail/api"
)

func TestExportCopiesTheDatabaseWithoutMigratingTheSource(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	src := filepath.Join(dir, "frostmail.db")
	d, err := Open(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	insertAccount(t, d, sampleAccount("a@mailtest.test"))
	oauth := sampleAccount("b@mailtest.test")
	oauth.Auth = api.AuthKindOAuth2
	insertAccount(t, d, oauth)
	known, err := KnownSchema()
	if err != nil || known < 2 {
		t.Fatalf("KnownSchema = %d, %v", known, err)
	}
	version, err := d.SchemaVersion(ctx)
	if err != nil || version != known {
		t.Fatalf("SchemaVersion = %d, %v; want %d", version, err, known)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "copy.db")
	snap, err := Export(ctx, src, dst)
	if err != nil {
		t.Fatal(err)
	}
	want := []ExportedAccount{{ID: 1, Email: "a@mailtest.test"}, {ID: 2, Email: "b@mailtest.test", OAuth: true}}
	if snap.SchemaVersion != known || len(snap.Accounts) != 2 || snap.Accounts[0] != want[0] || snap.Accounts[1] != want[1] {
		t.Fatalf("Export = %+v, want schema %d and %+v", snap, known, want)
	}
	if after, err := os.ReadFile(src); err != nil || !bytes.Equal(before, after) {
		t.Errorf("the source file changed: %v", err)
	}

	c, err := Open(ctx, dst)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	got, err := c.ListAccounts(ctx)
	if err != nil || len(got) != 2 || got[0].Email != "a@mailtest.test" || got[1].Email != "b@mailtest.test" || got[1].Auth != api.AuthKindOAuth2 {
		t.Fatalf("the copy lists %+v, %v", got, err)
	}

	// dst must be new.
	if _, err := Export(ctx, src, dst); err == nil {
		t.Error("Export over an existing file succeeded")
	}
	if _, err := Export(ctx, filepath.Join(dir, "missing.db"), filepath.Join(dir, "x.db")); err == nil {
		t.Error("Export of a missing database succeeded")
	}
	if _, err := os.Stat(filepath.Join(dir, "missing.db")); err == nil {
		t.Error("Export created the missing source")
	}
	if _, err := Export(ctx, src+"?x", filepath.Join(dir, "y.db")); err == nil {
		t.Error("Export of a path with ? succeeded")
	}
}
