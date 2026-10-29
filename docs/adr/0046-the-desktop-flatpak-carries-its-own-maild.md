# 0046 — The desktop Flatpak carries its own maild; lodge is a connection the user chooses

- **Status:** Accepted (Brian, 2026-10-28); amended 2026-10-29 (Brian,
  T491c): with the Background portal refused, maild runs only while a
  window is open
- **Date:** 2026-10-27

## Context

On 2026-10-27 the owner settled a product rule for the whole suite: each
frostyard app works on its own as a Flatpak by default. The Flatpak holds
the UI, the daemon and its helpers. Standalone use needs no separately
installed frostyard daemon, lodge, Tailscale, sysext or other suite app.
Normal Flatpak infrastructure (a runtime, the desktop portals) and the
services an app exists to use (here, the user's mail servers) are not extra
installs. A lodge mode stays supported and tested, but the user chooses it
explicitly, and the app never moves data or changes its backend silently.
The rule binds the suite, so this ADR does not reopen it; it settles how
frostmail meets it.

Three accepted decisions conflict with the rule:

- [ADR-0013](0013-app-as-a-local-flatpak.md) keeps maild and mailctl on
  the host, as a systemd user service that `make install` sets up, and
  gives the Flatpak the host's socket directory and, read-only, the host's
  cache.
- [ADR-0041](0041-maild-runs-as-a-lodge-app.md) makes the owner's desktop
  app a lodge client of one maild on lodge's host, and keeps a local maild
  only "for an install outside the owner's tailnet". Its local maild is
  still the host's.
- [ADR-0045](0045-the-desktop-app-builds-as-a-flatpak-in-ci.md) builds a
  Flatpak with one module, the Tauri app, "no Go and no maild in it", and
  records that "the Flatpak needs a maild: on the host, or behind lodge".
  It also records that Save As fails with a maild behind lodge, because the
  sandbox's home is read-only.

Core's [ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md)
lists "a maild for a Linux desktop outside the owner's tailnet" as not
shipped. Core amends that in its own repository; this ADR is frostmail's
half.

Frostnotes solved the same problem first, in its
[ADR-0031](https://github.com/frostyard/frostnotes/blob/main/docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md),
and tested it on a real Snow desktop on 2026-10-26. Its facts hold for
maild too:

- Each sandbox instance of an app has its own `$XDG_RUNTIME_DIR`, except
  `$XDG_RUNTIME_DIR/app/<app-id>`, which all instances of the app share.
  The window, a daemon started through the Flatpak portal, and a
  `flatpak run --command=…` are separate instances.
- A process in the window's sandbox dies with that sandbox. A Flatpak
  cannot install a systemd user unit. The Background portal is the
  supported way to run at login.
- All instances of the app share one `XDG_DATA_HOME`, `XDG_CACHE_HOME`,
  `XDG_CONFIG_HOME` and `XDG_STATE_HOME`, under
  `~/.var/app/org.frostyard.Frostmail/`.
- `flatpak-spawn` (the Flatpak portal) starts a command in a new instance
  of the same app, which outlives the window.

Facts about maild that shape the rest:

- maild keeps syncing, sending, running rules, Send Later and reminders
  while the app is closed ([ADR-0002](0002-daemon-and-thin-clients.md),
  [ADR-0024](0024-rules-run-in-maild-on-new-mail.md),
  [ADR-0025](0025-maild-keeps-send-later-remind-me-and-undo-send.md)).
  Two mailds on the same accounts would each run every rule, send each
  scheduled message, and raise each reminder. A duplicate engine is a
  correctness fault, not only waste.
- maild's only single-instance guard is its socket: on Linux a second
  maild refuses a socket where a live maild answers, and on Windows it
  finds the pipe taken and exits. Nothing guards the data directory, so
  two mailds with different sockets (a `FROSTMAIL_SOCKET`, or one in each
  of two sandboxes' runtime directories) can open the same database.
- maild keeps secrets in the Secret Service (ADR-0011). frostmail#124
  (merged 2026-10-27) ported frostnotes' keyring fixes: maild reads the
  `default` alias, makes the default collection when the session has none,
  and ends a prompt wait when the service leaves the bus. Without a
  Secret Service on the bus, maild today falls back to a plain file with
  a warning ([design/accounts.md](../design/accounts.md#secrets)).
- `message.save` writes a file on maild's machine, and `draft.attach`
  reads one by its path; both are `local` only. `draft.attachData` attaches
  by bytes, for `local` and `session` callers.
- `mailctl export DIR` copies a stopped maild's database, blobs and
  secrets into DIR, and `export --check` validates such a copy against a
  build (plan 0011, Phase 7). It refuses while maild answers on its socket.
- OAuth sign-in listens on a loopback port of maild's machine
  (ADR-0011), and a client off that machine can finish with the pasted
  address ([ADR-0043](0043-finish-a-sign-in-with-a-pasted-address.md)).
- A secret's item in the Secret Service has only the attributes
  `application=frostmail` and `key=<key>`, and keys such as
  `account/1/password` carry the database's own account number. Two
  stores on one keyring, such as a host maild and a Flatpak maild, both
  have an account 1, so they would read and replace each other's items.

## Decision

### Two modes, chosen by the user

- **Local, the default.** The Flatpak runs its own maild, and the app
  talks to it over a socket in the app's shared runtime directory. A fresh
  install starts here and opens on account setup. Nothing about lodge,
  Tailscale or a host maild is asked or needed.
- **Connected to lodge, by explicit choice.** The app talks to the maild
  on lodge's host as a session, exactly as ADR-0041 and
  [design/desktop.md](../design/desktop.md#on-a-maild-behind-lodge) build
  it. This is a remote connection, not a sync: the desktop holds no copy of
  the mail, and lodge's maild is the engine. Mail has no lodge sync mode.
- **The choice stays where ADR-0041 put it:** `app.json` in the app's own
  config directory. No `lodge` there means local. Only Settings' Server
  pane writes it, and only when the user saves a change.

### Which engine runs in each mode

| Mode | Engine | What runs on the desktop |
| --- | --- | --- |
| Local | the Flatpak's own maild | the window, and maild in its own sandbox instance; when the Background portal allows it, maild runs through logout and from login; when the portal is refused, only while a window is open |
| Lodge | lodge's maild | the window only; no maild |

- **Never a host maild.** The released Flatpak neither starts nor
  connects to a maild on the host. It loses ADR-0013's
  `--filesystem=xdg-run/frostmail` and `--filesystem=xdg-cache/frostmail:ro`.
  A host maild and the `make install-app` bundle stay a development path,
  as core ADR-0058 already says of ADR-0013's bundle.

### How duplicate engines are prevented

- **One maild per data directory.** maild takes an exclusive `flock` on
  `<data dir>/maild.lock` before it opens the database, and holds it until
  it exits. A second maild on the same directory exits at once with
  "another maild uses <dir>", whatever its socket. This holds on the host
  too.
- **The mode is checked under the lock.** In the Flatpak, maild reads
  `app.json` (it reads it, never writes it) after it holds the data lock
  and before it opens the database. In lodge mode it releases the lock,
  logs one line and exits 0. The launcher checks the mode too, to skip a
  needless start, but maild's check is the one that counts: a launcher
  that read "local" and was delayed past a change of mode starts a maild
  that exits at once. An autostart left from local mode starts no engine
  for the same reason.
- **Leaving local mode stops the engine, in a fixed order.** When the
  user saves lodge in the Server pane, the app:
  1. writes `app.json` with lodge (temporary file, synced, renamed);
  2. calls a new `rpc.shutdown` (caller `local` only; additive to
     protocol 1) when a maild answers on the socket. maild finishes the
     transaction it is in, releases its lock and exits;
  3. takes the data lock itself, waiting up to 30 s and repeating step 2
     each second, which catches a maild that took the lock before step 1
     and was not yet listening at step 2; then releases it. From then on
     every maild that takes the lock reads lodge and exits.
  4. If the lock is still held after 30 s, the app writes local back to
     `app.json`, connects to the local maild again, and shows "This
     computer's maild did not stop. Frostmail still uses it." The mode
     does not change half-way.
  Saving local again writes `app.json`, then starts maild through the
  launcher. The local data stays on disk, unsynced, while lodge is the
  mode.
- **The pane says what each change does,** before the user saves, from
  what the app knows locally (no request to lodge is needed, so the text
  is the same while lodge is unreachable):
  - to lodge: "This computer's maild stops. Its accounts and mail stay on
    this computer, not synced, until you switch back. Nothing moves to
    lodge.";
  - to local, when the local data directory holds a database: "This
    computer's maild starts again with the accounts it had. It runs
    their rules, Send Later and reminders here. If lodge's maild has the
    same accounts, both run them." The app checks only that the
    database file exists; it does not open it;
  - to local, when it holds none: "Mail on lodge keeps running
    there. If you add the same accounts here, both run their rules, Send
    Later and reminders."
  The app cannot stop two machines from syncing one account; it makes
  sure that no change of mode starts or resumes a second engine unseen.
- **An update replaces the engine, only forward.** maild's `rpc.hello`
  gains an optional `build`: in the Flatpak, the `app-commit` of the
  instance's `/.flatpak-info`, which differs for every build, even of one
  version. The launcher starts maild with `flatpak-spawn
  --latest-version`, never in its own deployment, and asks the latest
  deployment's commit the same way (`maild -build`). When the running
  maild's `build` is not the latest commit, the launcher calls
  `rpc.shutdown` and starts the latest. The target is always the latest
  installed deployment, so an old window or an old notification's click
  cannot bring back an old maild. Database migrations run only in the
  maild that holds the lock.

### The Flatpak's layout and lifecycle

These follow frostnotes ADR-0031; only the names differ.

- **Build.** The manifest gains a `maild` module before the app's. It
  builds `maild` and `mailctl` with the Go SDK extension and
  `CGO_ENABLED=0`, offline, from a generated `go-sources.json` (one `file`
  source per module, with checksums, and a `file://` `GOPROXY`).
  `third_party/go-imap` is a local `replace` and needs no download.
  `make flatpak-sources` writes all 3 sources files, and CI's sources job
  fails on any difference.
- **Install layout.** `/app/bin/frostmail` is a POSIX sh launcher (the
  manifest's `command`), `/app/bin/maild` and `/app/bin/mailctl` are the Go
  binaries, and `/app/libexec/frostmail-app` is the Tauri binary.
  `flatpak run --command=mailctl org.frostyard.Frostmail` works.
- **The socket** is
  `$XDG_RUNTIME_DIR/app/org.frostyard.Frostmail/frostmail/maild.sock` when
  `FLATPAK_ID` is `org.frostyard.Frostmail`. `internal/config` and the
  shell's `paths.rs` both derive it, with tests. `FROSTMAIL_SOCKET` still
  wins, and outside a Flatpak nothing changes.
- **The launcher.**
  - `frostmail --background` (local mode) runs `exec maild -log
    $XDG_STATE_HOME/frostmail/maild.log`; maild rotates the log once it
    holds the lock.
  - `frostmail [ARGS]` in local mode: when `mailctl hello` fails, it runs
    `flatpak-spawn --latest-version frostmail --background`, waits about 10 s for `mailctl
    hello`, then ends `flatpak-spawn` with `SIGKILL`, so the window's
    sandbox does not wait on maild. Without the Flatpak portal it starts no
    maild of its own, and the app shows that maild is not running. It then
    asks the Background portal for `background` and `autostart` with the
    command line `frostmail --background`, through a PyGObject helper that
    waits for the portal's `Response`, and `exec`s the app.
  - A refusal of the Background portal stops the start at login, and
    maild runs only while a window is open: the desktop ends its instance
    when the last window closes (Brian, T491c, 2026-10-29: the accepted
    behaviour; seen on GNOME 48 with xdg-desktop-portal 1.20.3 in plan
    0013's clean-install gate). The next window starts maild again.
- **Notifications in local mode** come from maild on the session bus, as
  on the host (`--talk-name=org.freedesktop.Notifications`, already
  granted). A click runs the launcher through `flatpak-spawn
  --latest-version`, so the window it opens gets its own sandbox, of the
  latest build, and does not die with maild's.
- **Lodge mode** is unchanged from ADR-0041: the window holds the
  connection and shows `message.announced` itself, while it is open.

### Where the data and the secrets live

In local mode everything is the app's own, under
`~/.var/app/org.frostyard.Frostmail/`, which `flatpak uninstall
--delete-data` removes:

| What | Where |
| --- | --- |
| database, blobs, `maild.lock` | `data/frostmail/` (maild's `XDG_DATA_HOME`) |
| decoded parts | `cache/frostmail/`, which the window reads as its own `XDG_CACHE_HOME` (no more `HOST_XDG_CACHE_HOME`) |
| `app.json`, the mode | `config/frostmail/` |
| `maild.log` | `.local/state/frostmail/` |
| the socket | `$XDG_RUNTIME_DIR/app/org.frostyard.Frostmail/frostmail/` |

- **Secrets go to the Secret Service**, through
  `--talk-name=org.freedesktop.secrets`, with the keys and labels of
  ADR-0011, in the default collection that frostmail#124 makes when the
  session has none.
- **The Flatpak's items have a namespace of their own.** In the Flatpak,
  maild gives each item the attributes
  `application=org.frostyard.Frostmail`, `store=<store id>` and
  `key=<key>`, and every search, write and delete uses all three.
  - The store id is a random value maild draws when it finds a database
    without one, and keeps in that database.
  - A host maild searches `application=frostmail`, so it never finds the
    Flatpak's items, and the Flatpak's maild never searches for the
    host's. Neither can read, replace or delete the other's secrets.
  - A reinstall after `flatpak uninstall --delete-data` makes a new
    database, so a new store id: it never reads the items the old one
    left. The keyring is not removed with the app's data; `secret-tool
    clear application org.frostyard.Frostmail` removes them.
  - Outside a Flatpak, items keep ADR-0011's attributes, unchanged.
- **No silent plain file in the Flatpak.** With `FLATPAK_ID` set and
  `FROSTMAIL_SECRETS` unset, maild does not fall back to `secrets.json`
  when the Secret Service is missing. It runs, and every secret write
  fails with `ErrNoKeyring`, whose message names `flatpak override --user
  --env=FROSTMAIL_SECRETS=file org.frostyard.Frostmail`. Before account
  setup saves an account, the app has maild write and delete a probe
  secret, so a keyring that cannot keep one fails the setup, not the first
  sync.
- **OAuth works as on the host.** `--share=network` puts the sandbox in the
  host's network namespace, so maild's loopback listener receives the
  browser's redirect; the authorization URL opens through the OpenURI
  portal. ADR-0043's pasted address stays the fallback.

### Files in the sandbox

- **Save As asks the file chooser portal.** In the Flatpak, the Rust layer
  calls `org.freedesktop.portal.FileChooser.SaveFile` over the session bus
  (zbus, which the app already uses) and waits for its `Response`. The
  path it answers is under the document portal, writable from every
  instance of the app. In local mode maild writes the file there with
  `message.save`; in lodge mode `save_content` downloads into it. This
  ends ADR-0045's known limit in both modes. It does not use rfd's
  `xdg-portal` feature, whose message dialogs need zenity (ADR-0045).
  Outside a Flatpak, and on Windows, Save As keeps Tauri's dialog.
- **Attaching sends bytes.** In the Flatpak the app attaches with
  `draft.attachData` in both modes, so maild never needs a path that the
  window chose.
- **The home stays read-only** (`--filesystem=home:ro`) for the attach
  dialog, until ADR-0045's open item, the file chooser portal for opening,
  is solved.

### Moving from a host maild: optional and explicit

- The app never reads, copies or stops a host maild by itself, and never
  touches the host's data or its keyring items.
- maild makes its database when it starts, so a first local launch has a
  database with no account. When the store has no account and the host's
  data directory (`~/.local/share/frostmail/frostmail.db`, visible
  through `home:ro`) exists, the app shows a dismissible note with the
  move's steps. It changes nothing.
- The move is three commands the user runs, with the window open or not:
  1. `systemctl --user disable --now maild.service` on the host;
  2. the host's `mailctl export DIR` (it refuses while that maild runs);
  3. `flatpak run --command=mailctl org.frostyard.Frostmail import DIR`.
- `mailctl import DIR` is new. In order, it:
  1. runs `export --check`'s validation of DIR, and stops on any fault;
  2. calls `rpc.shutdown` when the Flatpak's maild answers, then takes
     the data lock and holds it to the end, so no maild can start
     meanwhile (a launcher's maild exits with "another maild uses …");
  3. checks, under the lock, that the store is pristine: no database, or
     one with no account, no message, no draft and nothing in the outbox.
     Otherwise it stops and changes nothing;
  4. moves the pristine database and its `-wal` and `-shm` aside, as
     `frostmail.db.before-import-<stamp>`;
  5. places the export's database and blobs, gives the database a new
     store id, and places the export's plain `secrets.json` (0600);
  6. releases the lock, and tells the user to open Frostmail.
  The next maild moves the secrets into the Flatpak's own items and
  removes the file (ADR-0011). Import never deletes DIR or the host's
  data, and it prints that DIR holds secrets in plain text, to remove once
  the accounts sync.
- After the move the host maild must stay disabled: re-enabled, it would
  be a second engine on the same accounts, and a Microsoft account's
  refresh token, which rotates (ADR-0039), may no longer work for it.

### What this changes in the earlier decisions

- **ADR-0013:** "maild and mailctl stay on the host" and its sandbox paths
  no longer describe the release. Its local bundle remains a development
  path, with a host maild.
- **ADR-0041:** the owner's desktop is no longer bound to lodge. Lodge's
  maild is one of two modes on every desktop, chosen by the user; the
  local mode is the Flatpak's own maild, not a host's. Everything about
  maild on the server is unchanged.
- **ADR-0045:** the Flatpak gains Go and the `maild` module, a third
  sources file, the launcher, the Background helper and a smoke test, and
  the permissions below. Publishing gains a release candidate, which the
  clean-install gate tests before the same digest becomes `latest`
  (plan 0013).

The permissions become: `--socket=wayland`, `--socket=fallback-x11`,
`--share=ipc`, `--device=dri`, `--share=network`,
`--talk-name=org.freedesktop.Notifications`,
`--talk-name=org.freedesktop.secrets` and `--filesystem=home:ro`. The
Background, FileChooser, OpenURI and Flatpak portals need no finish-arg.

## Consequences

- A fresh desktop installs Mail and uses it with nothing else of
  frostyard's. The owner's lodge deployment, and the friend's Windows
  install, are unchanged.
- The Flatpak grows by two static Go binaries, and its build by the Go
  SDK extension. A change of `go.mod` or `go.sum` now also needs `make
  flatpak-sources`.
- The socket path, like the data directory, has two sources of truth:
  `internal/config` and `paths.rs`, each with tests.
- maild gains a lock (on every platform that has `flock`), the mode check
  under it, `rpc.shutdown`, `rpc.hello`'s `build`, `-build` and `-log`,
  the Flatpak's namespace and no-fallback rule for secrets, and `mailctl
  import`. The app gains the SaveFile portal call, attach by bytes
  in the Flatpak, the Server pane's texts and its call to stop maild.
- In local mode, when the user allows the Background portal, maild runs
  until logout after the window closes, and at login. When the user
  refuses it, maild runs only while a window is open, and nothing
  notifies with the window closed. The app has no switch for that yet;
  `flatpak kill org.frostyard.Frostmail` stops it.
- In lodge mode nothing runs on the desktop when the window is closed, so
  new mail does not notify then. ADR-0041 already accepted this.
- An owner who uses a host maild today must move once, or choose lodge.
  Until they do, an updated Flatpak in local mode starts empty beside the
  host maild; the note tells them, and nothing is lost or moved.
- Two machines can still sync the same account (the Flatpak on a laptop and
  lodge's maild, say). The app warns when the user's own change of mode
  could lead there; it cannot see other machines.
- The portal paths (`flatpak-spawn`, Background, SaveFile, notification
  clicks) have no automated test against a real portal. Each change to
  them needs the real-desktop check of
  [plan 0013](../plans/0013-m6-the-standalone-desktop-flatpak.md).
- `--talk-name=org.freedesktop.secrets` lets the app reach every item in
  the user's unlocked keyring, as frostnotes' Flatpak can. The Secret
  portal would scope it, at the cost below.

## Alternatives considered

- **Keep the host maild (ADR-0013):** it needs a separate install, which the
  product rule forbids, and a Flatpak cannot install a user unit.
- **maild in the window's sandbox:** it dies with the window, and with a
  duplicate window that the single-instance plugin closes at once
  (frostnotes' first design, rejected for the same reason).
- **A third mode that connects to a host maild:** it would keep a second,
  unpackaged install path in the release, and the host's socket and cache
  permissions with it. The development bundle keeps it instead.
- **Lodge as a sync target, with a local copy:** maild's state (rules,
  Send Later, Undo Send, reminders) would need a merge with lodge's maild,
  and the mail servers already are the shared store. Mail offers a
  connection only.
- **Leave the local maild running in lodge mode:** two engines on the same
  accounts, whenever the user had local accounts before.
- **The Secret portal**, with ADR-0044's sealed file under a key it gives:
  it scopes the app to its own secret, but frostnotes and every frostmail
  install use the Secret Service, the move from a host maild works through
  it unchanged, and not every desktop's portal backend implements it.
- **Write Save As through rfd's `xdg-portal` feature:** it routes every
  message dialog through zenity, which the GNOME runtime lacks (ADR-0045).
- **Move a host maild's data on first start:** fast for the owner, but it
  moves data without a decision, which the product rule forbids.
- **ADR-0011's item attributes in the Flatpak too:** two stores on one
  keyring both have an account 1, so each would read and replace the
  other's secrets.
- **The mode checked by the launcher only:** a launcher delayed past a
  change of mode would start an engine in lodge mode.

## References

- Shapes: [plan 0013](../plans/0013-m6-the-standalone-desktop-flatpak.md), [design/desktop.md](../design/desktop.md), [design/accounts.md](../design/accounts.md#secrets), [specs/settings-ui.md](../specs/settings-ui.md#server-pane), [specs/rpc-protocol.md](../specs/rpc-protocol.md), `packaging/flatpak/org.frostyard.Frostmail.json`
- Amends: [ADR-0013](0013-app-as-a-local-flatpak.md), [ADR-0041](0041-maild-runs-as-a-lodge-app.md), [ADR-0045](0045-the-desktop-app-builds-as-a-flatpak-in-ci.md)
- Builds on: [ADR-0002](0002-daemon-and-thin-clients.md), [ADR-0011](0011-sign-in-and-credentials.md), [ADR-0043](0043-finish-a-sign-in-with-a-pasted-address.md), [ADR-0044](0044-seal-the-servers-secrets-with-a-systemd-credential.md) (the server's store, unchanged), [plan 0011](../plans/0011-m8-mail-in-lodge.md) (Phase 7's export)
- Model: frostnotes [ADR-0031](https://github.com/frostyard/frostnotes/blob/main/docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)
- Core: [ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md) (to be amended in core), [plan 0010](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md)
