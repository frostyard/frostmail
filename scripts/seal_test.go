//go:build linux

package scripts_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The tests of seal-mail-secrets.sh (ADR-0044) run it against a staged
// root: lodge's host with the new frostmail.service, a data directory with
// a plain secrets.json, and stub commands on PATH that play systemctl,
// systemd-creds, runuser, chown and mailctl. The stub `systemctl start
// frostmail.service` plays maild: with FROSTMAIL_SECRETS=sealed and the key
// it moves secrets.json to secrets.sealed; without the key it stays
// inactive; with the rollback's drop-in it leaves the plain file alone.

const (
	sealBackup  = "var/backups/frostmail-seal-" + stamp
	plainSecret = `{"account/1/password":"hunter2"}`
	keyPath     = "etc/credstore.encrypted/frostmail-secrets-key"
	dropInPath  = "etc/systemd/system/frostmail.service.d/50-secrets-file.conf"
)

const sealStubs = `#!/bin/bash
R=$SEAL_ROOT
log() { printf '%s\n' "$*" >>"$R/stub/log"; }
u() { printf '%s' "$R/stub/units/$1"; }
case $(basename "$0") in
systemctl)
	case $1 in
	show)
		# daemon-reload has been run: the rollback's drop-in wins.
		if [ "$3" = Environment ] && [ -e "$R/etc/systemd/system/frostmail.service.d/50-secrets-file.conf" ]; then
			sed 's/FROSTMAIL_SECRETS=sealed/FROSTMAIL_SECRETS=file/' "$(u "$5")/$3"; exit 0
		fi
		cat "$(u "$5")/$3" 2>/dev/null || { [ "$3" = LoadState ] && echo not-found; }; exit 0 ;;
	cat) cat "$(u "$2")/cat" ;;
	is-active) s=$(cat "$(u "$2")/active" 2>/dev/null || echo inactive); echo "$s"; [ "$s" = active ] ;;
	is-enabled) cat "$(u "$2")/enabled" 2>/dev/null || echo disabled ;;
	*) log "systemctl $*"
		cmd=$1; shift
		for unit in "$@"; do
			[ -d "$(u "$unit")" ] || continue
			case $cmd in
			start) echo active >"$(u "$unit")/active" ;;
			stop) echo inactive >"$(u "$unit")/active" ;;
			esac
			if [ "$cmd" = start ] && [ "$unit" = frostmail.service ]; then
				sealed=; grep -q FROSTMAIL_SECRETS=sealed "$(u "$unit")/Environment" && [ ! -e "$R/etc/systemd/system/frostmail.service.d/50-secrets-file.conf" ] && sealed=1
				if [ -n "$sealed" ] && [ ! -f "$R/etc/credstore.encrypted/frostmail-secrets-key" ]; then
					echo inactive >"$(u "$unit")/active"
				elif [ -n "$sealed" ] && [ -f "$R/var/lib/frostmail/secrets.json" ]; then
					mv "$R/var/lib/frostmail/secrets.json" "$R/var/lib/frostmail/secrets.sealed"
				fi
			fi
		done ;;
	esac ;;
systemd-creds)
	[ "$1" = encrypt ] || exit 2
	log "systemd-creds $*"
	[ -e "$R/stub/creds-fails" ] && { echo "creds failed" >&2; exit 1; }
	for last; do :; done
	{ printf 'ENCRYPTED:'; base64 -w0; } >"$last" ;;
runuser)
	user=$2; shift 3; log "runuser $user $*"
	exec "$@" ;;
mailctl)
	[ "$1" = --socket ] && [ "$3 $4" = "account list" ] || exit 2
	[ "$(cat "$(u frostmail.service)/active" 2>/dev/null)" = active ] || { echo "cannot reach maild" >&2; exit 1; }
	echo "ID  EMAIL  IMAP  SMTP"; echo "1  alice@gmail.com  imap.example:993/tls  smtp.example:587/starttls" ;;
*) log "$(basename "$0") $*" ;;
esac
`

const sealedUnitText = "# /usr/lib/systemd/system/frostmail.service\n[Service]\nEnvironment=FROSTMAIL_SECRETS=sealed\nLoadCredentialEncrypted=frostmail-secrets-key:/etc/credstore.encrypted/frostmail-secrets-key\n"

