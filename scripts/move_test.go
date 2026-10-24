//go:build linux

package scripts_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The tests of move-mail-to-lodge.sh (plan 0011, Phase 7) run it against a
// staged root: lodge's host with frostmail.service and frostmail.socket
// (the data of an earlier test deployment, and a parts cache), an export
// copied from the desktop, and, for the desktop's half, the user unit
// maild.service. Stub commands on PATH play systemd, runuser, getent,
// lodgectl, chown and mailctl, keep the units' state in the root, and log
// what the script runs. A stub database is a file with one "ID EMAIL" line
// per account, which the stub mailctl lists.

const (
	exportID = "0123456789abcdef0123456789abcdef"
	stamp    = "20261026T090000Z"
	backup   = "var/backups/frostmail-move-" + stamp
	// the desktop's database, and so the export's.
	desktopDB = "1 alice@gmail.com\n2 bob@gmail.com\n"
	// the earlier test deployment's database.
	testDB = "1 test@mailtest.test\n"
	// the export's accounts, as its manifest lists them.
	manifestAccounts = "1:alice@gmail.com 2:bob@gmail.com"
)

const stubs = `#!/bin/bash
# One stub for every command, by the name it is called as.
R=$MOVE_ROOT
log() { printf '%s\n' "$*" >>"$R/stub/log"; }
u() { printf '%s' "$R/stub/units/$1"; }
case $(basename "$0") in
systemctl)
	pre=; if [ "${1:-}" = --user ]; then pre="--user "; shift; fi
	case $1 in
	show) cat "$(u "$5")/$3" 2>/dev/null || { [ "$3" = LoadState ] && echo not-found; }; exit 0 ;;
	is-active) s=$(cat "$(u "$2")/active" 2>/dev/null || echo inactive); echo "$s"; [ "$s" = active ] ;;
	is-enabled) cat "$(u "$2")/enabled" 2>/dev/null || echo disabled ;;
	*) log "systemctl $pre$*"
		cmd=$1; shift; now=; [ "${1:-}" = --now ] && { now=1; shift; }
		for unit in "$@"; do
			[ -d "$(u "$unit")" ] || continue
			case $cmd in
			enable) echo enabled >"$(u "$unit")/enabled"; if [ -n "$now" ]; then echo active >"$(u "$unit")/active"; fi ;;
			disable) echo disabled >"$(u "$unit")/enabled"; if [ -n "$now" ]; then echo inactive >"$(u "$unit")/active"; fi ;;
			start) echo active >"$(u "$unit")/active" ;;
			stop) echo inactive >"$(u "$unit")/active" ;;
			esac
			# CacheDirectory= makes the cache when the service starts.
			if [ "$unit" = frostmail.service ] && [ "$(cat "$(u "$unit")/active")" = active ]; then mkdir -p "$R/var/cache/frostmail"; fi
		done ;;
	esac ;;
runuser)
	user=$2; shift 3; log "runuser $user $*"
	exec "$@" ;;
getent)
	case $2 in
	frostmail) [ -e "$R/stub/no-frostmail" ] && exit 2; echo "frostmail:x:991:991::/var/lib/frostmail:/usr/sbin/nologin" ;;
	*) exit 2 ;;
	esac ;;
lodgectl)
	case $1 in
	status) [ -e "$R/stub/lodge-down" ] && exit 1; exit 0 ;;
	tokens) cat "$R/stub/tokens" 2>/dev/null; exit 0 ;;
	*) log "lodgectl $*" ;;
	esac ;;
mailctl)
	case $1 in
	help) echo "USAGE"; [ -e "$R/stub/old-mailctl" ] || echo "  mailctl export DIR [--flags]"; exit 0 ;;
	hello) [ -e "$R/stub/maild-answers" ]; exit ;;
	export)
		if [ "$2" = --check ]; then
			if [ -e "$R/stub/check-fails" ]; then echo "problem: frostmail.db does not match the manifest's checksum"; exit 1; fi
			echo "ok: frostmail.db matches its checksum"; echo "accounts: $(grep '^accounts=' "$3/MANIFEST" | cut -d= -f2)"; exit 0
		fi
		log "mailctl export $2"
		mkdir -m 0700 "$2" || exit 1
		if [ -e "$R/stub/export-fails" ]; then echo "export failed" >&2; exit 1; fi
		mkdir -m 0700 -p "$2/blobs/ab"
		printf 'a message' >"$2/blobs/ab/abcdef"
		printf '1 alice@gmail.com\n2 bob@gmail.com\n' >"$2/frostmail.db"
		printf '{"account/1/password":"x"}' >"$2/secrets.json"; chmod 0600 "$2/secrets.json"
		printf 'format=1\nid=0123456789abcdef0123456789abcdef\ncreated=2026-10-26T08:00:00.000Z\nhost=desktop\nschema=18\ndb_sha256=0\nblobs=1\nblob_bytes=9\naccounts=1:alice@gmail.com 2:bob@gmail.com\nsecrets=1\n' >"$2/MANIFEST"
		chmod 0600 "$2/MANIFEST" ;;
	--socket)
		[ "$3 $4" = "account list" ] || exit 2
		[ "$(cat "$(u frostmail.service)/active" 2>/dev/null)" = active ] || { echo "cannot reach maild" >&2; exit 1; }
		echo "ID  EMAIL  IMAP  SMTP"
		while read -r id email; do echo "$id  $email  imap.example:993/tls  smtp.example:587/starttls"; done <"$R/var/lib/frostmail/frostmail.db" ;;
	*) exit 2 ;;
	esac ;;
*) log "$(basename "$0") $*" ;;
esac
`

