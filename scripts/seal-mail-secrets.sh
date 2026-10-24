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
#
# What --apply does, in order, printing each command first:
#   1. stops frostmail.socket and frostmail.service;
#   2. makes /var/backups/frostmail-seal-<stamp> (0700), copies secrets.json there, and writes a
#      manifest that --rollback reads;
#   3. when /etc/credstore.encrypted/frostmail-secrets-key does not exist, makes it: 32 random bytes
#      sealed by systemd-creds (--with-key=auto: the TPM2 and the host key, as the machine has them);
#   4. starts the socket and the service: maild moves secrets.json into secrets.sealed and removes it;
#   5. checks that secrets.json is gone, that secrets.sealed exists, and that maild lists its accounts.
#
# The backup's secrets.json holds the secrets in plain text: remove it once mail works.
# It refuses any state it cannot read unambiguously, before it changes anything. It never deletes
# secrets.json (maild does), the key, or secrets.sealed.
#
# --rollback puts the plain file back: it stops the units, sets secrets.sealed aside in the backup (as
# secrets.sealed-after-seal), puts the backup's secrets.json back, and writes the drop-in
# /etc/systemd/system/frostmail.service.d/50-secrets-file.conf (FROSTMAIL_SECRETS=file, no
# credential). The key stays. Secrets that changed after the seal (a new sign-in, a rotated refresh
# token) are only in secrets.sealed-after-seal: those accounts can need Sign In Again. To seal again
# later: remove the drop-in, systemctl daemon-reload, and run this script again.
set -euo pipefail

# Tests stage a root and stub commands (scripts/seal_test.go); on a real machine all of these are
# empty or the system's own.
ROOT=${SEAL_ROOT:-}
SYSTEMCTL=${SYSTEMCTL:-systemctl}
SYSTEMD_CREDS=${SYSTEMD_CREDS:-systemd-creds}
RUNUSER=${RUNUSER:-runuser}
CHOWN=${CHOWN:-chown}
MAILCTL=${MAILCTL:-}
STAMP=${SEAL_STAMP:-$(date -u +%Y%m%dT%H%M%SZ)}

UNIT=frostmail.service
SOCKET_UNIT=frostmail.socket
APP_USER=frostmail
CRED=frostmail-secrets-key
CRED_DIR=/etc/credstore.encrypted
DROPIN_DIR=/etc/systemd/system/frostmail.service.d
DROPIN=$DROPIN_DIR/50-secrets-file.conf

mode=dry-run
rollback=

