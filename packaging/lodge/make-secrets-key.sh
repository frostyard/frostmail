#!/bin/sh
# Make the key that seals maild's secrets (ADR-0044, ADR-0047): 32 random
# bytes sealed as a systemd credential, in /etc/credstore.encrypted. It
# runs as root from frostmail-secrets-key.service, once. It never replaces
# a key, and it prints only paths and outcomes.
#
# Tests override CREDSTORE and SYSTEMD_CREDS.
set -eu
umask 077

dir=${CREDSTORE:-/etc/credstore.encrypted}
creds=${SYSTEMD_CREDS:-systemd-creds}
key=$dir/frostmail-secrets-key

if [ ! -d "$dir" ]; then
  install -d -m 0700 "$dir"
fi
if [ -e "$key" ]; then
  echo "make-secrets-key: $key exists and is kept"
  exit 0
fi

# A private directory beside the key, so that systemd-creds writes a new
# file however it writes it, and the link below stays on one file system.
tmpdir=$(mktemp -d "$dir/.frostmail-secrets-key.XXXXXX")
trap 'rm -rf -- "$tmpdir"' EXIT
tmp=$tmpdir/key
head -c 32 /dev/urandom | "$creds" encrypt --with-key=auto --name=frostmail-secrets-key - "$tmp"
chmod 0600 "$tmp"

# A hard link fails when the key exists, so a key that appears meanwhile
# is never replaced, and a partial file never passes for the key.
if ln "$tmp" "$key"; then
  echo "make-secrets-key: made $key"
elif [ -e "$key" ]; then
  echo "make-secrets-key: a key appeared at $key and is kept"
else
  echo "make-secrets-key: cannot publish $key" >&2
  exit 1
fi
