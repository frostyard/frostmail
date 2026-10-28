#!/bin/sh
# The Flatpak's smoke test, run by flatpak-builder's test-commands
# (docs/plans/0013-m6-the-standalone-desktop-flatpak.md, Phase 3). It runs the
# launcher, maild and mailctl as the Flatpak does, with fakes for what the
# build sandbox lacks: no portal, no display, no session bus. It covers:
#
#   - the state directory and the logs the launcher makes, private (0700,
#     0600), and a launcher whose flatpak-spawn fails still runs the app;
#   - the socket in the app's shared runtime directory, and a log over 1 MiB
#     that turns over;
#   - a second maild on the same data directory, with another socket, that
#     exits with "another maild uses";
#   - a rule written through mailctl that reads back;
#   - the launcher's normal start through a fake flatpak-spawn, which leaves
#     no flatpak-spawn behind, while maild lives on;
#   - a running maild whose build differs from the latest one, which the
#     launcher stops and replaces, and one whose build is equal, or whose
#     latest build is unknown, which it keeps;
#   - a latest-build query that hangs, and one that leaves a detached process
#     holding its output, as a stalled portal does: the launcher keeps the
#     maild and runs the app within its bounds, and leaves no file behind;
#   - app.json naming lodge: `frostmail --background` exits 0 and starts
#     nothing, a normal start starts no maild and asks no portal, and a
#     maild run directly exits 0 without making a database;
#   - a launcher that read "local" while app.json changed to lodge, whose
#     maild exits 0 without making a database;
#   - a socket where something accepts and never answers: the launcher still
#     runs the app within a bound, and leaves no flatpak-spawn behind;
#   - a maild in the Flatpak with FROSTMAIL_SECRETS unset and no Secret
#     Service (no session bus), which keeps no plain file: adding an account
#     fails, its message names `flatpak override --user
#     --env=FROSTMAIL_SECRETS=file org.frostyard.Frostmail`, and no account
#     and no secrets.json are left (ADR-0046).
#
# The fake flatpak-spawn fails, and records the fault, when the launcher
# calls it without --latest-version.
#
# FROSTMAIL_BIN_DIR is where frostmail (the launcher), maild and mailctl
# are; the default is the Flatpak's /app/bin. The test cleans up after itself.

set -eu

bin="${FROSTMAIL_BIN_DIR:-/app/bin}"

work="$(mktemp -d)"
pids=
cleanup() {
    # The detached process of the stalled-portal case, when it still runs.
    if [ -s "$work/detached.pid" ]; then
        kill -KILL "$(cat "$work/detached.pid")" 2>/dev/null || true
    fi
    for p in $pids; do
        if alive "$p"; then
            kill -TERM "$p" 2>/dev/null || true
        fi
    done
    # maild is not this shell's child (flatpak-spawn started it): give it up
    # to 10 s before its directories go.
    for p in $pids; do
        for _ in $(seq 1 40); do
            alive "$p" || break
            sleep 0.25
        done
        if alive "$p"; then
            kill -KILL "$p" 2>/dev/null || true
        fi
    done
    rm -rf "$work"
}
trap cleanup EXIT

state="$work/state/frostmail"
config="$work/config/frostmail"
log="$state/maild.log"

# alive PID: true while the process runs (a zombie has ended).
alive() {
    kill -0 "$1" 2>/dev/null || return 1
    case "$(sed -n 's/^State:[[:space:]]*\(.\).*/\1/p' "/proc/$1/status" 2>/dev/null)" in
    Z | "") return 1 ;;
    esac
    return 0
}

fail() {
    echo "smoke test: $*" >&2
    for f in "$SMOKE_SPAWN_FAULT" "$log.1" "$log" "$work/launcher.log"; do
        if [ -f "$f" ]; then
            echo "--- $f" >&2
            tail -n 30 "$f" | tr -d '\000' >&2 || true
        fi
    done
    exit 1
}

# wait_hello: until a maild answers, for up to 20 s.
wait_hello() {
    for _ in $(seq 1 40); do
        if mailctl hello >/dev/null 2>&1; then
            return 0
        fi
        sleep 0.5
    done
    return 1
}

# wait_gone PID: until the process has ended, for up to 10 s.
wait_gone() {
    for _ in $(seq 1 40); do
        alive "$1" || return 0
        sleep 0.25
    done
    return 1
}

