#!/bin/sh
# Frostmail's launcher in the Flatpak (docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md).
# The manifest installs it as /app/bin/frostmail, and the Tauri app as
# /app/libexec/frostmail-app.
#
#   frostmail --background   Run maild and nothing else. A second maild on the
#                            same data directory exits at once (maild locks it),
#                            so this is safe to run twice. The Background portal
#                            starts this at login. maild appends its output to
#                            maild.log in the state directory, and moves a log
#                            over 1 MiB to maild.log.1 itself, once it holds the
#                            data directory's lock, so no live log ever moves.
#                            When app.json names lodge, it exits 0 and starts
#                            nothing.
#   frostmail --start-maild   Start maild as a normal start does, and nothing
#                            else: no window, no exec of the app. The Server
#                            pane runs it when the user returns to local mode
#                            (docs/specs/settings-ui.md, Server pane). It asks
#                            the Background portal for autostart too, as a
#                            normal start does, and exits 0 once maild answers.
#                            Without an answer it prints the launcher's last
#                            line to the error output and exits 1. When
#                            app.json names lodge, it exits 0 and starts
#                            nothing.
#   frostmail [ARGS]         In local mode: make sure the latest maild runs, ask
#                            the Background portal for autostart, and run the
#                            app. In lodge mode: run the app only.
#
# The mode is the one that `maild -mode` prints, from app.json in the config
# directory: "local" (no lodge key) or "lodge". maild checks it again under
# its data lock, and that check is the one that counts: this launcher may
# read "local" and be delayed past a change of mode, and the maild it then
# starts exits at once.
#
# FROSTMAIL_LIBEXEC_DIR, when set, is the directory of frostmail-app and
# frostmail-request-background: /app/libexec in the Flatpak, and a test's
# own directory in its tests.
#
# A maild that the window's sandbox started would die with the sandbox, and
# with a duplicate window that tauri-plugin-single-instance closes at once.
# So the launcher has the Flatpak portal start `frostmail --background` in a
# new sandbox of this app, and then ends the flatpak-spawn process it made
# once that maild answers: flatpak-spawn forwards catchable signals to the
# command it spawned, but only SIGKILL ends flatpak-spawn alone, and the
# portal keeps the spawned maild without --watch-bus. That maild lives on
# when the window closes, until logout. There is no fallback in this sandbox:
# a maild here would die with a duplicate window while the primary window
# still needs it. Without the portal the launcher says so, and the app shows
# that maild is not running.
#
# --latest-version makes the portal start the command from the newest
# installed deployment of this app, never from the one this window runs. An
# old window, or an old notification's click, cannot bring back an old maild:
# when the running maild's build (`mailctl hello --build`, the app-commit of
# its deployment) differs from the latest deployment's (`maild -build`, run
# the same way), the launcher stops it and starts the latest.

set -u

libexec="${FROSTMAIL_LIBEXEC_DIR:-/app/libexec}"
app="$libexec/frostmail-app"
request_background="$libexec/frostmail-request-background"

state="${XDG_STATE_HOME:-$HOME/.local/state}/frostmail"

# bounded SECONDS COMMAND...: run the command for at most SECONDS (coreutils
# timeout, which the GNOME runtime has). Every call that waits on maild or
# on the portal goes through it, so a maild that accepts and never answers,
# or a portal that hangs, never keeps the window from starting.
if command -v timeout >/dev/null 2>&1; then
    bounded() {
        secs=$1
        shift
        # --kill-after: a command that catches SIGTERM, or a flatpak-spawn
        # that forwards it to a portal call that hangs, gets SIGKILL 1 s on.
        timeout --kill-after=1 "$secs" "$@"
    }
else
    bounded() {
        shift
        "$@"
    }
fi

# hello: true when a maild answers within 2 s.
hello() {
    bounded 2 mailctl hello >/dev/null 2>&1
}

# prepare_state: the state directory and the logs, private (0700 and 0600),
# whatever the umask: the shell's redirects would make a missing log 0644.
# A subshell keeps the umask of the app and of maild as it is.
prepare_state() {
    (
        umask 077
        mkdir -p "$state"
        : >>"$state/maild.log"
        : >>"$state/background.log"
    )
}

# have_spawn: true when the Flatpak portal's flatpak-spawn is there. Without
# it this sandbox starts no maild at all (see the header).
have_spawn() {
    command -v flatpak-spawn >/dev/null 2>&1
}

