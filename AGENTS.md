# frostyard/frostmail

Frostmail is a Linux mail client that works like Apple Mail.app without
looking like a GNOME or KDE app. A headless Go daemon, `maild`, owns sync,
storage and the RPC socket. A native Go app on NSGo and Frostyard's kit
(`cmd/frostmail-nsgo`) and `mailctl`, a CLI, are clients of that socket.
maild also runs as the `mail` app of
[lodge](https://github.com/frostyard/lodge), frostyard's tailnet web shell,
where the same client's web build runs in a browser. Start at [docs/README.md](docs/README.md);
the architecture entry point is
[docs/design/overview.md](docs/design/overview.md).

This is the canonical agent instruction file. `CLAUDE.md`, `GEMINI.md`,
`.cursorrules` and `.github/copilot-instructions.md` are relative symlinks
to it, and `.claude/skills` is a symlink to `.agents/skills/`; edit this
file only.

## Status

Pre-release. The order of work is the [roadmap](docs/plans/0002-roadmap.md),
and each milestone's plan carries its own status. M8, mail in lodge
([plan 0011](docs/plans/0011-m8-mail-in-lodge.md)), is in progress:
- **Built and merged on 2026-10-19:**
  - maild's lodge socket;
  - the web build, with its narrow layout;
  - compose and the other windows as routes;
  - attachments, Save As by download and links by Message-ID;
  - notifications, in the browser and in the desktop app;
  - `/mcp/mail`, whose scope grants reading, writing and sending;
  - the desktop app as a lodge client.
- **Still to see:** a few checks in a real browser and on real desktops
  (plan 0011's status).
- **Phase 7,** moving the user's mail to lodge's host: its tools are
  built (`mailctl export`, `scripts/move-mail-to-lodge.sh`), and so is the
  sealed secrets store of [ADR-0044](docs/adr/0044-seal-the-servers-secrets-with-a-systemd-credential.md)
  (`FROSTMAIL_SECRETS=sealed`, a key from a systemd credential, made by
  `frostmail-secrets-key.service` at first start, [ADR-0047](docs/adr/0047-make-the-secrets-key-at-first-start.md);
  `scripts/seal-mail-secrets.sh` can make it earlier). Phase 7 is done,
  accepted by the user on 2026-11-15 without its "Done when" evidence
  (plan 0011's status names what is missing).

M8.5 ([plan 0012](docs/plans/0012-calendar-tasks-and-contacts-for-agents.md)),
calendar, task and contact tools on `/mcp/mail` for Porter, is built and
checked live over lodge's agent socket (2026-10-21).

M6, the standalone desktop Flatpak
([plan 0013](docs/plans/0013-m6-the-standalone-desktop-flatpak.md)), is
in progress: [ADR-0046](docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md)
(accepted 2026-10-28) bundles maild and mailctl, runs them by default, and
keeps lodge as a connection the user chooses. Phases 2 and 3 (maild and
mailctl in the build, the launcher, the data lock, one local maild) are
merged (#126); Phase 4's secrets part (the Flatpak's keyring namespace, the
store id, no plain-file fallback, the probe in `account.create`), Phase 5's
files, Phase 6's first run and modes and Phase 7's move (`mailctl import`,
the move note, #135) are merged, with the notification click (#136). Their
desktop VM checks all pass (plan 0013, VM check 2026-10-28, and the OAuth
check of 2026-10-29). Phase 10's promotion workflow (#145) and release tag (#147) are built.
Phase 8 is done: Brian tagged `v0.4.0-rc.1` and `v0.4.0-rc.2`, and the clean
VM installed rc.1 from flatpak-index's staged catalogue. Phase 9 is done on
rc.2 (Brian's decisions T491a to T491c, 2026-10-29; plan 0013, Clean-install
gate 2026-10-29): with the Background portal refused, maild runs only while a
window is open (ADR-0046). Phase 10: Brian released `v0.4.0` from rc.2 (the
`make bump` tag, then Promote), flatpak-index lists it, and a fresh VM
installed it with the README's commands (plan 0013, Phase 10 check
2026-10-29). Core's plan 0010 Mail row is left (Lucilla).

Plan 0014 ([small features from the nexmail review](docs/plans/0014-small-features-from-the-nexmail-review.md),
Brian's T929a, 2026-11-05) is specified, with ADRs 0050 and 0051
(accepted). Phase 1, the sender check banner (#159), Phase 2, the
attachment reminder (#160), Phase 3, snippets (#161), and Phase 4, erasing
old Trash and Junk (#162), are built; #163 to #166 are its backlog. nexmail is AGPL-3.0:
take its ideas only, never its code.

Plan 0015 ([the desktop app in MyGo](docs/plans/0015-mygo-desktop.md)) and
[ADR-0054](docs/adr/0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
(Accepted, 2026-11-22; superseded by ADR-0063) replaced the old shell with a
native Go app on MyGo and frostui (`cmd/frostmail`), which shipped up to
`v0.10.0`; the React app was lodge's web app until plan 0017's Phase 3.5.
Tauri's shell is gone.
[ADR-0055](docs/adr/0055-maild-streams-its-activity-as-typed-entries.md)
(Accepted) adds maild's activity stream for the app's console; its maild
side is built (`mailctl activity`). Plan 0017's Phase 3 removes the MyGo
client, and with it plan 0015's parity gate, which ADR-0063 says will not
pass.

Plan 0016 ([a shared look for the NSGo apps](docs/plans/0016-a-shared-look-for-the-nsgo-apps.md))
and [ADR-0062](docs/adr/0062-give-the-nsgo-client-the-frostyard-look-from-the-shared-kit.md)
(Accepted, 2026-11-24) give the NSGo client (`cmd/frostmail-nsgo`,
[ADR-0061](docs/adr/0061-share-the-mail-client-with-nsgo.md)) Frostyard's
look from a kit that FrostMail, Frostnotes and PocketGo share. Phase 0 is
done (2026-11-24): the kit moved from `frostyard/pocketgo` to
`github.com/frostyard/frostui/workspaceui` (`workspaceui/v0.1.0`), and the
three apps build on it. Phase 0.5 is done (2026-11-24): FrostMail's and
Frostnotes' suites run in a real GTK window (`make nsgo-live`) and keep
their pictures. Phase 1 is done (2026-11-24; Brian accepted closing it on
a desktop picture of PocketGo's title row alone, plan 0016, Phase 1): the
kit has the Frostyard themes (`workspaceui/v0.2.0`), and the three clients
draw them, in Inter, with a saved choice of appearance. Phase 2 is done
(2026-11-24): the kit holds the accepted parts, the control map and the
rules for an app (frostui's `docs/specs/workspaceui.md`), with a gallery
that draws them (`workspaceui/v0.3.0`). Phase 3, the client's main window,
placed the kit's parts and NSGo's controls by the control map, and each
change carried its changed pictures. 3a, the shell
(the module tabs, the navigation's rows and foot, the icon toolbar:
`cmd/frostmail-nsgo/shell.go`), 3b, the list (summary rows, the search
field, the filter, selection by Ctrl and Shift:
`cmd/frostmail-nsgo/list.go`), and 3c, the reader (the header block, the
banners, the page's frame: `cmd/frostmail-nsgo/reader_pane.go`), are
built, and Phase 3 is done (2026-11-24): Brian accepted the main window
from the live suite's pictures, without setting them beside `main.png`
(plan 0016, Phase 3), and `dev/lodge/nsgo-smoke.py` passes. Phase 4, the
client's other surfaces, is in progress, in 5 changes: 4a, one filled
button for a region and confirmations on NSGo's alert dialog, 4b,
Settings on NSGo's settings view (`cmd/frostmail-nsgo/settings_view.go`),
4c, the compose window, and 4d, the other modules, each with its own
rows in the navigation (`cmd/frostmail-nsgo/modules.go`), are built. Of
4e, the narrow layout and Lodge's web build are checked in a browser, and
the style test of rule 2 is on with no exception (`TestNoLiteralLooks`;
the paddings take the kit's spacing, `workspaceui/v0.4.0`), and the
README's pictures are the NSGo suite's. Recipients on NSGo's tag input
are left, which waits on NSGo.

Plan 0017 ([NSGo as the only client](docs/plans/0017-nsgo-as-the-only-client.md))
and [ADR-0063](docs/adr/0063-make-the-nsgo-client-the-only-client.md)
(Accepted, 2026-11-24) make the NSGo client the only client, in 3 steps:
the default on the desktop, MyGo removed on Linux and Windows, then the
React app's place in Lodge. The owner answered the plan's questions the
same day. Of Phase 2, the NSGo client keeps its window, its panes' widths
and how the list is shown (`cmd/frostmail-nsgo/window.go`), and records
`bridge.socket_lost` as the MyGo client did, under the release's version,
and shows the first run's welcome
(`cmd/frostmail-nsgo/welcome.go`), and lays the day and the week out by
time, where a drag moves, resizes and draws an event
(`cmd/frostmail-nsgo/calendar_time.go`). Its package has the released
app's id, and **the Flatpak's launcher starts it**: `FROSTMAIL_UI=mygo`
starts the MyGo client, which the Flatpak holds for 1 release: `v0.10.0`
(2026-11-26). Phase 2 is done on Brian's report that the clean-install
gate on `v0.10.0-rc.1` passed (plan 0017, Phase 2, The gate, which names
what the report does not). Phase 3, MyGo's removal, is built and not
released: `cmd/frostmail` and the MyGo-only packages are gone, the
packages the NSGo client shares are split so that neither `go.mod` names
MyGo or frostui's root module, the Flatpak holds the NSGo client alone
(`FROSTMAIL_UI=mygo` is ignored, with a line on the launcher's error
output), and Windows' installer is the NSGo client's (`make
windows-installer`), which CI's Windows job builds and installs. Its
installer was checked on the Windows VM over a Tauri and a MyGo install
(2026-11-26); the release is still to do (plan 0017, Phase 3).

## Two kinds of agent work

- **Planning and core work** (architecture, contracts, the sync engine,
  security-sensitive code, reviews): follow this file and the docs.
- **Task cards** (`docs/tasks/`, run by `make task T=NNNN` with a local
  executor model): follow [docs/tasks/EXECUTOR.md](docs/tasks/EXECUTOR.md)
  and the card. The card's `touch` list is the only set of files you may
  change. [docs/design/agent-workflow.md](docs/design/agent-workflow.md)
  explains the loop.

## Map

- `cmd/maild`: the daemon. `cmd/mailctl`: the CLI (clix/cobra; one
  `newXxxCmd(opts)` per file).
- `api/`: the RPC contract. `zz_generated.go` comes from `schema/rpc/*.yaml`
  via `tools/rpcgen` (`make gen`); `api.go`, `client.go` and `conn.go` name
  the runtime, which lodge's kit provides (`github.com/frostyard/lodge/kit/jsonrpc`).
  Never edit generated files.
- `internal/store`: SQLite (modernc, pure Go): migrations, the single-writer
  `Tx`, the durable `changes` log, queries.
- `internal/events`: fans committed events out to subscribers and replays
  the log. `internal/activity`: what maild does in the background, as
  typed entries with no text ([ADR-0055](docs/adr/0055-maild-streams-its-activity-as-typed-entries.md)):
  the only package that makes an `api.ActivityEntry`; `Classify` reads an
  error's type, never its text; clients write lines with `Line`. `internal/rpcserver`: the socket server, on the kit's
  JSON-RPC server, with its private socket (the kit's on Linux, a named pipe
  on Windows). `internal/engine`: the API
  domains over the store. `internal/rpctest`: a real server for tests.
- `internal/lodge`: maild as lodge's `mail` app (`maild --lodge`, on
  lodge/kit): the app socket and its content route
  ([specs/lodge-app.md](docs/specs/lodge-app.md)). `internal/mcpserver`:
  the MCP tools behind it, which the token's scope selects; it is the only
  package that imports the MCP SDK. `packaging/lodge`: the manifest, units and `stage.sh`
  (`make lodge-package`, which stages under `usr/` only:
  core [ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md)); `dev/lodge/run.sh` runs maild behind lodge's
  development build; `tools/lodgeprobe` checks a lodge socket.
- `internal/imapx`: the only production package that imports go-imap
  (`third_party/` and the probe `tools/msprobe` aside;
  `internal/workflowcheck` holds the rule).
  `third_party/go-imap` is a patched fork (Gmail X-GM-EXT-1); see its
  `FROSTMAIL-PATCHES.md` and [ADR-0004](docs/adr/0004-go-imap-fork.md).
- `internal/mimex`: message parsing and normalization helpers.
- Problem records ([ADR-0049](docs/adr/0049-record-problems-in-the-local-journal.md)):
  `internal/problemrec` makes the recorders of maild and mailctl (std's
  `problems`; the version rule); `cmd/maild` records `store.migrate_failed`,
  `secrets.unseal_failed` and crashes (`CrashOutput`), `internal/mailsync`
  `sync.apply_failed` (`apply.go`), `internal/render` `mime.parse_failed`;
  `cmd/frostmail-nsgo` is the app's (`bridge.socket_lost`). Tests read
  records from a test socket (`internal/problemstest`).
- `internal/compose`: outgoing mail (reply rules, the message builder).
  `internal/smtpx`: the only package that imports go-smtp.
- `cmd/frostmail-nsgo`: the desktop app, the NSGo client, a module of its
  own (`cmd/frostmail-nsgo/go.mod`) on NSGo and the shared kit
  (`workspaceui`), for Linux, Windows and Lodge's web build.
  `internal/desktop/` holds its logic with no UI, which imports neither
  NSGo nor frostui: `conn` (the maild connection, local or through lodge),
  `live` (the state kept current from maild), `mail` (the models, the
  reader's page and its CSP, thumbnails), `compose`, `organize` and
  `settings` (their logic), `opener` (what a link or an attachment may
  open), `trayhost` (whether a panel hosts the tray), `icons` (the tray
  icon) and `launcher`, which shows the All Inboxes count on the launcher
  icon on Linux (Unity's LauncherEntry signal; no Flatpak permission).
- `app/`: the loader of the NSGo client's web build in lodge's frame
  (`index.html`, `src/nsgo-lodge.ts`, the bridge `window.frostmailLodge`
  that `cmd/frostmail-nsgo/platform_web.go` calls). `make web-build` builds
  the client for `js/wasm` with NSGo's tool and the loader with Vite into
  `build/web`, which `make deb` installs and `make deb-check` checks
  ([docs/design/app.md](docs/design/app.md)); it runs in lodge's development
  build, and `dev/lodge/nsgo-smoke.py --deb` drives the `.deb`'s in headless
  Firefox. maild has no other gateway for it. `app/notify` is the Windows
  toast notifier (Rust).
- `packaging/flatpak`: the app's Flatpak manifest, its metainfo and desktop
  entry, and the offline `go-sources.json` (`make flatpak-sources`;
  [ADR-0045](docs/adr/0045-the-desktop-app-builds-as-a-flatpak-in-ci.md)).
  The Flatpak holds the app, maild and mailctl (`/app/bin`, built from
  `go-sources.json`), the launcher `frostmail.sh`, the Background helper and
  a smoke test ([ADR-0046](docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md)).
  CI builds it in `.github/workflows/flatpak.yml`; on a `v*` tag,
  `stamp-version.sh` first stamps the version (without `-rc.N`) into the
  manifest, for maild's, mailctl's and the client's builds, and the
  metainfo, which keep `flatpak` and 0.0.0 in git. `install.sh`
  is the wrapper for the development bundle, with a host maild.
- `packaging/windows/installer.nsh`: Frostmail's steps in the Windows
  installer of the NSGo client (`make windows-installer`,
  `scripts/windows-installer.sh`), which NSGo's tool builds and
  `tools/nsgoinstaller` writes them into; `installer-check.nsh` holds a
  check's names ([design/desktop.md](docs/design/desktop.md#windows)).
  `packaging/icons`: the app's icons.
- `dev/`: the nsl machine (`dev/nsl/provision.sh`) and the incus test mail
  server (`dev/incus/mailtest.sh`).
- `tools/rpcgen`, `tools/taskrun`: the contract generator and the task-card
  runner. `tools/testshard`: deals the packages into the shards of CI's race
  gate; `weights.txt` holds the measured seconds that balance them.
- `internal/workflowcheck`: tests of the repository's own files, not of
  product code: the workflows follow frostyard/core ADR-0021, and the
  baseline files (below) are complete, and only `internal/imapx` imports go-imap.

## Gates

- `make check` formats, then runs `make verify`: tidy, `gen-check`, vet,
  gofmt, the pinned golangci-lint (`mise install`), unit tests, and the fork's
  tests. Run it before claiming any Go change is complete.
- `make engine-it` runs integration tests against the Dovecot container
  (resetting it first). `make ui-test` runs in the nsl machine. `make ci` is
  verify plus the race gate (`make race`) and the app checks.
- `make race` is the race gate: `go test -race ./...`. CI runs it as 4 shards on
  runners of their own (`RACE_SHARD=I/4`, split by `tools/testshard`, which puts
  every package of `go list ./...` in exactly 1 shard), with `-count=1`. The
  required check "Race detector" passes only when every shard passed
  (`internal/workflowcheck` holds this).
- `make flatpak-sources` regenerates the Flatpak's `go-sources.json`. Run it
  after any change to `go.mod` or `go.sum`; CI's job
  "Flatpak sources (make flatpak-sources)" fails on a stale file.
- `make nsgo-test` runs the NSGo client's workflows headless on fixture mail
  (`cmd/frostmail-nsgo`, `-tags integration`); `make nsgo-live` runs the same
  suite in a real GTK 4 window under Xvfb. A change of the client's look runs
  `make nsgo-pictures` and commits the changed PNGs of
  `cmd/frostmail-nsgo/comparison/linux`
  ([design/testing.md](docs/design/testing.md#the-nsgo-clients-live-suite)).
- `make vulncheck` runs govulncheck (pinned in the Makefile; needs the
  network). It is not part of `check`, `verify` or `ci`; the CI job
  "Vulnerabilities (make vulncheck)" runs it. Keep it at 0 reachable findings.
- Fix a lint finding rather than loosening `.golangci.yml`.

## Releases and dependencies

- Tags look like `v1.2.3` (`.svu.yml`: prefix `v`, `v0: true`). The next version is
  `svu next`, from Conventional Commits; on v0 a breaking change gives a minor. It names
  the candidate `vX.Y.Z-rc.N`; the release takes the candidate's version.
- A pushed `v*` tag runs `.github/workflows/release.yml`: it checks the tag is
  `vX.Y.Z` on the commit of a `vX.Y.Z-rc.N` tag, builds the web UI and
  `frostyard-maild_X.Y.Z_amd64.deb` (`make deb`, nfpm) from that commit,
  attests it, and makes the GitHub release with `gh release create` (generated notes).
- The Flatpak releases by promotion (plan 0013, Phase 10). A `vX.Y.Z-rc.N` tag
  publishes and attests a candidate (`flatpak.yml`); a `vX.Y.Z` tag publishes no
  image, and `release.yml` ignores candidates. After the gate and `make bump`,
  Brian runs `.github/workflows/promote.yml` by hand: it copies the attested
  candidate's digest to `vX.Y.Z` and `latest`, and builds nothing.
- The `.deb` and the Flatpak of `vX.Y.Z` hold the same code (ADR-0048): the git tag
  `vX.Y.Z` is the candidate's commit, and `release.yml` and Promote refuse it on any other.
- Brian tags and promotes. Agents never tag, push tags, release or run Promote.
  `make bump CANDIDATE=vX.Y.Z-rc.N` runs `make verify` on the candidate's commit, tags
  that commit `vX.Y.Z` and pushes the tag; Brian runs it.
- Internal Go modules (`go.mod`): `github.com/frostyard/clix` v0.5.0,
  `github.com/frostyard/lodge/kit` v0.6.0, `github.com/frostyard/std` v0.5.0 (direct:
  `problems`; clix needs it too). Neither `go.mod` requires MyGo
  (`github.com/egoist/mygo`) or frostui's root module
  (`github.com/frostyard/frostui`) since plan 0017's Phase 3.
- The NSGo client's own module (`cmd/frostmail-nsgo/go.mod`) also requires
  `github.com/frostyard/frostui/workspaceui` v0.5.1, the kit it shared with
  Frostnotes and PocketGo before Frostnotes dropped its desktop window
  and PocketGo was retired (plan 0017), and `github.com/nsgo-dev/nsgo`
  v0.3.0, which frostyard does not own. The kit is a module of its own in
  frostui (tags `workspaceui/vX.Y.Z`; frostui's `docs/design/workspaceui.md`).
- Internal UI packages (`app/package.json`, github tags): `@frostyard/lodge-client`
  `client/v0.3.2`, the loader's.
- The generated `packaging/flatpak/go-sources.json` (`make flatpak-sources`) mirrors
  the Go versions above; it changes with every bump. Run `make flatpak-sources`
  after a change to `go.mod` or `go.sum`. A dependency bump also updates every
  current-state version in `AGENTS.md`, ADRs and plans; dated historical text
  stays unchanged when it still describes the recorded fact.
  `mise.toml`, the other `packaging/` files, the workflows and `scripts/` pin no
  frostyard or bketelsen module.
- Users: lodge hosts maild as its `mail` app, and Porter calls `/mcp/mail`. Other users:
  not stated.

## Conventions

- Go: `ctx` first; errors are lowercase, wrapped with `%w`; `slog` for logs;
  `encoding/json/v2`; no mutable package state; modern idioms (`t.Context()`,
  `wg.Go`, `omitzero`). Tests use temp dirs, `rpctest`, `imapxtest`; no
  sleeps for synchronization.
- A test stops what it starts: a `live.Mail` it connects gets
  `t.Cleanup(m.Disconnect)` (its Follow Up ticker runs on
  `context.Background()`, so closing the client does not end it), and a
  call it holds is released in a cleanup too, so a failure leaves nothing
  blocked.
- Every write goes through `store.DB.Tx`; emit durable events with `Tx.Emit`
  in the same transaction as the change.
- An update of optional fields reads the row inside the `store.DB.Tx` that
  writes it (`IdentityUpdate`, `SnippetUpdate`): a read before the
  transaction can undo a concurrent change.
- API changes: edit `schema/rpc/*.yaml`, run `make gen`, implement. Changes
  within protocol 1 are additive only. Quote doc strings inside inline YAML
  `{...}` mappings.
- The app never fetches remote content. In the desktop app, message HTML is
  shown only in a web view with JavaScript off, in a page under a CSP that
  allows no script and no fetch (`internal/desktop/mail`, `page.go`;
  [ADR-0005](docs/adr/0005-html-mail-rendering.md),
  [ADR-0063](docs/adr/0063-make-the-nsgo-client-the-only-client.md)).
  Its links open in the system browser, after `internal/desktop/opener`'s
  check, and never make a window.
  In lodge's web build, the reader frame loads the page from lodge's content
  origin instead, under its sandbox policy, which allows no script and no
  same-origin access ([specs/lodge-app.md](docs/specs/lodge-app.md)).
- SQLite databases never live on `/mnt/host` (WAL needs shared memory).
- Downloads in scripts are version-pinned and checksum-verified
  (frostyard/core ADR-0023).

## Repository baseline

The repository follows core's baseline (frostyard/core ADR-0071, its spec
`docs/specs/repository-baseline.md`, rows B1 to B16; [org-adrs.md](docs/org-adrs.md)).
`internal/workflowcheck` fails `make verify` when one of these files breaks:

- The instruction symlinks above (B1). Edit `AGENTS.md`, never a link.
- `.memory/corrections/` (B2, core ADR-0081): 1 file for each correction
  record, named `<date>-<slug>.json` (from a PR: `<date>-pr-<N>-<words>.json`),
  with exactly `date`, `scope`, `correction`, `evidence` and `promoted_to`, in
  canonical form (`JSON.stringify(record, null, 2)` and a newline; core's
  `scripts/new-correction.mjs` writes it). The old single corrections file
  must not come back (the test fails while it exists); its lines are the
  `legacy-` records. A record is permanent: never remove or rename a file,
  and change only an empty `promoted_to` (no test checks this). A fix of a
  blocking review point adds its guard and 1 record file.
- `.github/risk-paths.json` (B6, core ADR-0068): schema version 2
  (core ADR-0078): each glob has a level, `L1` to `L4`, and the default is
  `L3`. The caller `.github/workflows/risk-paths.yml` runs core's
  `pr-triage.yml` (ADR-0077, ADR-0078). It adds `level:L1` to `level:L4`, and
  `risk:high` at L1 and L2. It removes `ready-for-human` when the head moves
  after the label, or when CI is not green at the label. The PR body names
  each matched path and why the change is safe. A new release workflow,
  install script, privileged helper, secrets store or sign-in path joins the
  list in the PR that adds it, at a level by the grading rule: `L1` for the
  risk categories, `L2` for governance. A glob that matches no file fails.
- `.github/dependabot.yml` (B13): 1 entry for each manifest (`go.mod`,
  `app/package.json`, each `Cargo.toml`). A new manifest gets an entry.
  Groups take only minor and patch updates; a major comes as its own PR.
  Each cargo git crate of frostyard or bketelsen has an `ignore` entry
  (Dependabot cannot resolve it; internal bumps go by hand). A gomod PR
  fails the Flatpak sources check until the owner runs
  `make flatpak-sources` on its branch and pushes the result during triage.

`.github/pull_request_template.md` (B7) holds the risk tier. Vulnerabilities
are reported privately ([SECURITY.md](SECURITY.md), B14). No `CODEOWNERS`
file (B15), and no other agent framework's tree (B16): the task-card loop
(`docs/tasks/`, ADR-0008, ADR-0021) is this repository's own.

## Repository boundary

Never commit `build/`, `app/node_modules`, `app/dist`, Rust targets,
credentials or real mail. Test credentials exist only for the
`frostmail-mailtest` container, which never delivers outside
`mailtest.test`. Use conventional commit subjects.

## Documentation

Docs follow core's four-category shape (frostyard/core ADR-0025): `adr/`
for why, `design/` for how, `specs/` for exact contracts, `plans/` for order.
Start new docs from the category's `TEMPLATE.md` and index them in
`docs/README.md`. Record repo-local decisions as ADRs before changing the
design they shape. Frostmail is pre-release: rewrite designs, specs and code
in place when decisions change.

A check's record (a plan's gate or VM check) states what its evidence
shows: the observed times, counts and texts. Never state a limit, a cause
or a pass that the samples do not establish.

A confirmation that a step happened does not close a phase whose "Done
when" asks for more evidence: record the step as done and keep the phase
open. The record that closes an item without its "Done when" evidence
(the plan's phase and status) names each missing piece and who accepted
closing without it. Every other line that marks the item done (a
roadmap row, a status line) names who accepted it and points to that
record. A change that takes a deployment's details (a host, a unit's
state) out of such a record keeps the record itself: each missing piece
and who accepted closing without it.
