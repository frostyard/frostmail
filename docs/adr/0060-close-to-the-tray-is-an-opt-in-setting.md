# 0060 — Close to the tray is an opt-in setting; the tray icon shows while the app runs

- **Status:** Accepted (Brian, 2026-11-15)
- **Date:** 2026-11-15

## Context

Mail.app keeps running when its last window closes, and its Dock icon
carries the unread count. The desktop app quits when its main window
closes. maild does not: it syncs and notifies from login on its own
(the Background portal in the Flatpak,
[ADR-0046](0046-the-desktop-flatpak-carries-its-own-maild.md)), so a
closed window loses no mail and no notification.

frostui v0.9.0 has a tray component for every app of the suite
(frostyard/frostui ADR-0012: `Tray`, `NewTray`, `TrayHosted`): 1 menu
shape (a status line, Open, the app's items, Quit), a dot drawn into the
icon, and `CloseToTray(e, enabled)`, which hides the window in its close
listener only when the app's own setting is on, no quit runs and a panel
hosts the icon. The facts that shape the app's use of it:

- **No tray keeps an app alive.** MyGo quits when the last window closes.
  To stay in the tray, the app must hide its window in place of closing
  it.
- **Not every desktop hosts a tray icon.** On Linux the icon is a
  StatusNotifierItem (`libayatana-appindicator3`). KDE Plasma hosts it;
  GNOME needs the AppIndicator extension, which Snow and Snowfield ship and
  enable. Without a host, MyGo makes the icon and reports no error, and
  nothing shows. A window hidden then could be found again only by a second
  launch.
- **A hidden window is not free.** It keeps GTK and the reader's WebKit
  views in memory.
- **The Flatpak lacks the parts.** The GNOME runtime has no
  `libayatana-appindicator3`, the sandbox may not talk to
  `org.kde.StatusNotifierWatcher`, and MyGo writes the icon to a file under
  the temporary directory, which in the sandbox is a private `/tmp` the
  host's panel cannot read.
- **The Background portal keeps 1 login command for each app.** It is
  maild's (`frostmail --background`).

## Decision

- **The tray icon shows while the app runs,** on every platform where
  `frostui.NewTray` succeeds. Its menu: "N unread" (the All Inboxes count,
  the count of the sidebar and of the launcher badge), Open Frostmail, New
  Message, Quit. A dot shows on the icon while the count is more than 0;
  the status line says the count too, so the dot is never the only cue.
  Where `NewTray` fails (no `libayatana-appindicator3` on a host install),
  the app runs as before, with no tray and no setting.
- **Close to tray is a setting, off by default:** "Close to tray" in the
  General pane, in a group "This Computer". It is this computer's choice,
  so the app keeps it beside `app.json` (the file `close-to-tray`; present
  means on), never in maild's settings, which every client of a maild
  behind lodge shares. With it off, a close quits as before.
- **The app asks frostui at each close.** The main window's `OnClose`
  calls `CloseToTray(e, setting)`. With the setting on, a panel hosting
  the icon (`TrayHosted`, which asks the session bus at that moment) and no
  quit running, the window hides; else the close goes on, and the app
  quits as before. The tray's Quit, the app menu's Quit and a logout always
  quit.
- **A hidden window comes back** from the tray's Open and the icon's
  activation (frostui), and from a second launch: `OnSecondInstance`, a
  notification's click and its Reply show the window before they restore
  and focus it.
- **No hidden window at login.** The tray shows while the app runs. maild
  keeps the 1 login command, its sync and its notifications.
- **The Flatpak carries the tray's parts.** It builds
  `libayatana-appindicator3` and its libraries from Flathub's shared
  modules (vendored, pinned), may talk to `org.kde.StatusNotifierWatcher`,
  and its launcher gives the app `TMPDIR=$XDG_RUNTIME_DIR/app/$FLATPAK_ID`,
  the app's runtime directory, which Flatpak shares with the host at the
  same path, so the panel can read the icon. maild keeps its own
  `TMPDIR`.

## Consequences

- A user who turns the setting on keeps the app, and its memory, after
  the window closes; one who does not sees no change but the icon.
- On GNOME without the AppIndicator extension there is no icon, and a
  close quits even with the setting on: no window is ever hidden with no
  way back but a second launch.
- The Flatpak grows by a few small native libraries (libdbusmenu,
  ayatana-ido, libayatana-indicator, libayatana-appindicator), which it
  builds from source. Their manifests are copies that need updating by hand from
  flathub/shared-modules; `packaging/flatpak/shared-modules/` is a risk
  path.
- The app's temporary files in the Flatpak go to a tmpfs that the host
  can read, as the user's own. The app writes no mail there: attachments
  go to the cache directory.
- The tray's count and the launcher badge read 1 count. When a dock
  draws the badge and a panel the icon, both show it.

## Alternatives considered

- **Always quit; tray only while a window is open:** simpler, but gives no
  way to keep the app at hand without its window.
- **Close to tray by default:** surprises a user whose panel shows no icon
  or who expects a close to free the memory; an opt-in costs 1 switch.
- **A hidden window at login:** it would need a second login command,
  which the Background portal does not keep beside maild's, and maild
  already notifies from login.
- **`mygo.NewTray` directly in the app:** each app of the suite would
  repeat the per-platform click rules, the watcher check and the close
  rule; frostui's component holds them once.
- **The setting in maild's settings:** a maild behind lodge serves several
  computers and the web app, and a tray is a property of 1 desktop.
- **A git submodule of flathub/shared-modules:** the CI checkout and the
  manifest's `dir` source would need submodule support; a vendored copy of
  3 files needs none.

## References

- Shapes: [design/desktop.md](../design/desktop.md#tray-icon),
  [specs/settings-ui.md](../specs/settings-ui.md#general-pane)
- Builds on: [ADR-0046](0046-the-desktop-flatpak-carries-its-own-maild.md),
  [ADR-0054](0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md);
  frostyard/frostui
  [ADR-0012](https://github.com/frostyard/frostui/blob/main/docs/adr/0012-one-tray-component-with-a-close-to-tray-check-the-app-switches.md)
