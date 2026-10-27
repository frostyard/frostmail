# 0046 — The desktop Flatpak carries its own maild; lodge is a connection the user chooses

- **Status:** Proposed
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
  by bytes, for any caller.
- `mailctl export DIR` copies a stopped maild's database, blobs and
  secrets into DIR, and `export --check` validates such a copy against a
  build (plan 0011, Phase 7). It refuses while maild answers on its socket.
- OAuth sign-in listens on a loopback port of maild's machine
  (ADR-0011), and a client off that machine can finish with the pasted
  address ([ADR-0043](0043-finish-a-sign-in-with-a-pasted-address.md)).

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
| Local | the Flatpak's own maild | the window, and maild in its own sandbox instance, through logout, and from login when the Background portal allows it |
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
- **The mode gates the start.** The launcher starts maild only in local
  mode. `frostmail --background`, which the Background portal runs at
  login, reads `app.json` first; in lodge mode it logs one line and exits
  0, so an autostart left from local mode starts no engine.
- **Leaving local mode stops the engine.** When the user saves lodge in
  the Server pane, the app calls a new `rpc.shutdown` (caller `local`
  only; additive to protocol 1). maild finishes the transaction it is in,
  releases its lock and exits. Saving local again starts it through the
  launcher. The local data stays on disk, unsynced, until then.
- **The pane says what each change does,** before the user saves:
  - to lodge: "This computer's maild stops. Its accounts and mail stay on
    this computer, not synced, until you switch back. Nothing moves to
    lodge.";
  - to local, when lodge's maild has accounts: "Your mail on lodge keeps
    running there. If you add the same accounts here, both run their
    rules, Send Later and reminders."
  The app cannot stop two machines from syncing one account; it can make
  sure that no change of mode starts a second engine unseen.
- **An update replaces the engine.** The launcher compares the `server`
  string of `rpc.hello` with the bundled maild's version. When they differ,
  it calls `rpc.shutdown`, waits for the socket to close, and starts the
  bundled maild. Database migrations run only in the maild that holds the
  lock.

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
    `flatpak-spawn frostmail --background`, waits about 10 s for `mailctl
    hello`, then ends `flatpak-spawn` with `SIGKILL`, so the window's
    sandbox does not wait on maild. Without the Flatpak portal it starts no
    maild of its own, and the app shows that maild is not running. It then
    asks the Background portal for `background` and `autostart` with the
    command line `frostmail --background`, through a PyGObject helper that
    waits for the portal's `Response`, and `exec`s the app.
  - A refusal of the Background portal stops only the start at login.
    maild still runs from the first window of a session until logout.
- **Notifications in local mode** come from maild on the session bus, as
  on the host (`--talk-name=org.freedesktop.Notifications`, already
  granted). A click runs the launcher through `flatpak-spawn`, so the
  window it opens gets its own sandbox and does not die with maild's.
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
  `--talk-name=org.freedesktop.secrets`, with the items, labels and
  attributes of ADR-0011, in the default collection that frostmail#124
  makes when the session has none. The keyring is not removed with the
  app's data.
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

- The app never reads, copies or stops a host maild by itself.
- When local mode starts with an empty store and the host's data
  directory (`~/.local/share/frostmail/frostmail.db`, visible through
  `home:ro`) exists, the app shows a dismissible note with the move's
  steps. It changes nothing.
- The move is three commands the user runs:
  1. `systemctl --user disable --now maild.service` on the host;
  2. the host's `mailctl export DIR` (it refuses while that maild runs);
  3. `flatpak run --command=mailctl org.frostyard.Frostmail import DIR`.
- `mailctl import DIR` is new. It runs `export --check`'s validation
  first, refuses while the Flatpak's maild runs or when its data directory
  holds a database, and then places the database and blobs and a plain
  `secrets.json` (0600). The next maild moves the secrets into the Secret
  Service and removes the file (ADR-0011). It never deletes DIR or the
  host's data; it prints that DIR holds secrets in plain text, to remove
  once the accounts sync.

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
  the permissions below. Publishing is unchanged.

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
- maild gains a lock (on every platform that has `flock`), `rpc.shutdown`,
  `-log` rotation, the Flatpak's no-fallback rule for secrets, and
  `mailctl import`. The app gains the SaveFile portal call, attach by bytes
  in the Flatpak, the Server pane's texts and its call to stop maild.
- In local mode maild runs until logout after the window closes, and at
  login when the user allows it. The app has no switch for that yet;
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

## References

- Shapes: [plan 0013](../plans/0013-m6-the-standalone-desktop-flatpak.md), [design/desktop.md](../design/desktop.md), [design/accounts.md](../design/accounts.md#secrets), [specs/settings-ui.md](../specs/settings-ui.md#server-pane), [specs/rpc-protocol.md](../specs/rpc-protocol.md), `packaging/flatpak/org.frostyard.Frostmail.json`
- Amends: [ADR-0013](0013-app-as-a-local-flatpak.md), [ADR-0041](0041-maild-runs-as-a-lodge-app.md), [ADR-0045](0045-the-desktop-app-builds-as-a-flatpak-in-ci.md)
- Builds on: [ADR-0002](0002-daemon-and-thin-clients.md), [ADR-0011](0011-sign-in-and-credentials.md), [ADR-0043](0043-finish-a-sign-in-with-a-pasted-address.md), [ADR-0044](0044-seal-the-servers-secrets-with-a-systemd-credential.md) (the server's store, unchanged), [plan 0011](../plans/0011-m8-mail-in-lodge.md) (Phase 7's export)
- Model: frostnotes [ADR-0031](https://github.com/frostyard/frostnotes/blob/main/docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)
- Core: [ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md) (to be amended in core), [plan 0010](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md)