// stage builds lodge's host and the desktop, and returns the root.
func stage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write := func(p, content string) { writeFile(t, root, p, content) }
	unit := func(name string, props map[string]string) {
		for k, v := range props {
			write("stub/units/"+name+"/"+k, v+"\n")
		}
	}
	unit("lodge.service", map[string]string{"LoadState": "loaded", "active": "active", "enabled": "enabled"})
	unit("frostmail.service", map[string]string{"LoadState": "loaded", "active": "active", "enabled": "enabled",
		"Environment": "FROSTMAIL_DATA_DIR=/var/lib/frostmail FROSTMAIL_CACHE_DIR=/var/cache/frostmail FROSTMAIL_SOCKET=/run/frostmail/maild.sock FROSTMAIL_SECRETS=file",
		"ExecStart":   "{ path=/usr/local/bin/maild ; argv[]=/usr/local/bin/maild --lodge ; ignore_errors=no }",
		"DropInPaths": "/etc/systemd/system/frostmail.service.d/override.conf"})
	unit("frostmail.socket", map[string]string{"LoadState": "loaded", "active": "active", "enabled": "enabled"})
	unit("maild.service", map[string]string{"LoadState": "loaded", "active": "active", "enabled": "enabled"})
	write("stub/log", "")
	write("stub/tokens", "tok1  app=mail  scope=mail:read account=2  expires never\ntok2  app=notes  scope=notes:write\n")
	write("usr/share/lodge/apps/mail/app.json", `{"id": "mail"}`)
	// The earlier test deployment's data and parts cache.
	write("var/lib/frostmail/frostmail.db", testDB)
	write("var/lib/frostmail/blobs/cd/cdef01", "a test message")
	write("var/lib/frostmail/secrets.json", `{"account/1/password":"test"}`)
	write("var/cache/frostmail/parts/1/body.html", "<p>a test</p>")
	// The export, copied from the desktop; its secrets file's mode is wrong
	// on purpose, for the script to set.
	exportTo(t, root, "root/frostmail-export")
	// The desktop's own data.
	write("home/u/.local/share/frostmail/frostmail.db", desktopDB)
	// The stubs: one script, many names; mailctl beside maild.
	bin := filepath.Join(root, "stub", "bin")
	write("stub/bin/stub", stubs)
	if err := os.Chmod(filepath.Join(bin, "stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"systemctl", "runuser", "getent", "lodgectl", "mailctl", "chown"} {
		if err := os.Symlink("stub", filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "usr", "local", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(bin, "mailctl"), filepath.Join(root, "usr", "local", "bin", "mailctl")); err != nil {
		t.Fatal(err)
	}
	return root
}

// exportTo writes the export of desktopDB into dir under root.
func exportTo(t *testing.T, root, dir string) {
	t.Helper()
	writeFile(t, root, dir+"/frostmail.db", desktopDB)
	writeFile(t, root, dir+"/blobs/ab/abcdef", "a message")
	writeFile(t, root, dir+"/secrets.json", `{"account/1/password":"x"}`)
	writeFile(t, root, dir+"/MANIFEST", "format=1\nid="+exportID+"\ncreated=2026-10-26T08:00:00.000Z\nhost=desktop\nschema=18\ndb_sha256=0\nblobs=1\nblob_bytes=9\naccounts="+manifestAccounts+"\nsecrets=1\ndesktop_maild=disabled\n")
}

func writeFile(t *testing.T, root, p, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, root, p string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func exists(root, p string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(p)))
	return err == nil
}

