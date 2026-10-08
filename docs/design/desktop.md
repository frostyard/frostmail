# Running on the desktop

Living document. How Frostmail runs as the user's daily client before the
Flatpak (M6): maild as a user service, the app as a desktop application,
and notifications. Rationale: [ADR-0002](../adr/0002-daemon-and-thin-clients.md).

## Installing for one user

`make install` (no root) builds release binaries and installs:

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

- maild notifies through `org.freedesktop.Notifications` (godbus) when new
  unread mail arrives in an inbox (Gmail: the `\Inbox` label) after the
  account's first full pass, never for mail it already knew.
- One notification per message: the sender's name as the summary, the
  subject and the start of the preview as the body, the app icon, and a
  default action. Four or more in one pass become one notification:
  "N new messages" with the first senders' names.
- A notification replaces the account's previous one
  (`replaces_id`), so the shade does not fill up.
- Clicking it (`ActionInvoked`, action `default`) runs
  `frostmail --open-message <id>`.
- Per account: notifications on or off (`account.update {notify}`); off
  for read-only accounts.

## One app instance

The app is single-instance (`tauri-plugin-single-instance`): a second
launch passes its arguments to the running app and exits. `--open-message
<id>` shows the main window, selects the message's inbox (or All Inboxes)
and the message, and focuses the window; with no running app it starts on
that message.
