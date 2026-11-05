# 0049 — Record problems in the local journal

- **Status:** Accepted
- **Date:** 2026-11-05

## Context

A panic of maild, mailctl or the app's shell, and a few failures that a user
cannot fix alone (a database that does not migrate, a secrets store that does
not unseal, a sync that cannot write what the server says, a message that
cannot be parsed, the app losing maild), leave at most a line in a log file
that nobody reads. frostyard/core decided how frostyard apps report problems
([core ADR-0069](https://github.com/frostyard/core/blob/main/docs/adr/0069-report-problems-local-first-opt-in-with-one-sender-per-host.md)):
apps write records to the local systemd journal under 1 `MESSAGE_ID`, with
`FROSTYARD_*` fields from 1 registry
([core spec problem-report](https://github.com/frostyard/core/blob/main/docs/specs/problem-report.md)),
and never send anything. On snosi images chairlift lists the records and
sends a report only with the user's consent. frostyard/std implements the
writer: the Go package `problems` (std v0.4.0) and the Rust crate
`frostyard-problems` (std v0.5.0, in `rust/`).

A record carries the app's release version, which must match the spec's
semver (`1.2.3` or `1.2.3-rc.1`). Development builds name themselves `dev`,
`flatpak` or a `git describe`, and the shell's `tauri.conf.json` keeps
`0.0.0` in git. The Flatpak bind-mounts the journal socket into every
sandbox, so no new permission is needed. The work is plan T857, task t11.

## Decision

- maild (component `daemon`), mailctl (`cli`) and the Tauri shell (`shell`)
  record problems with frostyard/std, under the app ID
  `org.frostyard.Frostmail`. They write only datagrams to the local journal
  socket; they open no network connection for it.
- Panics: maild and mailctl `defer rec.RecoverPanic()` in `main`; maild also
  calls `rec.CrashOutput()` once it holds the data directory's lock, so that a
  crash of any goroutine becomes a record at the next start. The shell calls
  `rec.install_panic_hook(env!("CARGO_MANIFEST_DIR"))` in `main.rs`, so a
  panic's location is a file of the shell's crate.
- Codes, each recorded where the failure is known:
  - `store.migrate_failed`: maild, when `store.Open` fails with
    `store.ErrMigrate` (a migration fails, or the database is newer).
  - `secrets.unseal_failed`: maild, when `secrets.Open` fails with
    `secrets.ErrUnseal` (the sealed store's key is missing, of the wrong size,
    or does not open the file).
  - `sync.apply_failed`: mailsync, when a store write that applies the
    server's state fails (the mailbox list, new headers, flags and labels,
    removals, the sync state of a pass), at most once in each account's streak
    of failed sessions; a healthy pass starts a new streak. Network, sign-in
    and stop errors are never recorded.
  - `mime.parse_failed`: the renderer, when mimex cannot parse a stored
    message for the reader or for a part (`mimex.ErrParse`).
  - `bridge.socket_lost`: the shell, when maild closes its local socket under
    a window or a read from it fails; not when the window connects again or
    closes, and never for lodge's WebSocket, whose losses are the network's.
- A code names the failure, never mail. The error's text goes only into the
  record's `MESSAGE`, which stays in the local journal and never enters a
  report (core's spec).
- Version rule: maild and mailctl strip 1 leading `v` from their ldflags
  version (`v0.4.1` gives `0.4.1`) and change nothing else
  (`internal/problemrec`). The shell uses Tauri's package version, which
  `packaging/flatpak/stamp-version.sh` stamps into `tauri.conf.json` on a
  `v*` tag (Cargo.toml stays `0.0.0`, so Cargo.lock does not change). A build
  that is no release (`dev`, `flatpak`, a snapshot `.deb`, a `git describe`
  past a tag, an unstamped `0.0.0` shell) gets an inert recorder: it records
  nothing rather than claim a version.

## Consequences

- A user on a snosi image sees Frostmail's panics and coded failures in
  chairlift, and can send them with consent; elsewhere they are only in the
  journal (`journalctl --user MESSAGE_ID=8915bdd215c84cbab0c31c05a1a33e7e`).
- Development builds and builds of main record nothing, by design.
- The std library caps records at 10 a minute for each process; a failure
  that repeats fast loses records, which is the intent.
- `MESSAGE` can hold a server's error text, a file path, or for
  `mime.parse_failed` a line of the broken message. It stays on the host, in
  the user's own journal.
- A normal stop of maild while the app is open (an update, a log-out) also
  records `bridge.socket_lost`.
- A new code needs an entry here and a test that reads it back from a test
  socket (`internal/problemstest`, `problems::testing`).

## Alternatives considered

- **Record every error that reaches a log line:** too many records, most of
  them the network's, and codes that say nothing.
- **Our own writer in Go and Rust:** two more copies of a contract that
  std's package and crate already test against core's vectors.
- **Make up a version for development builds (0.0.0):** a record would claim
  a release that does not exist; inert recorders keep reports honest.
- **Send reports from Frostmail:** core ADR-0069 allows 1 sender per host,
  and it is not an app.

## References

- Builds on: [core ADR-0069](https://github.com/frostyard/core/blob/main/docs/adr/0069-report-problems-local-first-opt-in-with-one-sender-per-host.md),
  [core spec problem-report](https://github.com/frostyard/core/blob/main/docs/specs/problem-report.md),
  [ADR-0045](0045-the-desktop-app-builds-as-a-flatpak-in-ci.md) (the stamp),
  [ADR-0044](0044-seal-the-servers-secrets-with-a-systemd-credential.md) (the sealed store)
- Shapes: [design/overview.md](../design/overview.md), the
  [README](../../README.md#problem-records), `internal/problemrec`,
  `app/src-tauri/src/problems.rs`
- Plan: T857 (frostyard's problem reports), task t11