// sealStage builds lodge's host: the new unit, a plain secrets.json, no key.
func sealStage(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	unit := func(name string, props map[string]string) {
		for k, v := range props {
			writeFile(t, root, "stub/units/"+name+"/"+k, v+"\n")
		}
	}
	unit("frostmail.service", map[string]string{"LoadState": "loaded", "active": "active", "enabled": "enabled",
		"Environment": "FROSTMAIL_DATA_DIR=/var/lib/frostmail FROSTMAIL_SOCKET=/run/frostmail/maild.sock FROSTMAIL_SECRETS=sealed",
		"ExecStart":   "{ path=/usr/local/bin/maild ; argv[]=/usr/local/bin/maild --lodge ; ignore_errors=no }",
		"cat":         strings.TrimSuffix(sealedUnitText, "\n")})
	unit("frostmail.socket", map[string]string{"LoadState": "loaded", "active": "active", "enabled": "enabled"})
	writeFile(t, root, "stub/log", "")
	writeFile(t, root, "var/lib/frostmail/frostmail.db", "1 alice@gmail.com\n")
	writeFile(t, root, "var/lib/frostmail/secrets.json", plainSecret)
	if err := os.Chmod(filepath.Join(root, "var/lib/frostmail/secrets.json"), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "stub", "bin")
	writeFile(t, root, "stub/bin/stub", sealStubs)
	if err := os.Chmod(filepath.Join(bin, "stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"systemctl", "systemd-creds", "runuser", "mailctl", "chown"} {
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

// seal runs the script against root and returns its output and exit code.
func seal(t *testing.T, root string, args ...string) (string, int) {
	t.Helper()
	bin := filepath.Join(root, "stub", "bin")
	cmd := exec.Command("bash", append([]string{"seal-mail-secrets.sh"}, args...)...)
	cmd.Env = append(os.Environ(), "SEAL_ROOT="+root, "SEAL_STAMP="+stamp, "PATH="+bin+":"+os.Getenv("PATH"),
		"MAILCTL=", "SYSTEMD_CREDS=", "SYSTEMCTL=", "RUNUSER=", "CHOWN=")
	return runScript(t, cmd)
}

func TestSealDryRunChecksAndChangesNothing(t *testing.T) {
	root := sealStage(t)
	before := snapshot(t, root)
	out, code := seal(t, root)
	if code != 0 {
		t.Fatalf("dry run exited %d:\n%s", code, out)
	}
	for _, want := range []string{
		"✓ frostmail.service is installed",
		"✓ the installed frostmail.service seals the secrets (FROSTMAIL_SECRETS=sealed) and loads the credential frostmail-secrets-key",
		"/var/lib/frostmail/secrets.json present, /var/lib/frostmail/secrets.sealed absent, the key /etc/credstore.encrypted/frostmail-secrets-key absent",
		"$ systemctl stop frostmail.socket frostmail.service",
		"$ cp -a " + filepath.Join(root, "var/lib/frostmail/secrets.json") + " " + filepath.Join(root, sealBackup, "secrets.json"),
		"$ head -c 32 /dev/urandom | systemd-creds encrypt --with-key=auto --name=frostmail-secrets-key - " + filepath.Join(root, keyPath) + ".new",
		"$ systemctl start frostmail.socket frostmail.service",
		"Roll back with:",
		"Dry run: nothing changed. Run again with --apply to seal.",
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
	if exists(root, sealBackup) || exists(root, "etc/credstore.encrypted") {
		t.Error("the dry run made the backup or the key")
	}
}

func TestSealApplyMakesTheKeyTheBackupAndTheManifestAndSeals(t *testing.T) {
	root := sealStage(t)
	out, code := seal(t, root, "--apply")
	if code != 0 {
		t.Fatalf("--apply exited %d:\n%s", code, out)
	}
	key := filepath.Join(root, keyPath)
	fi, err := os.Stat(key)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("the key: %v, %v; want 0600", fi, err)
	}
	if got := readFile(t, root, keyPath); !strings.HasPrefix(got, "ENCRYPTED:") || len(got) < 40 {
		t.Errorf("the key file = %q", got)
	}
	if fi, err := os.Stat(filepath.Dir(key)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("the credstore directory: %v, %v; want 0700", fi, err)
	}
	if exists(root, keyPath+".new") {
		t.Error("the key's temporary file is left")
	}
	if got := readFile(t, root, sealBackup+"/secrets.json"); got != plainSecret {
		t.Errorf("the backup's secrets.json = %q", got)
	}
	if fi, err := os.Stat(filepath.Join(root, sealBackup)); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("the backup directory: %v, %v; want 0700", fi, err)
	}
	m := readFile(t, root, sealBackup+"/manifest")
	for _, want := range []string{
		"target=" + filepath.Join(root, "var/lib/frostmail"), "unit_was=active", "socket_was=active",
		"unit_enabled=enabled", "secrets_json=present", "key_made=1",
	} {
		if !strings.Contains(m, want+"\n") {
			t.Errorf("the manifest lacks %q:\n%s", want, m)
		}
	}
	if exists(root, "var/lib/frostmail/secrets.json") || !exists(root, "var/lib/frostmail/secrets.sealed") {
		t.Error("maild did not seal: secrets.json is there, or secrets.sealed is not")
	}
	inOrder(t, changes(t, root),
		"systemctl stop frostmail.socket frostmail.service",
		"systemd-creds encrypt --with-key=auto --name=frostmail-secrets-key - "+key+".new",
		"systemctl start frostmail.socket frostmail.service",
	)
	for _, want := range []string{
		"✓ frostmail.service is active", "✓ secrets.json is gone and secrets.sealed exists",
		"✓ frostmail.service lists its accounts at /run/frostmail/maild.sock",
		"Roll back with: seal-mail-secrets.sh --rollback " + filepath.Join(root, sealBackup) + " --apply",
		"rm " + filepath.Join(root, sealBackup, "secrets.json"),
		"holds the secrets in plain text",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the output lacks %q:\n%s", want, out)
		}
	}
	// A second run finds it sealed.
	again, code := seal(t, root, "--apply")
	if code != 0 || !strings.Contains(again, "already sealed") {
		t.Errorf("a second run: exit %d\n%s", code, again)
	}
}

func TestSealWithTheKeyAlreadyThereKeepsIt(t *testing.T) {
	root := sealStage(t)
	writeFile(t, root, keyPath, "an existing key")
	out, code := seal(t, root, "--apply")
	if code != 0 {
		t.Fatalf("--apply exited %d:\n%s", code, out)
	}
	if got := readFile(t, root, keyPath); got != "an existing key" {
		t.Errorf("the key = %q", got)
	}
	if !strings.Contains(readFile(t, root, sealBackup+"/manifest"), "key_made=0\n") {
		t.Error("the manifest does not say the key was there")
	}
	for _, c := range changes(t, root) {
		if strings.HasPrefix(c, "systemd-creds") {
			t.Errorf("it made a key that existed: %q", c)
		}
	}
	if !exists(root, "var/lib/frostmail/secrets.sealed") {
		t.Error("secrets.sealed does not exist")
	}
}

func TestSealOfAFreshInstallMakesTheKeyAndNoSealedFileYet(t *testing.T) {
	root := sealStage(t)
	if err := os.Remove(filepath.Join(root, "var/lib/frostmail/secrets.json")); err != nil {
		t.Fatal(err)
	}
	out, code := seal(t, root, "--apply")
	if code != 0 || !strings.Contains(out, "there was no secrets.json") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if !exists(root, keyPath) || exists(root, sealBackup+"/secrets.json") || !strings.Contains(readFile(t, root, sealBackup+"/manifest"), "secrets_json=absent\n") {
		t.Error("the key is missing, or the backup holds a secrets.json, or the manifest is wrong")
	}
}

func TestSealRefusals(t *testing.T) {
	for name, tc := range map[string]struct {
		setup func(t *testing.T, root string)
		want  string
	}{
		"a sealed file and no key": {func(t *testing.T, root string) {
			writeFile(t, root, "var/lib/frostmail/secrets.sealed", "FMSEAL01...")
		}, "the key that sealed it is gone"},
		"the unit not updated": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/frostmail.service/Environment", "FROSTMAIL_DATA_DIR=/var/lib/frostmail FROSTMAIL_SECRETS=file\n")
			writeFile(t, root, "stub/units/frostmail.service/cat", "[Service]\nEnvironment=FROSTMAIL_SECRETS=file\n")
		}, "install the new packaging first"},
		"the credential not loaded": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/frostmail.service/cat", "[Service]\nEnvironment=FROSTMAIL_SECRETS=sealed\n")
		}, "install the new packaging first"},
		"a daemon reload needed": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/frostmail.service/NeedDaemonReload", "yes\n")
		}, "run systemctl daemon-reload first"},
		"the unit not installed": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/frostmail.service/LoadState", "not-found\n")
		}, "frostmail.service is not installed"},
		"a backup that exists": {func(t *testing.T, root string) {
			writeFile(t, root, sealBackup+"/x", "")
		}, "frostmail-seal-" + stamp + " exists"},
		"the rollback's drop-in": {func(t *testing.T, root string) {
			writeFile(t, root, "stub/units/frostmail.service/Environment", "FROSTMAIL_SECRETS=file\n")
			writeFile(t, root, dropInPath, "[Service]\n")
		}, "because of the drop-in"},
	} {
		t.Run(name, func(t *testing.T) {
			root := sealStage(t)
			tc.setup(t, root)
			before := snapshot(t, root)
			backups := exists(root, sealBackup)
			for _, args := range [][]string{nil, {"--apply"}} {
				out, code := seal(t, root, args...)
				if code == 0 || !strings.Contains(out, tc.want) || !strings.Contains(out, "nothing changed") {
					t.Fatalf("%v: exit %d, output lacks %q or \"nothing changed\":\n%s", args, code, tc.want, out)
				}
				if d := same(before, snapshot(t, root)); d != "" {
					t.Errorf("a refusal changed %s", d)
				}
				if c := changes(t, root); len(c) != 0 {
					t.Errorf("a refusal ran %q", c)
				}
				if !backups && exists(root, sealBackup) {
					t.Error("a refusal made the backup")
				}
			}
		})
	}
}

