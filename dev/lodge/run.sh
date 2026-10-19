#!/usr/bin/env bash
# maild as lodge's mail app on generated mail, in a lodge root, for lodge's
# development build (lodge's make e2e-build) and smoke tests
# (docs/specs/lodge-app.md, plan 0011). Never real mail: the account is
# tools/uifixture's, it sends to tools/smtpsink, and it has no IMAP server
# to reach, so it reads only what the fixture stored.
#
#   dev/lodge/run.sh ROOT [showcase|fixture]
#
# ROOT is the directory lodge-e2e runs with --root; keep it short, since a
# socket's path is at most 108 bytes. The first run stages maild, mailctl,
# the manifest and the web build (make web-build first, in the nsl machine)
# under ROOT in lodge's install layout, replacing only mail's own files (a
# root may hold lodge's state and other apps), and builds the mail:
#   showcase  hand-written mail (app/e2e/showcase), the default
#   fixture   200 mailgen messages and the hostile-HTML corpus
# Later runs keep ROOT's mail. maild serves ROOT/run/lodge/apps/mail.sock
# for the uid in LODGE_UID (default: this user's), until Ctrl-C.
set -euo pipefail

root=${1:?usage: dev/lodge/run.sh ROOT [showcase|fixture]}
kind=${2:-showcase}
here=$(cd "$(dirname "$0")/../.." && pwd)
cd "$here"
root=$(cd "$root" && pwd)
# The root is staged into, never removed, but it must be a lodge root of
# its own: never the machine's.
if [ "$root" = / ]; then
  echo "dev/lodge/run.sh: ROOT must be a directory of its own, not /" >&2
  exit 1
fi

[ -f build/web/index.html ] || { echo "no build/web: run make web-build first" >&2; exit 1; }
make -s lodge-package LODGE_DESTDIR="$root"
CGO_ENABLED=0 go build -o build/ ./tools/uifixture ./tools/smtpsink

data=$root/var/lib/frostmail
mkdir -p "$root/run/frostmail" "$root/var/cache/frostmail" "$root/var/lib/frostmail-sink"
chmod 700 "$root/run/frostmail"

# smtpsink keeps what the account sends in ROOT/var/lib/frostmail-sink, on
# the port the fixture was built with.
port_file=$root/var/lib/frostmail-sink/port
port=$(cat "$port_file" 2>/dev/null || echo 0)
build/smtpsink -addr "127.0.0.1:$port" -dir "$root/var/lib/frostmail-sink" > "$root/var/lib/frostmail-sink/log" &
sink=$!
trap 'kill $sink 2>/dev/null' EXIT
for _ in $(seq 50); do
  addr=$(sed -n 's/^listening //p' "$root/var/lib/frostmail-sink/log")
  [ -n "$addr" ] && break
  sleep 0.1
done
[ -n "$addr" ] || { echo "smtpsink did not start" >&2; exit 1; }
echo "${addr##*:}" > "$port_file"

if [ ! -e "$data/frostmail.db" ]; then
  case $kind in
    showcase) build/uifixture -out "$data" -showcase app/e2e/showcase -smtp "$addr" ;;
    fixture) build/uifixture -out "$data" -n 200 -hostile internal/render/testdata/hostile -smtp "$addr" ;;
    *) echo "unknown mail: $kind (showcase or fixture)" >&2; exit 1 ;;
  esac
fi

echo "maild: $root/run/lodge/apps/mail.sock; sent mail lands in $root/var/lib/frostmail-sink"
LODGE_ROOT=$root \
  FROSTMAIL_DATA_DIR=$data \
  FROSTMAIL_CACHE_DIR=$root/var/cache/frostmail \
  FROSTMAIL_SOCKET=$root/run/frostmail/maild.sock \
  FROSTMAIL_SECRETS=file \
  "$root/usr/local/bin/maild" --lodge