# wait_file FILE: until it exists, for up to 10 s.
wait_file() {
    for _ in $(seq 1 40); do
        [ -e "$1" ] && return 0
        sleep 0.25
    done
    return 1
}

# info FILE COMMIT: a stand-in /.flatpak-info.
info() {
    printf '[Application]\nname=org.frostyard.Frostmail\n\n[Instance]\napp-commit=%s\n' "$2" >"$1"
}

# A private runtime directory (mode 0700, as the real one is), home and data.
mkdir -p "$work/run" "$work/home" "$work/data" "$work/bin" "$work/libexec" "$work/held" "$state" "$config"
chmod 0700 "$work/run"
export XDG_RUNTIME_DIR="$work/run" HOME="$work/home" XDG_DATA_HOME="$work/data"
export XDG_STATE_HOME="$work/state" XDG_CONFIG_HOME="$work/config" XDG_CACHE_HOME="$work/cache"
export FLATPAK_ID=org.frostyard.Frostmail
# The Secret Service is absent, as in the build sandbox: maild keeps its
# secrets in a file. No session bus either.
export FROSTMAIL_SECRETS=file
export DBUS_SESSION_BUS_ADDRESS=unix:path="$work/no-bus"
unset FROSTMAIL_SOCKET FROSTMAIL_DATA_DIR FROSTMAIL_CACHE_DIR FROSTMAIL_FLATPAK_INFO

# A fake flatpak-spawn, first on PATH. It drops --latest-version, and runs the
# command with FROSTMAIL_FLATPAK_INFO pointing at the "latest" deployment's
# info file, which the test controls. For `frostmail` it records its own PID
# and its child's, as the portal's spawn does in a sandbox of its own, and
# waits for the child. The app and the Background helper are fakes that leave
# a marker, and exit at once.
cat >"$work/bin/flatpak-spawn" <<'FAKE'
#!/bin/sh
# The launcher must ask for the latest deployment (ADR-0046): without the
# flag, the real portal would run the window's own, perhaps old, build.
if [ "${1:-}" != "--latest-version" ]; then
    echo "fake flatpak-spawn: called without --latest-version: $*" | tee -a "$SMOKE_SPAWN_FAULT" >&2
    exit 2
fi
shift
export FROSTMAIL_FLATPAK_INFO="$SMOKE_LATEST_INFO"
if [ "${1:-}" = "frostmail" ]; then
    echo "$$" >"$SMOKE_SPAWN_PID"
    if [ -n "${SMOKE_SPAWN_FAIL:-}" ]; then
        exit 1
    fi
    "$@" &
    echo "$!" >"$SMOKE_CHILD_PID"
    wait "$!"
elif [ -n "${SMOKE_SPAWN_HANG_BUILD:-}" ] && [ "$*" = "maild -build" ]; then
    # A portal that never answers, and ignores SIGTERM (the ignore passes on
    # to sleep): only SIGKILL ends it.
    trap '' TERM
    echo "$$" >"$SMOKE_HANG_PID"
    sleep 60
elif [ -n "${SMOKE_SPAWN_DETACH:-}" ] && [ "$*" = "maild -build" ]; then
    # A process in a session of its own that keeps this command's output open
    # and says nothing, as the portal's new sandbox does; the query itself
    # ends. The launcher's timeout cannot reach it.
    setsid sh -c 'echo $$ >"$1"; exec sleep 60' sh "$SMOKE_DETACHED_PID" &
    exit 0
else
    exec "$@"
fi
FAKE
cat >"$work/libexec/frostmail-app" <<'FAKE'
#!/bin/sh
echo "$@" >>"$SMOKE_MARKS/app"
FAKE
cat >"$work/libexec/frostmail-request-background" <<'FAKE'
#!/bin/sh
echo run >>"$SMOKE_MARKS/background"
FAKE
chmod +x "$work/bin/flatpak-spawn" "$work/libexec/frostmail-app" "$work/libexec/frostmail-request-background"
mkdir -p "$work/marks"
SMOKE_SPAWN_PID="$work/spawn.pid"
SMOKE_CHILD_PID="$work/maild.pid"
SMOKE_SPAWN_FAULT="$work/spawn.fault"
SMOKE_LATEST_INFO="$work/info-latest"
SMOKE_MARKS="$work/marks"
SMOKE_HANG_PID="$work/hang.pid"
SMOKE_DETACHED_PID="$work/detached.pid"
export SMOKE_SPAWN_PID SMOKE_CHILD_PID SMOKE_SPAWN_FAULT SMOKE_LATEST_INFO SMOKE_MARKS SMOKE_HANG_PID SMOKE_DETACHED_PID
export FROSTMAIL_LIBEXEC_DIR="$work/libexec"
PATH="$work/bin:$bin:$PATH"
export PATH

