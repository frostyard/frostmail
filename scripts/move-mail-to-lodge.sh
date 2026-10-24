#!/usr/bin/env bash
# Moves the owner's mail from the desktop's maild to lodge's host
# (docs/plans/0011-m8-mail-in-lodge.md, Phase 7). It has 2 halves. Both
# default to a dry run: they check and print the plan, and change nothing.
#
#   scripts/move-mail-to-lodge.sh export --to DIR [--apply]                      # on the desktop, as your user
#   scripts/move-mail-to-lodge.sh --from DIR [--apply] [--set-aside]             # on lodge's host, as root
#   scripts/move-mail-to-lodge.sh --rollback /var/backups/frostmail-move-<stamp> [--apply]   # on lodge's host
#
# What export --apply does on the desktop, in order, printing each command
# first:
#   1. stops and disables the user unit maild.service, so that the copy is
#      a still one and no maild syncs the accounts here after the move;
#   2. runs mailctl export DIR: the database, the blobs, the secrets (from
#      the Secret Service or the secrets file, in plain text, mode 0600)
#      and a MANIFEST;
#   3. appends desktop_maild=disabled to DIR/MANIFEST.
# You copy DIR to lodge's host (scp -rp), run the host half there, and keep
# this desktop's maild disabled. It never touches the desktop app's config.
#
# What --from --apply does on lodge's host, in order, printing each command
# first:
#   1. stops frostmail.socket and frostmail.service;
#   2. sets aside what frostmail's data directory and cache hold (never
#      merged, never deleted), then puts the export's database, blobs and
#      secrets.json into a new data directory, owned by frostmail;
#   3. enables and starts frostmail.socket and frostmail.service, and checks
#      that its maild lists the export's accounts.
#
# It keeps what it sets aside, and a manifest --rollback reads, under
# /var/backups/frostmail-move-<stamp>. It refuses any state it cannot read
# unambiguously, before changing anything. It never deletes DIR.
#
# --rollback puts the machine back as the backup's manifest says: the data
# and cache directories as they were, the units started or enabled as they
# were. What the server's maild did since the move on the IMAP servers
# stays there; what it kept only in its database is in the backup's
# data-after-move.
set -euo pipefail

# Tests stage a root and stub commands (scripts/move_test.go); on a real
# machine all of these are empty or the system's own.
ROOT=${MOVE_ROOT:-}
SYSTEMCTL=${SYSTEMCTL:-systemctl}
RUNUSER=${RUNUSER:-runuser}
GETENT=${GETENT:-getent}
CHOWN=${CHOWN:-chown}
LODGECTL=${LODGECTL:-lodgectl}
MAILCTL=${MAILCTL:-mailctl}
STAMP=${MOVE_STAMP:-$(date -u +%Y%m%dT%H%M%SZ)}

DESKTOP_UNIT=maild.service
UNIT=frostmail.service
SOCKET_UNIT=frostmail.socket
LODGE_UNIT=lodge.service
APP_USER=frostmail

mode=dry-run
half=host
to=
from=
set_aside=
rollback=

usage() {
	sed -n '2,38p' "$0" | sed 's/^# \{0,1\}//'
	exit "${1:-0}"
}

if [ "${1:-}" = "export" ]; then half="export"; shift; fi
while [ $# -gt 0 ]; do
	case $1 in
	--dry-run) mode=dry-run ;;
	--apply) mode=apply ;;
	--to) to=${2:?--to needs a directory}; shift ;;
	--to=*) to=${1#--to=} ;;
	--from) from=${2:?--from needs a directory}; shift ;;
	--from=*) from=${1#--from=} ;;
	--set-aside) set_aside=1 ;;
	--rollback) rollback=${2:?--rollback needs a backup directory}; shift ;;
	-h | --help) usage 0 ;;
	*) echo "move-mail-to-lodge: unknown argument $1" >&2; usage 2 ;;
	esac
	shift
done
if [ "$half" = export ] && { [ -n "$from" ] || [ -n "$set_aside" ] || [ -n "$rollback" ]; }; then
	echo "move-mail-to-lodge: export takes --to DIR and --apply only" >&2; usage 2
