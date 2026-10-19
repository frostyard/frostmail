# Running on the desktop

Living document. How Frostmail runs as the user's daily client before
M6's release packaging: maild as a user service, the app as a local
Flatpak, notifications, and one app instance. Rationale: [ADR-0002](../adr/0002-daemon-and-thin-clients.md).

On lodge's host, maild runs instead as a system service under the user
`frostmail`, with no session bus: no Secret Service and no notifications
of its own. The desktop app then reaches it through lodge
([ADR-0041](../adr/0041-maild-runs-as-a-lodge-app.md),
[specs/lodge-app.md](../specs/lodge-app.md),
[plan 0011](../plans/0011-m8-mail-in-lodge.md)'s Phase 6).

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
  `FROSTMAIL_APP` names another binary.
- **Without a session bus** (behind lodge, or a session with none), maild
  sends the same notes to every connected client instead, as the
  transient event `message.announced {accountId, notes: [{title, body,
  messageId?}]}`, and each client shows them itself: the web build with
  the Notification API ([lodge-app.md](../specs/lodge-app.md#the-web-build)).
  Nothing is replayed to a client that connects later.
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

## On a maild behind lodge

[ADR-0041](../adr/0041-maild-runs-as-a-lodge-app.md) makes the desktop app a
lodge client: one maild on lodge's host serves the phone, browsers and the
desktop. [Plan 0011](../plans/0011-m8-mail-in-lodge.md)'s Phase 6 builds it.
A local maild, as above, stays the mode for installs outside the owner's
tailnet, such as the friend's Windows install.

- **Which maild.** The app reads `$XDG_CONFIG_HOME/frostmail/app.json`
  (the Flatpak's own, `~/.var/app/org.frostyard.Frostmail/config`;
  `%APPDATA%\Frostmail\app.json` on Windows): `{"lodge":
  "https://lodge.<tailnet>.ts.net"}` reaches maild through lodge, and no
  file, or no `lodge`, means the local socket. It is the app's own, never
  maild's, since it names which maild to ask (`src-tauri/src/remote.rs`).
  lodge's address is https, or http on loopback for lodge's development
  build. Settings' Server pane shows the choice
  ([settings-ui.md](../specs/settings-ui.md#server-pane)), and changing it
  ends every window's connection, which each then reconnects.
- **The bridge's remote mode.** `bridge.rs` opens
  `wss://LODGE/app/mail/api` in place of the socket and moves the same
  lines, one per text frame. It still never parses them.
  - It connects from the desktop's own tailnet node, which lodge's
    allow-list must name. lodge checks it by WhoIs, as it does a browser.
  - It sends `Origin: https://LODGE` and no fetch metadata. lodge requires
    an Origin on an upgrade, and the origin check guards browsers, which
    the bridge is not: identity still comes from WhoIs alone, with the
    desktop's node and login on lodge's allow-lists. lodge-http admits
    native clients this way (the lodge program's decision D7).
  - TLS is lodge's Tailscale certificate: rustls with ring and the web's
    roots on Linux, the system's TLS (SChannel) on Windows, which builds
    with no C toolchain for the target.
  - The webview's transport sends `rpc.ping` every 30 s, visible or not,
    since lodge closes a session after 90 s without a frame.
  - Each window has its own connection, as on the socket, so each is a
    lodge session. lodge keeps at most 8 per node and app, and past that
    closes the least recently active, which reconnects.
- **Parts.** The reader's images are already `data:` URLs (ADR-0014), so
  the reader frame is unchanged. `open_part` downloads an attachment from
  `<contentOrigin>/mail/<contentPath>` into the app's cache, and opens that
  file as today. The webview still fetches nothing: only the Rust layer
  reaches the network.
  - The content origin is `contentOrigin` from `GET https://LODGE/lodge/apps`,
    the JSON lodge's shell reads, asked with the same Origin. lodge can
    rename its content node, so the app derives the name (lodge's, with
    `lodge-content` for its first label) only when that request fails.
- **What a remote maild cannot do for the desktop.** maild's session
  caller cannot read or write the desktop's files, so attaching sends the
  file's bytes (`draft.attachData`), and Save As asks where with the
  native dialog as before, then `save_content` downloads the message
  `message.source {content: true}` names (`m/…`) from the content node
  into that file, in the Rust layer as `open_part` downloads parts.
- **Notifications.** A remote maild has no session bus, so it sends each
  connected client `message.announced` (above), and the app's webview
  hands the notes to `notify_mail` (`src-tauri/src/announce.rs`), which
  shows them as maild would on this machine:
  - on Linux, through `org.freedesktop.Notifications` on the app's own
    session bus (zbus), with maild's hints, its Open action and its
    group note per account that the next replaces; a click brings the
    main window forward at the message (the `open-message` event);
  - on Windows, through `frostmail-notify.exe` beside the app, with the
    operation maild writes it (`mail` group, `m<id>` or `a<account>`
    tags), whose click runs `frostmail.exe --open-message`, which the
    single instance hands to the running app;
  - elsewhere, not yet.

  A calendar reminder raises the reminder window, as with a local maild.
- **What does not change:** with no `lodge` in `app.json`, the bridge,
  the parts and every window are as above, on maild's own socket or pipe.
- **Tests:** `src-tauri/src/remote.rs`'s tests (addresses, the
  configuration, downloads through the content origin `/lodge/apps` names);
  `app/e2e/lodge.e2e.ts` runs lodge's development build, `maild --lodge` on
  a fixture and the built app with `app.json` naming lodge, and lists,
  reads and sends a reply through it. It needs lodge's `make e2e-build`
  output in `FROSTMAIL_LODGE_E2E`, and skips without it.
- **The Flatpak** gains `--share=network` for the Rust layer, and
  `--talk-name=org.freedesktop.Notifications` for its notifications. The
  webview's CSP still forbids every fetch, so ADR-0005's third layer
  holds, though the sandbox no longer cuts the network on its own.

## One app instance

The app is single-instance (`tauri-plugin-single-instance`): a second
launch passes its arguments to the running app, which shows and focuses
its main window, and exits. The first launch keeps its own
`--open-message` until the main window asks for it (`startup_message`);
later ones arrive as the `open-message` event (`app/src/app/openMessage.ts`). `--open-message
<id>` shows the main window, selects the message's inbox (or All Inboxes)
and the message, and focuses the window; with no running app it starts on
that message.
