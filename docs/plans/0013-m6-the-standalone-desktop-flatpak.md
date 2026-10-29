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

Status (2026-10-29): **Phase 1 is open:** Brian accepted ADR-0046 on
2026-10-28, and ADR-0013, 0041 and 0045 say that it amends them in part;
core's amendment of ADR-0058 and plan 0010 is still to come.
**Phases 2 and 3 are built** (PR #126, merged 2026-10-28): maild and
mailctl in the Flatpak's build, the launcher, the lock, `-log`, `rpc.shutdown`, `build`,
the paths and the smoke test. CI's "Flatpak build and test" runs the smoke
test; the desktop VM checked the portals, Background and `flatpak-spawn` on
2026-10-28 (below). **Phase 4's secrets part is built (PR #130, merged
2026-10-28):** `--talk-name=org.freedesktop.secrets`, the Flatpak's keyring
namespace and the store id, the no-fallback rule with `ErrNoKeyring`'s
message, the probe in `account.create`, and the unit tests on `dbustest`.
**Phase 4 is done:** the notification click (#136, merged 2026-10-28), and
the OAuth check passed on the VM on 2026-10-29 ([below](#oauth-check-2026-10-29)).
**Phase 5's code is built (PR #132, merged 2026-10-28):**
Save As through the file chooser portal in both modes, attaching by bytes
in both modes, with the portal's Rust tests and the Vitest tests. **Phase 6 is built (PR #131,
merged 2026-10-28):** the first-run welcome, the Server pane's notes, the
ordered change of mode in the shell (`mailctl shutdown`, the write-back,
`frostmail --start-maild`), the settings window without a maild, and their
unit, smoke and end-to-end tests. **Phase 7's code is built (PR #135, merged 2026-10-28):** `mailctl import DIR`, the
welcome's move note (`host_mail_found`) and the steps in
[design/desktop.md](../design/desktop.md#the-flatpaks-own-maild), with unit
tests on staged exports.
**The desktop VM checks of Phases 4 to 7 ran on 2026-10-28**
([VM check 2026-10-28](#vm-check-2026-10-28)): Phases 5 and 6 pass; Phase 4
passes items 1, 2, 5 and 6, fails item 3 (the click, not built) and did not
run item 4 (OAuth); Phase 7 fails, because a window left open during the
import does not reconnect, though its other checks pass. Fix PRs: #136 (the
click), #137 (the setup's error text) and #138 (import starts maild again).
The CI bundles of #136 and #138 passed the failed checks on the VM the same
day (below), and all 3 are merged. The OAuth check (item 4) passed on
2026-10-29, so every desktop VM check of Phases 4 to 7 passes.
**Phase 8's workflow part is built (PR #143):** a `vX.Y.Z-rc.N` tag
publishes and attests a candidate without moving `latest`, with a dry-run
test. **Phase 10's workflows are built (PR #145):** the Promote workflow
copies the tested candidate's digest to `vX.Y.Z` and `latest`, and a
`vX.Y.Z` tag no longer publishes the Flatpak. Phase 8's test remote and
tags, Phase 9, and the release itself are not done.

## Who helps

- **Brian:** accepts ADR-0046; pushes the candidate and release tags;
  runs the promotion (Phase 10); makes the GHCR package public (ADR-0045); answers the plan's open
  questions.
- **Lucilla** (package delivery): core's amendment of ADR-0058 and plan
  0010; a test remote for a release candidate (Phase 8); the listing in
  `flatpak-index/apps.json` (Phase 10).
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
- **Built (PR #132):** `app/src-tauri/src/portal.rs` (the portal's call
  and its unit and session-bus tests), the commands `in_flatpak` and
  `portal_save_file`, `filesAsBytes` in the window's transport,
  `saveMessage.flatpak.test.ts` and `ComposeDrop.flatpak.test.tsx`.
- **Done when,** on the desktop VM (the checks wait for it): Save As writes
  a message into `~/Documents` in local mode and in lodge mode (lodge's
  development build on the VM); a 25 MB attachment is sent in both modes; a
  cancelled Save As writes nothing.

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
- **Decided in the build:** the note sits in the first-run welcome
  ([ui.md](../specs/ui.md#first-run-m6)), its dismissal is kept in the
  window's `localStorage`, and it does not show in the development bundle
  (`FROSTMAIL_SOCKET` set). Import stages the export in the data directory
  first, and also moves aside the store's `blobs` and `secrets.json` when
  they exist, with the database's stamp.
- **Done when,** on the desktop VM with a host maild from `make install`
  and generated mail, after the Flatpak's first launch (its maild running,
  its store empty): nothing changes until the user runs the 3 steps; after
  them, with the window left open during the import, the Flatpak's maild
  lists the same accounts and syncs without a new sign-in, the host's maild
  is disabled, the host's keyring items and data and the export directory
  are untouched, and the pristine database sits aside. An import into a
  store with an account refuses and changes nothing.

## VM check 2026-10-28

On Duncan's desktop VM `frostmail-desktop` (Snow `20261028172907`, GNOME 48
on Wayland, real portals), with the CI bundle of main at `1fb303c`
(Flatpak run 40758470007, commit "Built from 1fb303c"; flatpak-index has no
`frostmail` yet) and the `frostmail-mailtest` container. The container's
certificate is self-signed, so each maild ran with `FROSTMAIL_INSECURE_TLS=1`
(`flatpak override --user --env=…`, and a drop-in for the host's
`maild.service`). Lodge mode used lodge's development build (`lodge-e2e`, lodge
`e76a17c`) on the VM's loopback and `maild --lodge` with a mailtest account,
staged as `app/e2e/lodge.e2e.ts` stages it. The screenshots and the check
scripts are in Irulan's home, `evidence/plan-0013-vm-2026-10-28/`.

**Phase 4.**

1. Pass. test1, added on the setup screen, has its item in the login
   collection: `application=org.frostyard.Frostmail`, `store=b5cc3370…` (the id
   in `frostmail.db`), `key=account/1/password`. There is no `secrets.json`,
   and no file under `~/.var/app/org.frostyard.Frostmail` holds the password.
2. Pass, with a wrong text. With only the `session` collection (the login
   keyring moved away, gnome-keyring restarted), Add Account asked "Choose
   password for new keyring … called 'Default keyring'". Cancel added no
   account, and maild logged "the Secret Service's prompt was dismissed"; but
   the setup screen said "Set up your Google client under Sign-In first." Fix:
   #137, the setup shows `account.create`'s own error.
3. Fail (not built). With no window, a new message notifies (from maild), and
   the click opens the window at the message; but that window
   (`frostmail-app --open-message 13`) is a child of maild, in maild's sandbox,
   and `flatpak ps` shows no new instance. Fix: #136, the click through
   `flatpak-spawn --latest-version`.
4. Not run on 2026-10-28: it needs a Google account that may sign in from
   the VM. It passed on 2026-10-29 ([OAuth check](#oauth-check-2026-10-29)).
5. Pass. The host's maild (`make install`, test1) and the Flatpak's (test2,
   another password), each with an account 1, both synced a new message. A
   write of a new value in one store changed only its own item; the other
   item kept its Modified time and its secret's hash. The host's item has
   `application=frostmail` and no store.
6. Pass. After `flatpak uninstall --delete-data` and a new install, test1
   added with no password in the new store (`store=5ebad2fe…`) showed "Sign in
   again" ("no password is stored"), though the old store's item
   (`store=5058caad…`, the same key) was still in the keyring, unchanged.
   `--delete-data` leaves the old store's items in the keyring.

**Phase 5.** Pass, in both modes.

- Save As opened the portal's "Save Message" dialog and wrote the message
  into `~/Documents`: mode 0600 in local mode (`message.save`), 0644 in lodge
  mode (`save_content`). A cancelled Save As wrote nothing, with no error.
- A 25,000,000-byte attachment (`AttachmentLimit`), attached through the
  portal, was sent in each mode; test2's copies decode to the file's SHA-256.
  A 25 MiB file (26,214,400 bytes) is refused: "would take the draft's
  attachments past the 25 MB limit".

**Phase 6.** Pass.

- The first run showed the welcome ("Add Account…", "Connect to lodge…") and
  the settings window on Accounts.
- A switch to lodge (`http://127.0.0.1:8470`) wrote `app.json` and stopped
  maild; no maild ran. After a log out and in, the autostart entry
  (`frostmail --background`) ran and started no maild (`flatpak ps`).
- With lodge down, the window offered "Server Settings…"; the switch back
  showed the pane's local-mode text, started maild, and brought back test1
  with its mail. `app/e2e/mode.e2e.ts` passed in CI at `1fb303c`.
- In this run, `flatpak ps` still listed maild's old instance after the
  switch, because the window of item 3 above ran in it; no maild ran.

**Phase 7.** Fail: a window left open during the import does not reconnect.
The other checks pass.

- The host's maild (`make install`) held test1 with 311 generated messages.
  The Flatpak's first launch showed the welcome with the move note and its
  3 commands; its store stayed empty, and the host's maild ran until step 1.
- The 3 steps: `export` (311 blobs, 1 secret, schema 19); `import`, with the
  window open, validated the export, stopped the Flatpak's maild, placed the
  export and put the earlier database aside
  (`frostmail.db.before-import-20261028T204240Z`, and its blobs).
- At the next start, maild moved the secret into its namespace and synced a
  new message with no sign-in. The host's `maild.service` stayed disabled;
  the host's database, its keyring item and the export directory kept their
  checksums.
- The gap: the window left open did not reconnect. Import stopped maild and
  nothing started it, and "open Frostmail again" from the shell only focused
  the open window, so the launcher did not run. Closing the window and
  opening it again started maild. Fix: #138, import starts the Flatpak's
  maild again (`frostmail --start-maild`).
- A second import, into the store with the account, refused ("holds 1
  account, 312 messages … nothing was moved", exit 1), and every file kept
  its checksum (SQLite added an empty `-wal` and its `-shm`). It also left
  maild stopped (the same fix, #138).

**The fix PRs, checked again on the VM (2026-10-28).**

- #136's bundle: maild's `FROSTMAIL_APP` is `/app/libexec/frostmail-open`.
  With no window, a click opened the window at the message in a new
  instance (`flatpak ps`), in another PID namespace than maild. After an
  update (main's bundle installed as the latest deployment while #136's
  maild ran), the click on the old maild's notification ran the latest
  launcher: it replaced the old maild with the latest, and the window opened
  with the latest `app-commit` and lived on. Until that window closed,
  the old instance stayed in `flatpak ps`, holding only the `flatpak-spawn`
  that waits for it.
- #138's bundle: with the window open, `import` printed "maild started with
  the imported accounts; open windows reconnect." The window reconnected by
  itself and listed the imported mail, and a new message synced. A second
  import refused and printed "maild started again"; maild answered after it.

For Phase 8: the Flatpak's maild reports the version `flatpak`, and the
bundle's version is 0.0.0.

### OAuth check 2026-10-29

Phase 4 item 4 passed, with the CI bundle of main at `8c5a5a7` (Flatpak run
40878636033). The account is the Google test account Brian named on the
desk (T421c); its address stays out of this record.

- The VM needed 2 things first. The clean Snow image has no web browser, so
  Firefox from Flathub became the default, for the OpenURI portal. The
  bridge's resolver answered `192.0.0.88` for `accounts.google.com`, so the
  VM used public resolvers for this check only (Duncan fixes the bridge).
- Brian entered his own Google Desktop-app client in Settings, Sign-In
  (Frostmail has no verified client yet), added the account as Gmail with
  OAuth, and signed in. Firefox went through Google's sign-in, the
  unverified-app warning and the consent page, then to
  `http://127.0.0.1:<port>/`: the loopback redirect reached maild, with no
  pasted address (ADR-0043).
- The account is `signedIn`, and it synced: INBOX 12, All Mail 16, Sent 2,
  Drafts 1, Spam 6, Trash 11 and a label; Google's "new sign-in" alert of
  the minute arrived in the INBOX.
- The keyring holds `account/1/refresh-token` and
  `oauth/google/client-secret` under `application=org.frostyard.Frostmail`
  and the store's id; the data directory has no `secrets.json`.
- maild logged "sync stopped: sign in again" once, in the same millisecond
  as the account's creation, before the sign-in: the setup creates the
  account, then authorizes it. The account synced after the sign-in.
- Afterwards the VM went back to its `clean` snapshot; no snapshot keeps the
  client's secret or the token.

## Phase 8 — A release candidate (Brian and Lucilla, small)

The clean-install gate comes before the public release, so the gate tests
a candidate, and the release promotes that same digest.

- The publishing workflow learns pre-release tags: Built (PR #143). In
  `.github/workflows/flatpak.yml`, the step "Name the OCI tags" sorts the
  tag: `vX.Y.Z-rc.N` pushes the version and `sha-` tags and attests the
  digest, and leaves `latest` and the Frostyard remote alone; any other
  `v*` tag fails before it pushes. (PR #143 also let `vX.Y.Z` publish and
  move `latest`; Phase 10 took that away: a `vX.Y.Z` tag now publishes
  nothing, and the promotion moves `latest`.) `release.yml` ignores `v*-*` tags, so a candidate makes
  no `.deb` and no GitHub release. `scripts/flatpak_publish_test.go` runs
  the publish job's steps with stub `skopeo` and `gh` for each kind of tag.
- A test remote that serves the candidate's digest only: Lucilla chose
  flatpak-index's staged catalogue
  ([Stage a candidate](https://github.com/frostyard/flatpak-index#stage-a-candidate),
  core [ADR-0062](https://github.com/frostyard/core/blob/main/docs/adr/0062-make-desktop-suite-apps-standalone-flatpaks-with-opt-in-lodge.md)):
  `scripts/build-index.py` builds it with the candidate's digest as the tag,
  and the VM serves it at `127.0.0.1:8765`. Lucilla writes the VM's steps
  (T488).
- Brian pushes 2 candidate tags: `v0.4.0-rc.1` for the gate, and
  `v0.4.0-rc.2` on a later commit of main for Phase 9's update check
  (item 5). `svu next` gives `v0.4.0` today, and still gives `v0.4.0`
  after the candidates, so the release tag follows them.
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

A Flatpak release is always a promoted candidate: the digest that passed
Phase 9 becomes the release, and nothing is built again.

- **The promotion workflow:** Built (PR #145).
  `.github/workflows/promote.yml` (Promote) runs only by hand
  (`workflow_dispatch`), with the inputs `candidate` (`vX.Y.Z-rc.N`) and
  `version` (`vX.Y.Z`). It runs on a GitHub-hosted runner, has no
  `id-token` and no `attestations: write`, and uses no action. In order:
  1. The inputs: `candidate` is `vX.Y.Z-rc.N`, `version` is `vX.Y.Z`, and
     the candidate is a candidate of that version.
  2. The candidate's digest, from its OCI tag (`skopeo inspect`).
  3. The git tag `vX.Y.Z` exists and contains the candidate's commit (the
     compare API answers `identical` or `ahead`): the release is
     `make bump`'s, below.
  4. The attestation: `gh attestation verify` on the digest, with
     `--signer-workflow` `flatpak.yml`, `--source-ref
     refs/tags/vX.Y.Z-rc.N` and `--deny-self-hosted-runners`, and an
     attestation whose `buildSignerURI` is `flatpak.yml` on the candidate's
     tag, as flatpak-index's `build-index.py` checks (T488). An attestation
     from another workflow file does not count. The promotion makes no
     second attestation: an attestation binds the digest, so a copy by
     digest keeps it.
  5. The OCI tag `vX.Y.Z` is free, or already names the candidate's digest
     (a run again after a failure passes).
  6. `skopeo copy --preserve-digests` of the digest to `:vX.Y.Z`, then to
     `:latest`.
  7. With `FLATPAK_INDEX_DISPATCH_TOKEN`, the refresh of flatpak-index
     (`gh workflow run publish.yml`), as a release did before.

  It refuses, before any copy, when a check fails: bad inputs, no
  candidate image, no release tag or one that does not contain the
  candidate, no valid attestation from `flatpak.yml`, or `vX.Y.Z` on
  another digest. `scripts/flatpak_promote_test.go` runs the job's steps
  with stub `skopeo`, `gh` and `docker` for each case.
- **A `vX.Y.Z` tag no longer publishes the Flatpak** (PR #145). In
  `flatpak.yml`, the step "Name the OCI tags" lets a `vX.Y.Z` tag build
  and test, and publish nothing, with a notice; only a `vX.Y.Z-rc.N` tag
  publishes and attests. So the release tag cannot replace the tested
  digest with a new build, and only the promotion moves `vX.Y.Z` and
  `latest`. `scripts/flatpak_publish_test.go` checks it.
- **The .deb and the GitHub release** (Irulan's choice, 2026-10-29, for
  Brian's review in PR #145):
  `make bump` makes them, as for every release (core ADR-0012: it stays
  the only way to tag a release). `release.yml` ignores the candidates'
  tags; the release tag `vX.Y.Z` runs it as before, and it builds
  `frostyard-maild_X.Y.Z_amd64.deb` from the tag's commit. The tag marks
  main's head, which contains the candidate's commit and may be later: the
  .deb (lodge's server) can carry commits that the Flatpak does not. The
  order for Brian:
  1. Phase 9 passes on a candidate `vX.Y.Z-rc.N`.
  2. `make bump` on main. `svu next` must give `vX.Y.Z`, the candidate's
     version; if it gives another, Brian stops and pushes a new candidate.
  3. Promote, with `candidate` `vX.Y.Z-rc.N` and `version` `vX.Y.Z`
     (GitHub, Actions, Promote, Run workflow; or `gh workflow run
     promote.yml -f candidate=vX.Y.Z-rc.N -f version=vX.Y.Z`).
- Brian makes `ghcr.io/frostyard/frostmail` public (ADR-0045). It already
  answers anonymous pulls (its tag list, checked 2026-10-29), so this step
  may be done; Brian confirms.
- Lucilla lists `org.frostyard.Frostmail` in `flatpak-index/apps.json`.
- **Done when:** `flatpak install frostyard org.frostyard.Frostmail` on a
  fresh VM installs the digest that passed Phase 9, and plan 0010's Mail
  row in core records it.

## Later / ideas

- A switch in Settings for maild in the background, beside the portal's.
- Notifications in lodge mode with the window closed (see Open questions).
- ~~Narrowing `home:ro` with the file chooser portal for opening (ADR-0045).~~
  Decided (T387a, Brian, 2026-10-28): `--filesystem=home:ro` stays, as
  ADR-0046 says, for Phase 7's note; Save As and attaching no longer need it.
- The Secret portal, if the suite moves to it together.

## Open questions

- ~~**Attestation of a candidate:** flatpak-index lists a digest that the
  release workflow attested on a tag. Lucilla confirms that a
  `-rc.N` tag's attestation, and a promotion by digest, satisfy it, by
  Phase 8.~~ Decided (T488, Lucilla, 2026-10-29): yes. `build-index.py`
  accepts an attestation from `flatpak.yml` on a GitHub-hosted runner whose
  ref matches `refs/tags/v[0-9][^/]*`, which `refs/tags/v0.4.0-rc.1` does.
  An attestation binds the digest, so Phase 10's copy by digest keeps it;
  an attestation from another workflow file would not count.

- **Lodge mode with the window closed:** nothing notifies (ADR-0041,
  ADR-0046). Brian decides whether a background connection is wanted, by
  Phase 6.
- ~~**The OAuth check of Phase 4:** it needs a Google account that may sign
  in from the test VM.~~ Decided (T421c, Brian, 2026-10-28): his test
  account; he signs in himself. Passed on 2026-10-29.

## References

- Implements: [ADR-0046](../adr/0046-the-desktop-flatpak-carries-its-own-maild.md)
- Context: [design/desktop.md](../design/desktop.md), [design/accounts.md](../design/accounts.md#secrets), [specs/settings-ui.md](../specs/settings-ui.md#server-pane), [ADR-0045](../adr/0045-the-desktop-app-builds-as-a-flatpak-in-ci.md), [plan 0011](0011-m8-mail-in-lodge.md)
- Model: frostnotes [ADR-0031](https://github.com/frostyard/frostnotes/blob/main/docs/adr/0031-the-desktop-app-ships-as-a-flatpak-with-its-own-notesd.md), with its Snow evidence
- Core: [plan 0010](https://github.com/frostyard/core/blob/main/docs/plans/0010-suite-packaging.md), [ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md)
