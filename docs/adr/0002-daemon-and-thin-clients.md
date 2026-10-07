# 0002 — A headless daemon with thin clients

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

The goal is a daily-driver Linux mail client that works like Apple
Mail.app and looks like neither GNOME nor KDE. The two hardest UI surfaces in
a mail client are web problems: real-world HTML mail needs a browser engine
to display, and composing needs a mature rich-text editor. Mail must keep
syncing, sending scheduled mail and notifying while the window is closed. The
code is written mostly by a local model, which is most reliable in Go and
TypeScript.

Toolkits considered for the UI: Flutter's Linux embedder is GTK3 underneath
and has historically lacked platform views, so webviews render offscreen into
textures, which breaks text selection and smooth scrolling in the pane that
matters most. GTK and Qt look like their desktops.

## Decision

- `maild`, a per-user Go daemon (pure Go, `CGO_ENABLED=0`), owns IMAP, SMTP,
  OAuth, sync, the SQLite store, the blob store, threading, rules and
  notifications. It serves JSON-RPC 2.0 on a Unix socket
  ([ADR-0007](0007-rpc-contract.md), [rpc-protocol](../specs/rpc-protocol.md)).
- The app is Tauri v2 with a React + TypeScript UI in the system WebKitGTK.
  Its Rust layer is a line bridge to the socket plus the `mailpart://`
  protocol, and holds no mail logic.
- `mailctl` is a CLI client of the same API.
- Desktop integration goes over D-Bus: notifications through
  `org.freedesktop.Notifications`, secrets through Secret Service. Both are
  desktop-neutral.

## Consequences

- The engine is built and tested headless, against in-process and container
  IMAP servers, before any UI exists. Modules have hard contracts, which suits
  small executor task cards.
- The UI can be replaced without touching the engine.
- Sync continues without the window (systemd user unit; the Background portal
  under Flatpak, M6).
- Two processes must be versioned together: `rpc.hello` negotiates the
  protocol, and changes within a major version are additive.
- Tauri on Linux still links GTK3 and WebKitGTK for the window and webview;
  nothing is drawn with GTK widgets.

## Alternatives considered

- **Flutter:** HTML rendering and rich-text editing are its weakest areas on
  Linux, and it is GTK3 underneath anyway.
- **GTK4/libadwaita or Qt:** look native to one desktop, which is the problem
  being solved.
- **Rust engine:** better MIME libraries (Stalwart's) and direct linking into
  Tauri, but a local model writes correct Go far more reliably, and the daemon
  boundary is wanted regardless.
- **Electron:** ships its own Chromium; heavier than the system WebKitGTK with
  no gain for this app.

## References

- Shapes: [design/overview.md](../design/overview.md), [specs/rpc-protocol.md](../specs/rpc-protocol.md)
- Related: [ADR-0005](0005-html-mail-rendering.md), [ADR-0007](0007-rpc-contract.md)