# ensure_latest: stop a maild of another build than the latest installed one,
# so that start_maild starts the latest. An update replaces the engine, only
# forward: `mailctl shutdown` returns once maild has released the data
# directory's lock, so the maild started afterwards can take it at once.
# Either build empty, or a failed or late call, keeps the maild that runs.
# The calls are bounded: 2 s for a probe, 10 s for the latest build's query,
# 35 s for the shutdown (mailctl's own limit is 30 s).
ensure_latest() {
    if ! hello || ! have_spawn; then
        return 0
    fi
    running=$(bounded 2 mailctl hello --build 2>/dev/null) || running=
    # The query's output goes to a regular file, not to a command
    # substitution: a substitution waits for EOF on its pipe, and
    # flatpak-spawn hands that pipe to the portal and the new sandbox, which
    # the timeout does not kill, so a stalled portal would hold the window
    # past the bound. Reading a regular file never waits.
    latest=
    latest_file=$(mktemp "$state/latest-build.XXXXXX" 2>/dev/null) || latest_file=
    if [ -n "$latest_file" ]; then
        if bounded 10 flatpak-spawn --latest-version maild -build </dev/null >"$latest_file" 2>/dev/null; then
            read -r latest <"$latest_file" || true
        fi
        rm -f "$latest_file"
    fi
    if [ -n "$running" ] && [ -n "$latest" ] && [ "$running" != "$latest" ]; then
        bounded 35 mailctl shutdown >>"$state/maild.log" 2>&1 || true
    fi
}

# launch_maild: start maild through the Flatpak portal when none answers,
# and wait, for about 10 s in all, until it does. The deadline is the
# clock's, since each probe may take up to 2 s. No flatpak-spawn outlives
# this function: the normal start, --start-maild and the end of the window
# all rely on that.
launch_maild() {
    spawned=
    if have_spawn; then
        # No --watch-bus: the spawned maild must outlive this sandbox. It
        # holds its own stdout and stderr, so flatpak-spawn may end.
        flatpak-spawn --latest-version frostmail --background </dev/null >>"$state/maild.log" 2>&1 &
        spawned=$!
    fi
    deadline=$(($(date +%s) + 10))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        if hello; then
            break
        fi
        if [ -z "$spawned" ] || ! kill -0 "$spawned" 2>/dev/null; then
            break
        fi
        sleep 0.5
    done
    # flatpak-spawn forwards catchable signals (TERM, INT, HUP) to the
    # command it spawned, so only SIGKILL ends flatpak-spawn alone; without
    # --watch-bus the portal keeps the spawned maild, and maild already
    # holds its own stdout and stderr. This runs on every path before the
    # launcher returns: no flatpak-spawn outlives it.
    if [ -n "$spawned" ]; then
        kill -KILL "$spawned" 2>/dev/null
        wait "$spawned" 2>/dev/null
    fi
}

# start_local: the local start, which the normal start and --start-maild
# share. It makes sure the latest maild runs, and returns 0 when a maild
# answers afterwards, else 1 without a word: maild_not_started says why.
start_local() {
    ensure_latest
    if ! hello; then
        launch_maild
    fi
    hello
}

# maild_not_started: the launcher's last line when no maild answers after a
# start, on the error output and in the log that the portal's maild writes.
maild_not_started() {
    msg="frostmail: maild did not start through the Flatpak portal (flatpak-spawn); see $state/maild.log"
    echo "$msg" >&2
    echo "$msg" >>"$state/maild.log"
}

# ask_background: ask the Background portal to run `frostmail --background`
# at login, in the background. The helper stays on the bus until the portal
# answers (it asks the user once).
ask_background() {
    "$request_background" >>"$state/background.log" 2>&1 &
}

# The mode: local or lodge. When maild cannot say (a malformed app.json, no
# maild), the launcher treats it as local; maild's own check decides.
mode=$(maild -mode 2>/dev/null) || mode=local

if [ "${1:-}" = "--background" ]; then
    if [ "$mode" = "lodge" ]; then
        exit 0
    fi
    prepare_state
    # maild rotates the log itself, with -log, once it holds the data
    # directory's lock: a shell check cannot tell 2 launchers apart, and a
    # duplicate must not move the log of the maild that runs. The redirect
    # keeps what maild says before the lock, such as "another maild uses".
    # Both the shell and maild open the file for appending.
    # shellcheck disable=SC2094
    exec maild -log "$state/maild.log" >>"$state/maild.log" 2>&1
fi

if [ "${1:-}" = "--start-maild" ]; then
    # The Server pane's return to local mode: start maild and nothing else,
    # and never exec the app.
    if [ "$mode" = "lodge" ]; then
        exit 0
    fi
    prepare_state
    start_local
    ask_background
    if hello; then
        exit 0
    fi
    maild_not_started
    exit 1
fi

if [ "$mode" = "lodge" ]; then
    # The window talks to the maild of lodge's host: nothing runs here.
    exec "$app" "$@"
fi

prepare_state
start_local || maild_not_started

ask_background

exec "$app" "$@"
