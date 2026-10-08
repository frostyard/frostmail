# frostyard/frostmail

Frostmail is a Linux mail client that works like Apple Mail.app without
looking like a GNOME or KDE app. A headless Go daemon, `maild`, owns sync,
storage and the RPC socket. A Tauri v2 app with a React UI and `mailctl`, a
CLI, are clients of that socket. Start at [docs/README.md](docs/README.md);
the architecture entry point is
[docs/design/overview.md](docs/design/overview.md).

This is the canonical agent instruction file. `CLAUDE.md` and `GEMINI.md` are
symlinks to it; edit this file only.

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
  via `tools/rpcgen` (`make gen`); `api.go` and `client.go` are the
  hand-written runtime. Never edit generated files.
- `internal/store`: SQLite (modernc, pure Go): migrations, the single-writer
  `Tx`, the durable `changes` log, queries.
- `internal/events`: fans committed events out to subscribers and replays
  the log. `internal/rpcserver`: the socket server. `internal/engine`: the API
  domains over the store. `internal/rpctest`: a real server for tests.
- `internal/imapx`: the only package that imports go-imap.
  `third_party/go-imap` is a patched fork (Gmail X-GM-EXT-1); see its
  `FROSTMAIL-PATCHES.md` and [ADR-0004](docs/adr/0004-go-imap-fork.md).
- `internal/mimex`: message parsing and normalization helpers.
- `internal/compose`: outgoing mail (reply rules, the message builder).
  `internal/smtpx`: the only package that imports go-smtp.
- `app/`: Tauri v2 + React + TypeScript. `app/src-tauri/src/bridge.rs` only
  moves lines; JSON-RPC multiplexing is in `app/src/rpc/transport.ts`.
  `app/src/rpc/gen/api.ts` is generated.
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
- Fix a lint finding rather than loosening `.golangci.yml`.

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
- The app never fetches remote content. Message HTML is rendered only in a
  `srcdoc` iframe with `sandbox="allow-same-origin"` (never `allow-scripts`)
  under the CSP in `app/src-tauri/tauri.conf.json`
  ([ADR-0005](docs/adr/0005-html-mail-rendering.md)).
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