// move runs the script against root and returns its output and exit code.
func move(t *testing.T, root string, args ...string) (string, int) {
	t.Helper()
	return moveEnv(t, root, nil, args...)
}

func moveEnv(t *testing.T, root string, env []string, args ...string) (string, int) {
	t.Helper()
	bin := filepath.Join(root, "stub", "bin")
	cmd := exec.Command("bash", append([]string{"move-mail-to-lodge.sh"}, args...)...)
	cmd.Env = append(os.Environ(), "MOVE_ROOT="+root, "MOVE_STAMP="+stamp, "PATH="+bin+":"+os.Getenv("PATH"))
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		return string(out), ee.ExitCode()
	case err != nil:
		t.Fatal(err)
	}
	return string(out), 0
}

// desktopEnv is the user's environment for the desktop's half.
func desktopEnv(root string) []string {
	return []string{"HOME=" + filepath.Join(root, "home", "u"), "XDG_DATA_HOME=", "FROSTMAIL_DATA_DIR=",
		"MAILCTL=" + filepath.Join(root, "stub", "bin", "mailctl")}
}

// snapshot is every file and directory under root but the stubs' and the
// backups', by path: a file's content's hash.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() && (rel == "stub" || rel == filepath.FromSlash("var/backups")) {
			return filepath.SkipDir
		}
		if d.IsDir() || d.Type()&fs.ModeSymlink != 0 {
			out[rel+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(p)
		sum := sha256.Sum256(data)
		out[rel] = hex.EncodeToString(sum[:])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// same reports how two snapshots differ, or "".
func same(before, after map[string]string) string {
	var diff []string
	for _, k := range slices.Sorted(maps.Keys(before)) {
		if v, ok := after[k]; !ok {
			diff = append(diff, "-"+k)
		} else if v != before[k] {
			diff = append(diff, "~"+k)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(after)) {
		if _, ok := before[k]; !ok {
			diff = append(diff, "+"+k)
		}
	}
	return strings.Join(diff, " ")
}

// changes are the logged commands that change the machine.
func changes(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	for l := range strings.SplitSeq(strings.TrimSpace(readFile(t, root, "stub/log")), "\n") {
		if l != "" && !strings.HasPrefix(l, "runuser ") {
			out = append(out, l)
		}
	}
	return out
}

// inOrder fails unless every step is in got, in order.
func inOrder(t *testing.T, got []string, steps ...string) {
	t.Helper()
	last := -1
	for _, step := range steps {
		i := slices.Index(got, step)
		if i <= last {
			t.Fatalf("%q is missing or out of order in %q", step, got)
		}
		last = i
	}
}

func TestTheHostDryRunChecksAndChangesNothing(t *testing.T) {
	root := stage(t)
	before := snapshot(t, root)
	out, code := move(t, root, "--from", filepath.Join(root, "root/frostmail-export"))
	if code != 0 {
		t.Fatalf("dry run exited %d:\n%s", code, out)
	}
	for _, want := range []string{
		"✓ lodge is installed and up",
		"✓ mail's manifest is in /usr/share/lodge/apps/mail",
		"✓ user frostmail exists (uid 991, gid 991)",
		"frostmail.service runs /usr/local/bin/maild as frostmail, data /var/lib/frostmail, cache /var/cache/frostmail, private socket /run/frostmail/maild.sock",
		"with drop-ins: /etc/systemd/system/frostmail.service.d/override.conf",
		"export --check passes for " + filepath.Join(root, "root/frostmail-export"),
		"export " + exportID + " from desktop, made 2026-10-26T08:00:00.000Z; accounts: " + manifestAccounts,
		"✓ /var/lib/frostmail holds only generated mail (test@mailtest.test): it goes aside, never merged",
		"the parts cache /var/cache/frostmail is keyed by message ID",
		"tok1  app=mail  scope=mail:read account=2",
		"a scope that names an account ID (account=<id>) names the export's account with that ID",
		"$ systemctl stop frostmail.socket frostmail.service",
		"$ systemctl enable --now frostmail.socket frostmail.service",
		"Roll back with:",
		"Dry run: nothing changed. Run again with --apply to move.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the dry run's output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "tok2") || strings.Contains(out, "does not say the desktop's maild is disabled") {
		t.Errorf("the dry run lists a token of another app, or warns of a disabled desktop:\n%s", out)
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("the dry run changed %s", d)
	}
	if c := changes(t, root); len(c) != 0 {
		t.Errorf("the dry run ran %q", c)
	}
}

func TestWarnsWhenTheDesktopsMaildIsNotDisabled(t *testing.T) {
	root := stage(t)
	writeFile(t, root, "root/frostmail-export/MANIFEST", strings.Replace(readFile(t, root, "root/frostmail-export/MANIFEST"), "desktop_maild=disabled\n", "", 1))
	out, code := move(t, root, "--from", filepath.Join(root, "root/frostmail-export"))
	if code != 0 || !strings.Contains(out, "! the manifest does not say the desktop's maild is disabled") {
		t.Fatalf("exit %d, without the warning:\n%s", code, out)
	}
}

func TestHostRefusals(t *testing.T) {
	from := func(root string) []string { return []string{"--from", filepath.Join(root, "root/frostmail-export")} }
	for name, tc := range map[string]struct {
		setup func(t *testing.T, root string)
		args  func(root string) []string
		want  string
	}{
		"lodge down": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/lodge-down", "")
		}, from, "lodge is not up"},
		"no frostmail user": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/no-frostmail", "")
		}, from, "user frostmail does not exist"},
		"mail not installed": {func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "usr/share/lodge/apps/mail/app.json")); err != nil {
				t.Fatal(err)
			}
		}, from, "mail is not installed for lodge"},
		"secrets not in the file store": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/frostmail.service/Environment", "FROSTMAIL_DATA_DIR=/var/lib/frostmail\n")
		}, from, "FROSTMAIL_SECRETS='', not 'file'"},
		"the service not installed": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/frostmail.service/LoadState", "not-found\n")
		}, from, "frostmail.service is not installed"},
		"the socket not installed": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/frostmail.socket/LoadState", "not-found\n")
		}, from, "frostmail.socket is not installed"},
		"a mailctl without export": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/old-mailctl", "")
		}, from, "has no 'export'"},
		"an export that fails its check": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/check-fails", "")
		}, from, "export --check " + "%ROOT%/root/frostmail-export fails"},
		"an export with no manifest": {func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "root/frostmail-export/MANIFEST")); err != nil {
				t.Fatal(err)
			}
		}, from, "MANIFEST is missing"},
		"an export that is not a directory": {func(*testing.T, string) {}, func(root string) []string {
			return []string{"--from", filepath.Join(root, "root/nothing-here")}
		}, "nothing-here is not a directory"},
		"an export moved already": {func(t *testing.T, root string) {
			writeFile(t, root, "var/backups/frostmail-move-20261001T000000Z/manifest", "export="+exportID+"\ntarget=/x\n")
		}, from, "this export was moved already (see "},
		"target data of real accounts, without --set-aside": {func(t *testing.T, root string) {
			writeFile(t, root, "var/lib/frostmail/frostmail.db", "1 carol@gmail.com\n2 test@mailtest.test\n")
		}, from, "holds accounts that look real: 1:carol@gmail.com. Mail is never merged: pass --set-aside"},
		"target data and the unit stopped": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/frostmail.service/active", "inactive\n")
		}, from, "holds a database and frostmail.service is not running to say what"},
		"no --from": {func(*testing.T, string) {}, func(string) []string { return nil }, "--from DIR is required"},
		"an aside that exists": {func(t *testing.T, root string) {
			writeFile(t, root, "var/lib/frostmail.aside-"+stamp+"/x", "")
		}, from, "frostmail.aside-" + stamp + " exists"},
	} {
		t.Run(name, func(t *testing.T) {
			root := stage(t)
			tc.setup(t, root)
			before := snapshot(t, root)
			for _, apply := range []bool{false, true} {
				args := tc.args(root)
				if apply {
					args = append(args, "--apply")
				}
				out, code := move(t, root, args...)
				want := strings.ReplaceAll(tc.want, "%ROOT%", root)
				if code == 0 || !strings.Contains(out, want) || !strings.Contains(out, "nothing changed") {
					t.Fatalf("apply=%v: exit %d, output lacks %q or \"nothing changed\":\n%s", apply, code, want, out)
				}
				if d := same(before, snapshot(t, root)); d != "" {
					t.Errorf("a refusal changed %s", d)
				}
				if c := changes(t, root); len(c) != 0 {
					t.Errorf("a refusal ran %q", c)
				}
				if exists(root, backup) {
					t.Errorf("a refusal made the backup")
				}
			}
		})
	}
}

