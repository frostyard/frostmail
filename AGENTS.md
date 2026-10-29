# frostyard/frostmail

Frostmail is a Linux mail client that works like Apple Mail.app without
looking like a GNOME or KDE app. A headless Go daemon, `maild`, owns sync,
storage and the RPC socket. A Tauri v2 app with a React UI and `mailctl`, a
CLI, are clients of that socket. maild also runs as the `mail` app of
[lodge](https://github.com/frostyard/lodge), frostyard's tailnet web shell,
where the same UI runs in a browser. Start at [docs/README.md](docs/README.md);
the architecture entry point is
[docs/design/overview.md](docs/design/overview.md).

This is the canonical agent instruction file. `CLAUDE.md` and `GEMINI.md` are
symlinks to it; edit this file only.

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
  `scripts/seal-mail-secrets.sh` can make it earlier). The move waits on
  the user.

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
the move note, #135) are merged. Their desktop VM checks ran on 2026-10-28
(plan 0013, VM check 2026-10-28); the notification click of Phase 4 and
Phases 8 to 10 are not built.

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
  the log. `internal/rpcserver`: the socket server, on the kit's
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
- `internal/imapx`: the only package that imports go-imap.
  `third_party/go-imap` is a patched fork (Gmail X-GM-EXT-1); see its
  `FROSTMAIL-PATCHES.md` and [ADR-0004](docs/adr/0004-go-imap-fork.md).
- `internal/mimex`: message parsing and normalization helpers.
- `internal/compose`: outgoing mail (reply rules, the message builder).
  `internal/smtpx`: the only package that imports go-smtp.
- `app/`: Tauri v2 + React + TypeScript. `app/src-tauri/src/bridge.rs` only
  moves lines, to the local socket or, in remote mode, to lodge
  (`remote.rs`); JSON-RPC multiplexing is in `app/src/rpc/transport.ts`.
  `app/src/rpc/gen/api.ts` is generated. `app/src/lodge/` runs the UI in
  lodge's frame (routes, windows as routes), over
  `app/src/rpc/transport/lodge.ts`.
- `packaging/flatpak`: the app's Flatpak manifest, its metainfo and desktop
  entry, and the offline `cargo-sources.json`, `node-sources.json` and
  `go-sources.json` (`make flatpak-sources`; [ADR-0045](docs/adr/0045-the-desktop-app-builds-as-a-flatpak-in-ci.md)).
  The Flatpak holds the app, maild and mailctl (`/app/bin`, built from
  `go-sources.json`), the launcher `frostmail.sh`, the Background helper and
  a smoke test ([ADR-0046](docs/adr/0046-the-desktop-flatpak-carries-its-own-maild.md)).
  CI builds it in `.github/workflows/flatpak.yml`. `install.sh` is the
  wrapper for the development bundle, with a host maild.
- `dev/`: the nsl machine (`dev/nsl/provision.sh`) and the incus test mail
  server (`dev/incus/mailtest.sh`).
- `tools/rpcgen`, `tools/taskrun`: the contract generator and the task-card
  runner.

## Gates

- `make check` formats, then runs `make verify`: tidy, `gen-check`, vet,
  gofmt, the pinned golangci-lint (`mise install`), unit tests, and the fork's
  tests. Run it before claiming any Go change is complete.
- `make engine-it` runs integration tests against the Dovecot container
  (resetting it first). `make ui-test` and `make app-build` run in the nsl
  machine. `make ci` is verify plus race tests and the app checks.
- `make flatpak-sources` regenerates the 3 sources files of the Flatpak. Run
  it after any change to `app/src-tauri/Cargo.lock`, `app/pnpm-lock.yaml`,
  `go.mod` or `go.sum`, in the nsl machine (it needs Python 3 with venv and
  Go, not Node or Rust); CI's job
  "Flatpak sources (make flatpak-sources)" fails on a stale file.
- `make vulncheck` runs govulncheck (pinned in the Makefile; needs the
  network). It is not part of `check`, `verify` or `ci`; the CI job
  "Vulnerabilities (make vulncheck)" runs it. Keep it at 0 reachable findings.
- Fix a lint finding rather than loosening `.golangci.yml`.

## Releases and dependencies

- Tags look like `v1.2.3` (`.svu.yml`: prefix `v`, `v0: true`). The next version is
  `svu next`, from Conventional Commits; on v0 a breaking change gives a minor.
- A pushed `v*` tag runs `.github/workflows/release.yml`: it checks the tag is
  `vX.Y.Z`, builds the web UI and `frostyard-maild_X.Y.Z_amd64.deb` (`make deb`, nfpm),
  attests it, and makes the GitHub release with `gh release create` (generated notes).
- Brian tags. Agents never tag, push tags or release. `make bump` (on a clean, current
  `main`, after `make verify`) tags `svu next` and pushes the tag; Brian runs it.
- Internal Go modules (`go.mod`): `github.com/frostyard/clix` v0.5.0,
  `github.com/frostyard/lodge/kit` v0.3.1, `github.com/frostyard/std` v0.3.0 (indirect, from clix).
- Internal UI packages (`app/package.json`, github tags): `@frostyard/lodge-client`
  `client/v0.3.2` and `@frostyard/lodge-ui` `ui/v0.6.0`.
- The generated Flatpak sources (`packaging/flatpak/go-sources.json` and
  `node-sources.json`, from `make flatpak-sources`) mirror the Go and UI versions above
  and change with every bump. `mise.toml`, the other `packaging/` files, the workflows,
  `scripts/`, `cargo-sources.json` and `app/src-tauri/Cargo.toml` pin no frostyard or
  bketelsen module; they only cite frostyard/core ADRs.
- Users: lodge hosts maild as its `mail` app, and Porter calls `/mcp/mail`. Other users:
  not stated.

## Conventions

- Go: `ctx` first; errors are lowercase, wrapped with `%w`; `slog` for logs;
  `encoding/json/v2`; no mutable package state; modern idioms (`t.Context()`,
  `wg.Go`, `omitzero`). Tests use temp dirs, `rpctest`, `imapxtest`; no
  sleeps for synchronization.
- Every write goes through `store.DB.Tx`; emit durable events with `Tx.Emit`
  in the same transaction as the change.
- API changes: edit `schema/rpc/*.yaml`, run `make gen`, implement. Changes
  within protocol 1 are additive only. Quote doc strings inside inline YAML
  `{...}` mappings.
- The app never fetches remote content. In the desktop app, message HTML is
  rendered only in a `srcdoc` iframe with
  `sandbox="allow-same-origin allow-popups"` (never `allow-scripts`) under
  the CSP in `app/src-tauri/tauri.conf.json`
  ([ADR-0005](docs/adr/0005-html-mail-rendering.md)). Its links open in the
  system browser through the shell's new-window handler, which never makes a
  window ([ADR-0036](docs/adr/0036-reader-links-open-as-new-window-requests.md)).
  In lodge's web build, the reader frame loads the page from lodge's content
  origin instead, under its sandbox policy, which allows no script and no
  same-origin access ([specs/lodge-app.md](docs/specs/lodge-app.md)).
- SQLite databases never live on `/mnt/host` (WAL needs shared memory).
- Downloads in scripts are version-pinned and checksum-verified
  (frostyard/core ADR-0023).

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
