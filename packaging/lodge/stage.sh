#!/bin/sh
# Stage maild as lodge's mail app under DESTDIR, in lodge's install layout
# (docs/specs/lodge-app.md, What maild installs): maild and mailctl, the
# manifest with VERSION, the web build when BUILD/web holds one, the units
# (with the key unit's helper) and the sysusers.d entry. make lodge-package
# runs it.
#
# It writes only under DESTDIR/usr (core ADR-0058: a sysext payload is /usr
# only): binaries in usr/bin, the key unit's helper in usr/libexec/frostmail,
# units in usr/lib/systemd/system, the sysusers.d entry in usr/lib/sysusers.d,
# the manifest and web build in usr/share/lodge.
#
#   packaging/lodge/stage.sh DESTDIR VERSION [BUILD]
#
# DESTDIR may be a lodge root that other apps and lodge itself share
# (dev/lodge/run.sh), so this removes nothing of theirs: only mail's own
# app directory is replaced, so a web build's stale files go, and every
# other file it stages is overwritten in place. DESTDIR must not be empty
# or /.
set -eu

usage="usage: packaging/lodge/stage.sh DESTDIR VERSION [BUILD]"
dest=${1:?$usage}
version=${2:?$usage}
build=${3:-build}
here=$(cd "$(dirname "$0")" && pwd)

if [ "$(realpath -m -- "$dest")" = / ]; then
  echo "stage.sh: refusing to stage into /; install from a staged root instead" >&2
  exit 1
fi
case $version in
  *[!A-Za-z0-9._+-]*) echo "stage.sh: version $version has characters a manifest cannot carry" >&2; exit 1 ;;
esac

app=$dest/usr/share/lodge/apps/mail
rm -rf -- "$app"
install -Dm755 "$build/maild" "$dest/usr/bin/maild"
install -Dm755 "$build/mailctl" "$dest/usr/bin/mailctl"
install -d "$app"
sed "s/\"version\": \"[^\"]*\"/\"version\": \"$version\"/" "$here/app.json" > "$app/app.json"
if [ -d "$build/web" ]; then cp -R "$build/web" "$app/web"; fi
install -Dm644 "$here/frostmail.socket" "$dest/usr/lib/systemd/system/frostmail.socket"
install -Dm644 "$here/frostmail.service" "$dest/usr/lib/systemd/system/frostmail.service"
install -Dm644 "$here/sysusers.d-frostmail.conf" "$dest/usr/lib/sysusers.d/frostmail.conf"
install -Dm644 "$here/frostmail-secrets-key.service" "$dest/usr/lib/systemd/system/frostmail-secrets-key.service"
install -Dm755 "$here/make-secrets-key.sh" "$dest/usr/libexec/frostmail/make-secrets-key"
