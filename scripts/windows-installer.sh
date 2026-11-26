#!/bin/sh
# Build the Windows installer, whose program is the NSGo client, into
# build/windows-installer (make windows-installer; plan 0017, Phase 3;
# docs/design/desktop.md, Windows).
#
# NSGo's tool builds the client and writes the installer's NSIS script
# (go tool nsgo build -target windows_x64). maild, mailctl, the notifier and
# the toast's icon ship beside the app, as the client's resources for this
# build alone. NSGo's script has no place for an app's own steps, so
# tools/nsgoinstaller writes packaging/windows/installer.nsh into it,
# and makensis compiles the result here.
#
# makensis is the one on PATH, or NSIS's own install on Windows. Without
# either, on Windows the script downloads NSIS, pinned and checked against
# its SHA-256, into build/nsis; elsewhere it stops after writing the script.
#
# Usage: windows-installer.sh NOTIFIER
#   NOTIFIER  frostmail-notify.exe, which cargo builds in app/notify
#
# Environment:
#   FROSTMAIL_VERSION          the release's version, as build-nsgo-native.sh
#                              takes it: the client's problem records carry
#                              it, and MAJOR.MINOR.PATCH is the installer's
#                              version too
#   FROSTMAIL_INSTALLER_CHECK  1 builds a check of the installer into
#                              build/windows-installer-check, under names of
#                              its own (packaging/windows/installer-check.nsh),
#                              which installs beside a real install; delete
#                              builds the check whose silent uninstall
#                              deletes the check's mail
set -eu

die() {
	printf 'windows-installer: %s\n' "$*" >&2
	exit 1
}

