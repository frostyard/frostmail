#!/bin/sh
# Build the checksummed offline module proxy shared by maild and both GUI
# modules. A local replace (including the parent FrostMail module) already
# comes from the checkout.
set -eu

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
go build -o "$stage/flatpakgo" ./tools/flatpakgo
"$stage/flatpakgo" > "$stage/root.json"
(cd cmd/frostmail-nsgo && "$stage/flatpakgo") > "$stage/nsgo.json"

python3 - "$stage/root.json" "$stage/nsgo.json" packaging/flatpak/go-sources.json <<'PY'
import json
import sys
from pathlib import Path

root, gui, output = map(Path, sys.argv[1:])
sources = {}
for manifest in (root, gui):
    for source in json.loads(manifest.read_text()):
        key = (source["dest"], source.get("dest-filename", source["url"].rsplit("/", 1)[-1]))
        if key in sources and source != sources[key]:
            raise ValueError(f"conflicting module sources for {key}")
        sources[key] = source
output.write_text(json.dumps(list(sources.values()), indent=2) + "\n")
PY