usage() {
	sed -n '2,31p' "$0" | sed 's/^# \{0,1\}//'
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

keyfile=$(p "$CRED_DIR/$CRED")
dropin=$(p "$DROPIN")

# ---- rollback -------------------------------------------------------------

if [ -n "$rollback" ]; then
	manifest=$rollback/manifest
	[ -f "$manifest" ] || fail "$manifest is missing: is $rollback a backup of this script?"
	declare -A m
	while IFS='=' read -r k v; do [ -n "$k" ] && m[$k]=$v; done <"$manifest"
	[ -z "${m[rolled_back]:-}" ] || fail "$rollback was rolled back already (${m[rolled_back]})"
	[ -n "${m[target]:-}" ] || fail "$manifest has no target"
	target=${m[target]}
	step "Rollback of the seal recorded in $rollback ($mode)"
	[ ! -e "$rollback/secrets.sealed-after-seal" ] || fail "$rollback/secrets.sealed-after-seal exists: a rollback stopped half way; finish it by hand from the manifest"
	[ ! -e "$dropin" ] || fail "$dropin exists: remove it, or compare it with this rollback, before another"
	if [ "${m[secrets_json]:-}" = present ]; then
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
	if [ "${m[secrets_json]:-}" = present ]; then
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
	if [ "${m[unit_was]:-}" = active ]; then
		run "$SYSTEMCTL" start "$SOCKET_UNIT" "$UNIT"
	elif [ "${m[socket_was]:-}" = active ]; then
		run "$SYSTEMCTL" start "$SOCKET_UNIT"
	fi
	if [ "$mode" = apply ]; then printf 'rolled_back=%s\n' "$STAMP" >>"$manifest"; fi
	say "  The key $CRED_DIR/$CRED stays: it opens secrets.sealed-after-seal again, and a new seal reuses it."
	say "  ! Secrets that changed after the seal (a new sign-in, a rotated refresh token) are only in"
	say "    $rollback/secrets.sealed-after-seal: those accounts can need Sign In Again."
	say "  To seal again later: rm $DROPIN; systemctl daemon-reload; run $0 again."
	[ "$mode" = apply ] || say $'\nDry run: nothing changed. Run again with --apply to roll back.'
	exit 0
fi

# ---- what is there --------------------------------------------------------

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
unit_text=$("$SYSTEMCTL" cat "$UNIT" 2>/dev/null || true)
say "    $UNIT runs ${exec_start:-?} as $APP_USER, data $target, private socket $app_socket"
if [ "$secrets_mode" = sealed ] && grep -q "^LoadCredentialEncrypted=$CRED\\b" <<<"$unit_text"; then
	ok "the installed $UNIT seals the secrets (FROSTMAIL_SECRETS=sealed) and loads the credential $CRED"
elif [ -e "$dropin" ]; then
	problem "$UNIT has FROSTMAIL_SECRETS='${secrets_mode}' because of the drop-in $DROPIN (a rollback wrote it): remove it and run systemctl daemon-reload"
else
	problem "the installed $UNIT does not seal the secrets: it needs FROSTMAIL_SECRETS=sealed (now '${secrets_mode}') and LoadCredentialEncrypted=$CRED: install the new packaging first (packaging/lodge/frostmail.service), then systemctl daemon-reload"
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

has_json=0; has_sealed=0; has_key=0
[ ! -f "$(p "$target")/secrets.json" ] || has_json=1
[ ! -f "$(p "$target")/secrets.sealed" ] || has_sealed=1
[ ! -f "$keyfile" ] || has_key=1
yn() { if [ "$1" = 1 ]; then printf present; else printf absent; fi; }
say "    $target/secrets.json $(yn $has_json), $target/secrets.sealed $(yn $has_sealed), the key $CRED_DIR/$CRED $(yn $has_key)"

if [ $has_sealed = 1 ] && [ $has_key = 0 ]; then
	problem "$target/secrets.sealed exists and the key $CRED_DIR/$CRED does not: the key that sealed it is gone. Moving secrets.sealed aside makes every account sign in again; decide that yourself"
fi
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
record() { if [ "$mode" = apply ]; then printf '%s=%s\n' "$1" "$2" >>"$backup/manifest"; fi; }
if [ "$mode" = apply ]; then
	trap '[ $? -eq 0 ] || if [ -f "$backup/manifest" ]; then printf "seal-mail-secrets: the seal stopped at the command above; roll back with: %s --rollback %s --apply\n" "$0" "$backup" >&2; else printf "seal-mail-secrets: it stopped before the backup was made; nothing else changed: start the units again: systemctl start %s %s\n" "$SOCKET_UNIT" "$UNIT" >&2; fi' EXIT
fi

step "1. Stop mail"
# The socket first: a connection to it would start maild while its files change.
run "$SYSTEMCTL" stop "$SOCKET_UNIT" "$UNIT"

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
	run mv "$keyfile.new" "$keyfile"
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
	# maild is active before it opens its secrets: wait for it to seal the file.
	for _ in $(seq 60); do
		if [ ! -e "$(p "$target")/secrets.json" ] && { [ $has_json = 0 ] || [ -f "$(p "$target")/secrets.sealed" ]; }; then break; fi
		sleep 0.5
	done
	[ ! -e "$(p "$target")/secrets.json" ] || fail "$target/secrets.json is still there: maild did not seal it: journalctl -u $UNIT"
	if [ $has_json = 1 ]; then
		[ -f "$(p "$target")/secrets.sealed" ] || fail "$target/secrets.sealed does not exist: journalctl -u $UNIT"
		ok "secrets.json is gone and secrets.sealed exists"
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
	say "  then check that $target/secrets.json is gone, that $target/secrets.sealed exists, and that $UNIT lists its accounts"
fi

step "Done"
say "Roll back with: $0 --rollback $backup --apply"
if [ $has_json = 1 ]; then
	say "! $backup/secrets.json holds the secrets in plain text. When mail works, remove it:"
	say "    rm $backup/secrets.json"
fi
[ "$mode" = apply ] || say $'\nDry run: nothing changed. Run again with --apply to seal.'
