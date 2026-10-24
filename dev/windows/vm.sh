#!/usr/bin/env bash
# The Windows test machine (docs/design/testing.md#windows): an incus
# Windows VM reached over SSH, as the Linux developer's way to run maild's
# Go suite and binaries on Windows (ADR-0038).
#
#   dev/windows/vm.sh setup          install the pinned Go into the VM
#   dev/windows/vm.sh sync           copy the worktree (tracked and new files) there
#   dev/windows/vm.sh go ARGS...     run go in the copy, through cmd, with its exit code
#   dev/windows/vm.sh ps COMMAND     run a PowerShell command in the copy
#   dev/windows/vm.sh push FILE...   copy files to the VM's bin directory
#
# FM_WIN_VM names the incus VM (frostmail-win11), FM_WIN_USER the Windows
# user to log in as (required), FM_WIN_KEY the SSH key
# (~/.ssh/frostmail-windows) and FM_WIN_DIR the work directory (C:\fm). The
# VM runs OpenSSH Server with the key authorized; everything lives in
# FM_WIN_DIR, outside the user's profile. Downloads are version-pinned and
# checksum-verified (frostyard/core ADR-0023): bump a pin and its checksum
# together, with go.mod's toolchain.
set -euo pipefail

GO_VERSION=1.27.3
GO_WINDOWS_SHA256=f26081f438319757f8a1b46d7d0b2c5ec7583d4038e485e0aa725cfde7885fff

VM=${FM_WIN_VM:-frostmail-win11}
KEY=${FM_WIN_KEY:-$HOME/.ssh/frostmail-windows}
DIR=${FM_WIN_DIR:-'C:\fm'}
: "${FM_WIN_USER:?set FM_WIN_USER to the Windows user to log in as}"
ROOT=$(git -C "$(dirname "$0")" rev-parse --show-toplevel)

ip=$(incus list "$VM" -f csv -c 4 | cut -d' ' -f1)
[[ -n "$ip" ]] || { echo "vm.sh: $VM has no address (is it running?)" >&2; exit 1; }
host="$FM_WIN_USER@$ip"
ssh_opts=(-i "$KEY" -o IdentitiesOnly=yes -o ConnectTimeout=10)
fwd=${DIR//\\//}
envs="\$env:Path=\"$DIR\\go\\bin;\"+\$env:Path; \$env:GOPATH=\"$DIR\\gopath\"; \$env:GOCACHE=\"$DIR\\gocache\";"
envs+=" \$env:GOTOOLCHAIN=\"local\"; \$env:CGO_ENABLED=\"0\"; Set-Location $DIR\\src;"

# shellcheck disable=SC2029 # the commands are built here, for the VM's PowerShell
remote() { ssh "${ssh_opts[@]}" "$host" "$@"; }

case "${1:-}" in
setup)
	remote "New-Item -ItemType Directory -Force $DIR | Out-Null
\$ErrorActionPreference = 'Stop'; \$ProgressPreference = 'SilentlyContinue'
\$zip = '$DIR\\go.zip'
Invoke-WebRequest -UseBasicParsing -Uri 'https://go.dev/dl/go$GO_VERSION.windows-amd64.zip' -OutFile \$zip
if ((Get-FileHash -Algorithm SHA256 \$zip).Hash.ToLower() -ne '$GO_WINDOWS_SHA256') { Remove-Item \$zip; throw 'checksum mismatch' }
if (Test-Path $DIR\\go) { Remove-Item -Recurse -Force $DIR\\go }
Expand-Archive -Path \$zip -DestinationPath $DIR; Remove-Item \$zip
& $DIR\\go\\bin\\go.exe version"
	;;
sync)
	tmp=$(mktemp --suffix=.tar)
	trap 'rm -f "$tmp"' EXIT
	(cd "$ROOT" && git ls-files -co --exclude-standard -z | tar --null -cf "$tmp" -T -)
	scp -q "${ssh_opts[@]}" "$tmp" "$host:$fwd/src.tar"
	remote "if (Test-Path $DIR\\src) { Remove-Item -Recurse -Force $DIR\\src }
New-Item -ItemType Directory $DIR\\src | Out-Null
tar -xf $DIR\\src.tar -C $DIR\\src; Remove-Item $DIR\\src.tar; 'synced to $DIR\\src'"
	;;
go)
	shift
	remote "$envs cmd /c 'go $* 2>&1'; exit \$LASTEXITCODE"
	;;
ps)
	shift
	remote "$envs $*"
	;;
push)
	shift
	remote "New-Item -ItemType Directory -Force $DIR\\bin | Out-Null"
	scp -q "${ssh_opts[@]}" "$@" "$host:$fwd/bin/"
	;;
*)
	sed -n '2,12p' "$0" >&2
	exit 2
	;;
esac
