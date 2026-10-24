#!/usr/bin/env bash
# Seals the secrets of frostmail's maild on lodge's host (ADR-0044, docs/plans/0011-m8-mail-in-lodge.md,
# Phase 7). maild keeps them in secrets.sealed, under a key that is a systemd credential; this script
# makes the key and lets maild move a plain secrets.json in. It defaults to a dry run: it checks and
# prints the plan, and changes nothing.
#
#   scripts/seal-mail-secrets.sh [--apply]                       # on lodge's host, as root
#   scripts/seal-mail-secrets.sh --rollback /var/backups/frostmail-seal-<stamp> [--apply]
#
# It needs the new packaging installed first: frostmail.service with FROSTMAIL_SECRETS=sealed and
# LoadCredentialEncrypted=frostmail-secrets-key (packaging/lodge), and systemctl daemon-reload run.
# The unit must load the credential in effect: the script asks systemd (busctl), so that a later
# drop-in which resets the credential is found.
#
# --apply and --rollback take a host-wide lock (/run/lock/frostmail-seal.lock): 1 run at a time.
#
# What --apply does, in order, printing each command first:
#   1. stops frostmail.socket and frostmail.service, then looks at secrets.json, secrets.sealed and
#      the key again: what it finds after the stop is what it acts on;
#   2. makes /var/backups/frostmail-seal-<stamp> (0700), copies secrets.json there, and writes a
#      manifest that --rollback reads;
#   3. when /etc/credstore.encrypted/frostmail-secrets-key does not exist, makes it: 32 random bytes
#      sealed by systemd-creds (--with-key=auto: the TPM2 and the host key, as the machine has them).
#      It never replaces a key that exists, even one that appears while it runs;
#   4. starts the socket and the service: maild moves secrets.json into secrets.sealed and removes it;
#   5. checks that secrets.json is gone, that secrets.sealed exists (when secrets.json held a secret),
#      and that maild lists its accounts.
#
# The backup's secrets.json holds the secrets in plain text: remove it once mail works.
# It refuses any state it cannot read unambiguously, before it changes anything: among them, a
# secrets.json and a secrets.sealed together. It never deletes secrets.json (maild does), the key,
# or secrets.sealed.
#
# --rollback puts the plain file back: it stops the units, sets secrets.sealed aside in the backup (as
# secrets.sealed-after-seal), puts the backup's secrets.json back, and writes the drop-in
# /etc/systemd/system/frostmail.service.d/zz-frostmail-secrets-file.conf (FROSTMAIL_SECRETS=file, no
# credential; its name sorts after other drop-ins). After daemon-reload it checks that systemd
# really has FROSTMAIL_SECRETS=file and no credential, then starts the units as they were, and
# checks that maild answers. The key stays. Secrets that changed after the seal (a new sign-in, a
# rotated refresh token) are only in secrets.sealed-after-seal: those accounts can need Sign In
# Again. To seal again later: remove the drop-in, systemctl daemon-reload, and run this script again.
set -euo pipefail

# Tests stage a root and stub commands (scripts/seal_test.go); on a real machine all of these are
# empty or the system's own.
ROOT=${SEAL_ROOT:-}
SYSTEMCTL=${SYSTEMCTL:-systemctl}
SYSTEMD_CREDS=${SYSTEMD_CREDS:-systemd-creds}
BUSCTL=${BUSCTL:-busctl}
RUNUSER=${RUNUSER:-runuser}
CHOWN=${CHOWN:-chown}
FLOCK=${FLOCK:-flock}
MAILCTL=${MAILCTL:-}
STAMP=${SEAL_STAMP:-$(date -u +%Y%m%dT%H%M%SZ)}

UNIT=frostmail.service
SOCKET_UNIT=frostmail.socket
APP_USER=frostmail
CRED=frostmail-secrets-key
CRED_DIR=/etc/credstore.encrypted
DROPIN_DIR=/etc/systemd/system/frostmail.service.d
DROPIN=$DROPIN_DIR/zz-frostmail-secrets-file.conf
LOCK=/run/lock/frostmail-seal.lock
BUS_NAME=org.freedesktop.systemd1
UNIT_OBJECT=/org/freedesktop/systemd1/unit/frostmail_2eservice

