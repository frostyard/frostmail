#!/usr/bin/env bash
# Run maild and the Tauri app together inside the nsl machine (make app-dev,
# make app-run).
# Unix sockets cannot cross the host/VM boundary and SQLite WAL must not live
# on /mnt/host, so this maild uses machine-local state under /var/tmp/frostmail
# (docs/adr/0006-development-environment.md). Run from the repository root.
set -euo pipefail

MAILD=${MAILD:-build/maild}
[[ -x "$MAILD" ]] || { echo "dev-app: $MAILD is missing; run make build on the host first" >&2; exit 1; }

export FROSTMAIL_DATA_DIR=/var/tmp/frostmail/maild
export FROSTMAIL_CACHE_DIR=/var/tmp/frostmail/cache
export FROSTMAIL_SOCKET=${XDG_RUNTIME_DIR:?}/frostmail/maild.sock

# The spike's mailpart:// check serves this file (removed with the spike in M2).
mkdir -p "$FROSTMAIL_CACHE_DIR/parts/spike"
cp app/src-tauri/icons/128x128.png "$FROSTMAIL_CACHE_DIR/parts/spike/frost.png"

"$MAILD" &
maild_pid=$!
trap 'kill "$maild_pid" 2>/dev/null; wait "$maild_pid" 2>/dev/null' EXIT
for _ in $(seq 50); do [[ -S "$FROSTMAIL_SOCKET" ]] && break; sleep 0.1; done
[[ -S "$FROSTMAIL_SOCKET" ]] || { echo "dev-app: maild did not create $FROSTMAIL_SOCKET" >&2; exit 1; }

# APP_BINARY runs a built app (make app-build) instead of tauri dev. Under
# tauri dev the page is http://127.0.0.1:5173, from which WebKitGTK will not
# load mailpart:// (a local scheme); built apps load from tauri://localhost.
if [[ -n "${APP_BINARY:-}" ]]; then
	"$APP_BINARY" "$@"
else
	cd app
	pnpm tauri dev --no-watch "$@"
fi
