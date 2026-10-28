# Design: testing

| Layer | Where | Runs with |
| --- | --- | --- |
| Unit | every `internal/` package; `tools/` | `make test` (in `make verify`) |
| RPC wire | `internal/rpcserver/server_test.go` over real sockets via `internal/rpctest` | `make test` |
| lodge's app socket | `internal/lodge/*_test.go`: lodge/kit's `kittest` plays lodge against `maild --lodge`'s handler on a real socket, checking each rule of [lodge-app.md](../specs/lodge-app.md): the peer check, each path's caller kinds and headers, `/health`, an `/api` session, the content route's pages, parts and messages with their headers, and `/mcp` through the MCP SDK's client, its tools, limits and scopes. Linux only: they skip elsewhere, as lodge does | `make test` |
| Web build | `app/src/lodge/*.test.ts(x)` and the lodge cases of `app/src/app/*.test.tsx` (routes, the shell's history, windows as routes, the narrow layout, Save As, permalinks, notifications) in Vitest, with a fake lodge-client connection; `make web-build` builds the bundle for `/app/mail/`. In lodge's development build, `dev/lodge/run.sh ROOT fixture` serves generated mail to check it in a browser (plan 0011) | `make ui-test`, `make web-build` (nsl) |
| IMAP, fast | `internal/imapx/imapxtest` (go-imap's in-memory server: no CONDSTORE, QRESYNC or Gmail extensions) | `make test` |
| IMAP fork | `third_party/go-imap/imapclient/gmail_test.go` (scripted server) | `make test-fork` |
| IMAP, recorded | `internal/mailsync/replay_test.go`: Gmail and iCloud sessions replayed (below) | `make test` |
| IMAP, real | `*_integration_test.go` (build tag `integration`) against `frostmail-mailtest` | `make engine-it` |
| DAV and Tasks, recorded | `internal/pimsync/replay_test.go`: Google's and iCloud's CardDAV, CalDAV and Tasks sessions replayed (below) | `make test` |
| UI | `app/src/**/*.test.ts` (Vitest) | `make ui-test` (nsl) |
| App shell | the M0 spike report (`FROSTMAIL_SPIKE_EXIT=1`) | `make app-run`, or natively from `build/frostmail-app` |
| Desktop app behind lodge | `app/e2e/lodge.e2e.ts`: lodge's development build, `maild --lodge` and the built app with `app.json` naming lodge (skips without `FROSTMAIL_LODGE_E2E`) | after `make ui-e2e`'s builds, in the nsl machine with `FROSTMAIL_LODGE_E2E` naming a copy of lodge's `make e2e-build` output it can read (under this repository's `build/`), `pnpm exec vitest run -c vitest.e2e.config.ts lodge` under `xvfb-run` |
| Change of mode (M6) | `app/e2e/mode.e2e.ts`: the built app with `FLATPAK_ID` set, stand-ins for `mailctl` and `frostmail --start-maild` on its PATH: local to lodge (maild stops, the data stays), back to local with lodge unreachable (the note, the same accounts), and a maild that will not stop (the mode stays local, the pane's error) | CI's App shell job, under `dbus-run-session` and `xvfb-run`; `make ui-e2e F=mode` in the nsl machine |
| Screenshots | `app/e2e/screenshots.e2e.ts`: the built app on `app/e2e/showcase`'s made-up mail (`tools/uifixture -showcase`, maild with `FROSTMAIL_SYNC=off`), written to `docs/images` for the README | `make screenshots` (nsl) |

## CI

`.github/workflows/ci.yml` runs the jobs of `make ci` on every push to
`main`, every pull request and merge-queue entry, and nightly (so upstream
drift shows within a day): **Verify** (`make verify`, with golangci-lint
from `mise.lock`), **Race detector** (`go test -race ./...`), **App UI**
(`make ui-check`) and **App shell** (`make app-build`, `make app-test`),
**Windows** (`go test ./...` on Windows) and **Windows app** (the
installer; see below).
The app targets normally run in the nsl machine; CI installs the same
Node, pnpm (corepack, from `app/package.json`), Rust and system libraries
as `dev/nsl/provision.sh` and runs them with `IN_NSL='sh -c'`. CI sets
`FROSTMAIL_REQUIRE_DBUS`, so the notification tests fail rather than skip
without `dbus-daemon`. Workflows follow frostyard/core ADR-0021 (actions
pinned to full commit SHAs, `permissions: {}`, checkouts without
persisted credentials), which `internal/workflowcheck` tests. Integration
tests against the mail server, `make ui-e2e` and `make e2e` need incus or
a display and stay local.

## Windows

maild and mailctl run on Windows (ADR-0038), so the Go suite runs there
too:

- `make verify` vets the module for Windows (`GOOS=windows go vet ./...`),
  which compiles every Windows file and test.
- CI's Windows job runs `go test ./...` on `windows-2025`, after turning
  off `core.autocrlf`, since fixtures and golden files are byte-exact.
- On the developer's machine, an incus Windows VM reached over SSH stands
  in: `dev/windows/vm.sh setup` installs the pinned Go into it, `sync`
  copies the worktree, and `go test ./...` runs the suite there. Its work
  directory (`C:\fm`) is outside the Windows user's profile.
- The app shell and the notifier (`app/notify`) are type-checked for
  Windows by `make app-check-windows` in the nsl machine, without the
  sidecars the bundle takes; the notifier's tests run with `make
  app-test`, its toast XML, activation URIs and RPC calls being the same
  on Linux.
- CI's **Windows app** job builds `maild.exe` and `mailctl.exe`, tests
  and builds the notifier (its C runtime linked in), runs the shell's
  tests, and builds the NSIS installer with `pnpm tauri build --bundles
  nsis`, which it uploads as the `frostmail-windows-installer` artifact
  (kept 14 days). It then runs hostile, links, smoke and remote on that
  app in WebView2: the harness starts the app with a DevTools port
  (`FROSTMAIL_DEVTOOLS_PORT`, which the app adds to every window's
  WebView2 arguments, since WebView2 lets an app's arguments override
  `WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS`) and attaches the runner's
  msedgedriver, which matches the runner's WebView2, to it
  (`debuggerAddress`); a session msedgedriver launched would look for that
  port in a data folder of its own. It also runs the window chrome and the
  keyboard map (`app/e2e/window.e2e.ts`). The harness waits up to 45 s for
  the driver to answer and tries a second time on a new port, because the
  first msedgedriver start on a fresh runner can take more than 20 s.
  `FROSTMAIL_OPEN_LOG` stands in for the Linux opener stand-ins. Installs
  and trials take that installer to the VM or one of the user's machines.
- Tests that need a socket take one from `rpctest.SocketPath`: a short
  private path on Linux, a fresh named pipe on Windows. Contract tests
  that check Unix file modes (the blob store, the plain secrets file) or
  Maildir names (`tools/mailgen`) are skipped on Windows by a `TestMain`
  in a `_windows_test.go` file, which leaves the contract tests as they
  are; the DPAPI store has Windows tests of its own.

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

## DAV and Tasks recordings

Google's and iCloud's CardDAV and CalDAV, and the Google Tasks API, have
quirks `davtest` and Radicale don't imitate, so pimsync is also tested
against their recorded sessions (plan 0007, Phase 5).

1. **Record.** `MAILD_DAV_TRACE=dir` makes maild record every request
   pimsync sends and the answer it gets to `dir/dav-<time>.trace` (0700,
   the file 0600), one JSON exchange per line (`internal/httprec`): the
   method, URL, headers and body of each. `Authorization`,
   `Proxy-Authorization`, `Cookie` and `Set-Cookie` values become
   `[redacted]`, and a binary body is base64. OAuth's token requests use
   their own client and are never recorded. A trace holds the account's
   contacts, calendars and tasks and never leaves the machine.
2. **Scrub.** `tools/davrec` turns a trace into one that can be checked
   in. Every personal value (names, addresses, phone numbers, postal
   addresses, notes, event and task text, places, people, UIDs,
   collection names, Google task list IDs, and the account's address
   wherever it appears, URLs included) becomes a fake of the same shape,
   keyed per run as imaprec's are, and photos become a one-pixel image.
   Dates, IANA time zone names and a format's own words (XML element and
   attribute names, vCard and iCalendar property and parameter names, JSON
   keys) are kept, so request bodies replay byte for byte. It refuses to write a trace
   in which a replaced value remains, and writes the account's fake
   address as a `# account` comment for the replay test's account.
3. **Replay.** `httprec.Serve` answers a client on a loopback port with
   the first unused exchange for the same method and path (PROPFIND and
   REPORT bodies must match too; writes' bodies hold timestamps and are
   not compared), standing in for every host of the session, with the
   recorded origins in answers rewritten to its own URL. A request the
   trace lacks fails the test, and so does an exchange never asked for.
4. **Test.** `internal/pimsync/replay_test.go` replays the traces in
   `internal/pimsync/testdata/replay/` to a new account and checks the
   store: collections, contacts, events and their instances, tasks.

**Write sessions** are scripted, so the replay can send exactly what the
recording did. `internal/davsession` holds the scripts: each step is one
write through the engine and the pass that sends it. `tools/davsession`
runs one against the provider as one of maild's accounts: it reads the
account's credentials from a snapshot of maild's database and the
keyring, syncs into a new store of its own (so the trace starts with a
first sync), stops pimsync's clock at the start, and writes the raw
trace for `davrec`, with the start as a `# recorded` line. The replay test
runs the same script on that clock. The engine's `Deps.NewUID` gives new
objects the names the recording's creating PUTs used (`If-None-Match: *`),
in order; Graph's creations are POSTs to the collection, whose paths name
none. The script finds what it made again by what scrubbing keeps,
times and IDs, never by summaries or names. `TestGoogleSessionOnDavtest`
and `TestMicrosoftSessionOnTheFake` run each script against `davtest` or
`graphtest` first, so a broken script fails before it reaches a
provider. Only the throwaway Gmail account and the user's Outlook.com
test account are recorded: the repository is public, and a scrubbed
trace keeps dates and counts. A dev maild that keeps its secrets in a
file (`FROSTMAIL_SECRETS=file`) lends `tools/davsession` its sign-in. A
server's sealed store (`FROSTMAIL_SECRETS=sealed`) is tested with a key in
a temporary `$CREDENTIALS_DIRECTORY` (`internal/secrets/sealed_test.go`),
and the scripts that seal and move it with a staged root and stub
commands (`scripts/seal_test.go`, `scripts/move_test.go`).

## Planned layers

- **Fault injection (M1):** a `faultconn` that drops, delays and truncates;
  a fake clock; kill-during-sync resume tests.
- **MIME corpus (M1):** anonymized real messages (`mailctl scrub`) with golden
  JSON for structure, chosen body, attachments and decoded headers.
- **Sanitizer (M2):** golden tests, `go test -fuzz`, and a Playwright WebKit
  suite asserting zero network requests on hostile HTML.
- **Performance budgets (M2):** 200k seeded messages; list range < 10 ms,
  search < 150 ms, open < 50 ms, maild RSS < 300 MB.
