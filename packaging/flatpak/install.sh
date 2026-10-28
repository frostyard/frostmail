#!/usr/bin/env bash
# Wrap build/frostmail-app (make app-build) in a local Flatpak on the GNOME
# runtime and install it for this user, with ~/.local/bin/frostmail to run
# it (docs/adr/0013-app-as-a-local-flatpak.md). Needs no SDK: the binary is
# built in the nsl machine.
#
# This is the development bundle, with a maild on the host (make install). It
# is not the release: the release Flatpak carries its own maild, a launcher
# and mailctl (packaging/flatpak/org.frostyard.Frostmail.json, ADR-0046), and
# never reaches a host maild. The bundle has the same app ID, so install one
# of them at a time.
set -euo pipefail
cd "$(dirname "$0")/../.."

app=org.frostyard.Frostmail
runtime_version=51
out=build/flatpak
prefix=${PREFIX:-$HOME/.local}

# The host maild's socket and parts cache, as internal/config resolves them,
# computed now and handed to the sandbox as absolute paths: in a Flatpak with
# this app ID, the app's own paths.rs looks in the app's shared runtime
# directory and in its own cache, which are the release's, not the host's.
if [[ -n ${FROSTMAIL_SOCKET:-} ]]; then
	host_socket=$FROSTMAIL_SOCKET
else
	host_socket=${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/frostmail/maild.sock
fi
if [[ -n ${FROSTMAIL_CACHE_DIR:-} ]]; then
	host_cache=$FROSTMAIL_CACHE_DIR
else
	host_cache=${XDG_CACHE_HOME:-$HOME/.cache}/frostmail
fi

if [[ ! -x build/frostmail-app ]]; then
	echo "build/frostmail-app is missing; run make app-build" >&2
	exit 1
fi
if ! flatpak info --user org.gnome.Platform//$runtime_version >/dev/null 2>&1 &&
	! flatpak info --system org.gnome.Platform//$runtime_version >/dev/null 2>&1; then
	echo "install the runtime first: flatpak --user install flathub org.gnome.Platform//$runtime_version" >&2
	exit 1
fi

rm -rf "$out"
mkdir -p "$out"
flatpak build-init "$out/app" "$app" org.gnome.Sdk org.gnome.Platform "$runtime_version"
install -Dm755 build/frostmail-app "$out/app/files/bin/frostmail"
install -Dm644 "packaging/flatpak/$app.desktop" "$out/app/files/share/applications/$app.desktop"
for size in 32 128; do
	install -Dm644 "app/src-tauri/icons/${size}x${size}.png" \
		"$out/app/files/share/icons/hicolor/${size}x${size}/apps/$app.png"
done
install -Dm644 app/src-tauri/icons/icon.png "$out/app/files/share/icons/hicolor/512x512/apps/$app.png"

# The sandbox (ADR-0013): display, GPU and IPC; maild's socket directory;
# its parts cache and the home directory read-only, and the two paths above
# in the environment, which win over the app's Flatpak defaults. The network
# is for the Rust layer to reach a maild behind lodge, and the notification
# service for its announcements, which that maild cannot show
# (docs/design/desktop.md, On a maild behind lodge); the webview's CSP still
# forbids every fetch.
flatpak build-finish "$out/app" --command=frostmail \
	--socket=wayland --socket=fallback-x11 --share=ipc --device=dri --share=network \
	--talk-name=org.freedesktop.Notifications \
	--filesystem=xdg-run/frostmail --filesystem=xdg-cache/frostmail:ro --filesystem=home:ro \
	--env=FROSTMAIL_SOCKET="$host_socket" --env=FROSTMAIL_CACHE_DIR="$host_cache"

flatpak build-export "$out/repo" "$out/app"
flatpak build-bundle "$out/repo" "$out/frostmail.flatpak" "$app"
flatpak --user install -y --noninteractive --reinstall --bundle "$out/frostmail.flatpak"
install -Dm755 packaging/flatpak/frostmail "$prefix/bin/frostmail"
echo "installed $app; run it from the app menu or with frostmail"
