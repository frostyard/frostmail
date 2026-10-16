# Running on the desktop

Living document. How Frostmail runs as the user's daily client before
M6's release packaging: maild as a user service, the app as a local
Flatpak, notifications, and one app instance. Rationale: [ADR-0002](../adr/0002-daemon-and-thin-clients.md).

## Installing for one user

No root is needed. maild and mailctl run on the host; the app is a local
Flatpak, because the host may have no WebKitGTK
([ADR-0013](../adr/0013-app-as-a-local-flatpak.md)).

- `make install` builds release binaries and installs maild and mailctl
  in `~/.local/bin`, `packaging/systemd/maild.service` in
  `~/.config/systemd/user/`, then enables and restarts the service.
  `make uninstall` reverses it and leaves the data.
- `make install-app` builds the app in the nsl machine and runs
  `packaging/flatpak/install.sh`: the binary, the desktop entry
  (`org.frostyard.Frostmail.desktop`) and icons go into a Flatpak on
  `org.gnome.Platform//51`, installed for the user, and
  `~/.local/bin/frostmail` runs it. The runtime must be installed
  (`flatpak --user install flathub org.gnome.Platform//51`).

maild's data stays in `$XDG_DATA_HOME/frostmail`, its cache in
`$XDG_CACHE_HOME/frostmail`, its socket in
`$XDG_RUNTIME_DIR/frostmail/maild.sock`. The Flatpak sees the socket's
directory and, read-only, the cache and the home directory.

`maild.service` is a simple service: `ExecStart` maild,
`Restart=on-failure`, `RestartSec=2`, `FROSTMAIL_APP` set to
`~/.local/bin/frostmail`, started with the user's session
(`WantedBy=default.target`). maild keeps syncing, sending and notifying
while the app is closed.

## Application icon

The icon is a faceted ice-blue envelope carrying a snowflake-marked letter,
on a chamfered dark tile ([ADR-0015](../adr/0015-use-a-frostyard-mail-icon.md)).
Its colors come from core's `frostyard-design/tokens/colors.css`; the SVG
master records the token names and values in a comment.

Edit `app/src-tauri/icons/icon.svg`, then run `python3 scripts/render-icons.py`
with PyGObject, librsvg and GdkPixbuf installed. Commit the master and all three
rendered RGBA PNGs (32, 128 and 512 pixels). Inspect them at native size on
light and dark backgrounds. Tauri's icon configuration and the Flatpak
installer share these PNGs; rebuilding and reinstalling the app updates the
desktop icon.

## Notifications

- maild notifies through `org.freedesktop.Notifications` (godbus,
  `internal/notify/desktop_unix.go`) when new unread mail arrives in an inbox
  (Gmail: the `\Inbox` label). Only folders synced before collect new
  mail, so an account's first sync, and a folder resynced after a
  UIDVALIDITY change, announce nothing; mail that arrived while maild was
  stopped is announced at its next start. Announcements go out when a pass
  settles (`internal/mailsync/announce.go`).
- One notification per message: the sender's name as the summary, the
  subject and the start of the preview as the body, the app icon, and a
  default action. Four or more in one pass become one notification:
  "N new messages" with the first senders' names.
- A group notification replaces the account's previous group notification
  (`replaces_id`), so a busy inbox does not fill the shade; one-message
  notifications stay until dismissed.
- Clicking one (`ActionInvoked`, action `default`) runs
  `frostmail --open-message <id>` (a group runs `frostmail`);
  `FROSTMAIL_APP` names another binary. Without a session bus maild runs
  without notifications.
- Per account: notifications on or off (`account.update {notify}`); off
  for read-only accounts.