# A maild that held a launcher: it says "local" and flips app.json to lodge
# before the launcher starts the real maild.
cat >"$work/held/maild" <<FAKE
#!/bin/sh
if [ "\${1:-}" = "-mode" ]; then
    echo local
    printf '{"lodge":"https://lodge.test"}\n' >"$config/app.json"
    exit 0
fi
exec "$bin/maild" "\$@"
FAKE
chmod +x "$work/held/maild"

socket="$XDG_RUNTIME_DIR/app/org.frostyard.Frostmail/frostmail/maild.sock"

# --- 0. The launcher makes the state directory and its logs private, with the
# umask a login session has, and runs the app although flatpak-spawn fails.
rm -rf "$state"
umask 022
SMOKE_SPAWN_FAIL=1 "$bin/frostmail" >"$work/launcher.log" 2>&1
unset SMOKE_SPAWN_FAIL
wait_file "$SMOKE_MARKS/app" || fail "the launcher did not run the app when flatpak-spawn failed"
[ "$(stat -c %a "$state")" = "700" ] || fail "the state directory has mode $(stat -c %a "$state"), not 700"
[ "$(stat -c %a "$log")" = "600" ] || fail "maild.log has mode $(stat -c %a "$log"), not 600"
[ "$(stat -c %a "$state/background.log")" = "600" ] || fail "background.log has mode $(stat -c %a "$state/background.log"), not 600"
grep -q "did not start" "$log" || fail "the launcher did not log that maild did not start"
mailctl hello >/dev/null 2>&1 && fail "a maild answers although flatpak-spawn failed"
rm -f "$SMOKE_SPAWN_PID" "$SMOKE_MARKS/app" "$SMOKE_MARKS/background"

# --- 1. A maild from an older build, and a log over 1 MiB that turns over.
info "$work/info-old" oldcommit
info "$SMOKE_LATEST_INFO" newcommit
dd if=/dev/zero of="$log" bs=1024 count=1025 2>/dev/null
# The subshell leaves maild without a parent in this shell, so that it never
# stays a zombie.
old="$( (FROSTMAIL_FLATPAK_INFO="$work/info-old" "$bin/frostmail" --background </dev/null >/dev/null 2>&1 &
    echo $!) )"
pids="$pids $old"
wait_hello || fail "maild did not answer within 20 s"
alive "$old" || fail "maild $old is not running"
[ -S "$socket" ] || fail "no socket at $socket"
[ -f "$log.1" ] || fail "the old maild log did not move to $log.1"
size="$(wc -c <"$log")"
[ "$size" -le 1048576 ] || fail "maild.log holds $size bytes; want a new log of at most 1 MiB"
got="$(mailctl hello --build)"
[ "$got" = "oldcommit" ] || fail "the first maild reports build '$got', not oldcommit"

# --- 2. A second maild on the same data directory exits at once. Another
# socket shows that the lock, and not the socket check, stops it.
rc=0
FROSTMAIL_SOCKET="$work/second.sock" timeout 10 maild >"$work/second.log" 2>&1 || rc=$?
if [ "$rc" -eq 0 ] || [ "$rc" -eq 124 ] || ! grep -q "another maild uses" "$work/second.log"; then
    cat "$work/second.log" >&2
    fail "a second maild exited with $rc, not an 'another maild uses' error"
fi
mailctl hello >/dev/null || fail "the first maild stopped"

# --- 3. A write through mailctl reads back (a rule needs no network or account).
printf '%s\n' '[{"name":"smoke test rule","enabled":true,"conditions":{"match":"all","conditions":[]},"actions":[{"kind":"read"}]}]' |
    mailctl rules import - >/dev/null || fail "mailctl rules import failed"
mailctl rules ls | grep -q "smoke test rule" || fail "the rule did not read back"

