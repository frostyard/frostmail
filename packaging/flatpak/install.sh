#!/usr/bin/env bash
# Wrap build/frostmail-app (make app-build) in a local Flatpak on the GNOME
# runtime and install it for this user, with ~/.local/bin/frostmail to run
# it (docs/adr/0013-app-as-a-local-flatpak.md). Needs no SDK: the binary is
# built in the nsl machine.
set -euo pipefail
cd "$(dirname "$0")/../.."

app=org.frostyard.Frostmail
runtime_version=51
out=build/flatpak
prefix=${PREFIX:-$HOME/.local}

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
# its parts cache and the home directory read-only. No network.
flatpak build-finish "$out/app" --command=frostmail \
	--socket=wayland --socket=fallback-x11 --share=ipc --device=dri \
	--filesystem=xdg-run/frostmail --filesystem=xdg-cache/frostmail:ro --filesystem=home:ro

flatpak build-export "$out/repo" "$out/app"
flatpak build-bundle "$out/repo" "$out/frostmail.flatpak" "$app"
flatpak --user install -y --noninteractive --reinstall --bundle "$out/frostmail.flatpak"
install -Dm755 packaging/flatpak/frostmail "$prefix/bin/frostmail"
echo "installed $app; run it from the app menu or with frostmail"