func TestAnExportThatWasRolledBackCanMoveAgain(t *testing.T) {
	root := stage(t)
	writeFile(t, root, "var/backups/frostmail-move-20261001T000000Z/manifest", "export="+exportID+"\nrolled_back=20261002T000000Z\n")
	if out, code := move(t, root, "--from", filepath.Join(root, "root/frostmail-export")); code != 0 {
		t.Fatalf("dry run exited %d:\n%s", code, out)
	}
}

// applied moves the staged export in with --apply, and returns the output.
func applied(t *testing.T, root string, extra ...string) string {
	t.Helper()
	out, code := move(t, root, append([]string{"--from", filepath.Join(root, "root/frostmail-export"), "--apply"}, extra...)...)
	if code != 0 {
		t.Fatalf("--apply exited %d:\n%s", code, out)
	}
	return out
}

func TestApplyMovesTheExportAndRollbackPutsTheMachineBack(t *testing.T) {
	root := stage(t)
	before := snapshot(t, root)
	out := applied(t, root)

	// The target holds the export's files, as frostmail's, the secrets 0600.
	for p, want := range map[string]string{
		"var/lib/frostmail/frostmail.db":                            desktopDB,
		"var/lib/frostmail/blobs/ab/abcdef":                         "a message",
		"var/lib/frostmail/secrets.json":                            `{"account/1/password":"x"}`,
		"var/lib/frostmail.aside-" + stamp + "/frostmail.db":        testDB,
		"var/lib/frostmail.aside-" + stamp + "/blobs/cd/cdef01":     "a test message",
		"var/cache/frostmail.aside-" + stamp + "/parts/1/body.html": "<p>a test</p>",
	} {
		if got := readFile(t, root, p); got != want {
			t.Errorf("%s = %q, want %q", p, got, want)
		}
	}
	fi, err := os.Stat(filepath.Join(root, "var/lib/frostmail/secrets.json"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("secrets.json: %v, %v; want 0600", fi, err)
	}
	if fi, err := os.Stat(filepath.Join(root, "var/lib/frostmail")); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("the data directory: %v, %v; want 0700", fi, err)
	}
	if exists(root, "var/lib/frostmail.new") {
		t.Error("frostmail.new is left")
	}
	if exists(root, "var/cache/frostmail/parts") {
		t.Error("the old parts cache is still in place: it is keyed by message IDs of the old database")
	}
	// The export is as it was: the script never deletes it.
	if got := readFile(t, root, "root/frostmail-export/frostmail.db"); got != desktopDB {
		t.Errorf("the export's database = %q", got)
	}
	m := readFile(t, root, backup+"/manifest")
	for _, want := range []string{
		"target=" + filepath.Join(root, "var/lib/frostmail"), "cache=" + filepath.Join(root, "var/cache/frostmail"),
		"export=" + exportID, "unit_was=active", "unit_enabled=enabled", "socket_was=active",
		"app_socket=/run/frostmail/maild.sock", "mailctl=" + filepath.Join(root, "usr/local/bin/mailctl"),
		"accounts=" + manifestAccounts, "aside=" + filepath.Join(root, "var/lib/frostmail.aside-"+stamp),
		"cache_aside=" + filepath.Join(root, "var/cache/frostmail.aside-"+stamp),
	} {
		if !strings.Contains(m, want+"\n") {
			t.Errorf("the backup's manifest lacks %q:\n%s", want, m)
		}
	}
	if fi, err := os.Stat(filepath.Join(root, backup)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("the backup directory: %v, %v; want 0700", fi, err)
	}

	c := changes(t, root)
	inOrder(t, c,
		"systemctl stop frostmail.socket frostmail.service",
		"chown -R frostmail:frostmail "+filepath.Join(root, "var/lib/frostmail.new"),
		"systemctl enable --now frostmail.socket frostmail.service",
	)
	for _, want := range []string{
		"✓ frostmail.service is active", "✓ its accounts are the export's: " + manifestAccounts,
		"Roll back with:", "secret-tool clear application frostmail", "Settings, Server pane",
		"remove " + filepath.Join(root, "root/frostmail-export") + " on this host",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the output lacks %q:\n%s", want, out)
		}
	}

	// The rollback: the old data and cache come back as they were; what the
	// move made is kept in the backup.
	rb := filepath.Join(root, backup)
	out, code := move(t, root, "--rollback", rb, "--apply")
	if code != 0 {
		t.Fatalf("--rollback exited %d:\n%s", code, out)
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("after the rollback, the machine differs: %s", d)
	}
	if got := readFile(t, root, backup+"/data-after-move/frostmail.db"); got != desktopDB {
		t.Errorf("the data kept in the backup = %q", got)
	}
	if !exists(root, backup+"/cache-after-move") {
		t.Error("the cache the move's maild made is not kept in the backup")
	}
	if !strings.Contains(readFile(t, root, backup+"/manifest"), "\nrolled_back="+stamp+"\n") {
		t.Error("the manifest does not say the move was rolled back")
	}
	inOrder(t, changes(t, root)[len(c):],
		"systemctl disable --now frostmail.socket frostmail.service",
		"systemctl start frostmail.socket frostmail.service",
		"systemctl enable frostmail.socket frostmail.service",
	)
	for _, want := range []string{"What the server's maild did since the move stays", filepath.Join(root, backup) + "/data-after-move", "systemctl --user enable --now maild.service"} {
		if !strings.Contains(out, want) {
			t.Errorf("the rollback's output lacks %q:\n%s", want, out)
		}
	}
	for _, unit := range []string{"frostmail.service", "frostmail.socket"} {
		if got := readFile(t, root, "stub/units/"+unit+"/active"); got != "active\n" {
			t.Errorf("%s after the rollback is %q", unit, got)
		}
	}
	if out, code := move(t, root, "--rollback", rb, "--apply"); code == 0 || !strings.Contains(out, "rolled back already") {
		t.Errorf("a second rollback: exit %d:\n%s", code, out)
	}
	// The export may move again after a rollback.
	if out, code := move(t, root, "--from", filepath.Join(root, "root/frostmail-export")); code != 0 {
		t.Errorf("a dry run after the rollback exited %d:\n%s", code, out)
	}
}

