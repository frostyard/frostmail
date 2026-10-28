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

Status (2026-10-28): **Phase 1 is open:** Brian accepted ADR-0046 on
2026-10-28, and ADR-0013, 0041 and 0045 say that it amends them in part;
core's amendment of ADR-0058 and plan 0010 is still to come.
**Phases 2 and 3 are built** (PR #126, merged 2026-10-28): maild and
mailctl in the Flatpak's build, the launcher, the lock, `-log`, `rpc.shutdown`, `build`,
the paths and the smoke test. CI's "Flatpak build and test" runs the smoke
test; the checks on a real desktop (portals, Background, `flatpak-spawn`)
wait for the desktop VM. **Phase 4's secrets part is built (PR #130, in
review):** `--talk-name=org.freedesktop.secrets`, the Flatpak's keyring
namespace and the store id, the no-fallback rule with `ErrNoKeyring`'s
message, the probe in `account.create`, and the unit tests on `dbustest`.
**Not done in Phase 4:** the notification click, the OAuth check and the 6
checks on the desktop VM. Phases 5 to 10 are not built.

## Who helps

- **Brian:** accepts ADR-0046; pushes the candidate and release tags;
  makes the GHCR package public (ADR-0045); answers the plan's open
  questions.
- **Lucilla** (package delivery): core's amendment of ADR-0058 and plan
  0010; a test remote for a release candidate (Phase 8); the promotion of
  the tested digest, and the listing in `flatpak-index/apps.json`
  (Phase 10).
- **Duncan** (nsl and test machines): `make flatpak-sources` with a third
  generator in the nsl machine; a disposable desktop VM for Phases 4 to 10:
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
- maild: the `flock` on `<data dir>/maild.lock`; in the Flatpak, the mode
  check of `app.json` under the lock; `-log` with rotation past 1 MiB once
  it holds the lock; `rpc.shutdown` (caller `local`) and `rpc.hello`'s
  optional `build` (`schema/rpc/rpc.yaml`, `make gen`); `-build`, which
  prints the instance's `app-commit`.
- The launcher: `--background`; the start through `flatpak-spawn
  --latest-version` and its `SIGKILL`; the forward-only replacement by
  `build`; the Background request helper (PyGObject).
- `finish-args` as ADR-0046 lists; `install.sh` keeps its own for the
  development bundle with a host maild.
- The smoke test in the build, without portals: `frostmail --background`
  at the Flatpak's socket; a second maild on the same data directory
  exits; `mailctl` reads and writes through it; the launcher with a
  stand-in `flatpak-spawn` ends it, and maild lives on; with `app.json`
  naming lodge, `--background` exits 0 and starts nothing; a launcher
  held after it read "local", while `app.json` changes to lodge, starts a
  maild that exits without opening the database; a running maild whose
  `build` differs from the stand-in latest commit is shut down and
  replaced, and one that matches is kept, also when both builds report the
  same version; the log turns over.
- **Done when:** the smoke test passes in CI, and `make check` passes.

## Phase 4 — Secrets, sign-in and notifications in the sandbox (planner, medium)

- `--talk-name=org.freedesktop.secrets`; the Flatpak's item namespace
  (`application=org.frostyard.Frostmail`, `store=<store id>`) and the
  store id in the database; the no-fallback rule and `ErrNoKeyring`'s
  message; the probe before account setup saves an account. **Decided:**
  the probe runs inside `account.create`, before the insert: it writes,
  reads back and deletes a probe secret (`secrets.Probe`), and fails with
  `unavailable`, adding no account, when the store cannot keep one. It is an
  existing method, so there is no new IDL method and no app change: the
  setup screen already shows `account.create`'s error. Unit tests on
  `dbustest`: two stores with an account 1 each on one service keep
  their own secrets.
