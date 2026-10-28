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

# The mode: local or lodge. When maild cannot say (a malformed app.json, no
# maild), the launcher treats it as local; maild's own check decides.
mode=$(maild -mode 2>/dev/null) || mode=local

if [ "${1:-}" = "--background" ]; then
    if [ "$mode" = "lodge" ]; then
        exit 0
    fi
    mkdir -p "$state"
    # maild rotates the log itself, with -log, once it holds the data
    # directory's lock: a shell check cannot tell 2 launchers apart, and a
    # duplicate must not move the log of the maild that runs. The redirect
    # keeps what maild says before the lock, such as "another maild uses".
    # Both the shell and maild open the file for appending.
    # shellcheck disable=SC2094
    exec maild -log "$state/maild.log" >>"$state/maild.log" 2>&1
fi

if [ "$mode" = "lodge" ]; then
    # The window talks to the maild of lodge's host: nothing runs here.
    exec "$app" "$@"
fi

mkdir -p "$state"

have_spawn=
if command -v flatpak-spawn >/dev/null 2>&1; then
    have_spawn=1
fi

# An update replaces the engine, only forward: when the running maild's build
# is not the latest installed one, stop it. `mailctl shutdown` returns once
# maild has released the data directory's lock, so the maild started below
# can take it at once. Either build empty, or a failed call, keeps the maild
# that runs.
if mailctl hello >/dev/null 2>&1 && [ -n "$have_spawn" ]; then
    running=$(mailctl hello --build 2>/dev/null) || running=
    latest=$(flatpak-spawn --latest-version maild -build 2>/dev/null) || latest=
    if [ -n "$running" ] && [ -n "$latest" ] && [ "$running" != "$latest" ]; then
        mailctl shutdown >>"$state/maild.log" 2>&1 || true
    fi
fi

if ! mailctl hello >/dev/null 2>&1; then
    spawned=
    if [ -n "$have_spawn" ]; then
        # No --watch-bus: the spawned maild must outlive this sandbox. It
        # holds its own stdout and stderr, so flatpak-spawn may end.
        flatpak-spawn --latest-version frostmail --background </dev/null >>"$state/maild.log" 2>&1 &
        spawned=$!
    fi
    # Wait, for about 10 s, until that maild answers.
    tries=0
    while [ "$tries" -lt 20 ]; do
        if mailctl hello >/dev/null 2>&1; then
            break
        fi
        if [ -z "$spawned" ] || ! kill -0 "$spawned" 2>/dev/null; then
            break
        fi
        tries=$((tries + 1))
        sleep 0.5
    done
    # flatpak-spawn forwards catchable signals (TERM, INT, HUP) to the
    # command it spawned, so only SIGKILL ends flatpak-spawn alone; without
    # --watch-bus the portal keeps the spawned maild, and maild already
    # holds its own stdout and stderr.
    if [ -n "$spawned" ] && kill -0 "$spawned" 2>/dev/null; then
        kill -KILL "$spawned" 2>/dev/null
        wait "$spawned" 2>/dev/null
    fi
    if ! mailctl hello >/dev/null 2>&1; then
        msg="frostmail: maild did not start through the Flatpak portal (flatpak-spawn); see $state/maild.log"
        echo "$msg" >&2
        echo "$msg" >>"$state/maild.log"
    fi
fi

# Ask the Background portal to run `frostmail --background` at login. The
# helper stays on the bus until the portal answers (it asks the user once).
"$request_background" >>"$state/background.log" 2>&1 &

exec "$app" "$@"
