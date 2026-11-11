#!/bin/sh
# Build the Windows installer into build/mygo/windows-amd64 (make
# desktop-installer; plan 0015, Phase 9). maild, mailctl, the notifier and the
# toast's icon ship beside the app, as MyGo's resources for windows-amd64;
# packaging/windows/installer.nsh does the rest. Needs makensis off Windows;
# on Windows mygo downloads NSIS, checking its hash.
#
# Usage: windows-installer.sh NOTIFIER
#   NOTIFIER  frostmail-notify.exe, which cargo builds in app/notify
set -eu

notifier=$1
[ -f "$notifier" ] || {
	echo "windows-installer: no notifier at $notifier" >&2
	exit 1
}
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)
commit=$(git rev-parse --short HEAD 2>/dev/null || echo none)
stamp="-X main.version=$version -X main.commit=$commit"

res=resources/windows-amd64
rm -rf resources
mkdir -p "$res"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w $stamp -H=windowsgui" -o "$res/maild.exe" ./cmd/maild
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w $stamp" -o "$res/mailctl.exe" ./cmd/mailctl
cp "$notifier" "$res/frostmail-notify.exe"
cp packaging/icons/128x128.png "$res/frostmail.png"
MYGO_LDFLAGS="$stamp" go tool mygo build -platform windows/amd64