# --- 4. The launcher finds a maild of another build than the latest one,
# stops it, and starts the latest through flatpak-spawn, which ends.
rm -f "$SMOKE_SPAWN_PID" "$SMOKE_CHILD_PID" "$SMOKE_MARKS/app" "$SMOKE_MARKS/background"
"$bin/frostmail" --open-message 1 >"$work/launcher.log" 2>&1
wait_file "$SMOKE_CHILD_PID" || fail "the launcher did not start maild through flatpak-spawn"
spawned="$(cat "$SMOKE_SPAWN_PID" 2>/dev/null || true)"
new="$(cat "$SMOKE_CHILD_PID")"
pids="$pids $new"
wait_gone "$old" || fail "the old maild $old still runs"
wait_hello || fail "the replacement maild did not answer within 20 s"
alive "$new" || fail "the replacement maild $new is not running"
if [ -n "$spawned" ] && alive "$spawned"; then
    fail "flatpak-spawn $spawned outlived the launcher"
fi
got="$(mailctl hello --build)"
[ "$got" = "newcommit" ] || fail "the replacement reports build '$got', not newcommit"
mailctl rules ls | grep -q "smoke test rule" || fail "the rule is gone after the replacement"
wait_file "$SMOKE_MARKS/background" || fail "the launcher did not ask the Background portal"
wait_file "$SMOKE_MARKS/app" || fail "the launcher did not run the app"
[ "$(cat "$SMOKE_MARKS/app")" = "--open-message 1" ] || fail "the app got '$(cat "$SMOKE_MARKS/app")'"
# maild lives on, with no flatpak-spawn: it holds its own output.
sleep 1
alive "$new" || fail "maild $new ended with flatpak-spawn"

# --- 5. The same build is kept, with the same process, although both builds
# report the same version.
rm -f "$SMOKE_SPAWN_PID" "$SMOKE_CHILD_PID"
"$bin/frostmail" >"$work/launcher.log" 2>&1
[ ! -e "$SMOKE_SPAWN_PID" ] || fail "the launcher started a maild beside one of the latest build"
alive "$new" || fail "the maild of the latest build was stopped"
mailctl hello >/dev/null || fail "the maild of the latest build stopped answering"

# An unknown latest build keeps the running maild too.
SMOKE_LATEST_INFO="$work/no-info"
export SMOKE_LATEST_INFO
"$bin/frostmail" >"$work/launcher.log" 2>&1
[ ! -e "$SMOKE_SPAWN_PID" ] || fail "the launcher started a maild when the latest build was unknown"
alive "$new" || fail "the maild was stopped when the latest build was unknown"
mailctl hello >/dev/null || fail "the maild stopped answering when the latest build was unknown"
SMOKE_LATEST_INFO="$work/info-latest"
export SMOKE_LATEST_INFO

# A latest-build query that hangs and ignores SIGTERM: the launcher's bound
# kills it, keeps the running maild, and still runs the app.
rm -f "$SMOKE_SPAWN_PID" "$SMOKE_MARKS/app"
start="$(date +%s)"
rc=0
SMOKE_SPAWN_HANG_BUILD=1 timeout 40 "$bin/frostmail" >"$work/launcher.log" 2>&1 || rc=$?
took=$(($(date +%s) - start))
[ "$rc" -eq 0 ] || fail "the launcher exited $rc beside a latest-build query that hangs"
[ "$took" -le 20 ] || fail "the launcher took $took s beside a latest-build query that hangs"
[ -e "$SMOKE_MARKS/app" ] || fail "the launcher did not run the app beside a latest-build query that hangs"
hung="$(cat "$SMOKE_HANG_PID" 2>/dev/null || true)"
[ -n "$hung" ] || fail "the latest-build query did not reach the fake portal"
wait_gone "$hung" || fail "the latest-build query $hung outlived the launcher"
[ ! -e "$SMOKE_SPAWN_PID" ] || fail "the launcher started a maild when the latest build did not answer"
alive "$new" || fail "the maild was stopped when the latest build did not answer"