- A notification's click through `flatpak-spawn --latest-version`.
- [design/accounts.md](../design/accounts.md#secrets) and
  [design/desktop.md](../design/desktop.md) follow.
- **Done when,** on Duncan's desktop VM with a CI bundle:
  1. a password account on the mailtest container is added, and its secret
     is in the keyring under the store's namespace (`secret-tool search
     application org.frostyard.Frostmail store <store id> key
     account/<id>/password`), not in a file;
  2. in a session with only the `session` collection, setup asks for a new
     "Default keyring", and a cancel fails the setup with no account saved;
  3. a new message notifies with the window closed, and its click opens the
     window at the message, in its own sandbox (`flatpak ps`);
  4. an OAuth sign-in completes through the loopback redirect (see Open
     questions for the account);
  5. with a host maild (from `make install`) and the Flatpak each holding
     an account 1 with different passwords, both sync, and neither item
     changes when the other store writes;
  6. after `flatpak uninstall --delete-data` and a new install, the new
     store asks for its account's password and does not read the old
     items.

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
- The Server pane's texts (ADR-0046), from local facts only; the ordered
  stop (write `app.json`, `rpc.shutdown`, take the data lock, or write
  local back after 30 s); saving local starts maild through the launcher.
  [specs/settings-ui.md](../specs/settings-ui.md#server-pane) first, then
  the card.
- An e2e test in the nsl machine: local, then lodge (maild stops, the
  local data stays), then local again (the pane warns that the accounts
  resume, then the same accounts return); the warning also with lodge
  unreachable; a maild that will not stop leaves the mode local, with the
  pane's error.
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
  and generated mail, after the Flatpak's first launch (its maild running,
  its store empty): nothing changes until the user runs the 3 steps; after
  them, with the window left open during the import, the Flatpak's maild
  lists the same accounts and syncs without a new sign-in, the host's maild
  is disabled, the host's keyring items and data and the export directory
  are untouched, and the pristine database sits aside. An import into a
  store with an account refuses and changes nothing.

## Phase 8 — A release candidate (Brian and Lucilla, small)

The clean-install gate comes before the public release, so the gate tests
a candidate, and the release promotes that same digest.

- The publishing workflow learns pre-release tags (`vX.Y.Z-rc.N`): it
  pushes and attests the image with its version and `sha-` tags, and does
  not move `latest`.
- A test remote that serves the candidate's digest only, by the Frostyard
  remote's mechanism (Lucilla names it: a test catalogue of flatpak-index,
  or a local OCI remote on the VM).
- Brian pushes the candidate's tag, and a second one for Phase 9's update
  check.
- **Done when:** the VM installs the candidate from the test remote, and
  `flatpak info` shows the attested digest.

## Phase 9 — The clean-install gate (planner, with Duncan)

On a fresh Snow VM from Duncan, with real portals, against the candidate
of Phase 8. Each check is recorded here with the candidate's digest and the
date.

1. Nothing else is installed: no lodge, Tailscale, host maild, sysext or
   other suite app (`flatpak list`, `systemctl --user list-units`, `which
   maild`).
2. Install from the test remote.
3. The core local workflow: add an account (mailtest), receive, read, reply
   with an attachment, Save As, search, a rule that moves a message, Send
   Later.
4. Close and reopen the window; maild keeps syncing while it is closed.
   Log out and in: maild starts at login (Background portal allowed), and
   a Send Later due during the logout goes at login. Refuse the portal on a
   second VM: maild starts with the first window only.
5. `flatpak update` to the second candidate, with a window and a
   notification of the first still open: the old maild is replaced by the
   new build, never the reverse; the database migrates; nothing is lost.
6. Opt into lodge (lodge's development build reachable from the VM):
   read, send, Save As, and a notification while the window is open.
7. Disconnect lodge, then switch modes both ways: no data moves or
   disappears, no second maild runs, and the pane's texts match what
   happened.

- **Done when:** all 7 pass on one candidate digest.

## Phase 10 — Release (Brian and Lucilla, small)

- Brian runs the promotion: the OCI tags `vX.Y.Z` and `latest` move to
  the digest that passed Phase 9, already attested (a step in the
  workflow that copies by digest and builds nothing).
- Brian makes `ghcr.io/frostyard/frostmail` public (ADR-0045).
- Lucilla lists `org.frostyard.Frostmail` in `flatpak-index/apps.json`.
- **Done when:** `flatpak install frostyard org.frostyard.Frostmail` on a
  fresh VM installs the digest that passed Phase 9, and plan 0010's Mail
  row in core records it.

## Later / ideas

- A switch in Settings for maild in the background, beside the portal's.
- Notifications in lodge mode with the window closed (see Open questions).
- Narrowing `home:ro` with the file chooser portal for opening (ADR-0045).
- The Secret portal, if the suite moves to it together.

## Open questions

- **Attestation of a candidate:** flatpak-index lists a digest that the
  release workflow attested on a tag. Lucilla confirms that a
  `-rc.N` tag's attestation, and a promotion by digest, satisfy it, by
  Phase 8.

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