mode=dry-run
rollback=

usage() {
	sed -n '2,41p' "$0" | sed 's/^# \{0,1\}//'
	exit "${1:-0}"
}

while [ $# -gt 0 ]; do
	case $1 in
	--dry-run) mode=dry-run ;;
	--apply) mode=apply ;;
	--rollback) rollback=${2:?--rollback needs a backup directory}; shift ;;
	--rollback=*) rollback=${1#--rollback=} ;;
	-h | --help) usage 0 ;;
	*) echo "seal-mail-secrets: unknown argument $1" >&2; usage 2 ;;
	esac
	shift
done

p() { printf '%s%s' "$ROOT" "$1"; }
say() { printf '%s\n' "$*"; }
step() { printf '\n== %s\n' "$*"; }
fail() { printf 'seal-mail-secrets: %s\n' "$*" >&2; exit 1; }
# run prints a command that changes the machine, and runs it with --apply.
run() {
	printf '  $ %s\n' "$*"
	if [ "$mode" = apply ]; then "$@"; fi
}
as() { local u=$1; shift; "$RUNUSER" -u "$u" -- "$@"; }
show() { "$SYSTEMCTL" show -p "$2" --value "$1" 2>/dev/null || true; }
# envof prints variable $2 of unit $1's Environment=, or nothing; the last assignment wins.
envof() {
	local kv v=
	for kv in $(show "$1" Environment); do
		case $kv in "$2"=*) v=${kv#*=} ;; esac
	done
	printf '%s' "$v"
}
# credprop prints what systemd has for LoadCredentialEncrypted= of the unit, as busctl shows it:
# a(ss) N "ID" "PATH" ... (systemctl show prints [unprintable] for it).
credprop() {
	"$BUSCTL" get-property "$BUS_NAME" "$UNIT_OBJECT" org.freedesktop.systemd1.Service LoadCredentialEncrypted 2>/dev/null | tr -s ' \t' ' ' || true
}
# nocred says whether systemd has no credential to load for the unit.
nocred() { [ "$(credprop)" = 'a(ss) 0' ]; }
# loadscred says whether systemd loads our credential from our file.
loadscred() { grep -qF "\"$CRED\" \"$CRED_DIR/$CRED\"" <<<"$(credprop)"; }
# start_as_was $1 $2 starts the units as they were: unit_was $1, socket_was $2.
start_as_was() {
	if [ "$1" = active ]; then
		"$SYSTEMCTL" start "$SOCKET_UNIT" "$UNIT"
	elif [ "$2" = active ]; then
		"$SYSTEMCTL" start "$SOCKET_UNIT"
	fi
}
# wait_answering waits for the unit, and then for mailctl $1 to list accounts at the socket $2.
wait_answering() {
	local mailctl=$1 sock=$2 _
	for _ in $(seq 60); do
		[ "$("$SYSTEMCTL" is-active "$UNIT" 2>/dev/null)" = active ] && break
		sleep 0.5
	done
	[ "$("$SYSTEMCTL" is-active "$UNIT" 2>/dev/null)" = active ] || fail "$UNIT did not start: journalctl -u $UNIT"
	ok "$UNIT is active"
	for _ in $(seq 30); do
		if now_out=$(as "$APP_USER" "$mailctl" --socket "$sock" account list 2>/dev/null); then break; fi
		now_out=
		sleep 0.5
	done
	[ -n "${now_out:-}" ] || fail "$UNIT does not answer at $sock: journalctl -u $UNIT"
	ok "$UNIT lists its accounts at $sock"
}
# holds_secret says whether the plain secrets file $1 holds a secret: more than {}.
holds_secret() { [ "$(tr -d ' \t\r\n' <"$1")" != '{}' ]; }

