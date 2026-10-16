# 0038 — Windows is a second platform, with maild and the app as on Linux

- **Status:** Accepted
- **Date:** 2026-10-15

## Context

On 2026-10-08 the user kept Frostmail Linux-only after a portability
review. On 2026-10-15 they chose to support Windows, for a friend who will
read Hotmail on it. macOS comes later and is not ruled out.

Cross-compiling `maild` and `mailctl` for Windows on 2026-10-15 failed in
one place: the RPC server's same-user check (`SO_PEERCRED`) and its socket
ownership check (`syscall.Stat_t`). Everything else builds, but some parts
work on Linux only:

- **The socket:** maild listens on a 0600 Unix socket under
  `$XDG_RUNTIME_DIR`, and the app's bridge connects with tokio's
  `UnixStream`, which tokio does not offer on Windows.
- **Paths:** both sides resolve XDG directories, which Windows lacks.
- **Secrets:** they live in the Secret Service over D-Bus, with a plain
  0600 file as the fallback (ADR-0011). Windows' Credential Manager caps a
  secret at 2,560 bytes. Microsoft's refresh tokens are opaque and rotate,
  with no promised size.
- **Notifications and reminders:** they use `org.freedesktop.Notifications`
  over D-Bus. On Windows, a toast from a desktop program needs an
  AppUserModelID that a Start Menu shortcut carries.
- **Background running:** maild runs as a systemd user service, so it
  keeps syncing and reminding with the app closed (ADR-0002).
- **Time zones:** Go reads tzdata from the system on Linux. On Windows it
  reads `$GOROOT/lib/time`, which users do not have. Windows names its
  local zone with a Windows name, not an IANA one.
- **Rendering:** the app runs in WebKitGTK. Windows' webview is WebView2
  (Chromium). The reader's safety (ADR-0005's sandboxed `srcdoc` frame and
  CSP, ADR-0014's embedded images, ADR-0036's links) has only been tested
  on WebKitGTK. The app's WebKit workarounds are Linux-specific.
- **Development:** it runs on a Linux host, the nsl machine and incus
  (ADR-0006). The user has several Windows machines for trials and checks.

## Decision

- **Windows 10 and 11 on x64 are supported**, beside Linux. The design
  stays the same: a per-user maild owns sync, storage and the socket, and
  the app and mailctl are its clients. Platform code goes in per-OS files
  behind the existing seams (`rpcserver` listening, `config` paths, the
  `secrets.Store` interface, notifications), so macOS can be added the
  same way.
- **The socket is a named pipe** on Windows,
  `\\.\pipe\frostmail-<user SID>`. Its DACL grants only the user, and it
  rejects remote clients. maild checks each client's user SID, as
  `SO_PEERCRED` does on Linux. The bridge connects with tokio's named-pipe
  client and moves the same lines.
- **Paths:** the database, blobs and logs go under
  `%LOCALAPPDATA%\Frostmail`, and the parts cache under its `Cache`
  folder. Local app data does not roam, so SQLite stays on a local disk.
  `FROSTMAIL_SOCKET` and the other overrides work as on Linux.
- **Secrets:** a file encrypted with DPAPI for the user
  (`CryptProtectData`) behind `secrets.Store`. It has no size cap, and only
  the user's Windows sign-in can decrypt it. The plain file stays a
  development fallback.
- **Notifications:** Windows toasts carry Frostmail's AppUserModelID,
  which the installer's Start Menu shortcut registers. maild runs a small
  notifier that the app's build ships (`frostmail-notify.exe`, in Rust,
  using WinRT) once per notification. A click opens the message in the
  running app or starts it (single instance, as on Linux).
- **Background running:** maild starts when the user signs in to Windows
  (the per-user `Run` key), and the app starts it when it is not running.
  It is not a Windows service: it is per user, like its secrets.
- **Time zones:** maild embeds Go's tzdata, and maps Windows' local zone to
  IANA with the table it already has for Outlook's invitations.
- **Packaging:** a per-user NSIS installer that Tauri builds, with maild,
  mailctl, the notifier and the WebView2 bootstrapper. It needs no
  administrator rights. Releases are unsigned at first, so SmartScreen
  warns once per release. Signing and automatic updates come later.
- **Rendering:** the reader keeps the same frame, sandbox and CSP on
  WebView2. The zero-network hostile-mail suite and the link checks must
  pass on WebView2 before a release. The WebKit workarounds apply on
  Linux only.
- **Development:** it stays on Linux (ADR-0006). maild and mailctl
  cross-compile with `CGO_ENABLED=0`. A Windows CI job builds the app and
  runs the Go tests and the app's end-to-end tests on WebView2. The user's
  Windows machines are for installs and trials.
- **Linux does not change:** Flatpak (ADR-0013), the Secret Service, D-Bus
  notifications and the systemd user service stay as they are.

## Consequences

- Every new platform-specific piece needs a Windows version or a stated
  gap, and the Windows CI job keeps the build honest.
- Windows users read mail with the same engine, store and sync as Linux
  users. A fix lands on both.
- The reader's security has to be proved twice, on WebKitGTK and on
  WebView2, and kept proved as both update.
- Unsigned installers make the first install a "More info, Run anyway"
  step. That is acceptable for one friend, not for a public release.
- A per-user maild without a service stops when the user signs out of
  Windows, as it does on Linux without lingering.
- A toast is posted by a separate small program, not by maild itself,
  because WinRT toasts from Go would need COM bindings that maild does not
  have.

## Alternatives considered

- **Unix sockets on Windows (AF_UNIX):** Windows 10 has them and Go
  supports them, but tokio does not, and Windows gives no peer credentials
  for them. Named pipes have first-class support on both sides, and an
  ACL of their own.
- **Windows Credential Manager:** native and visible to the user, but
  capped at 2,560 bytes per secret.
- **maild as a Windows service:** it would run without anyone signed in,
  but it would hold one user's mail and secrets as a machine account.
- **Toasts from maild over PowerShell:** no extra binary, but it starts a
  PowerShell process per notification, and click handling is fragile.
- **Building the Windows app on Linux** (cargo-xwin, NSIS under Wine):
  Tauri calls it experimental. A Windows CI runner builds and tests it on
  the real webview.

## References

- Shapes: [design/desktop.md](../design/desktop.md),
  [design/accounts.md](../design/accounts.md),
  [design/app.md](../design/app.md),
  [design/testing.md](../design/testing.md),
  [plans/0010](../plans/0010-m7-microsoft-and-windows.md)
- Builds on: [ADR-0002](0002-daemon-and-thin-clients.md),
  [ADR-0005](0005-html-mail-rendering.md),
  [ADR-0006](0006-development-environment.md),
  [ADR-0011](0011-sign-in-and-credentials.md),
  [ADR-0013](0013-app-as-a-local-flatpak.md),
  [ADR-0036](0036-reader-links-open-as-new-window-requests.md)