func TestApplyMovesOntoNothingWhenTheTargetDoesNotExist(t *testing.T) {
	root := stage(t)
	for _, p := range []string{"var/lib/frostmail", "var/cache/frostmail"} {
		if err := os.RemoveAll(filepath.Join(root, p)); err != nil {
			t.Fatal(err)
		}
	}
	before := snapshot(t, root)
	out := applied(t, root)
	if got := readFile(t, root, "var/lib/frostmail/frostmail.db"); got != desktopDB {
		t.Errorf("the target's database = %q", got)
	}
	if exists(root, "var/lib/frostmail.aside-"+stamp) || strings.Contains(readFile(t, root, backup+"/manifest"), "aside=") {
		t.Errorf("something went aside that did not exist:\n%s", out)
	}
	if out, code := move(t, root, "--rollback", filepath.Join(root, backup), "--apply"); code != 0 {
		t.Fatalf("--rollback exited %d:\n%s", code, out)
	}
	// Starting maild again makes its cache (CacheDirectory=): the only
	// difference. What the move's maild made is kept in the backup.
	if d := same(before, snapshot(t, root)); d != "+var/cache/frostmail/" {
		t.Errorf("after the rollback, the machine differs: %s", d)
	}
}

func TestRealAccountsGoAsideOnlyWithSetAside(t *testing.T) {
	root := stage(t)
	writeFile(t, root, "var/lib/frostmail/frostmail.db", "1 carol@gmail.com\n2 test@mailtest.test\n")
	out := applied(t, root, "--set-aside")
	if !strings.Contains(out, "! /var/lib/frostmail holds accounts that look real: 1:carol@gmail.com. They go aside (--set-aside), never merged.") {
		t.Errorf("no warning of the real accounts:\n%s", out)
	}
	if got := readFile(t, root, "var/lib/frostmail.aside-"+stamp+"/frostmail.db"); got != "1 carol@gmail.com\n2 test@mailtest.test\n" {
		t.Errorf("the real accounts' database aside = %q", got)
	}
	if got := readFile(t, root, "var/lib/frostmail/frostmail.db"); got != desktopDB {
		t.Errorf("the target's database = %q", got)
	}
}