problems=()
problem() { problems+=("$*"); say "  ✗ $*"; }
ok() { say "  ✓ $*"; }
nothing_changed() {
	if [ ${#problems[@]} -gt 0 ]; then
		printf '\nseal-mail-secrets: %d problem(s); nothing changed.\n' ${#problems[@]} >&2
		exit 1
	fi
}

if [ -z "$ROOT" ] && [ "$(id -u)" -ne 0 ]; then
	fail "run me as root on lodge's host (sudo $0 …)"
fi

# One run at a time on this host: 2 runs could both see no key, and the second would replace the
# key that maild already used.
if [ "$mode" = apply ]; then
	lockfile=$(p "$LOCK")
	mkdir -p "$(dirname "$lockfile")"
	exec 9>"$lockfile"
	"$FLOCK" -n 9 || fail "another run of this script holds the lock $LOCK: wait for it to end"
fi

keyfile=$(p "$CRED_DIR/$CRED")
dropin=$(p "$DROPIN")

# ---- rollback -------------------------------------------------------------

if [ -n "$rollback" ]; then
	manifest=$rollback/manifest
	[ -f "$manifest" ] || fail "$manifest is missing: is $rollback a backup of this script?"
	declare -A m
	while IFS='=' read -r k v; do [ -n "$k" ] && m[$k]=$v; done <"$manifest"
	[ -z "${m[rolled_back]:-}" ] || fail "$rollback was rolled back already (${m[rolled_back]})"
	for k in target secrets_json unit_was socket_was; do
		[ -n "${m[$k]:-}" ] || fail "$manifest has no $k: it is not whole; nothing changed"
	done
	target=${m[target]}
	step "Rollback of the seal recorded in $rollback ($mode)"
	[ ! -e "$rollback/secrets.sealed-after-seal" ] || fail "$rollback/secrets.sealed-after-seal exists: a rollback stopped half way; finish it by hand from the manifest"
	[ ! -e "$dropin" ] || fail "$dropin exists: remove it, or compare it with this rollback, before another"
	command -v "$BUSCTL" >/dev/null 2>&1 || fail "$BUSCTL does not exist: the rollback needs it to check what systemd loads; nothing changed"
	if [ "${m[secrets_json]}" = present ]; then
		[ -f "$rollback/secrets.json" ] || fail "$rollback/secrets.json is missing: the backup is not whole"
		if [ -e "$target/secrets.json" ] && ! cmp -s "$rollback/secrets.json" "$target/secrets.json"; then
			fail "$target/secrets.json exists and differs from the backup's: this rollback would overwrite it"
		fi
	fi
	if [ "$mode" = apply ]; then
		trap '[ $? -eq 0 ] || printf "seal-mail-secrets: the rollback stopped at the command above; what is left is in %s\n" "$rollback" >&2' EXIT
	fi
	run "$SYSTEMCTL" stop "$SOCKET_UNIT" "$UNIT"
	if [ -e "$target/secrets.sealed" ]; then
		run mv "$target/secrets.sealed" "$rollback/secrets.sealed-after-seal"
	fi
	if [ "${m[secrets_json]}" = present ]; then
		# After a seal that stopped before maild moved the file, it is still there.
		if [ -e "$target/secrets.json" ]; then
			say "  $target/secrets.json is in place already, as the backup's"
		else
			run cp -a "$rollback/secrets.json" "$target/secrets.json"
		fi
		run "$CHOWN" "$APP_USER:$APP_USER" "$target/secrets.json"
		run chmod 0600 "$target/secrets.json"
	fi
	run install -d -m 0755 "$(p "$DROPIN_DIR")"
	printf '  $ write %s: [Service], Environment=FROSTMAIL_SECRETS=file, LoadCredentialEncrypted= (resets the list)\n' "$dropin"
	if [ "$mode" = apply ]; then
		printf '[Service]\nEnvironment=FROSTMAIL_SECRETS=file\nLoadCredentialEncrypted=\n' >"$dropin"
	fi
	run "$SYSTEMCTL" daemon-reload
	if [ "$mode" = apply ]; then
		# A later drop-in (override.conf, a package's) can set the old values again.
		now_secrets=$(envof "$UNIT" FROSTMAIL_SECRETS)
		if [ "$now_secrets" != file ] || ! nocred; then
			fail "systemd still has FROSTMAIL_SECRETS='${now_secrets}' or a credential to load, though $dropin sets file: another drop-in wins (DropInPaths: $(show "$UNIT" DropInPaths)). The units are stopped and the files are restored: fix the drop-ins, systemctl daemon-reload, then systemctl start $SOCKET_UNIT $UNIT"
		fi
		ok "systemd has FROSTMAIL_SECRETS=file and no credential to load"
	else
		say "  then check that systemd has FROSTMAIL_SECRETS=file and no credential to load for $UNIT"
	fi
	if [ "${m[unit_was]}" = active ]; then
		run "$SYSTEMCTL" start "$SOCKET_UNIT" "$UNIT"
	elif [ "${m[socket_was]}" = active ]; then
		run "$SYSTEMCTL" start "$SOCKET_UNIT"
	fi
	if [ "$mode" = apply ]; then
		if [ "${m[unit_was]}" = active ]; then
			wait_answering "${m[mailctl]:-$(command -v mailctl || true)}" "${m[app_socket]:-/run/frostmail/maild.sock}"
		fi
		printf 'rolled_back=%s\n' "$STAMP" >>"$manifest"
	fi
	say "  The key $CRED_DIR/$CRED stays: it opens secrets.sealed-after-seal again, and a new seal reuses it."
	say "  ! Secrets that changed after the seal (a new sign-in, a rotated refresh token) are only in"
	say "    $rollback/secrets.sealed-after-seal: those accounts can need Sign In Again."
	say "  To seal again later: rm $DROPIN; systemctl daemon-reload; run $0 again."
	[ "$mode" = apply ] || say $'\nDry run: nothing changed. Run again with --apply to roll back.'
	exit 0
fi

# ---- what is there --------------------------------------------------------

# inspect reads the 3 files that decide the seal; apply reads them again after the units stop.
inspect() {
	has_json=0; has_sealed=0; has_key=0; has_secret=0
	[ ! -f "$(p "$target")/secrets.json" ] || has_json=1
	[ ! -f "$(p "$target")/secrets.sealed" ] || has_sealed=1
	[ ! -f "$keyfile" ] || has_key=1
	if [ $has_json = 1 ] && holds_secret "$(p "$target")/secrets.json"; then has_secret=1; fi
}
yn() { if [ "$1" = 1 ]; then printf present; else printf absent; fi; }
# state_problems names a state that this script refuses.
state_problems() {
	if [ $has_json = 1 ] && [ $has_sealed = 1 ]; then
		problem "$target/secrets.json and $target/secrets.sealed both exist: with the sealed unit installed, start $UNIT and maild moves secrets.json in by itself; or decide by hand which of the two holds the secrets"
	fi
	if [ $has_sealed = 1 ] && [ $has_key = 0 ]; then
		problem "$target/secrets.sealed exists and the key $CRED_DIR/$CRED does not: the key that sealed it is gone. Moving secrets.sealed aside makes every account sign in again; decide that yourself"
	fi
}

step "Checks"

if [ "$(show "$UNIT" LoadState)" = loaded ]; then
	ok "$UNIT is installed"
else
	problem "$UNIT is not installed (packaging/lodge/)"
fi
if [ "$(show "$UNIT" NeedDaemonReload)" = yes ]; then
	problem "$UNIT changed on disk and systemd has not read it: run systemctl daemon-reload first"
fi
unit_was=$("$SYSTEMCTL" is-active "$UNIT" 2>/dev/null || true)
socket_was=$("$SYSTEMCTL" is-active "$SOCKET_UNIT" 2>/dev/null || true)
unit_enabled=$("$SYSTEMCTL" is-enabled "$UNIT" 2>/dev/null || true)
target=$(envof "$UNIT" FROSTMAIL_DATA_DIR)
target=${target:-/var/lib/frostmail}
app_socket=$(envof "$UNIT" FROSTMAIL_SOCKET)
app_socket=${app_socket:-/run/frostmail/maild.sock}
secrets_mode=$(envof "$UNIT" FROSTMAIL_SECRETS)
exec_start=$(show "$UNIT" ExecStart | sed -n 's/.*path=\([^ ;]*\).*/\1/p')
say "    $UNIT runs ${exec_start:-?} as $APP_USER, data $target, private socket $app_socket"
if ! command -v "$BUSCTL" >/dev/null 2>&1; then
	problem "$BUSCTL does not exist: it is needed to read the credential systemd loads for $UNIT"
elif [ "$secrets_mode" = sealed ] && loadscred; then
	ok "systemd has FROSTMAIL_SECRETS=sealed for $UNIT, and it loads the credential $CRED from $CRED_DIR/$CRED"
elif [ -e "$dropin" ]; then
	problem "$UNIT has FROSTMAIL_SECRETS='${secrets_mode}' or no credential because of the drop-in $DROPIN (a rollback wrote it): remove it and run systemctl daemon-reload"
else
	problem "systemd does not load the credential for $UNIT: it needs FROSTMAIL_SECRETS=sealed (now '${secrets_mode}') and LoadCredentialEncrypted=$CRED:$CRED_DIR/$CRED in effect (now: $(credprop)); a later drop-in can reset it (DropInPaths: $(show "$UNIT" DropInPaths)). Install the new packaging first (packaging/lodge/frostmail.service), then systemctl daemon-reload"
fi
if command -v "$SYSTEMD_CREDS" >/dev/null 2>&1; then
	ok "$SYSTEMD_CREDS exists"
else
	problem "$SYSTEMD_CREDS does not exist: systemd 250 or later is needed"
fi

mailctl=$MAILCTL
if [ -z "$mailctl" ] && [ -n "$exec_start" ] && [ -x "$(p "$(dirname "$exec_start")/mailctl")" ]; then
	mailctl=$(p "$(dirname "$exec_start")/mailctl")
fi
[ -n "$mailctl" ] || mailctl=$(command -v mailctl || true)
if [ -z "$mailctl" ]; then problem "no mailctl beside ${exec_start:-maild} or on PATH: install the one maild came with"; fi

inspect
say "    $target/secrets.json $(yn $has_json), $target/secrets.sealed $(yn $has_sealed), the key $CRED_DIR/$CRED $(yn $has_key)"
state_problems
if [ ${#problems[@]} -eq 0 ] && [ $has_json = 0 ] && [ $has_key = 1 ]; then
	if [ $has_sealed = 1 ]; then
		say "  ✓ already sealed: $target/secrets.sealed and the key exist, and there is no plain secrets.json"
	else
		say "  ✓ nothing to seal: the key exists, and no secrets.json: maild seals the first secret it stores"
	fi
	say $'\nNothing changed.'
	exit 0
fi
backup=$(p /var/backups)/frostmail-seal-$STAMP
if [ -e "$backup" ]; then problem "$backup exists: choose another SEAL_STAMP, or clear it yourself"; fi
nothing_changed

# ---- the seal -------------------------------------------------------------

step "Plan ($mode); backup and manifest in $backup"
restored=
record() { if [ "$mode" = apply ]; then printf '%s=%s\n' "$1" "$2" >>"$backup/manifest"; fi; }
# shellcheck disable=SC2329  # called by the EXIT trap below
seal_stopped() {
	[ "$1" -eq 0 ] || [ -n "$restored" ] && return 0
	if [ -f "$backup/manifest" ]; then
		printf 'seal-mail-secrets: the seal stopped at the command above; roll back with: %s --rollback %s --apply\n' "$0" "$backup" >&2
	else
		printf 'seal-mail-secrets: it stopped before the backup was made; nothing else changed: start the units again: systemctl start %s %s\n' "$SOCKET_UNIT" "$UNIT" >&2
	fi
}
if [ "$mode" = apply ]; then trap 'seal_stopped $?' EXIT; fi

step "1. Stop mail"
# The socket first: a connection to it would start maild while its files change.
run "$SYSTEMCTL" stop "$SOCKET_UNIT" "$UNIT"
if [ "$mode" = apply ]; then
	# What was true at the checks may not be true now: act on what is true after the stop.
	inspect
	say "    after the stop: $target/secrets.json $(yn $has_json), $target/secrets.sealed $(yn $has_sealed), the key $CRED_DIR/$CRED $(yn $has_key)"
	before=${#problems[@]}
	state_problems
	if [ ${#problems[@]} -gt "$before" ]; then
		restored=1
		start_as_was "$unit_was" "$socket_was" || true
		fail "the files changed while the units stopped, to a state this script refuses (above). The units are started again as they were; nothing else changed"
	fi
fi

step "2. The backup"
run mkdir -p -m 0700 "$backup"
record target "$(p "$target")"
record unit_was "$unit_was"
record socket_was "$socket_was"
record unit_enabled "$unit_enabled"
record app_socket "$app_socket"
record mailctl "$mailctl"
if [ $has_json = 1 ]; then
	run cp -a "$(p "$target")/secrets.json" "$backup/secrets.json"
	record secrets_json present
else
	record secrets_json absent
fi
if [ $has_key = 0 ]; then record key_made 1; else record key_made 0; fi

step "3. The key"
if [ $has_key = 1 ]; then
	ok "the key $CRED_DIR/$CRED exists: kept as it is"
else
	run install -d -m 0700 "$(p "$CRED_DIR")"
	# Into a new name first: a partial file must never pass for the key.
	printf '  $ head -c 32 /dev/urandom | %s encrypt --with-key=auto --name=%s - %s\n' "$SYSTEMD_CREDS" "$CRED" "$keyfile.new"
	if [ "$mode" = apply ]; then
		rm -f "$keyfile.new"
		head -c 32 /dev/urandom | "$SYSTEMD_CREDS" encrypt --with-key=auto --name="$CRED" - "$keyfile.new"
	fi
	run chmod 0600 "$keyfile.new"
	# A hard link fails when the key exists: a key that appeared meanwhile is never replaced.
	printf '  $ ln %s %s\n' "$keyfile.new" "$keyfile"
	if [ "$mode" = apply ]; then
		ln "$keyfile.new" "$keyfile" || { rm -f "$keyfile.new"; fail "the key $CRED_DIR/$CRED appeared while this script made its own: it is not replaced. Check who made it, then roll back or run again"; }
	fi
	run rm -f "$keyfile.new"
fi

step "4. Start mail"
run "$SYSTEMCTL" start "$SOCKET_UNIT" "$UNIT"

step "5. Checks"
if [ "$mode" = apply ]; then
	for _ in $(seq 60); do
		[ "$("$SYSTEMCTL" is-active "$UNIT" 2>/dev/null)" = active ] && break
		sleep 0.5
	done
	[ "$("$SYSTEMCTL" is-active "$UNIT" 2>/dev/null)" = active ] || fail "$UNIT did not start: journalctl -u $UNIT"
	ok "$UNIT is active"
	# maild is active before it opens its secrets: wait for it to seal the file. A file with no secret
	# ({}) is removed and makes no secrets.sealed.
	for _ in $(seq 60); do
		if [ ! -e "$(p "$target")/secrets.json" ] && { [ $has_secret = 0 ] || [ -f "$(p "$target")/secrets.sealed" ]; }; then break; fi
		sleep 0.5
	done
	[ ! -e "$(p "$target")/secrets.json" ] || fail "$target/secrets.json is still there: maild did not seal it: journalctl -u $UNIT"
	if [ $has_secret = 1 ]; then
		[ -f "$(p "$target")/secrets.sealed" ] || fail "$target/secrets.sealed does not exist: journalctl -u $UNIT"
		ok "secrets.json is gone and secrets.sealed exists"
	elif [ $has_json = 1 ]; then
		ok "secrets.json held no secret and is gone; maild seals the first secret it stores"
	else
		ok "there was no secrets.json; maild seals the first secret it stores"
	fi
	for _ in $(seq 30); do
		if now_out=$(as "$APP_USER" "$mailctl" --socket "$app_socket" account list 2>/dev/null); then break; fi
		now_out=
		sleep 0.5
	done
	[ -n "${now_out:-}" ] || fail "$UNIT does not answer at $app_socket: journalctl -u $UNIT"
	ok "$UNIT lists its accounts at $app_socket"
else
	say "  then check that $target/secrets.json is gone, that $target/secrets.sealed exists (when secrets.json holds a secret: $(if [ $has_secret = 1 ]; then echo it does; else echo it does not; fi)), and that $UNIT lists its accounts"
fi

step "Done"
say "Roll back with: $0 --rollback $backup --apply"
if [ $has_json = 1 ]; then
	say "! $backup/secrets.json holds the secrets in plain text. When mail works, remove it:"
	say "    rm $backup/secrets.json"
fi
[ "$mode" = apply ] || say $'\nDry run: nothing changed. Run again with --apply to seal.'