[ "$#" -eq 1 ] || die 'usage: windows-installer.sh NOTIFIER'
notifier=$1
[ -f "$notifier" ] || die "no notifier at $notifier"
case "$notifier" in
	/* | [A-Za-z]:*) ;;
	*) notifier=$PWD/$notifier ;;
esac
cd "$(dirname "$0")/.."

app=cmd/frostmail-nsgo
name=$(sed -n 's/^name: //p' "$app/nsgo.yaml")
id=$(sed -n 's/^id: //p' "$app/nsgo.yaml")
version=$(sed -n 's/^version: //p' "$app/nsgo.yaml")
: "${name:?missing NSGo app name}" "${id:?missing NSGo app ID}" "${version:?missing NSGo app version}"
# A release's version, MAJOR.MINOR.PATCH, is the app's own too, in place of
# the manifest's (scripts/build-nsgo-native.sh has the same rule).
case "${FROSTMAIL_VERSION:-}" in
	*[!0-9.]* | "" | .* | *. | *..*) ;;
	*.*.*.*) ;;
	*.*.*) version=$FROSTMAIL_VERSION ;;
esac

out=build/windows-installer
check=${FROSTMAIL_INSTALLER_CHECK:-}
delete_mail=
case "$check" in
	"") ;;
	1 | delete)
		[ "$check" = delete ] && delete_mail=1
		name="$name Check"
		id=$id.Check
		out=$out-check
		;;
	*) die "FROSTMAIL_INSTALLER_CHECK is '$check'; use 1 or delete" ;;
esac

# The resources are every NSGo build's (the web build's too), so they are
# here only while this build runs.
res=$app/resources
[ ! -e "$res" ] || die "$res exists: this script makes it and removes it; remove what an interrupted run left"
trap 'rm -rf "$res"' EXIT
trap 'exit 1' HUP INT TERM
mkdir "$res"

stamp="-X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev) -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo none)"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w $stamp -H=windowsgui" -o "$res/maild.exe" ./cmd/maild
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w $stamp" -o "$res/mailctl.exe" ./cmd/mailctl
cp "$notifier" "$res/frostmail-notify.exe"
cp packaging/icons/128x128.png "$res/frostmail.png"

rm -rf "$out"
(cd "$app" && go tool nsgo build -target windows_x64 -output "../../$out" \
	-name "$name" -id "$id" -version "$version" \
	-ldflags "-X main.version=${FROSTMAIL_VERSION:-dev}")
rm -rf "$res"
# What NSGo releases is its own: install scripts for a GitHub release and,
# where it found makensis, an installer without Frostmail's additions.
rm -rf "$out/releases"

pkg=$out/$name-$version-windows_x64
nsi=$pkg/$name.nsi
[ -f "$nsi" ] || die "nsgo build wrote no $nsi"
setup=$(sed -n 's/^OutFile "\(.*\)"$/\1/p' "$nsi")
[ -n "$setup" ] || die "$nsi names no OutFile"
rm -f "$pkg/$setup"
for f in "$name.exe" maild.exe mailctl.exe frostmail-notify.exe frostmail.png; do
	[ -f "$pkg/$f" ] || die "the package has no $f"
done

set -- -define "FM_NAME=$name" -define "FM_EXE=$name.exe" -define "FM_APP_ID=$id"
if [ -n "$delete_mail" ]; then
	set -- "$@" -define FM_DELETE_SILENT=IDYES
fi
if [ -n "$check" ]; then
	set -- "$@" packaging/windows/installer-check.nsh
fi
go run ./tools/nsgoinstaller -script "$nsi" "$@" packaging/windows/installer.nsh

# NSIS 3.13, the official zip, pinned by its SHA-256 (the release MyGo
# v0.10.0 pinned, from its mirror or from SourceForge).
nsis_version=3.13
nsis_sha256=ba63dffc4410ee89193e1cb5a41989991bd77c61068da17e3156d136b7b0b3d8
makensis=
if command -v makensis >/dev/null 2>&1; then
	makensis=makensis
else
	case "$(uname -s)" in
		MINGW* | MSYS* | CYGWIN*)
			nsis=$PWD/build/nsis/nsis-$nsis_version
			for m in "/c/Program Files (x86)/NSIS/makensis.exe" "/c/Program Files/NSIS/makensis.exe" "$nsis/Bin/makensis.exe"; do
				if [ -f "$m" ]; then
					makensis=$m
					break
				fi
			done
			if [ -z "$makensis" ]; then
				tmp=build/nsis/download
				rm -rf "$tmp" "$nsis"
				mkdir -p "$tmp"
				for url in \
					"https://github.com/egoist/mygo/releases/download/nsis-$nsis_version/nsis-$nsis_version.zip" \
					"https://downloads.sourceforge.net/project/nsis/NSIS%203/$nsis_version/nsis-$nsis_version.zip"; do
					rm -f "$tmp/nsis.zip"
					if curl -fsSL --retry 2 --connect-timeout 30 -o "$tmp/nsis.zip" "$url" &&
						[ "$(sha256sum "$tmp/nsis.zip" | cut -c1-64)" = "$nsis_sha256" ]; then
						echo "windows-installer: downloaded NSIS $nsis_version from $url, SHA-256 $nsis_sha256"
						break
					fi
					echo "windows-installer: $url did not give NSIS $nsis_version with its SHA-256" >&2
					rm -f "$tmp/nsis.zip"
				done
				[ -f "$tmp/nsis.zip" ] || die "could not download NSIS $nsis_version; install NSIS (makensis)"
				if command -v unzip >/dev/null 2>&1; then
					unzip -q "$tmp/nsis.zip" -d "$tmp"
				else
					# Windows' own tar reads a zip.
					/c/Windows/System32/tar.exe -xf "$tmp/nsis.zip" -C "$tmp"
				fi
				# The archive's files are in one top folder.
				top=
				for d in "$tmp"/*/; do
					top=${d%/}
				done
				[ -f "$top/Bin/makensis.exe" ] || die "the NSIS archive holds no Bin/makensis.exe"
				mv "$top" "$nsis"
				rm -rf "$tmp"
				makensis=$nsis/Bin/makensis.exe
			fi
			;;
	esac
fi
[ -n "$makensis" ] || die "no makensis: install NSIS 3.06 or later and compile $nsi, which is ready"

echo "windows-installer: compiling with $makensis $("$makensis" -VERSION)"
(cd "$pkg" && "$makensis" -V2 -INPUTCHARSET UTF8 "$name.nsi")
mv "$pkg/$setup" "$out/$setup"
ls -l "$out/$setup"