func TestGeneratedMailDomains(t *testing.T) {
	for email, generated := range map[string]bool{
		"a@example.com": true, "a@example.org": true, "a@example.net": true, "a@mail.example.com": true,
		"a@mailtest.test": true, "a@x.invalid": true, "a@box.localhost": true, "a@corp.example": true,
		"a@gmail.com": false, "a@notexample.com": false, "a@example.com.evil.io": false, "a@test.io": false,
	} {
		root := stage(t)
		writeFile(t, root, "var/lib/frostmail/frostmail.db", "1 "+email+"\n")
		out, code := move(t, root, "--from", filepath.Join(root, "root/frostmail-export"))
		if generated && code != 0 || !generated && code == 0 {
			t.Errorf("%s: generated=%v but the dry run exited %d:\n%s", email, generated, code, out)
		}
	}
}

func TestAFailedAccountCheckPrintsTheRollbackThatUndoesIt(t *testing.T) {
	root := stage(t)
	before := snapshot(t, root)
	// The export's database lists 1 account where its manifest lists 2.
	writeFile(t, root, "root/frostmail-export/frostmail.db", "1 alice@gmail.com\n")
	out, code := move(t, root, "--from", filepath.Join(root, "root/frostmail-export"), "--apply")
	if code == 0 || !strings.Contains(out, "frostmail.service lists the accounts '1:alice@gmail.com', not the export's '"+manifestAccounts+"'") ||
		!strings.Contains(out, "roll back with: move-mail-to-lodge.sh --rollback "+filepath.Join(root, backup)+" --apply") {
		t.Fatalf("exit %d, without the failure or the rollback:\n%s", code, out)
	}
	afterFail := snapshot(t, root)
	if out, code := move(t, root, "--rollback", filepath.Join(root, backup), "--apply"); code != 0 {
		t.Fatalf("--rollback exited %d:\n%s", code, out)
	}
	// Only the broken export's own file differs, as the test wrote it.
	want := same(before, afterFail)
	if want == "" {
		t.Fatal("the failed move changed nothing")
	}
	if d := same(before, snapshot(t, root)); d != "~root/frostmail-export/frostmail.db" {
		t.Errorf("after the rollback, the machine differs: %s (before the rollback: %s)", d, want)
	}
}

