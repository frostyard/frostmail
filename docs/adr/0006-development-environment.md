# 0006 — Development environment: host engine, nsl app toolchain, incus mail server

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

The development host (Snow Linux 13, an atomic Debian trixie, AMD Strix
Halo, GNOME on Wayland) cannot install `-dev` packages. It has Go 1.27 and
Rust. Tauri needs WebKitGTK development headers. Two tools are available:

- **nsl:** persistent distro machines (systemd-nspawn in one VM). The host
  home is at `/mnt/host`, machine ports are reachable on host `127.0.0.1`, and
  Wayland windows reach the host through Waypipe. Limits found or documented:
  host file edits raise no inotify events inside machines; idle machines stop
  even with a service running; Unix sockets do not cross the VM boundary;
  there is no `/dev/dri` (software rendering); `--cloud-init` is specified but
  not implemented yet.
- **incus:** system containers on the host.

## Decision

- **Engine on the host.** maild, mailctl and all Go tests build and run
  natively (pure Go).
- **App toolchain in the nsl machine `frostmail`** (Debian 13), provisioned
  by `dev/nsl/provision.sh` with pinned, checksum-verified Node 24, Go and
  Rust, and Debian's WebKitGTK `-dev` packages. Build output and the pnpm
  store stay machine-local in `/var/tmp/frostmail`. Vite polls when `NSL=1`.
  `scripts/dev-app.sh` runs a machine-local maild beside the app, because the
  socket cannot cross the VM.
- **The test mail server in incus:** `frostmail-mailtest` runs Dovecot 2.4
  and Postfix, seeded and snapshotted as `clean`. It delivers only to
  `mailtest.test`.
- **GPU checks on the host:** release binaries built in nsl link only
  libraries the host also has, so `build/frostmail-app` runs natively.
  `FROSTMAIL_WEBKIT_SAFE=1` selects the WebKit CPU-rendering workarounds
  (`WEBKIT_DISABLE_DMABUF_RENDERER`, `WEBKIT_SKIA_ENABLE_CPU_RENDERING`) if a
  WebKitGTK update regresses on AMD.

## Consequences

- No host package changes; everything is reproducible from the repository.
- App debugging inside nsl uses software rendering; GPU behavior is checked
  with a native run.
- Dovecot 2.4 config syntax differs from 2.3; edit
  `dev/incus/dovecot.conf` with care and validate with `doveconf -n`.
- pnpm 12 ignores `npm_config_store_dir`; `app/pnpm-workspace.yaml` sets
  `storeDir`.

## Alternatives considered

- **Toolbox or distrobox:** would share the host home and sockets, but
  frostyard standardizes on nsl.
- **Mail server in nsl:** idle machines stop.
- **Flatpak SDK builds for development:** slower to iterate; that is the M6
  packaging path.

## References

- Shapes: [design/testing.md](../design/testing.md), `dev/`, `scripts/dev-app.sh`
