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
  `internal/notify/desktop.go`) when new unread mail arrives in an inbox
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

## One app instance

The app is single-instance (`tauri-plugin-single-instance`): a second
launch passes its arguments to the running app, which shows and focuses
its main window, and exits. The first launch keeps its own
`--open-message` until the main window asks for it (`startup_message`);
later ones arrive as the `open-message` event (`app/src/app/openMessage.ts`). `--open-message
<id>` shows the main window, selects the message's inbox (or All Inboxes)
and the message, and focuses the window; with no running app it starts on
that message.