func TestTheDesktopDryRunChangesNothing(t *testing.T) {
	root := stage(t)
	before := snapshot(t, root)
	to := filepath.Join(root, "home/u/export")
	out, code := moveEnv(t, root, desktopEnv(root), "export", "--to", to)
	if code != 0 {
		t.Fatalf("dry run exited %d:\n%s", code, out)
	}
	for _, want := range []string{
		"✓ " + filepath.Join(root, "stub/bin/mailctl") + " has 'export'",
		"✓ the desktop's maild keeps its data in " + filepath.Join(root, "home/u/.local/share/frostmail"),
		"✓ the user unit maild.service is active and enabled",
		"$ systemctl --user disable --now maild.service",
		"$ " + filepath.Join(root, "stub/bin/mailctl") + " export " + to,
		"echo desktop_maild=disabled >> " + to + "/MANIFEST",
		"scp -rp " + to + " root@HOST:/root/",
		"keep this desktop's maild disabled",
		"Dry run: nothing changed.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the dry run's output lacks %q:\n%s", want, out)
		}
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("the dry run changed %s", d)
	}
	if c := changes(t, root); len(c) != 0 {
		t.Errorf("the dry run ran %q", c)
	}
}

func TestTheDesktopApplyStopsMaildExportsAndMarksTheManifest(t *testing.T) {
	root := stage(t)
	to := filepath.Join(root, "home/u/export")
	out, code := moveEnv(t, root, desktopEnv(root), "export", "--to", to, "--apply")
	if code != 0 {
		t.Fatalf("--apply exited %d:\n%s", code, out)
	}
	inOrder(t, changes(t, root), "systemctl --user disable --now maild.service", "mailctl export "+to)
	if got := readFile(t, root, "stub/units/maild.service/active"); got != "inactive\n" {
		t.Errorf("maild.service is %q", got)
	}
	m := readFile(t, root, "home/u/export/MANIFEST")
	if !strings.HasSuffix(m, "secrets=1\ndesktop_maild=disabled\n") {
		t.Errorf("the manifest = %q", m)
	}
	if got := readFile(t, root, "home/u/export/frostmail.db"); got != desktopDB {
		t.Errorf("the export's database = %q", got)
	}
	// The host half takes the export as it is.
	if out, code := move(t, root, "--from", to); code != 0 || strings.Contains(out, "does not say the desktop's maild is disabled") {
		t.Errorf("the host half's dry run on the desktop's export: exit %d\n%s", code, out)
	}
}