- Which new mail notifies is the `notifyScope` setting: Inbox only (the
  default), VIPs, Contacts, All Mailboxes or a smart mailbox; a rule can
  ask for a notification whatever the scope ([organize.md](organize.md#notifications)).
- A Remind Me reminder notifies when it fires, with "Reminder" before the
  subject ([organize.md](organize.md#remind-me)).

## Windows

ADR-0038; plan 0010's Phase 4 builds maild's side, and Phase 5 the app,
the notifier and the installer.

- **Builds:** `make build-windows` cross-compiles `maild.exe`, a GUI
  program (`-H windowsgui`) so that no console window opens at sign-in,
  and `mailctl.exe`, a console program, into `build/windows/`.
- **Starting at sign-in:** `maild -autostart on` names the running
  executable in the per-user Run key
  (`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, value
  `Frostmail`), and `-autostart off` removes it. The installer calls it;
  the app starts maild when it is not running. maild is not a service: it
  is per user, like its secrets and its pipe.
- **One maild:** a second maild finds the user's pipe taken and exits.
- **Where things are:** the installer puts the app and its sidecars in
  `%LOCALAPPDATA%\Frostmail`; maild keeps its database, blobs, sealed
  secrets and logs in its `Data` folder and decoded parts in `Cache`, so
  an install never touches them.
- **Logs:** `%LOCALAPPDATA%\Frostmail\Data\logs\maild.log`, rotated at
  start past 10 MB (one old file kept), and stderr too when maild has a
  console.
- **Notifications:** maild runs `frostmail-notify.exe`, from beside
  `maild.exe` or `FROSTMAIL_NOTIFIER`, once per change, with one JSON
  object on stdin:
  - `{"op":"show","tag":…,"group":…,"title":…,"body":…}`, with `"open":
    <message id>` for mail and `"reminder": <calendar.reminders id>` for a
    reminder, shows a toast, replacing one with the same tag and group;
  - `{"op":"remove","tag":…,"group":…}` removes one.

  Mail goes in group `mail`, with tag `m<message id>`, or `a<account id>`
  for a group note, which the next replaces. Reminders go in group
  `reminders` with tag `r` and a hash of the reminder's id, since a tag
  holds at most 64 characters. maild without the notifier sends no
  notifications.

  The notifier (`app/notify`, Rust with WinRT) posts toasts under the
  AppUserModelID `org.frostyard.Frostmail`, which Windows knows from the
  Start Menu shortcut and the installer's
  `HKCU\Software\Classes\AppUserModelId` key. Every activation is a
  `frostmail-toast:` URI, which the installer registers for the user as
  `frostmail-notify.exe --activate "%1"`, so a click works whether or not
  anything of Frostmail's is running:
  - a mail toast is `frostmail-toast:open?message=<id>`, and a group note
    `frostmail-toast:open`, which run `frostmail.exe --open-message <id>`
    (or `frostmail.exe`) from beside the notifier, or `FROSTMAIL_APP`;
  - a reminder (`scenario="reminder"`, on screen until acted on) opens the
    app on a click, and its Snooze and Dismiss are
    `frostmail-toast:snooze?reminder=<id>` and `…dismiss?…`, which call
    `calendar.snooze` (ten minutes, at the minute, as maild's own Snooze)
    and `calendar.dismiss` over the pipe, starting maild first if it is
    not running.

  Any program the user runs can send such a URI; each does only what the
  user could with a click, and a reminder's id is opaque. Without a
  console the notifier's errors reach only maild's log (when maild ran it).
- **The app on WebView2:** a release build is a GUI program. WebView2
  serves the app at `http://tauri.localhost` and custom schemes as
  `http://<scheme>.localhost`, so the navigation guard allows that origin
  on Windows only, the app rewrites maild's `mailpart://localhost/` URLs
  to `http://mailpart.localhost/` in what it shows and back in what it
  saves (`app/src/lib/partURL.ts`), and the Windows CSP
  (`tauri.windows.conf.json`) allows images from it. The commands that
  open windows (compose, event, settings, reminders) are async, since a
  window built in a synchronous command deadlocks WebView2's app; every
  window shares one WebView2 environment, so all take the same browser
  arguments (`main.rs`, `window`). The bridge opens the
  pipe at the identification level and starts `maild.exe` from beside the
  app when no pipe is there. The attachment types that never open
  (`open_part`) include Windows' own (`.lnk`, `.url`, `.hta`, `.reg` and
  the like).
- **The installer:** Tauri's NSIS installer, per user (no administrator
  rights), built by the Windows CI job (`frostmail-windows-installer`).
  It installs `frostmail.exe` with `maild.exe`, `mailctl.exe` and
  `frostmail-notify.exe` beside it, a Start Menu shortcut, and
  `frostmail.png` for the toasts' icon. Its hooks
  (`app/src-tauri/windows/hooks.nsh`):
  - before copying, rename each running sidecar aside (Windows lets a
    running file be renamed, not replaced) and stop the user's maild, so
    an app still open cannot restart the old maild;
  - after copying, delete the renamed files, register the AppUserModelID
    and `frostmail-toast:`, run `maild -autostart on`, and start maild as
    the user;
  - on uninstall, stop maild and remove both registrations (the template
    removes the Run value); "Delete the application data" also removes
    `Data` and `Cache`. An upgrade runs the old uninstaller in update
    mode, which keeps the data, the Run value and the registrations.

  Releases are unsigned (ADR-0038): SmartScreen asks once per release
  ("More info", "Run anyway").

## One app instance

The app is single-instance (`tauri-plugin-single-instance`): a second
launch passes its arguments to the running app, which shows and focuses
its main window, and exits. The first launch keeps its own
`--open-message` until the main window asks for it (`startup_message`);
later ones arrive as the `open-message` event (`app/src/app/openMessage.ts`). `--open-message
<id>` shows the main window, selects the message's inbox (or All Inboxes)
and the message, and focuses the window; with no running app it starts on
that message.
