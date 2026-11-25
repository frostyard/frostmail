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
# FROSTMAIL_VERSION is the release's version, which the client's problem
# records carry (internal/problemrec): a release's build sets it. Any other
# build is "dev", and records nothing.
# A release's version, MAJOR.MINOR.PATCH, is the app's own too, in place of
# the manifest's: what the window and the platform show is the release's.
case "${FROSTMAIL_VERSION:-}" in
    *[!0-9.]* | "" | .* | *. | *..*) ;;
    *.*.*.*) ;;
    *.*.*) version=$FROSTMAIL_VERSION ;;
esac
flags="-s -w -X '$core.appID=$id' -X '$core.appName=$name' -X '$core.appVersion=$version' -X 'main.version=${FROSTMAIL_VERSION:-dev}'"
if [ "${GOOS:-}" = windows ]; then
    flags="$flags -H=windowsgui"
fi
go build -trimpath -ldflags "$flags" -o "$out" .
