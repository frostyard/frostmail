# 0045 — The desktop app builds as a Flatpak in CI

- **Status:** Proposed
- **Date:** 2026-10-26

## Context

[ADR-0013](0013-app-as-a-local-flatpak.md) wraps a binary built in the nsl
machine into a local Flatpak and left the build inside the SDK to M6.
[ADR-0041](0041-maild-runs-as-a-lodge-app.md) added the network and the
notification permissions. Core's
[plan 0010](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md),
Phase 5, wants each suite desktop app as a Flatpak that CI builds and
publishes to GHCR, for the Frostyard Flatpak remote. Frostnotes did this in
its [ADR-0031](https://github.com/frostyard/frostnotes/blob/main/docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md).
Frostmail differs on one point: maild is not part of the Flatpak. It runs on
the host (ADR-0013) or behind lodge (ADR-0041).

## Decision

- **CI builds the Flatpak inside the SDK, offline.** One manifest,
  `packaging/flatpak/org.frostyard.Frostmail.json`, has one module: the
  Tauri app. It uses `org.gnome.Platform//51`, `org.gnome.Sdk` and the
  `node24` and `rust-stable` SDK extensions. There is no Go and no maild in
  it. The binary installs as `/app/bin/frostmail`.
- **Sources files.** `cargo-sources.json` (from `app/src-tauri/Cargo.lock`)
  and `node-sources.json` (from `app/pnpm-lock.yaml`, pnpm store v11) let
  flatpak-builder fetch every crate and package before the build, so the
  build needs no network. `make flatpak-sources` writes them with pinned
  flatpak-builder-tools. The CI job "Flatpak sources" regenerates them and
  fails on a difference.
- **The sandbox stays as it is.** The permissions are those of
  `packaging/flatpak/install.sh`, as ADR-0013 and ADR-0041 set them: display,
  IPC, GPU, network, the notification service, `xdg-run/frostmail`,
  `xdg-cache/frostmail` read-only and the home read-only.
- **Tests run in the build:** `cargo test`, `desktop-file-validate`,
  `appstreamcli validate` and a check that the binary links every library.
- **Publishing.** A push of a `v*` tag exports an OCI image and pushes it to
  `ghcr.io/frostyard/frostmail` with the tags `vX.Y.Z` and `sha-<12 chars>`.
  The workflow attests the digest, and only then moves `latest` to it. The
  Frostyard remote serves an attested `latest` only. Pushes to main and pull
  requests build and test but publish nothing. Brian makes the GHCR package
  public after the first tag.
- **`make install-app` stays** for a local build, until the Frostyard remote
  lists the app.

## Open items

- **The file chooser portal waits.** ADR-0013 planned to narrow the home
  permission with it. The `xdg-portal` feature of `tauri-plugin-dialog` makes
  rfd show message dialogs (`ask`) through zenity, and the GNOME runtime has
  no zenity. The app keeps `--filesystem=home:ro` until that is solved.
- **Known limit: Save As.** With a maild behind lodge, the app's Rust layer
  writes the saved file itself, from the sandbox (`save_content`). The home
  is read-only there, so Save As to a folder in the home fails. With a maild
  on the host, maild writes the file (`message.save`), and Save As works.

## Consequences

- The app builds and tests on every pull request, in the runtime it runs on.
- A change of `Cargo.lock` or `pnpm-lock.yaml` needs `make flatpak-sources`.
- The Flatpak needs a maild: on the host, or behind lodge.
- The two new job names (Flatpak sources, Flatpak build and test) can join
  the required checks of the ruleset.

## References

- Builds on: [ADR-0013](0013-app-as-a-local-flatpak.md),
  [ADR-0041](0041-maild-runs-as-a-lodge-app.md)
- Shapes: [design/desktop.md](../design/desktop.md)
- Related: core plan 0010 Phase 5; frostnotes ADR-0031