func TestSealAlreadySealedChangesNothing(t *testing.T) {
	root := sealStage(t)
	writeFile(t, root, keyPath, "key")
	writeFile(t, root, "var/lib/frostmail/secrets.sealed", "FMSEAL01...")
	if err := os.Remove(filepath.Join(root, "var/lib/frostmail/secrets.json")); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, root)
	for _, args := range [][]string{nil, {"--apply"}} {
		out, code := seal(t, root, args...)
		if code != 0 || !strings.Contains(out, "already sealed") {
			t.Fatalf("%v: exit %d\n%s", args, code, out)
		}
	}
	if d := same(before, snapshot(t, root)); d != "" {
		t.Errorf("it changed %s", d)
	}
	if c := changes(t, root); len(c) != 0 || exists(root, sealBackup) {
		t.Errorf("it ran %q, or made a backup", c)
	}
}

func TestSealRollbackPutsThePlainFileBackAndASecondOneIsRefused(t *testing.T) {
	root := sealStage(t)
	if out, code := seal(t, root, "--apply"); code != 0 {
		t.Fatalf("--apply exited %d:\n%s", code, out)
	}
	// A secret the server stored after the seal.
	writeFile(t, root, "var/lib/frostmail/secrets.sealed", "sealed, with a newer token")
	rb := filepath.Join(root, sealBackup)
	n := len(changes(t, root))

	dry, code := seal(t, root, "--rollback", rb)
	if code != 0 || !strings.Contains(dry, "Dry run: nothing changed") || exists(root, dropInPath) || exists(root, "var/lib/frostmail/secrets.json") {
		t.Fatalf("the rollback's dry run: exit %d\n%s", code, dry)
	}

	out, code := seal(t, root, "--rollback", rb, "--apply")
	if code != 0 {
		t.Fatalf("--rollback exited %d:\n%s", code, out)
	}
	if got := readFile(t, root, "var/lib/frostmail/secrets.json"); got != plainSecret {
		t.Errorf("secrets.json = %q", got)
	}
	if fi, err := os.Stat(filepath.Join(root, "var/lib/frostmail/secrets.json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("secrets.json: %v, %v; want 0600", fi, err)
	}
	if exists(root, "var/lib/frostmail/secrets.sealed") {
		t.Error("secrets.sealed is still in the data directory")
	}
	if got := readFile(t, root, sealBackup+"/secrets.sealed-after-seal"); got != "sealed, with a newer token" {
		t.Errorf("secrets.sealed-after-seal = %q: the sealed file must be kept, never deleted", got)
	}
	if got := readFile(t, root, dropInPath); got != "[Service]\nEnvironment=FROSTMAIL_SECRETS=file\nLoadCredentialEncrypted=\n" {
		t.Errorf("the drop-in = %q", got)
	}
	if !exists(root, keyPath) {
		t.Error("the key is gone")
	}
	if !strings.Contains(readFile(t, root, sealBackup+"/manifest"), "\nrolled_back="+stamp+"\n") {
		t.Error("the manifest does not say the seal was rolled back")
	}
	inOrder(t, changes(t, root)[n:],
		"systemctl stop frostmail.socket frostmail.service",
		"chown frostmail:frostmail "+filepath.Join(root, "var/lib/frostmail/secrets.json"),
		"systemctl daemon-reload",
		"systemctl start frostmail.socket frostmail.service",
	)
	// The maild of the stub leaves the plain file alone under the drop-in.
	if !exists(root, "var/lib/frostmail/secrets.json") {
		t.Error("the restarted maild sealed the file again")
	}
	for _, want := range []string{"The key /etc/credstore.encrypted/frostmail-secrets-key stays", "secrets.sealed-after-seal", "Sign In Again", "To seal again later"} {
		if !strings.Contains(out, want) {
			t.Errorf("the rollback's output lacks %q:\n%s", want, out)
		}
	}
	if out, code := seal(t, root, "--rollback", rb, "--apply"); code == 0 || !strings.Contains(out, "rolled back already") {
		t.Errorf("a second rollback: exit %d:\n%s", code, out)
	}
	// While the drop-in is there, a new seal says to remove it.
	if out, code := seal(t, root); code == 0 || !strings.Contains(out, "because of the drop-in") {
		t.Errorf("a seal under the drop-in: exit %d:\n%s", code, out)
	}
}

func TestSealThatFailsPrintsTheRollbackThatUndoesIt(t *testing.T) {
	root := sealStage(t)
	before := snapshot(t, root)
	writeFile(t, root, "stub/creds-fails", "")
	out, code := seal(t, root, "--apply")
	rb := filepath.Join(root, sealBackup)
	if code == 0 || !strings.Contains(out, "roll back with: seal-mail-secrets.sh --rollback "+rb+" --apply") {
		t.Fatalf("exit %d, without the rollback:\n%s", code, out)
	}
	if got := readFile(t, root, "var/lib/frostmail/secrets.json"); got != plainSecret {
		t.Errorf("secrets.json = %q", got)
	}
	if exists(root, keyPath) {
		t.Error("a key exists after a failed encrypt")
	}
	if out, code := seal(t, root, "--rollback", rb, "--apply"); code != 0 {
		t.Fatalf("--rollback exited %d:\n%s", code, out)
	}
	// Only what the rollback adds differs: the drop-in, the credstore directory, and no sealed file.
	if d := same(before, snapshot(t, root)); d != "+etc/ +etc/credstore.encrypted/ +etc/systemd/ +etc/systemd/system/ +etc/systemd/system/frostmail.service.d/ +etc/systemd/system/frostmail.service.d/50-secrets-file.conf" {
		t.Errorf("after the rollback, the machine differs: %s", d)
	}
	if got := readFile(t, root, "stub/units/frostmail.service/active"); got != "active\n" {
		t.Errorf("frostmail.service is %q", got)
	}
}

func TestSealHelpAndArguments(t *testing.T) {
	root := sealStage(t)
	if out, code := seal(t, root, "--help"); code != 0 || !strings.Contains(out, "Seals the secrets of frostmail's maild") ||
		!strings.Contains(out, "--rollback") || !strings.Contains(out, "To seal again") || strings.Contains(out, "set -euo") {
		t.Errorf("--help: exit %d\n%s", code, out)
	}
	if _, code := seal(t, root, "--nope"); code != 2 {
		t.Errorf("an unknown argument: exit %d", code)
	}
}

// runScript runs cmd in the scripts directory and returns its output and exit code.
func runScript(t *testing.T, cmd *exec.Cmd) (string, int) {
	t.Helper()
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