# A latest-build query that leaves a detached process on its output and prints
# no build, as a stalled portal does: the launcher does not wait for that
# process, keeps the maild, and runs the app, all within 20 s (the query's
# bound is 10 s). The outer timeout only ends a launcher that hangs.
rm -f "$SMOKE_SPAWN_PID" "$SMOKE_MARKS/app"
start="$(date +%s)"
rc=0
SMOKE_SPAWN_DETACH=1 timeout 40 "$bin/frostmail" >"$work/launcher.log" 2>&1 || rc=$?
took=$(($(date +%s) - start))
detached="$(cat "$SMOKE_DETACHED_PID" 2>/dev/null || true)"
if [ -z "$detached" ] || ! alive "$detached"; then
    fail "the latest-build query did not leave its detached process"
fi
[ "$rc" -eq 0 ] || fail "the launcher exited $rc beside a detached process on the query's output"
[ "$took" -le 20 ] || fail "the launcher took $took s beside a detached process on the query's output"
[ -e "$SMOKE_MARKS/app" ] || fail "the launcher did not run the app beside a detached process on the query's output"
[ ! -e "$SMOKE_SPAWN_PID" ] || fail "the launcher started a maild beside a detached process on the query's output"
alive "$new" || fail "the maild was stopped beside a detached process on the query's output"
mailctl hello >/dev/null || fail "the maild stopped answering beside a detached process on the query's output"
if ls "$state"/latest-build.* >/dev/null 2>&1; then
    fail "the launcher left a latest-build file in $state"
fi
kill -KILL "$detached" 2>/dev/null || true

# --- 6. Lodge mode. Stop the maild first: a switch to lodge stops it.
mailctl shutdown >/dev/null || fail "mailctl shutdown failed"
wait_gone "$new" || fail "maild $new did not stop on mailctl shutdown"
mailctl hello >/dev/null 2>&1 && fail "maild still answers after mailctl shutdown"
[ "$(maild -mode)" = "local" ] || fail "maild -mode is not local without app.json"
printf '{"lodge":"https://lodge.test"}\n' >"$config/app.json"
[ "$(maild -mode)" = "lodge" ] || fail "maild -mode is not lodge with app.json naming lodge"

# --background exits 0 and starts nothing.
rc=0
timeout 20 "$bin/frostmail" --background >"$work/background.out" 2>&1 || rc=$?
[ "$rc" -eq 0 ] || fail "'frostmail --background' in lodge mode exited with $rc"
if mailctl hello >/dev/null 2>&1; then
    fail "'frostmail --background' in lodge mode started a maild"
fi

# A normal start runs the app only: no maild, no portal request.
rm -f "$SMOKE_SPAWN_PID" "$SMOKE_CHILD_PID" "$SMOKE_MARKS/app" "$SMOKE_MARKS/background"
"$bin/frostmail" >"$work/launcher.log" 2>&1
[ -e "$SMOKE_MARKS/app" ] || fail "the launcher did not run the app in lodge mode"
sleep 1
[ ! -e "$SMOKE_SPAWN_PID" ] || fail "the launcher started a maild in lodge mode"
[ ! -e "$SMOKE_MARKS/background" ] || fail "the launcher asked the Background portal in lodge mode"
if mailctl hello >/dev/null 2>&1; then
    fail "a maild answers after a normal start in lodge mode"
fi

# maild, run directly, exits 0 before it makes a database.
rc=0
XDG_DATA_HOME="$work/data2" timeout 20 maild >"$work/lodge.log" 2>&1 || rc=$?
[ "$rc" -eq 0 ] || fail "maild in lodge mode exited with $rc: $(cat "$work/lodge.log")"
[ ! -e "$work/data2/frostmail/frostmail.db" ] || fail "maild in lodge mode made a database"

# --- 7. A launcher that read "local" while app.json changed to lodge starts
# a maild that exits 0 without opening the database.
rm -f "$config/app.json"
rc=0
PATH="$work/held:$PATH" XDG_DATA_HOME="$work/data3" timeout 20 "$bin/frostmail" --background >"$work/held.out" 2>&1 || rc=$?
[ "$rc" -eq 0 ] || fail "the held launcher's maild exited with $rc"
[ ! -e "$work/data3/frostmail/frostmail.db" ] || fail "the held launcher's maild made a database"
if mailctl hello >/dev/null 2>&1; then
    fail "the held launcher's maild answers in lodge mode"
fi
[ "$(grep -c lodge "$config/app.json")" -eq 1 ] || fail "app.json did not change to lodge"

