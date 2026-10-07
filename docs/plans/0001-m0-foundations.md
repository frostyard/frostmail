# Plan 0001: M0 foundations

Goal: everything later milestones stand on, proven on the real hardware
before any mail feature is built. Status: **done except step 7**, which
waits for the executor's inference server.

## Steps and evidence

1. **Repository and environment.** Done. `frostyard/frostmail` with core's
   docs shape, gate triad and agent surface. nsl machine `frostmail`
   provisioned by `dev/nsl/provision.sh` (Node 24.21.0, Go 1.27.1, Rust
   1.99.0, WebKitGTK 2.54.0 `-dev`). incus `frostmail-mailtest`: Dovecot
   2.4.1 and Postfix 3.10, seeded, `clean` snapshot. SMTP submission
   delivers to `mailtest.test`; `someone@example.com` is rejected with 550.
2. **Docs.** Done: ADRs 0001–0008, design and spec docs, `AGENTS.md`,
   `docs/tasks/TEMPLATE.md`, `docs/tasks/EXECUTOR.md`.
3. **Contracts.** Done. `tools/rpcgen` with the protocol-1 schema
   (`rpc.hello`, `events.subscribe`, `account.*`, `mailbox.list`); the store
   with `0001_init.sql`, migrations, the single-writer `Tx` and the changes
   log; the broker; the socket server; maild and mailctl skeletons.
4. **Fork.** Done. go-imap with X-GM-EXT-1 (`make test-fork`). Findings:
   modernc FTS5 works with `remove_diacritics` and `contentless_delete`
   (`TestFTS5Available`); go-imap's in-memory server lacks CONDSTORE and
   QRESYNC; Dovecot passes over IMAPS and STARTTLS with RFC 2047 subjects
   (`TestDovecotCapabilitiesAndSeed`).
5. **Go/no-go spike: GO.** `app/src/Spike.tsx` checks five things and
   reports them to stdout.

   | Run | WebKitGTK | hello | subscribe | event delivery | sandboxed iframe | mailpart |
   | --- | --- | --- | --- | --- | --- | --- |
   | nsl, `tauri dev` | 2.54.0 | pass | pass | (before T-0002) | pass | **fail** (http origin) |
   | nsl, release | 2.54.0 | pass | pass | (before T-0002) | pass | pass |
   | host, release, GPU | 2.52.6 | pass | pass | pass* | pass | pass |
   | host, release, `FROSTMAIL_WEBKIT_SAFE=1` | 2.52.6 | pass | pass | — | pass | pass |

   60 fps at device pixel ratio 2 on the host, with no CSP violations. The
   iframe ran no script and made zero network requests. *Event delivery was
   run against a reference store implementation that was then removed, so
   T-0002 remains real work. The AMD Skia bug did not appear; the window
   should still be checked by eye on this hardware.
6. **Task tooling.** Done. `tools/taskrun` and the `make task`, `accept`,
   `task-verify` and `task-finish` targets; `opencode.json` limits the
   executor to make, go and read-only commands.
7. **Executor calibration.** Waiting. Run T-0001 (CLI), T-0002 (SQL) and
   T-0003 (pure logic) with `make task T=000N`; each card's given tests were
   checked against a reference solution first.

## Done when

`make check` is green on the host and `make ui-test` in nsl (done); the app
shows data from maild (done); the executor passes at least two of the three
calibration cards without help (pending). Then the planner writes the M1
cards.