func TestAFailedExportPrintsTheCommandsThatPutTheDesktopBack(t *testing.T) {
	root := stage(t)
	before := snapshot(t, root)
	writeFile(t, root, "stub/export-fails", "")
	to := filepath.Join(root, "home/u/export")
	out, code := moveEnv(t, root, desktopEnv(root), "export", "--to", to, "--apply")
	if code == 0 || !strings.Contains(out, "the export stopped; to put this desktop back, run:") ||
		!strings.Contains(out, "  systemctl --user enable --now maild.service\n") || !strings.Contains(out, "  rm -rf "+to+"\n") {
		t.Fatalf("exit %d, without the rollback:\n%s", code, out)
	}
	// Run what it printed: the desktop is as it was.
	if err := os.RemoveAll(to); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "stub/units/maild.service/active", "active\n")
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("after the printed rollback, the machine differs: %s", d)
	}
	// An inactive, disabled unit gets no start or enable line.
	writeFile(t, root, "stub/units/maild.service/active", "inactive\n")
	writeFile(t, root, "stub/units/maild.service/enabled", "disabled\n")
	out, _ = moveEnv(t, root, desktopEnv(root), "export", "--to", to, "--apply")
	if strings.Contains(out, "systemctl --user enable") || strings.Contains(out, "systemctl --user start") {
		t.Errorf("a unit that was off gets a command to put it back:\n%s", out)
	}
}

func TestDesktopRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(t *testing.T, root string)
		want  string
	}{
		"a mailctl without export": {func(t *testing.T, root string) { writeFile(t, root, "stub/old-mailctl", "") }, "has no 'export'"},
		"no database": {func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "home/u/.local/share/frostmail/frostmail.db")); err != nil {
				t.Fatal(err)
			}
		}, "frostmail.db is missing"},
		"the directory exists": {func(t *testing.T, root string) { writeFile(t, root, "home/u/export/x", "") }, "export exists"},
		"a maild that is not the user unit": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/maild.service/LoadState", "not-found\n")
			writeFile(t, root, "stub/maild-answers", "")
		}, "a maild answers, but maild.service is not a user unit here"},
	} {
		t.Run(name, func(t *testing.T) {
			root := stage(t)
			tc.setup(t, root)
			before := snapshot(t, root)
			out, code := moveEnv(t, root, desktopEnv(root), "export", "--to", filepath.Join(root, "home/u/export"), "--apply")
			if code == 0 || !strings.Contains(out, tc.want) || !strings.Contains(out, "nothing changed") {
				t.Fatalf("exit %d, output lacks %q or \"nothing changed\":\n%s", code, tc.want, out)
			}
			if d := same(before, snapshot(t, root)); d != "" {
				t.Errorf("a refusal changed %s", d)
			}
			if c := changes(t, root); len(c) != 0 {
				t.Errorf("a refusal ran %q", c)
			}
		})
	}
	root := stage(t)
	if out, code := moveEnv(t, root, desktopEnv(root), "export", "--apply"); code == 0 || !strings.Contains(out, "--to DIR is required") {
		t.Errorf("no --to: exit %d\n%s", code, out)
	}
}

func TestTheDesktopWithoutAUserUnitAndNoMaildStillExports(t *testing.T) {
	root := stage(t)
	writeFile(t, root, "stub/units/maild.service/LoadState", "not-found\n")
	to := filepath.Join(root, "home/u/export")
	out, code := moveEnv(t, root, desktopEnv(root), "export", "--to", to, "--apply")
	if code != 0 || !strings.Contains(out, "there is no user unit maild.service, and no maild answers: nothing to stop") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if c := changes(t, root); !slices.Equal(c, []string{"mailctl export " + to}) {
		t.Errorf("the commands run = %q", c)
	}
}

func TestTheHalvesRefuseEachOthersArguments(t *testing.T) {
	root := stage(t)
	for _, args := range [][]string{{"export", "--from", "/x"}, {"export", "--set-aside"}, {"--to", "/x"}} {
		if out, code := move(t, root, args...); code != 2 || !strings.Contains(out, "USAGE") && !strings.Contains(out, "Moves the owner's mail") {
			t.Errorf("%q: exit %d\n%s", args, code, out)
		}
	}
	if out, code := move(t, root, "--help"); code != 0 || !strings.Contains(out, "Moves the owner's mail from the desktop's maild to lodge's host") ||
		!strings.Contains(out, "data-after-move.") || strings.Contains(out, "set -euo") {
		t.Errorf("--help: exit %d\n%s", code, out)
	}
}