fi
if [ "$half" = host ] && [ -n "$to" ]; then
	echo "move-mail-to-lodge: --to belongs to the export half: move-mail-to-lodge.sh export --to DIR" >&2; usage 2
fi

p() { printf '%s%s' "$ROOT" "$1"; }
say() { printf '%s\n' "$*"; }
step() { printf '\n== %s\n' "$*"; }
fail() { printf 'move-mail-to-lodge: %s\n' "$*" >&2; exit 1; }
# run prints a command that changes the machine, and runs it with --apply.
run() {
	printf '  $ %s\n' "$*"
	if [ "$mode" = apply ]; then "$@"; fi
}
# as runs a command as user $1, for reading in either mode.
as() { local u=$1; shift; "$RUNUSER" -u "$u" -- "$@"; }
show() { "$SYSTEMCTL" show -p "$2" --value "$1" 2>/dev/null || true; }
indent() { sed 's/^/      /'; }
# envof prints variable $2 of unit $1's Environment=, or nothing.
envof() {
	local kv
	for kv in $(show "$1" Environment); do
		case $kv in "$2"=*) printf '%s' "${kv#*=}"; return ;; esac
	done
}

problems=()
problem() { problems+=("$*"); say "  ✗ $*"; }
ok() { say "  ✓ $*"; }
nothing_changed() {
	if [ ${#problems[@]} -gt 0 ]; then
		printf '\nmove-mail-to-lodge: %d problem(s); nothing changed.\n' ${#problems[@]} >&2
		exit 1
	fi
}

# ---- the desktop's half ---------------------------------------------------

if [ "$half" = export ]; then
	if [ -z "$ROOT" ] && [ "$(id -u)" -eq 0 ]; then
		fail "run the export as your own user, not as root: it reads your maild's data and your keyring"
	fi
	dsystemctl() { "$SYSTEMCTL" --user "$@"; }

	step "Checks (export)"
	[ -n "$to" ] || problem "--to DIR is required: the new directory to export into"
	if "$MAILCTL" help export 2>/dev/null | grep -q 'mailctl export DIR'; then
		ok "$MAILCTL has 'export'"
	else
		problem "$MAILCTL has no 'export': install the mailctl of this checkout (make install) first"
	fi
	data=${FROSTMAIL_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/frostmail}
	if [ -f "$data/frostmail.db" ]; then
		ok "the desktop's maild keeps its data in $data"
	else
		problem "$data/frostmail.db is missing: is this the machine and user whose mail moves? (FROSTMAIL_DATA_DIR names another directory)"
	fi
	if [ -n "$to" ] && [ -e "$to" ]; then problem "$to exists: export into a new directory"; fi
	load_state=$(dsystemctl show -p LoadState --value "$DESKTOP_UNIT" 2>/dev/null || true)
	unit_was=
	unit_enabled=
	if [ "$load_state" = loaded ]; then
		unit_was=$(dsystemctl is-active "$DESKTOP_UNIT" 2>/dev/null || true)
		unit_enabled=$(dsystemctl is-enabled "$DESKTOP_UNIT" 2>/dev/null || true)
		ok "the user unit $DESKTOP_UNIT is $unit_was and $unit_enabled"
	elif "$MAILCTL" hello >/dev/null 2>&1; then
		problem "a maild answers, but $DESKTOP_UNIT is not a user unit here: stop that maild yourself, so that the copy is a still one"
	else
		ok "there is no user unit $DESKTOP_UNIT, and no maild answers: nothing to stop"
	fi
	nothing_changed

	step "Plan (export, $mode)"
	# What puts the desktop back as it was, for a failure after the stop.
	restore=
	if [ "$unit_enabled" = enabled ] && [ "$unit_was" = active ]; then
		restore="systemctl --user enable --now $DESKTOP_UNIT"
	elif [ "$unit_enabled" = enabled ]; then
		restore="systemctl --user enable $DESKTOP_UNIT"
	elif [ "$unit_was" = active ]; then
		restore="systemctl --user start $DESKTOP_UNIT"
	fi
	# shellcheck disable=SC2329  # called by the EXIT trap below
	export_stopped() {
		printf 'move-mail-to-lodge: the export stopped; to put this desktop back, run:\n' >&2
		if [ -n "$restore" ]; then printf '  %s\n' "$restore" >&2; fi
		printf '  rm -rf %s\n' "$to" >&2
	}
	if [ "$mode" = apply ]; then
		trap '[ $? -eq 0 ] || export_stopped' EXIT
	fi
	if [ "$load_state" = loaded ]; then run "$SYSTEMCTL" --user disable --now "$DESKTOP_UNIT"; fi
	run "$MAILCTL" export "$to"
	printf '  $ echo desktop_maild=disabled >> %s/MANIFEST\n' "$to"
	if [ "$mode" = apply ]; then printf 'desktop_maild=disabled\n' >>"$to/MANIFEST"; fi

	step "Done"
	say "Next:"
	say "  1. copy $to to lodge's host, for example:  scp -rp $to root@HOST:/root/"
	say "  2. on lodge's host, as root: move-mail-to-lodge.sh --from /root/$(basename "$to")   (then again with --apply)"
	say "  3. keep this desktop's maild disabled: with it running, 2 mailds sync the same accounts and send the same outbox"
	say "  4. remove $to when the move is done: it holds the secrets in plain text"
	say "To put this desktop's maild back (before the host half): ${restore:-nothing to start}; rm -rf $to"
	[ "$mode" = apply ] || say $'\nDry run: nothing changed. Run again with --apply to export.'
	exit 0
fi

if [ -z "$ROOT" ] && [ "$(id -u)" -ne 0 ]; then
	fail "run me as root on lodge's host (sudo $0 …)"
fi

# ---- rollback -------------------------------------------------------------

if [ -n "$rollback" ]; then
	manifest=$rollback/manifest
	[ -f "$manifest" ] || fail "$manifest is missing: is $rollback a backup of this script?"
	declare -A m
	while IFS='=' read -r k v; do [ -n "$k" ] && m[$k]=$v; done <"$manifest"
	[ -z "${m[rolled_back]:-}" ] || fail "$rollback was rolled back already (${m[rolled_back]})"
	[ ! -e "$rollback/data-after-move" ] || fail "$rollback/data-after-move exists: a rollback stopped half way; finish it by hand from the manifest"
	step "Rollback of the move recorded in $rollback ($mode)"
	if [ "$mode" = apply ]; then
		trap '[ $? -eq 0 ] || printf "move-mail-to-lodge: the rollback stopped at the command above; what is left is in %s\n" "$rollback" >&2' EXIT
	fi
	run "$SYSTEMCTL" disable --now "$SOCKET_UNIT" "$UNIT"
	if [ -n "${m[target]:-}" ] && [ -e "${m[target]}" ]; then
		run mv "${m[target]}" "$rollback/data-after-move"
	fi
	if [ -n "${m[aside]:-}" ]; then run mv "${m[aside]}" "${m[target]}"; fi
	if [ -n "${m[cache]:-}" ] && [ -e "${m[cache]}" ]; then
		run mv "${m[cache]}" "$rollback/cache-after-move"
	fi
	if [ -n "${m[cache_aside]:-}" ]; then run mv "${m[cache_aside]}" "${m[cache]}"; fi
	if [ "${m[unit_was]:-}" = active ]; then
		run "$SYSTEMCTL" start "$SOCKET_UNIT" "$UNIT"
	elif [ "${m[socket_was]:-}" = active ]; then
		run "$SYSTEMCTL" start "$SOCKET_UNIT"
	fi
	if [ "${m[unit_enabled]:-}" = enabled ]; then run "$SYSTEMCTL" enable "$SOCKET_UNIT" "$UNIT"; fi
	if [ "$mode" = apply ]; then printf 'rolled_back=%s\n' "$STAMP" >>"$manifest"; fi
	say "  ! What the server's maild did since the move stays: on the IMAP servers, its changes to mail, flags and sent messages."
	say "    What it kept only in its database (drafts, rules, Remind Me, the outbox) is in $rollback/data-after-move."
	say "  To take the mail back on the desktop: systemctl --user enable --now $DESKTOP_UNIT"
	[ "$mode" = apply ] || say $'\nDry run: nothing changed. Run again with --apply to roll back.'
	exit 0
fi

# ---- what is there --------------------------------------------------------

# generated_domain says whether an address's domain is generated mail:
# RFC 2606's example domains and reserved top-level names.
generated_domain() {
	case ${1##*@} in
	example.com | example.net | example.org | *.example.com | *.example.net | *.example.org) return 0 ;;
	*.example | *.test | *.invalid | *.localhost) return 0 ;;
	esac
	return 1
}

step "Checks"

if "$LODGECTL" status >/dev/null 2>&1 && [ "$("$SYSTEMCTL" is-active "$LODGE_UNIT" 2>/dev/null)" = active ]; then
	ok "lodge is installed and up"
else
	problem "lodge is not up: install lodge and start $LODGE_UNIT first"
fi
if [ -f "$(p /usr/share/lodge/apps/mail/app.json)" ]; then
	ok "mail's manifest is in /usr/share/lodge/apps/mail"
else
	problem "mail is not installed for lodge: run make lodge-package and install its stage (packaging/lodge)"
fi

app_pw=$("$GETENT" passwd "$APP_USER" || true)
if [ -n "$app_pw" ]; then
	app_uid=$(cut -d: -f3 <<<"$app_pw")
	app_gid=$(cut -d: -f4 <<<"$app_pw")
	ok "user $APP_USER exists (uid $app_uid, gid $app_gid)"
else
	problem "user $APP_USER does not exist: install packaging/lodge's sysusers file and run systemd-sysusers"
fi

for u in "$UNIT" "$SOCKET_UNIT"; do
	if [ "$(show "$u" LoadState)" = loaded ]; then ok "$u is installed"; else problem "$u is not installed (packaging/lodge/)"; fi
done
unit_was=$("$SYSTEMCTL" is-active "$UNIT" 2>/dev/null || true)
socket_was=$("$SYSTEMCTL" is-active "$SOCKET_UNIT" 2>/dev/null || true)
unit_enabled=$("$SYSTEMCTL" is-enabled "$UNIT" 2>/dev/null || true)
target=$(envof "$UNIT" FROSTMAIL_DATA_DIR)
target=${target:-/var/lib/frostmail}
cache=$(envof "$UNIT" FROSTMAIL_CACHE_DIR)
cache=${cache:-/var/cache/frostmail}
app_socket=$(envof "$UNIT" FROSTMAIL_SOCKET)
app_socket=${app_socket:-/run/frostmail/maild.sock}
secrets_mode=$(envof "$UNIT" FROSTMAIL_SECRETS)
exec_start=$(show "$UNIT" ExecStart | sed -n 's/.*path=\([^ ;]*\).*/\1/p')
dropins=$(show "$UNIT" DropInPaths)
say "    $UNIT runs ${exec_start:-?} as $APP_USER, data $target, cache $cache, private socket $app_socket"
[ -z "$dropins" ] || say "    with drop-ins: $dropins (kept as they are: the binary, data and socket above are theirs)"
if [ "$secrets_mode" != file ]; then
	problem "$UNIT has FROSTMAIL_SECRETS='${secrets_mode}', not 'file': the server's maild must keep its secrets in its file store (ADR-0044 decides what comes next)"
fi

mailctl=$(command -v mailctl || true)
if [ -n "$exec_start" ] && [ -x "$(p "$(dirname "$exec_start")/mailctl")" ]; then
	mailctl=$(p "$(dirname "$exec_start")/mailctl")
fi
if [ -z "$mailctl" ]; then
	problem "no mailctl beside ${exec_start:-maild} or on PATH: install the one maild came with"
elif ! "$mailctl" help export 2>/dev/null | grep -q 'mailctl export DIR'; then
	problem "$mailctl has no 'export' (an older mailctl?): install the one ${exec_start:-maild} came with beside it"
fi

# The export.
export_id=
accounts=
if [ -z "$from" ]; then
	problem "--from DIR is required: the export, copied here from the desktop (move-mail-to-lodge.sh export --to DIR)"
elif [ ! -d "$from" ]; then
	problem "$from is not a directory"
else
	if [ -n "$mailctl" ] && "$mailctl" help export 2>/dev/null | grep -q 'mailctl export DIR'; then
		if check_out=$("$mailctl" export --check "$from" 2>&1); then
			ok "$mailctl export --check passes for $from"
			indent <<<"$check_out"
		else
			problem "$mailctl export --check $from fails:"
			indent <<<"$check_out"
		fi
	fi
	declare -A man
	if [ -f "$from/MANIFEST" ]; then
		while IFS='=' read -r k v; do [ -n "$k" ] && man[$k]=$v; done <"$from/MANIFEST"
		export_id=${man[id]:-}
		accounts=${man[accounts]:-}
		say "    export ${export_id:-?} from ${man[host]:-?}, made ${man[created]:-?}; accounts: ${accounts:-none}"
		[ -n "$export_id" ] || problem "$from/MANIFEST has no id"
		if [ "${man[desktop_maild]:-}" != disabled ]; then
			say "  ! the manifest does not say the desktop's maild is disabled: it must stay stopped, or 2 mailds sync the same accounts and send the same outbox"
		fi
	else
		problem "$from/MANIFEST is missing: is $from an export of mailctl export?"
	fi
	[ -d "$from/blobs" ] || problem "$from/blobs is missing"
	[ -f "$from/frostmail.db" ] || problem "$from/frostmail.db is missing"
	[ -f "$from/secrets.json" ] || problem "$from/secrets.json is missing"
fi

# Not moved already: a backup of this script that took this export in.
if [ -n "$export_id" ]; then
	for bm in "$(p /var/backups)"/frostmail-move-*/manifest; do
		[ -f "$bm" ] || continue
		if grep -qx "export=$export_id" "$bm" && ! grep -q '^rolled_back=' "$bm"; then
			problem "this export was moved already (see ${bm%/manifest}): roll that move back first, or make a new export"
		fi
	done
fi

# What frostmail.service's data directory holds now.
aside=
cache_aside=
if [ -f "$(p "$target")/frostmail.db" ]; then
	if [ "$unit_was" != active ]; then
		problem "$target holds a database and $UNIT is not running to say what: start it, or move $target aside yourself"
	elif [ -z "$mailctl" ]; then
		problem "$target holds a database, and without a mailctl it cannot be asked what it holds"
	else
		listed=$(as "$APP_USER" "$mailctl" --socket "$app_socket" account list 2>/dev/null) || listed=
		if [ -z "$listed" ]; then
			problem "$UNIT does not list its accounts at $app_socket: $target holds a database nothing can describe: move it aside yourself"
		else
			real=()
			while read -r id email _; do
				[ -n "${email:-}" ] || continue
				generated_domain "$email" || real+=("$id:$email")
			done < <(tail -n +2 <<<"$listed")
			if [ ${#real[@]} -eq 0 ]; then
				ok "$target holds only generated mail ($(tail -n +2 <<<"$listed" | awk '{print $2}' | paste -sd' ')): it goes aside, never merged"
				aside=$target.aside-$STAMP
			elif [ -n "$set_aside" ]; then
				say "  ! $target holds accounts that look real: ${real[*]}. They go aside (--set-aside), never merged."
				aside=$target.aside-$STAMP
			else
				problem "$target holds accounts that look real: ${real[*]}. Mail is never merged: pass --set-aside to set them aside, or move $target yourself"
			fi
		fi
	fi
elif [ -e "$(p "$target")" ]; then
	ok "$target holds no database: it goes aside too, empty or not, and is made again"
	aside=$target.aside-$STAMP
else
	ok "$target does not exist yet"
fi
if [ -n "$aside" ] && [ -e "$(p "$aside")" ]; then problem "$aside exists: choose another MOVE_STAMP, or clear it yourself"; fi
if [ -e "$(p "$cache")" ]; then
	cache_aside=$cache.aside-$STAMP
	say "    the parts cache $cache is keyed by message ID, so the old one is wrong for the new database: it goes aside to $cache_aside"
	if [ -e "$(p "$cache_aside")" ]; then problem "$cache_aside exists: choose another MOVE_STAMP, or clear it yourself"; fi
fi

# Lodge's tokens for mail: information, never a problem.
tokens=$("$LODGECTL" tokens 2>/dev/null || true)
mail_tokens=$(grep -E '(^|[^[:alnum:]_-])mail([^[:alnum:]_-]|$)' <<<"$tokens" || true)
if [ -n "$mail_tokens" ]; then
	say "    lodge's tokens for app mail:"
	indent <<<"$mail_tokens"
	say "  ! After the move, a scope that names an account ID (account=<id>) names the export's account with that ID: check each, and revoke and issue again where needed."
else
	say "    no lodge token names app mail"
fi

nothing_changed

# ---- the move -------------------------------------------------------------

backup=$(p /var/backups)/frostmail-move-$STAMP
step "Plan ($mode); backup and manifest in $backup"
run mkdir -p -m 0700 "$backup"
record() { if [ "$mode" = apply ]; then printf '%s=%s\n' "$1" "$2" >>"$backup/manifest"; fi; }
if [ "$mode" = apply ]; then
	trap '[ $? -eq 0 ] || printf "move-mail-to-lodge: the move stopped; roll back with: %s --rollback %s --apply\n" "$0" "$backup" >&2' EXIT
fi
record target "$(p "$target")"
record cache "$(p "$cache")"
record export "$export_id"
record from "$from"
record unit_was "$unit_was"
record unit_enabled "$unit_enabled"
record socket_was "$socket_was"
record app_socket "$app_socket"
record mailctl "$mailctl"
record accounts "$accounts"

step "1. Stop mail"
# The socket first: a connection to it would start maild on a directory
# that is being replaced.
run "$SYSTEMCTL" stop "$SOCKET_UNIT" "$UNIT"

step "2. The export into $target"
if [ -n "$aside" ]; then
	run mv "$(p "$target")" "$(p "$aside")"
	record aside "$(p "$aside")"
fi
if [ -n "$cache_aside" ]; then
	run mv "$(p "$cache")" "$(p "$cache_aside")"
	record cache_aside "$(p "$cache_aside")"
fi
run rm -rf "$(p "$target").new"
run install -d -m 0700 "$(p "$target").new"
run cp -a "$from/frostmail.db" "$from/blobs" "$from/secrets.json" "$(p "$target").new/"
run "$CHOWN" -R "$APP_USER:$APP_USER" "$(p "$target").new"
run chmod 0700 "$(p "$target").new"
run chmod 0600 "$(p "$target").new/secrets.json"
run mv "$(p "$target").new" "$(p "$target")"
say "  (systemd's CacheDirectory= makes $cache again when $UNIT starts.)"

step "3. Start mail"
run "$SYSTEMCTL" enable --now "$SOCKET_UNIT" "$UNIT"
if [ "$mode" = apply ]; then
	for _ in $(seq 60); do
		[ "$("$SYSTEMCTL" is-active "$UNIT" 2>/dev/null)" = active ] && break
		sleep 0.5
	done
	[ "$("$SYSTEMCTL" is-active "$UNIT" 2>/dev/null)" = active ] || fail "$UNIT did not start: journalctl -u $UNIT"
	ok "$UNIT is active"
	for _ in $(seq 30); do
		if now_out=$(as "$APP_USER" "$mailctl" --socket "$app_socket" account list 2>/dev/null); then break; fi
		now_out=
		sleep 0.5
	done
	[ -n "${now_out:-}" ] || fail "$UNIT does not answer at $app_socket: journalctl -u $UNIT"
	now=$(tail -n +2 <<<"$now_out" | awk '{print $1 ":" $2}' | sort | paste -sd' ')
	want=$(tr ' ' '\n' <<<"$accounts" | sort | paste -sd' ')
	[ "$now" = "$want" ] || fail "$UNIT lists the accounts '$now', not the export's '$want'"
	ok "its accounts are the export's: $want"
else
	say "  then check that $UNIT lists the export's accounts: ${accounts:-none}"
fi

step "Done"
say "Roll back with: $0 --rollback $backup --apply"
say "Next:"
say "  1. on the desktop, point the app at lodge: Settings, Server pane, or app.json with {\"lodge\": \"https://lodge.<tailnet>.ts.net\"}"
say "  2. keep the desktop's maild disabled (systemctl --user disable --now $DESKTOP_UNIT)"
say "  3. remove $from on this host, and the export on the desktop: they hold the secrets in plain text"
say "  4. after the first sync, check the mail against the servers: sudo -u $APP_USER $mailctl --socket $app_socket verify"
say "  5. after plan 0011's week of use, remove the desktop's Secret Service items: secret-tool clear application frostmail"
[ "$mode" = apply ] || say $'\nDry run: nothing changed. Run again with --apply to move.'
