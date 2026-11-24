#!/bin/sh
# Build the executable from the NSGo manifest in native and offline package
# builds. The app ID is also the namespace of NSGo's desktop launch handoff.
set -eu
out=${1:?usage: build-nsgo-native.sh output}
case "$out" in
    /*) ;;
    *) out="$PWD/$out" ;;
esac
cd "$(dirname "$0")/../cmd/frostmail-nsgo"
id=$(sed -n 's/^id: //p' nsgo.yaml)
name=$(sed -n 's/^name: //p' nsgo.yaml)
version=$(sed -n 's/^version: //p' nsgo.yaml)
: "${id:?missing NSGo app ID}" "${name:?missing NSGo app name}" "${version:?missing NSGo app version}"
core=github.com/nsgo-dev/nsgo/core
flags="-s -w -X '$core.appID=$id' -X '$core.appName=$name' -X '$core.appVersion=$version'"
if [ "${GOOS:-}" = windows ]; then
    flags="$flags -H=windowsgui"
fi
go build -trimpath -ldflags "$flags" -o "$out" .
