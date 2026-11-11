#!/usr/bin/env bash
# Run maild and the desktop app together (make app-dev, make app-run), on
# machine-local state under /var/tmp/frostmail, away from a real install. Run
# from the repository root.
set -euo pipefail

MAILD=${MAILD:-build/maild}
[[ -x "$MAILD" ]] || { echo "dev-app: $MAILD is missing; run make build on the host first" >&2; exit 1; }

export FROSTMAIL_DATA_DIR=/var/tmp/frostmail/maild
export FROSTMAIL_CACHE_DIR=/var/tmp/frostmail/cache
export FROSTMAIL_SOCKET=${XDG_RUNTIME_DIR:?}/frostmail/maild.sock

"$MAILD" &
maild_pid=$!
trap 'kill "$maild_pid" 2>/dev/null; wait "$maild_pid" 2>/dev/null' EXIT
for _ in $(seq 50); do [[ -S "$FROSTMAIL_SOCKET" ]] && break; sleep 0.1; done
[[ -S "$FROSTMAIL_SOCKET" ]] || { echo "dev-app: maild did not create $FROSTMAIL_SOCKET" >&2; exit 1; }

"${APP_BINARY:-build/frostmail}" "$@"
