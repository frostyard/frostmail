# 0013 — The app installs as a local Flatpak; maild stays on the host

- **Status:** Accepted
- **Date:** 2026-10-08

## Context

M4 makes Frostmail the user's daily client, which needs the app installed on
their desktop. The user's host is image-based (Snow Linux, Debian 13) and
has no WebKitGTK, so the release binary that `make app-build` produces in
the nsl machine cannot run there as a plain file in `~/.local/bin`. The
nsl machine has WebKitGTK, but Unix sockets and the session bus do not
cross into it, so an app running there cannot reach a host maild or the
desktop's notifications. The host has Flatpak with the GNOME 51 runtime,
which carries `webkit2gtk-4.1`, `libsoup-3.0` and GTK 3: every library
the binary links. maild is a static Go binary that runs on any host. M6
was always going to ship a Flatpak.

## Decision

- **maild and mailctl stay on the host:** `make install` puts them in
  `~/.local/bin` and runs maild as a systemd user service.
- **The app is a local Flatpak** (`org.frostyard.Frostmail`, the app's
  identifier) on `org.gnome.Platform//51`. `make install-app` wraps the
  binary built in nsl, without flatpak-builder or the SDK
  (`flatpak build-init`, `build-finish`, `build-export`, `build-bundle`),
  and installs the bundle for the user.
- **Sandbox:** Wayland with an X11 fallback, IPC and the GPU; the
  `xdg-run/frostmail` directory (maild's socket); `xdg-cache/frostmail`
  read-only (the decoded parts maild writes); the home directory
  read-only, for attaching files. No network: the app never fetches
  anything itself (ADR-0005).
- **Paths:** inside the Flatpak the app reads maild's cache from the
  host's cache directory (`$HOST_XDG_CACHE_HOME`, else `~/.cache`), since
  its own `XDG_CACHE_HOME` is private.
- **Launching:** `~/.local/bin/frostmail` runs the Flatpak; maild's unit
  names it in `FROSTMAIL_APP`, so a notification click opens the message
  in the running app (single instance, docs/design/desktop.md).

## Consequences

- The app runs on any host with Flatpak, including image-based ones,
  and M6 starts from a working sandbox rather than a new one.
- The binary is built against Debian 13 and run on the GNOME runtime. That
  works while the runtime's libraries are as new or newer; M6 builds
  inside the SDK instead (a flatpak-builder manifest with vendored crates
  and npm packages).
- Read-only home access is broader than the file chooser portal needs;
  M6 narrows it once attaching goes through the portal.
- Opening links and attachments goes through `xdg-open` in the sandbox,
  which calls the OpenURI portal.

## Alternatives considered

- **A distrobox with WebKitGTK:** least work, but distrobox would stay
  part of the daily setup and it is not the release path.
- **The app in the nsl machine through Waypipe:** already used for
  development, but the socket and the session bus do not reach the host.
- **Building inside the SDK now:** the M6 manifest needs offline crate and
  npm sources; not needed to start the trial.

## References

- Shapes: [design/desktop.md](../design/desktop.md)
- Builds on: [ADR-0002](0002-daemon-and-thin-clients.md),
  [ADR-0005](0005-html-mail-rendering.md),
  [ADR-0006](0006-development-environment.md)
