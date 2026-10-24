package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/rpctest"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
)

// exportFixture is a stopped maild's data directory: a real database with
// a password account and an oauth2 account, 2 blobs, and the plain secrets
// file with both credentials and a stored OAuth client secret. It points
// the environment at it, with a socket nothing listens on.
type exportFixture struct {
	data   string
	socket string
	dir    string // the export's directory, which does not exist yet
}

func newExportFixture(t *testing.T) exportFixture {
	t.Helper()
	ctx := t.Context()
	data := t.TempDir()
	socket := filepath.Join(t.TempDir(), "maild.sock")
	t.Setenv("FROSTMAIL_SECRETS", "file")
	t.Setenv("FROSTMAIL_DATA_DIR", data)
	t.Setenv("FROSTMAIL_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	t.Setenv("FROSTMAIL_SOCKET", socket)

	db, err := store.Open(ctx, filepath.Join(data, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	srv := func(host string) store.ServerConfig {
		return store.ServerConfig{Host: host, Port: 993, TLS: api.TLSModeTLS, Username: "u"}
	}
	for _, a := range []store.Account{
		{Kind: api.AccountKindIMAP, Email: "pw@mailtest.test", Auth: api.AuthKindPassword, IMAP: srv("imap.mailtest.test"), SMTP: srv("smtp.mailtest.test")},
		{Kind: api.AccountKindIMAP, Email: "oa@mailtest.test", Auth: api.AuthKindOAuth2, IMAP: srv("imap.mailtest.test"), SMTP: srv("smtp.mailtest.test")},
	} {
		if err := db.Tx(ctx, func(tx *store.Tx) error {
			_, err := tx.InsertAccount(ctx, a)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	sec := secrets.NewFile(filepath.Join(data, "secrets.json"))
	for k, v := range map[string]string{
		secrets.AccountPassword(1):          "pw-secret-1",
		secrets.RefreshToken(2):             "refresh-secret-2",
		secrets.OAuthClientSecret("google"): "client-secret-google",
		secrets.AccountPassword(7):          "an account that is gone",
	} {
		if err := sec.Set(ctx, k, v); err != nil {
			t.Fatal(err)
		}
	}
	blobs := blob.New(filepath.Join(data, "blobs"))
	for _, raw := range []string{"first message", "second message"} {
		if _, err := blobs.Put(ctx, strings.NewReader(raw)); err != nil {
			t.Fatal(err)
		}
	}
	return exportFixture{data: data, socket: socket, dir: filepath.Join(t.TempDir(), "export")}
}

func readManifestFile(t *testing.T, dir string) map[string]string {
	t.Helper()
	m, err := readManifest(filepath.Join(dir, "MANIFEST"))
	if err != nil {
		t.Fatal(err)
	}
	return m.values
}

func TestExportWritesTheDatabaseBlobsSecretsAndManifest(t *testing.T) {
	f := newExportFixture(t)
	out, err := runMailctl(t, "export", f.dir)
	if err != nil {
		t.Fatalf("export = %v\n%s", err, out)
	}
	for _, want := range []string{"account 1: pw@mailtest.test", "account 2: oa@mailtest.test", "2 blobs", "3 secrets", "holds the secrets in plain text"} {
		if !strings.Contains(out, want) {
			t.Errorf("the summary lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "warning") {
		t.Errorf("the summary warns although both credentials exist:\n%s", out)
	}
	for _, secret := range []string{"pw-secret-1", "refresh-secret-2", "client-secret-google"} {
		if strings.Contains(out, secret) {
			t.Errorf("the summary prints a secret: %q", secret)
		}
	}

	// The secrets: exactly the keys of the exported accounts and the providers.
	all, err := secrets.NewFile(filepath.Join(f.dir, "secrets.json")).All()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"account/1/password":         "pw-secret-1",
		"account/2/refresh-token":    "refresh-secret-2",
		"oauth/google/client-secret": "client-secret-google",
	}
	if len(all) != len(want) {
		t.Errorf("secrets.json holds %v, want exactly the keys of %v", all, want)
	}
	for k, v := range want {
		if all[k] != v {
			t.Errorf("secrets.json[%s] = %q, want %q", k, all[k], v)
		}
	}

	// The files, and their modes.
	if runtime.GOOS != "windows" {
		for p, mode := range map[string]os.FileMode{".": 0o700, "blobs": 0o700, "secrets.json": 0o600, "frostmail.db": 0o600, "MANIFEST": 0o600} {
			fi, err := os.Stat(filepath.Join(f.dir, p))
			if err != nil || fi.Mode().Perm() != mode {
				t.Errorf("%s: mode %v, %v; want %v", p, fi, err, mode)
			}
		}
	}
	db, err := store.Open(t.Context(), filepath.Join(f.dir, "frostmail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	accounts, err := db.ListAccounts(t.Context())
	if err != nil || len(accounts) != 2 {
		t.Fatalf("the copy lists %v, %v", accounts, err)
	}

	m := readManifestFile(t, f.dir)
	known, _ := store.KnownSchema()
	for k, v := range map[string]string{
		"format": "1", "schema": itoa(known), "blobs": "2", "blob_bytes": "27", "secrets": "3",
		"accounts": "1:pw@mailtest.test 2:oa@mailtest.test",
	} {
		if m[k] != v {
			t.Errorf("MANIFEST %s = %q, want %q", k, m[k], v)
		}
	}
	if len(m["id"]) != 32 || m["created"] == "" || m["host"] == "" || len(m["db_sha256"]) != 64 {
		t.Errorf("MANIFEST = %v", m)
	}
	raw, err := os.ReadFile(filepath.Join(f.dir, "MANIFEST"))
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		k, _, _ := strings.Cut(l, "=")
		keys = append(keys, k)
	}
	if got := strings.Join(keys, " "); got != "format id created host schema db_sha256 blobs blob_bytes accounts secrets" {
		t.Errorf("MANIFEST keys, in order: %s", got)
	}

	// The source is as it was.
	if _, err := os.Stat(filepath.Join(f.data, "frostmail.db-wal")); err == nil {
		t.Error("the export left a WAL beside the source")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func TestExportCheckAcceptsAnExport(t *testing.T) {
	f := newExportFixture(t)
	if out, err := runMailctl(t, "export", f.dir); err != nil {
		t.Fatalf("export = %v\n%s", err, out)
	}
	out, err := runMailctl(t, "export", "--check", f.dir)
	if err != nil {
		t.Fatalf("check = %v\n%s", err, out)
	}
	for _, want := range []string{"ok: frostmail.db matches its checksum", "ok: 2 blobs", "ok: secrets.json: 3 keys", "accounts: 1:pw@mailtest.test 2:oa@mailtest.test"} {
		if !strings.Contains(out, want) {
			t.Errorf("the check's output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "warning") || strings.Contains(out, "secret-") {
		t.Errorf("the check's output:\n%s", out)
	}
}

func TestExportCheckRefuses(t *testing.T) {
	known, _ := store.KnownSchema()
	for name, tc := range map[string]struct {
		damage func(t *testing.T, dir string)
		want   string
	}{
		"a changed byte of the database": {func(t *testing.T, dir string) {
			p := filepath.Join(dir, "frostmail.db")
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			b[len(b)-1] ^= 0xff
			if err := os.WriteFile(p, b, 0o600); err != nil {
				t.Fatal(err)
			}
		}, "does not match the manifest's checksum"},
		"no database": {func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "frostmail.db")); err != nil {
				t.Fatal(err)
			}
		}, "frostmail.db is missing"},
		"a schema newer than this build": {func(t *testing.T, dir string) {
			p := filepath.Join(dir, "MANIFEST")
			b, _ := os.ReadFile(p)
			s := strings.Replace(string(b), "schema="+itoa(known), "schema="+itoa(known+1), 1)
			if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "install a newer maild here first"},
		"a missing blob": {func(t *testing.T, dir string) {
			files, _ := filepath.Glob(filepath.Join(dir, "blobs", "*", "*"))
			if len(files) == 0 {
				t.Fatal("no blobs")
			}
			if err := os.Remove(files[0]); err != nil {
				t.Fatal(err)
			}
		}, "blobs/ holds 1 files, the manifest says 2"},
		"no manifest": {func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "MANIFEST")); err != nil {
				t.Fatal(err)
			}
		}, "MANIFEST is missing"},
		"another format": {func(t *testing.T, dir string) {
			p := filepath.Join(dir, "MANIFEST")
			b, _ := os.ReadFile(p)
			if err := os.WriteFile(p, []byte(strings.Replace(string(b), "format=1", "format=2", 1)), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "not 1"},
		"no secrets file": {func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "secrets.json")); err != nil {
				t.Fatal(err)
			}
		}, "secrets.json is missing"},
		"secrets that do not parse": {func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte("not json"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, "secrets.json does not parse"},
		"secrets others can read": {func(t *testing.T, dir string) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows has no file modes")
			}
			if err := os.Chmod(filepath.Join(dir, "secrets.json"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, "want 0600"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newExportFixture(t)
			if out, err := runMailctl(t, "export", f.dir); err != nil {
				t.Fatalf("export = %v\n%s", err, out)
			}
			tc.damage(t, f.dir)
			out, err := runMailctl(t, "export", "--check", f.dir)
			if err == nil {
				t.Fatalf("the check passed:\n%s", out)
			}
			if got := out + err.Error(); !strings.Contains(got, tc.want) {
				t.Errorf("the check's answer lacks %q:\n%s\n%v", tc.want, out, err)
			}
		})
	}
}

func TestExportWarnsOfAccountsWithoutACredential(t *testing.T) {
	f := newExportFixture(t)
	if err := secrets.NewFile(filepath.Join(f.data, "secrets.json")).Delete(t.Context(), secrets.RefreshToken(2)); err != nil {
		t.Fatal(err)
	}
	out, err := runMailctl(t, "export", f.dir)
	if err != nil {
		t.Fatalf("export = %v\n%s", err, out)
	}
	if !strings.Contains(out, "warning: account 2 (oa@mailtest.test) has no stored credential: it will need Sign In Again after the move") ||
		strings.Contains(out, "account 1 (pw") {
		t.Errorf("the export's warnings:\n%s", out)
	}
	out, err = runMailctl(t, "export", "--check", f.dir)
	if err != nil {
		t.Fatalf("check = %v\n%s", err, out)
	}
	if !strings.Contains(out, "warning: account 2 (oa@mailtest.test) has no credential in secrets.json") || strings.Contains(out, "account 1 (pw") {
		t.Errorf("the check's warnings:\n%s", out)
	}
}

func TestExportRefusesWhileMaildAnswers(t *testing.T) {
	f := newExportFixture(t)
	srv := rpctest.Start(t)
	out, err := runMailctl(t, "--socket", srv.Socket, "export", f.dir)
	if err == nil || !strings.Contains(err.Error(), "stop it first (systemctl --user stop maild.service)") {
		t.Fatalf("export beside a running maild = %v\n%s", err, out)
	}
	if _, err := os.Stat(f.dir); err == nil {
		t.Error("the refused export made the directory")
	}
}

func TestExportRefusesAnExistingDirectoryAndMissingData(t *testing.T) {
	f := newExportFixture(t)
	if err := os.Mkdir(f.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if out, err := runMailctl(t, "export", f.dir); err == nil || !strings.Contains(err.Error(), "exists") {
		t.Errorf("export into an existing directory = %v\n%s", err, out)
	}
	if entries, _ := os.ReadDir(f.dir); len(entries) != 0 {
		t.Errorf("the refused export wrote into the directory: %v", entries)
	}

	if err := os.Remove(filepath.Join(f.data, "frostmail.db")); err != nil {
		t.Fatal(err)
	}
	if out, err := runMailctl(t, "export", filepath.Join(t.TempDir(), "new")); err == nil || !strings.Contains(err.Error(), "no database") {
		t.Errorf("export without a database = %v\n%s", err, out)
	}
}

func TestExportWithoutBlobsOrSecretsStillMakesBoth(t *testing.T) {
	f := newExportFixture(t)
	if err := os.RemoveAll(filepath.Join(f.data, "blobs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(f.data, "secrets.json")); err != nil {
		t.Fatal(err)
	}
	out, err := runMailctl(t, "export", f.dir)
	if err != nil {
		t.Fatalf("export = %v\n%s", err, out)
	}
	if strings.Count(out, "warning: account") != 2 {
		t.Errorf("both accounts should warn:\n%s", out)
	}
	if m := readManifestFile(t, f.dir); m["blobs"] != "0" || m["secrets"] != "0" {
		t.Errorf("MANIFEST = %v", m)
	}
	if out, err := runMailctl(t, "export", "--check", f.dir); err != nil {
		t.Errorf("check = %v\n%s", err, out)
	}
}

func TestExportHasNoDryRun(t *testing.T) {
	f := newExportFixture(t)
	if out, err := runMailctl(t, "--dry-run", "export", f.dir); err == nil || !strings.Contains(err.Error(), "no dry run") {
		t.Errorf("export --dry-run = %v\n%s", err, out)
	}
	if _, err := os.Stat(f.dir); err == nil {
		t.Error("export --dry-run made the directory")
	}
}
