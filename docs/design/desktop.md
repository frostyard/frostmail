# Running on the desktop

Living document. How Frostmail runs as the user's daily client before the
Flatpak (M6): maild as a user service, the app as a desktop application,
and notifications. Rationale: [ADR-0002](../adr/0002-daemon-and-thin-clients.md).

## Installing for one user

`make install` (no root) builds release binaries and installs maild and
mailctl with the user service (`packaging/systemd/maild.service`); `make uninstall`
removes them. How the app is installed on an image-based host without
WebKitGTK is still open (see below). The full set is:

| File | Path |
| --- | --- |
| maild, mailctl | `~/.local/bin/` |
| the app | `~/.local/bin/frostmail` |
| `maild.service` | `~/.config/systemd/user/` |
| `frostmail.desktop`, icon | `~/.local/share/applications/`, `~/.local/share/icons/hicolor/scalable/apps/` |

and runs `systemctl --user enable --now maild.service`. `make uninstall`
reverses it and leaves the data. maild's data stays in
`$XDG_DATA_HOME/frostmail`, its cache in `$XDG_CACHE_HOME/frostmail`, its
socket in `$XDG_RUNTIME_DIR/frostmail/maild.sock`.

`maild.service` is a simple service: `ExecStart` maild,
`Restart=on-failure`, `RestartSec=2`, started with the graphical session
(`WantedBy=default.target`). maild keeps syncing, sending and notifying
while the app is closed.

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
