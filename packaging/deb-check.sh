#!/bin/sh
# Check a staged .deb (make deb-check; frostyard/core ADR-0011, ADR-0058).
#
#   packaging/deb-check.sh FILE PACKAGE VERSION
#
# It fails unless the metadata is right, every path is under ./usr/, every
# entry is owned root/root and none is group- or world-writable, and the
# payload is there: maild and mailctl, the units, the sysusers.d entry, and
# the lodge app's manifest (with VERSION) and web build.
set -eu

usage="usage: packaging/deb-check.sh FILE PACKAGE VERSION"
deb=${1:?$usage}
pkg=${2:?$usage}
version=${3:?$usage}

fail() { echo "deb-check: $*" >&2; exit 1; }
[ -f "$deb" ] || fail "$deb does not exist"

field() { dpkg-deb -f "$deb" "$1"; }
[ "$(field Package)" = "$pkg" ] || fail "Package is $(field Package), want $pkg"
[ "$(field Version)" = "$version" ] || fail "Version is $(field Version), want $version"
[ "$(field Architecture)" = amd64 ] || fail "Architecture is $(field Architecture), want amd64"

list=$(mktemp)
trap 'rm -f "$list"' EXIT
dpkg-deb -c "$deb" > "$list"
cat "$list"

# Columns: mode owner/group size date time path [-> target].
while read -r mode owner _ _ _ path _; do
  case $path in
    ./usr/ | ./usr/*) ;;
    *) fail "path outside ./usr/: $path" ;;
  esac
  [ "$owner" = root/root ] || fail "$path is owned by $owner, want root/root"
  case $mode in
    ?????w???? | ????????w?) fail "$path is group- or world-writable ($mode)" ;;
  esac
done < "$list"

want() { grep -q " $1\$" "$list" || fail "missing $1"; }
want ./usr/bin/maild
want ./usr/bin/mailctl
want ./usr/lib/systemd/system/frostmail.socket
want ./usr/lib/systemd/system/frostmail.service
want ./usr/lib/sysusers.d/frostmail.conf
want ./usr/share/lodge/apps/mail/app.json
want ./usr/share/lodge/apps/mail/web/index.html

manifest=$(dpkg-deb --fsys-tarfile "$deb" | tar -xOf - ./usr/share/lodge/apps/mail/app.json)
printf '%s\n' "$manifest" | grep -q "\"version\": \"$version\"" || fail "app.json does not carry version $version"
echo "deb-check: $deb ok"
