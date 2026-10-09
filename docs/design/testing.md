# Design: testing

| Layer | Where | Runs with |
| --- | --- | --- |
| Unit | every `internal/` package; `tools/` | `make test` (in `make verify`) |
| RPC wire | `internal/rpcserver/server_test.go` over real sockets via `internal/rpctest` | `make test` |
| IMAP, fast | `internal/imapx/imapxtest` (go-imap's in-memory server: no CONDSTORE, QRESYNC or Gmail extensions) | `make test` |
| IMAP fork | `third_party/go-imap/imapclient/gmail_test.go` (scripted server) | `make test-fork` |
| IMAP, recorded | `internal/mailsync/replay_test.go`: Gmail and iCloud sessions replayed (below) | `make test` |
| IMAP, real | `*_integration_test.go` (build tag `integration`) against `frostmail-mailtest` | `make engine-it` |
| UI | `app/src/**/*.test.ts` (Vitest) | `make ui-test` (nsl) |
| App shell | the M0 spike report (`FROSTMAIL_SPIKE_EXIT=1`) | `make app-run`, or natively from `build/frostmail-app` |
| Screenshots | `app/e2e/screenshots.e2e.ts`: the built app on `app/e2e/showcase`'s made-up mail (`tools/uifixture -showcase`, maild with `FROSTMAIL_SYNC=off`), written to `docs/images` for the README | `make screenshots` (nsl) |

## CI

`.github/workflows/ci.yml` runs the jobs of `make ci` on every push to
`main`, every pull request and merge-queue entry, and nightly (so upstream
drift shows within a day): **Verify** (`make verify`, with golangci-lint
from `mise.lock`), **Race detector** (`go test -race ./...`), **App UI**
(`make ui-check`) and **App shell** (`make app-build`, `make app-test`).
The app targets normally run in the nsl machine; CI installs the same
Node, pnpm (corepack, from `app/package.json`), Rust and system libraries
as `dev/nsl/provision.sh` and runs them with `IN_NSL='sh -c'`. CI sets
`FROSTMAIL_REQUIRE_DBUS`, so the notification tests fail rather than skip
without `dbus-daemon`. Workflows follow frostyard/core ADR-0021 (actions
pinned to full commit SHAs, `permissions: {}`, checkouts without
persisted credentials), which `internal/workflowcheck` tests. Integration
tests against the mail server, `make ui-e2e` and `make e2e` need incus or
a display and stay local.

## The mail server

`dev/incus/mailtest.sh up` builds `frostmail-mailtest` (Debian 13, Dovecot
2.4.1, Postfix 3.10): users `test1`–`test5@mailtest.test`, password
`frostmail-test`, IMAP 143/993, submission 587, self-signed TLS (tests set
`InsecureSkipVerify`). test1's INBOX holds the five messages in
`dev/incus/seed/`. The `clean` snapshot is restored before every
`make engine-it`. Postfix delivers only to `mailtest.test`; other recipients
are rejected with 550. After login Dovecot advertises CONDSTORE, QRESYNC,
IDLE, MOVE, UIDPLUS, ESEARCH, SPECIAL-USE, LIST-STATUS and NOTIFY.

## Recorded sessions

Gmail and iCloud speak extensions and quirks that neither go-imap's memory
server nor Dovecot imitates, so their paths are also tested against
sessions recorded from the real servers.

1. **Record.** `MAILD_IMAP_TRACE=dir` writes each traced connection (C1,
   which syncs, and C2, which idles) to its own file in `dir` (0700, files
   0600): `C: <line>` for what maild sent and `S: <line>` for what it
   received, after TLS, with LOGIN, AUTHENTICATE and SASL lines replaced by
   `[redacted]` (`internal/imapx/trace.go`). STARTTLS connections are not
   traced. A trace holds the account's mail and never leaves the machine.
2. **Scrub.** `tools/imaprec` turns a trace into a script that can be
   checked in. `-through TAG` keeps the session up to that command;
   `-keep N` keeps the N newest messages each mailbox fetched and rewrites
   the session as a first sync of only those (EXISTS, SEARCH results,
   FETCH sets and message numbers), which makes a big account's first sync
   small, and turns a sync that resumed into a first sync. Every word of
   the mail (names, addresses, subjects, bodies, Message-IDs, boundaries,
   file names, mailbox names and labels) becomes a fake of the same shape,
   derived from a key made for the run, so a word is the same fake
   everywhere: a References header still cites its message, and the
   client's SELECT still names a mailbox from LIST. Dates, MIME types,
   charsets, flags, UIDs and the protocol stay. imaprec refuses to write a
   script in which a replaced word of six or more characters remains.
   Scripts that continue one another (two sessions of one account) share
   their fakes through `-keyfile`, a private key file that is never checked
   in.
3. **Replay.** `internal/imapx/replay` serves a script on a loopback port.
   It answers each command with the first unused recorded exchange for the
   same command, recorded with the same mailbox selected (SELECT, LIST,
   STATUS and the like may come in any order), maps the recorded tags,
   including ESEARCH's `(TAG …)`, to the client's, and keeps a literal's
   lines with their response whatever they look like. It compares FETCH
   and STATUS items as sets, since go-imap orders them differently from
   run to run, and fails the test on a command the script lacks or a
   recorded command never sent.
4. **Test.** `internal/mailsync/replay_test.go` plays the scripts in
   `internal/mailsync/testdata/replay/` to a new account and checks the
   store: messages, roles, labels, threads, flags and the stored mailbox
   state. The first syncs of Gmail and iCloud run one pass; the Gmail
   operations test replays two sessions on one store, calling what the
   actor called in the recorded order (passes, each queued operation's
   replay, a body fetch), so star, archive, junk, not junk, delete and
   restore must send exactly the commands Gmail got.

```sh
go run ./tools/imaprec -through T14 -note "Gmail, first sync" \
  -o internal/mailsync/testdata/replay/gmail-first-sync.txt TRACE
```

## Planned layers

- **Fault injection (M1):** a `faultconn` that drops, delays and truncates;
  a fake clock; kill-during-sync resume tests.
- **MIME corpus (M1):** anonymized real messages (`mailctl scrub`) with golden
  JSON for structure, chosen body, attachments and decoded headers.
- **Sanitizer (M2):** golden tests, `go test -fuzz`, and a Playwright WebKit
  suite asserting zero network requests on hostile HTML.
- **Performance budgets (M2):** 200k seeded messages; list range < 10 ms,
  search < 150 ms, open < 50 ms, maild RSS < 300 MB.
