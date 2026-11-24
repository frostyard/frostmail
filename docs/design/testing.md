# Design: testing

| Layer | Where | Runs with |
| --- | --- | --- |
| Unit | every `internal/` package; `tools/` | `make test` (in `make verify`) |
| RPC wire | `internal/rpcserver/server_test.go` over real sockets via `internal/rpctest` | `make test` |
| lodge's app socket | `internal/lodge/*_test.go`: lodge/kit's `kittest` plays lodge against `maild --lodge`'s handler on a real socket, checking each rule of [lodge-app.md](../specs/lodge-app.md): the peer check, each path's caller kinds and headers, `/health` with its badge (the inboxes' unread count, `TestHealthBadge`), an `/api` session, the content route's pages, parts and messages with their headers, and `/mcp` through the MCP SDK's client, its tools, limits and scopes. Linux only: they skip elsewhere, as lodge does | `make test` |
| Web build | `app/src/lodge/*.test.ts(x)` and the lodge cases of `app/src/app/*.test.tsx` (routes, the shell's history, windows as routes, the narrow layout, Save As, permalinks, notifications) in Vitest, with a fake lodge-client connection; `make web-build` builds the bundle for `/app/mail/`. In lodge's development build, `dev/lodge/run.sh ROOT fixture` serves generated mail to check it in a browser (plan 0011) | `make ui-test`, `make web-build` (nsl) |
| IMAP, fast | `internal/imapx/imapxtest` (go-imap's in-memory server: no CONDSTORE, QRESYNC or Gmail extensions) | `make test` |
| IMAP fork | `third_party/go-imap/imapclient/gmail_test.go` (scripted server) | `make test-fork` |
| IMAP, recorded | `internal/mailsync/replay_test.go`: Gmail and iCloud sessions replayed (below) | `make test` |
| IMAP, real | `*_integration_test.go` (build tag `integration`) against `frostmail-mailtest` | `make engine-it` |
| DAV and Tasks, recorded | `internal/pimsync/replay_test.go`: Google's and iCloud's CardDAV, CalDAV and Tasks sessions replayed (below) | `make test` |
| UI | `app/src/**/*.test.ts` (Vitest) | `make ui-test` (nsl) |
| Desktop app | `internal/desktop/**` with `ui.Tester` against a real maild. `mainwin`'s `TestModulesInBothThemes` draws Mail, Calendar, People and Tasks in Frostyard and Frostyard light; with `FROSTMAIL_SNAPSHOT_DIR` set, it and the other snapshot tests write their PNGs there | `make test` |
| NSGo client | `cmd/frostmail-nsgo/*_test.go` (build tags `linux` and `integration`): NSGo's `nsgotest` drives the shared client against a real maild on `tools/uifixture`'s showcase, by label, text and test id. 24 workflows (`workspace_test.go`) and 9 picture tours (`pictures_test.go`) | `make nsgo-test`, headless (no pixels) |
| NSGo client, real window | The same suite in a GTK 4 window with the platform's own input (`TestMain` is `nsgotest.Main`). The tours' `app.Screenshot` calls are PNGs there. No workflow skips on a target | `make nsgo-live`, under Xvfb |
| Desktop app, real windows | `internal/desktop/e2e` (build tag `e2e`): the automation build on fixture mail: the hostile corpus, the reader's script probe (`TestReaderPageRunsNoScript`: 22 ways to run script, counted by the find bar and a canary, once past maild's sanitizer and once through it), a real link click, a forward with an inline image, dragging an event, the reminders window, and on Linux the Server pane's change of mode | `make desktop-e2e`, under Xvfb with xdotool, or on Windows' desktop |

## The NSGo client's live suite

The shared client has one suite, run two ways
([plan 0016](../plans/0016-a-shared-look-for-the-nsgo-apps.md), rule 10).

- **Headless,** `make nsgo-test`: `go test -tags integration` in
  `cmd/frostmail-nsgo`. NSGo models the views, with approximate text and no
  pixels: `app.Screenshot` only logs.
- **In a real window,** `make nsgo-live`: `go tool nsgo test -target linux`
  under `xvfb-run`, each test in a process of its own, in a GTK 4 window
  with WebKitGTK for the message's page. It needs the `xvfb` package, GTK 4
  and WebKitGTK 6. The target has no `-tags` flag, so the Makefile passes
  the tag in `GOFLAGS`. It sets `GDK_BACKEND=x11`, which is what keeps the
  windows on Xvfb: GTK tries Wayland first, and with `WAYLAND_DISPLAY`
  unset it still finds a desktop's compositor at the default socket,
  `wayland-0`. Until 2026-11-24 the Makefile only unset the variable, so on
  a Wayland desktop the windows opened on that desktop, with its GTK
  settings and its title bar; a runner has no compositor, so CI's runs were
  on Xvfb throughout. GTK also reads `testdata/xdg`'s settings, which stop
  the caret's blinking. On a target the tests' working directory is under
  `cmd/frostmail-nsgo/.nsgo/`, so they find the repository with
  `repoRoot`. Pictures go to `cmd/frostmail-nsgo/.nsgo/test/screenshots/linux`,
  which git ignores; CI uploads them as `nsgo-live-pictures`.
- **The kept pictures,** `cmd/frostmail-nsgo/comparison/linux`: 31 PNGs.
  25 are the main views in Frostyard light (3 of Settings' categories, an account's panel and a mailbox's, the list, the To-Do Bar
  beside it, a row under the pointer, a message, its More menu, a
  confirmation, several rows chosen, a search with no match, the reader
  with an invitation, with a file and with a conversation's banner, a conversation, compose,
  the five modules, and the narrow list and message), and 6, named
  `dark-`, are a message, its actions menu, a confirmation, the Calendar,
  compose and the narrow message in Frostyard. A tour names its appearance
  (`fixtureLook`), so a run does not follow its host's. They are 1280 by
  820 and 620 by 820, taken on Xvfb's X11.
  (The first set, of 2026-11-24, was 781 high and in the desktop's font:
  it was taken on a Wayland desktop, as above.) `make nsgo-pictures` takes
  them again. A change of
  the look commits its changed pictures. No test compares pixels: the
  fixture's dates follow the day it is generated, so the list's dates and
  the Calendar differ from day to day, and fonts differ between hosts.
- **What the real window found** (2 workflows skipped on a target from
  2026-11-24 until plan 0016's Phase 3b, the same day):
  - `TestMessageRowQuickActions` (frostyard/frostmail#391): no quick
    action showed under the pointer. The view that listened for the
    pointer was 0 points high: it had a flex basis of 0 in a column, and
    its content stood outside it (346 by 0 in the headless tree). Why
    the headless run passed was not looked into. The quick actions now follow the list row's own hover state, which a
    target does not report, so the test checks it headless and the tour
    keeps a picture of it (`mail-row-hover`).
  - `TestDragMessageToMailbox` (frostyard/frostmail#392): the drop was
    made, and the same release also pressed the mailbox's row, so the
    list showed that mailbox, with the message in it. The client now
    passes over a press of the navigation in the drop's own turn of the
    loop, and the test looks for the message in the list, not in the
    window.
  Neither was tried with a person's mouse on a desktop.
- **A blank picture.** With a message open, the picture of the window is
  blank from about a second after the To-Do Bar opens beside it (seen
  2026-11-24 on Xvfb's X11, on main at 65637f05 and with plan 0016's
  Phase 3a). A picture taken sooner shows the window, and so does one of
  the bar beside the list with no message open, which is what the tour
  keeps. The view tree is whole while the picture is blank. Not tried on
  a desktop, and the cause is not found.
- **The web target** is not run. The test files are Linux-only, and each
  test starts a maild with `os/exec`, which a browser cannot. Lodge's
  browser path stays with `dev/lodge/nsgo-smoke.py`.

## CI

`.github/workflows/ci.yml` runs the jobs of `make ci` on every push to
`main`, every pull request and merge-queue entry, and nightly (so upstream
drift shows within a day): **Verify** (`make verify`, with golangci-lint
from `mise.lock`), **Race detector** (`make race`, in shards; see below), **App UI**
(`make ui-check`), **NSGo client** (`make nsgo-test nsgo-build`, then
`make nsgo-live` under Xvfb, whose pictures it uploads) and **Desktop app**
(Linux and Windows release builds, then `make desktop-e2e` on Linux),
**Windows** (`go test ./...` on Windows) and **Windows app** (the
installer and native app real-window checks; see below).
The app targets normally run in the nsl machine; CI installs the same
Node, pnpm (corepack, from `app/package.json`), Rust and system libraries
as `dev/nsl/provision.sh` and runs them with `IN_NSL='sh -c'`. CI sets
`FROSTMAIL_REQUIRE_DBUS`, so the notification tests fail rather than skip
without `dbus-daemon`. Workflows follow frostyard/core ADR-0021 (actions
pinned to full commit SHAs, `permissions: {}`, checkouts without
persisted credentials), which `internal/workflowcheck` tests. Integration
tests against the mail server and `make e2e` need incus or a display and
stay local.

### The race gate

`make race` runs `go test -race` on every package of `go list ./...`, and
`make ci` runs it as one command. CI splits it over 4 runners, since one
runner took 9 minutes for it (below):

- Each shard job runs `make race` with `RACE_SHARD=I/4` and
  `RACE_FLAGS=-count=1`. `tools/testshard` deals the packages into the
  shards: the heaviest first, each to the shard that is lightest so far.
  The 4 shards together hold every package of `go list ./...` exactly
  once. A package that `tools/testshard/weights.txt` does not name still
  lands in a shard, with a weight of 1.
- `weights.txt` holds the seconds each slow package took under the race
  detector on CI's runner. The weights only balance the shards. When one
  shard takes much longer than the others, refresh them from the `ok`
  lines of the shard jobs' logs.
- `-count=1` makes every test run: no result comes from a cache.
- The job named **Race detector**, which main's rulesets require, needs
  the shards, runs whatever their results (`if: always()`) and passes only
  when every shard passed. Without `always()` a failed shard would leave
  it skipped, and a skipped required check counts as passed.
  `internal/workflowcheck` holds this shape
  (`TestRaceDetectorCheckNeedsEveryShard`).
- Each shard keeps the race build of the standard library and the
  dependencies in a build cache of its own (`GOCACHE` in the runner's
  temporary directory, saved with `actions/cache`). The cache that
  setup-go restores has no race build: every Linux job shares its key, and
  the first job to finish saves it, which is govulncheck's. Only a run on
  `main` saves a shard's cache, when `go.mod` or `go.sum` changed; pull
  requests restore main's. The step writes today's time to the cache's
  `trim.txt` first: go trims entries unused for 5 days at the end of a
  build, at most once a day, and the first build of the step is
  `tools/testshard`'s, which would empty an older restored cache before
  the tests use it.

Where the time went, in the one job that ran every package (CI run
45427387961, 2026-11-24): the job took 557 seconds, of which
`go test -race ./...` took 526, and checkout, setup-go and the Pango
install 26. The packages' test times add up to 1535 seconds; the same
packages took 270 seconds without the race detector in that run's Verify
job. The slowest were `internal/desktop/pim` (240 s),
`internal/desktop/mainwin` (194 s), `internal/store` (166 s),
`internal/mailsync` (144 s) and `internal/desktop/settings` (121 s).

CPU profiles of 7 of them under the race detector on a developer's
machine, the same day (`pim`, `mainwin`, `store`, `mailsync`, `engine`,
`pimsync` and `cmd/mailctl`): the CPU samples were 95% to 122% of the wall
time, so these tests compute and do not wait. `store.Open`, which applies
the 23 migrations to each test's new database, took 25% to 63% of the
samples. `-race` also turns on the compiler's `checkptr` checks, which are
costly in SQLite's translated C (`modernc.org/sqlite`):
`runtime.checkptrBase` alone took 21% to 38%.

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
- The notifier (`app/notify`) is type-checked for Windows by `make
  app-check-windows` in the nsl machine; its tests run with `make
  app-test` in the nsl machine, its toast XML, activation URIs and RPC
  calls being the same on Linux.
- CI's **Windows app** job tests and builds the notifier, builds the
  installer (`scripts/windows-installer.sh`), which it uploads as the
  `frostmail-windows-installer` artifact (kept 14 days), installs and
  uninstalls it silently, and runs `internal/desktop/e2e` on WebView2 in the
  runner's interactive session. Installs and trials take that installer to
  the VM or one of the user's machines.
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

## README screenshots

The README's pictures (`docs/images`, which the Flatpak's metainfo also
names) are the native app in real windows on the showcase, made-up mail,
contacts, calendars and tasks (`tools/uifixture/showcase`). No make target
takes them yet: plan 0015 lists `make screenshots` as not done. Neither
`ui.Tester` nor the automation plugin's `capture` draws the reader's web
view, so the pictures come from the desktop's compositor. They were taken
on 2026-11-11 like this:

1. `go run ./tools/uifixture -out DATA -showcase tools/uifixture/showcase`,
   then maild on `DATA` with `FROSTMAIL_SYNC=off` and
   `FROSTMAIL_SECRETS=file`, and the automation build of the app
   (`go build -tags automation ./cmd/frostmail`), each picture on a fresh
   copy, with the e2e harness's private paths (`isolated` in
   `internal/desktop/e2e/harness_test.go`).
2. The app runs on a private headless GNOME Shell (`gnome-shell --headless
   --wayland --no-x11 --virtual-monitor 1600x1100` under `dbus-run-session`,
   with its own `HOME` and `XDG_RUNTIME_DIR`), out of the overview (the
   Shell's `OverviewActive` property set to false). The Shell's
   `org.gnome.desktop.interface` `color-scheme` picks the theme:
   `prefer-dark` for Frostyard, `prefer-light` for Frostyard light, which
   also takes `GTK_THEME=Adwaita` because Snow's GTK 3 theme is dark only
   and would draw the window buttons white.
3. `mygo-automation` (`github.com/egoist/mygo/plugins/automation/cmd/mygo-automation`,
   at go.mod's MyGo) drives each scene, then moves the pointer off the
   rows it clicked, so that none shows its hover actions.
4. The Shell's `org.gnome.Shell.Screenshot.ScreenshotWindow` takes the
   focused window. The Shell answers only known callers, so the caller
   owns `org.gnome.Screenshot` on the private bus. The picture is cropped
   to the window's opaque frame (1200 × 780 for the main window), without
   its shadow, and recompressed losslessly.

## Planned layers

- **Fault injection (M1):** a `faultconn` that drops, delays and truncates;
  a fake clock; kill-during-sync resume tests.
- **MIME corpus (M1):** anonymized real messages (`mailctl scrub`) with golden
  JSON for structure, chosen body, attachments and decoded headers.
- **Sanitizer (M2):** golden tests, `go test -fuzz`, and a Playwright WebKit
  suite asserting zero network requests on hostile HTML.
- **Performance budgets (M2):** 200k seeded messages; list range < 10 ms,
  search < 150 ms, open < 50 ms, maild RSS < 300 MB.
