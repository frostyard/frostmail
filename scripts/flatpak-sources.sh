#!/bin/sh
# Regenerate the Flatpak's offline sources (make flatpak-sources;
# docs/adr/0045-the-desktop-app-builds-as-a-flatpak-in-ci.md):
#
#   packaging/flatpak/go-sources.json   the Go modules of the app, maild and
#                                       mailctl, from go.mod and go.sum
#
# Run it after any change to those 2 files; CI regenerates the file and fails
# when it differs. It needs the network and Go on the host. The generator is
# tools/flatpakgo, in this repository.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
out=packaging/flatpak

# The Go modules: one file source per module, with its checksum, for a
# file:// GOPROXY (the frostmail module in the manifest).
go run ./tools/flatpakgo >"$out/go-sources.json.tmp"
mv "$out/go-sources.json.tmp" "$out/go-sources.json"