# --- 8. Something accepts on the socket and never answers: the launcher ends
# its probes, starts what it can, and still runs the app within a bound, with
# no flatpak-spawn left behind.
rm -f "$config/app.json"
[ "$(maild -mode)" = "local" ] || fail "maild -mode is not local after app.json was removed"
mkdir -p "$(dirname "$socket")"
rm -f "$socket"
python3 - "$socket" <<'PY' &
import os, socket, sys

s = socket.socket(socket.AF_UNIX)
s.bind(sys.argv[1])
s.listen(8)
held = []
while True:
    conn, _ = s.accept()
    held.append(conn)
PY
silent=$!
pids="$pids $silent"
wait_file "$socket" || fail "the fake socket did not appear"
rm -f "$SMOKE_SPAWN_PID" "$SMOKE_CHILD_PID" "$SMOKE_MARKS/app" "$SMOKE_MARKS/background"
started=$(date +%s)
rc=0
timeout 40 "$bin/frostmail" >"$work/launcher.log" 2>&1 || rc=$?
elapsed=$(($(date +%s) - started))
[ "$rc" -eq 0 ] || fail "the launcher exited with $rc beside a socket that never answers (after $elapsed s)"
[ "$elapsed" -lt 30 ] || fail "the launcher took $elapsed s beside a socket that never answers"
[ -e "$SMOKE_MARKS/app" ] || fail "the launcher did not run the app beside a socket that never answers"
if [ -s "$SMOKE_CHILD_PID" ]; then
    pids="$pids $(cat "$SMOKE_CHILD_PID")"
fi
spawned="$(cat "$SMOKE_SPAWN_PID" 2>/dev/null || true)"
if [ -n "$spawned" ] && alive "$spawned"; then
    fail "flatpak-spawn $spawned outlived the launcher beside a socket that never answers"
fi
kill -KILL "$silent" 2>/dev/null || true

# --- 9. No Secret Service, FROSTMAIL_SECRETS unset: the Flatpak's maild keeps
# no plain file. It runs on a data directory and a socket of its own; adding
# an account fails with the way to keep secrets in a file, and no account and
# no secrets file are left behind (ADR-0046). The session bus is the missing
# one of the environment above.
secdata="$work/data-secrets"
secsock="$work/secrets.sock"
mkdir -p "$secdata"
env -u FROSTMAIL_SECRETS XDG_DATA_HOME="$secdata" FROSTMAIL_SOCKET="$secsock" maild >"$work/secrets-maild.log" 2>&1 &
secmaild=$!
pids="$pids $secmaild"
secctl() { mailctl --socket "$secsock" "$@"; }
for _ in $(seq 1 40); do
    secctl hello >/dev/null 2>&1 && break
    sleep 0.5
done
secctl hello >/dev/null 2>&1 || {
    cat "$work/secrets-maild.log" >&2
    fail "the maild with no Secret Service did not answer within 20 s"
}
grep -q "no Secret Service" "$work/secrets-maild.log" || fail "the maild with no Secret Service did not log it"
rc=0
printf 'not-a-real-password\n' |
    secctl account add smoke@mailtest.test --imap imap.mailtest.test:993/tls --smtp smtp.mailtest.test:587/starttls --password-stdin \
        >"$work/secrets-add.out" 2>&1 || rc=$?
[ "$rc" -ne 0 ] || fail "account add succeeded although no keyring can keep its password"
# mailctl wraps a long error to the terminal's width: join its lines first.
tr -s '[:space:]' ' ' <"$work/secrets-add.out" |
    grep -q "flatpak override --user --env=FROSTMAIL_SECRETS=file org.frostyard.Frostmail" || {
    cat "$work/secrets-add.out" >&2
    fail "the failure does not name the flatpak override command"
}
if grep -q "not-a-real-password" "$work/secrets-add.out" "$work/secrets-maild.log"; then
    fail "a password reached the output or the log"
fi
[ "$(secctl account list | grep -c '@')" -eq 0 ] || fail "an account was added although the probe failed"
[ ! -e "$secdata/frostmail/secrets.json" ] || fail "the maild with no Secret Service made a secrets.json"
secctl shutdown >/dev/null || fail "mailctl shutdown failed for the maild with no Secret Service"
wait_gone "$secmaild" || fail "the maild with no Secret Service $secmaild did not stop"

[ ! -e "$SMOKE_SPAWN_FAULT" ] || fail "the launcher called flatpak-spawn without --latest-version"

echo "smoke test passed"
