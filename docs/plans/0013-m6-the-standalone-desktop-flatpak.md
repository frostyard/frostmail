# Plan 0013: M6 The standalone desktop Flatpak

M6 makes `org.frostyard.Frostmail` a Flatpak that works on its own: the
app, maild and mailctl in one package, local mail by default, and lodge as
a connection the user chooses
([ADR-0046](../adr/0046-the-desktop-flatpak-carries-its-own-maild.md)).
It builds on the CI Flatpak of
[ADR-0045](../adr/0045-the-desktop-app-builds-as-a-flatpak-in-ci.md) and
reuses frostnotes' pattern
([ADR-0031](https://github.com/frostyard/frostnotes/blob/main/docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md)).
It is frostmail's part of the suite's standalone Flatpak program
(Odrade's desk; core
[plan 0010](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md)),
whose steps 3 (Mail) and 6 (the clean-install gate) it carries. It
replaces the roadmap's M6 row for packaging; Google and Microsoft
verification stay there.

The local engine exists. This plan is packaging, lifecycle, permissions
and onboarding, not new mail features. Tests use generated mail and the
`frostmail-mailtest` container, as plan 0011 does, unless a phase says
otherwise.

Status (2026-10-27): **Phase 1 is open:** ADR-0046 is proposed. Nothing is
built.

## Who helps

- **Brian:** accepts ADR-0046; pushes the `v*` tag; makes the GHCR package
  public (ADR-0045); answers the plan's open questions.
- **Lucilla** (package delivery): core's amendment of ADR-0058 and plan
  0010; listing the app in `flatpak-index/apps.json` once an attested
  `latest` exists.
- **Duncan** (nsl and test machines): `make flatpak-sources` with a third
  generator in the nsl machine; a disposable desktop VM for Phases 4 to 9:
  Snow (GNOME on Wayland, real xdg-desktop-portal, Flatpak, a user install
  of `org.gnome.Platform//51`), with snapshots, and a route to the
  `frostmail-mailtest` container.

## Phase 1 — Decide (planner, small)

- [ADR-0046](../adr/0046-the-desktop-flatpak-carries-its-own-maild.md) and
  this plan, indexed in [docs/README.md](../README.md).
- On acceptance, link-only changes to the Status lines of ADR-0013, 0041
  and 0045: "Accepted; amended in part by 0046".
- Core amends ADR-0058's "not shipped" line and plan 0010's Mail row (a
  core change, with Lucilla).
- **Done when:** Brian accepts ADR-0046, and core's amendment is merged.

## Phase 2 — maild and mailctl in the build (planner, medium)

- The manifest's `maild` module: the Go SDK extension, `CGO_ENABLED=0`,
  `go build` of `./cmd/maild` and `./cmd/mailctl` into `/app/bin`,
  offline from `go-sources.json` with a `file://` `GOPROXY`.
- A Go sources generator: port frostnotes' `tools/flatpakgo` (it must skip
  the local `replace` of `third_party/go-imap`). `make flatpak-sources`
  writes 3 files; `scripts/flatpak-sources.sh` and CI's "Flatpak sources"
  job check all 3.
- The install layout: `/app/bin/frostmail` (a launcher that only `exec`s
  the app in this phase), `/app/bin/maild`, `/app/bin/mailctl`,
  `/app/libexec/frostmail-app`.
- The build's tests add `maild -version` and `mailctl --help`.
- **Done when:** CI's "Flatpak build and test" passes on a PR, and the
  bundle it uploads runs `flatpak run --command=mailctl
  org.frostyard.Frostmail --help`.

## Phase 3 — One local maild (planner, large)

- `internal/config` and `paths.rs`: the shared socket under
  `$XDG_RUNTIME_DIR/app/org.frostyard.Frostmail/`, and the window's cache
  from its own `XDG_CACHE_HOME` in the Flatpak, with unit tests.
- maild: the `flock` on `<data dir>/maild.lock`; `-log` with rotation
  past 1 MiB once it holds the lock; `rpc.shutdown` (caller `local`;
  `schema/rpc/rpc.yaml`, `make gen`).
- The launcher: `--background` with the mode gate; the start through
  `flatpak-spawn` and its `SIGKILL`; the version check against
  `rpc.hello`'s `server`; the Background request helper (PyGObject).
- `finish-args` as ADR-0046 lists; `install.sh` keeps its own for the
  development bundle with a host maild.
- The smoke test in the build, without portals: `frostmail --background`
  at the Flatpak's socket; a second maild on the same data directory
  exits; `mailctl` reads and writes through it; the launcher with a
  stand-in `flatpak-spawn` ends it, and maild lives on; with `app.json`
  naming lodge, `--background` exits 0 and starts nothing; a launcher with
  another version shuts the old maild down and starts its own; the log
  turns over.
- **Done when:** the smoke test passes in CI, and `make check` passes.

## Phase 4 — Secrets, sign-in and notifications in the sandbox (planner, medium)

- `--talk-name=org.freedesktop.secrets`; the Flatpak's no-fallback rule
  and `ErrNoKeyring`'s message; the probe before account setup saves an
  account (an IDL method or an option of an existing one, decided in the
  phase).
- A notification's click through `flatpak-spawn`.
- [design/accounts.md](../design/accounts.md#secrets) and
  [design/desktop.md](../design/desktop.md) follow.
- **Done when,** on Duncan's desktop VM with a CI bundle:
  1. a password account on the mailtest container is added, and its secret
     is in the keyring (`secret-tool search application frostmail`), not in
     a file;
  2. in a session with only the `session` collection, setup asks for a new
     "Default keyring", and a cancel fails the setup with no account saved;
  3. a new message notifies with the window closed, and its click opens the
     window at the message, in its own sandbox (`flatpak ps`);
  4. an OAuth sign-in completes through the loopback redirect (see Open
     questions for the account).

## Phase 5 — Files in the sandbox (planner, medium)

- Save As through `FileChooser.SaveFile` in the Flatpak, in both modes;
  Tauri's dialog elsewhere.
- Attaching by `draft.attachData` in the Flatpak, in both modes.
- ADR-0045's Save As limit is closed in [design/desktop.md](../design/desktop.md).
- **Done when,** on the desktop VM: Save As writes a message into
  `~/Documents` in local mode and in lodge mode (lodge's development build
  on the VM); a 25 MB attachment is sent in both modes; a cancelled Save As
  writes nothing.

## Phase 6 — First run and the modes (planner and Codex, medium)

- A store with no account opens on account setup, with "Connect to
  lodge…" as a secondary action.
- The Server pane's texts (ADR-0046) and its call to `rpc.shutdown`;
  saving local starts maild through the launcher.
  [specs/settings-ui.md](../specs/settings-ui.md#server-pane) first, then
  the card.
- An e2e test in the nsl machine: local, then lodge (maild stops, the
  local data stays), then local again (the same accounts return).
- **Done when:** the e2e test passes, and on the desktop VM a switch to
  lodge leaves no maild in `flatpak ps`, and a login with the autostart
  still registered starts none.

## Phase 7 — The optional move from a host maild (planner, medium)

- `mailctl import DIR`, with `export --check`'s validation and its
  refusals; tests with a staged export.
- The app's note when the host's data directory exists and the store is
  empty.
- The steps in [design/desktop.md](../design/desktop.md).
- **Done when,** on the desktop VM with a host maild from `make install`
  and generated mail: nothing changes until the user runs the steps; after
  them, the Flatpak's maild lists the same accounts and syncs without a new
  sign-in, the host's maild is disabled, and the export directory and the
  host's data are untouched.

## Phase 8 — Publication (Brian and Lucilla, small)

- Brian pushes a `v*` tag; the workflow pushes and attests the image;
  Brian makes `ghcr.io/frostyard/frostmail` public.
- Lucilla lists `org.frostyard.Frostmail` in `flatpak-index/apps.json`.
- **Done when:** `flatpak install frostyard org.frostyard.Frostmail` on the
  VM installs the attested `latest`.

## Phase 9 — The clean-install gate (planner, with Duncan)

On a fresh Snow VM from Duncan, with real portals. Each check is recorded
here with the bundle's digest and the date.

1. Nothing else is installed: no lodge, Tailscale, host maild, sysext or
   other suite app (`flatpak list`, `systemctl --user list-units`, `which
   maild`).
2. Install from the Frostyard remote.
3. The core local workflow: add an account (mailtest), receive, read, reply
   with an attachment, Save As, search, a rule that moves a message, Send
   Later.
4. Close and reopen the window; maild keeps syncing while it is closed.
   Log out and in: maild starts at login (Background portal allowed), and
   a Send Later due during the logout goes at login. Refuse the portal on a
   second VM: maild starts with the first window only.
5. `flatpak update` to a newer build: the old maild is replaced, the
   database migrates, nothing is lost.
6. Opt into lodge (lodge's development build reachable from the VM):
   read, send, Save As, and a notification while the window is open.
7. Disconnect lodge, then switch modes both ways: no data moves or
   disappears, no second maild runs, and the pane's texts match what
   happened.

- **Done when:** all 7 pass, and plan 0010's Mail row in core records it.

## Later / ideas

- A switch in Settings for maild in the background, beside the portal's.
- Notifications in lodge mode with the window closed (see Open questions).
- Narrowing `home:ro` with the file chooser portal for opening (ADR-0045).
- The Secret portal, if the suite moves to it together.

## Open questions

- **Lodge mode with the window closed:** nothing notifies (ADR-0041,
  ADR-0046). Brian decides whether a background connection is wanted, by
  Phase 6.
- **The OAuth check of Phase 4:** it needs a Google account that may sign
  in from the test VM; generated mail cannot. Brian names one, or the
  check is recorded as not run, by Phase 4.

## References

- Implements: [ADR-0046](../adr/0046-the-desktop-flatpak-carries-its-own-maild.md)
- Context: [design/desktop.md](../design/desktop.md), [design/accounts.md](../design/accounts.md#secrets), [specs/settings-ui.md](../specs/settings-ui.md#server-pane), [ADR-0045](../adr/0045-the-desktop-app-builds-as-a-flatpak-in-ci.md), [plan 0011](0011-m8-mail-in-lodge.md)
- Model: frostnotes [ADR-0031](https://github.com/frostyard/frostnotes/blob/main/docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md), with its Snow evidence
- Core: [plan 0010](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md), [ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md)
