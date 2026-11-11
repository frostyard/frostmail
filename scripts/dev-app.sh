#!/usr/bin/env bash
# Run maild and the desktop app together (make app-dev, make app-run), on
# machine-local state under /var/tmp/frostmail, away from a real install. Run
# from the repository root.
set -euo pipefail

MAILD=${MAILD:-build/maild}
[[ -x "$MAILD" ]] || { echo "dev-app: $MAILD is missing; run make build on the host first" >&2; exit 1; }

export FROSTMAIL_DATA_DIR=/var/tmp/frostmail/maild
export FROSTMAIL_CACHE_DIR=/var/tmp/frostmail/cache
# Its own socket and app.json: the default socket is the user's maild, and
# the user's app.json can name their lodge.
export FROSTMAIL_SOCKET=${XDG_RUNTIME_DIR:?}/frostmail-dev/maild.sock
export XDG_CONFIG_HOME=/var/tmp/frostmail/config
# Its own secrets file: the host keyring would share account/1/password with a
# real maild on this machine.
export FROSTMAIL_SECRETS=file
# Its own single instance: the app's would hand this run's arguments to the
# user's app, or the user's to this one.
export FROSTMAIL_INSTANCE=dev

"$MAILD" &
maild_pid=$!
trap 'kill "$maild_pid" 2>/dev/null; wait "$maild_pid" 2>/dev/null' EXIT
for _ in $(seq 50); do [[ -S "$FROSTMAIL_SOCKET" ]] && break; sleep 0.1; done
[[ -S "$FROSTMAIL_SOCKET" ]] || { echo "dev-app: maild did not create $FROSTMAIL_SOCKET" >&2; exit 1; }

"${APP_BINARY:-build/frostmail}" "$@"
