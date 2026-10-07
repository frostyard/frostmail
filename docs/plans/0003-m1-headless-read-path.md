# Plan 0003: M1 headless read path

M1 makes maild a working mail engine for one generic IMAP account, driven
entirely from `mailctl`, before any UI exists. It follows
[M0](0001-m0-foundations.md) and comes before the UI read path (M2) in the
[roadmap](0002-roadmap.md). Work is split per
[ADR-0008](../adr/0008-local-executor-workflow.md): the planner owns
contracts and the sync core; the executor owns cards T-0004 onward.

## Decisions taken for M1

- **CONDSTORE + ESEARCH, not QRESYNC.** The go-imap client has no QRESYNC
  (no SELECT parameter, no VANISHED parsing). Gmail and Outlook lack QRESYNC
  anyway, so every provider needs the CONDSTORE path; QRESYNC becomes an
  optional fork patch later. Expunges are found by comparing ESEARCH
  `UID SEARCH RETURN (ALL) ALL` with local UIDs, only when counts disagree.
- **One reconcile pass per mailbox.** Fetch headers for server UIDs missing
  locally (newest first, chunks of 500), delete local UIDs missing on the
  server, fetch flags `CHANGEDSINCE` the stored modseq. Inserts are
  idempotent on `(mailbox, uid)`, so a pass killed at any point resumes by
  running again. A fast path skips the pass when UIDVALIDITY, UIDNEXT,
  HIGHESTMODSEQ and the count are unchanged ([sync.md](../design/sync.md)).
- **Two connections per account:** C1 for commands, polling and body fetches;
  C2 IDLEs on INBOX and wakes the actor on any unsolicited response. Other
  mailboxes are checked with STATUS on a poll interval (5 minutes by default,
  configurable for tests).
- **Bodies on demand.** Headers, BODYSTRUCTURE and a preview (BINARY partial
  fetch when available, else BODY partial plus local decoding) arrive during
  sync. The full message is fetched into the blob store when it is opened.
- **Credentials** go to a file secret store (`secrets.json`, 0600) behind an
  interface that Secret Service replaces in M4. Over RPC they are write-only
  (`account.setPassword`).
- **Views are per connection.** `view.open` snapshots ordered message IDs;
  maild recomputes the snapshot after relevant changes and sends `view.delta`
  only to the connection that opened the view. Rows are messages in M1;
  thread grouping is an additive option in M2.

## Phase 1 — Contracts (planner)

- Schema (protocol 1, additive): `account.setPassword`; a `sync` domain
  (`status`, `now`, transient `progress` events); a `message` domain (`get`,
  `body`, `setFlags`, `move`, `delete`; durable `changed` and `removed`
  events); a `view` domain (`open`, `range`, `close`; transient `delta`).
- Store, secrets, blob and mimex signatures with doc comments, as stubs, plus
  contract tests checked against reference solutions.
- Cards T-0004 to T-0014 in `docs/tasks/todo`.
- **Done when:** `make check` is green with every stub in place and each card
  has passed its reference solution.

## Phase 2 — Executor batch 1 (runs alongside Phase 3)

| Card | Package | What |
| --- | --- | --- |
| T-0004 | `internal/secrets` | File store: Get, Set, Delete; 0600, atomic writes |
| T-0005 | `internal/blob` | Content-addressed blob store: Put, Open, Has, Remove |
| T-0006 | `internal/mimex` | `ParseMessageIDs` for References and In-Reply-To |
| T-0007 | `internal/mimex` | `HTMLToText` for previews and search |
| T-0008 | `internal/mimex` | `Preview`: first lines without quotes or signatures |
| T-0009 | `internal/mimex` | `DecodePart`: transfer encoding and charset to UTF-8, tolerant of truncation |
| T-0010 | `internal/view` | `Diff`: insert and remove ops that turn one ID list into another |
| T-0011 | `internal/store` | Full-text index: index, remove, search, and the query builder |
| T-0012 | `internal/store` | Mailboxes: upsert from LIST, list, delete, sync state |
| T-0013 | `internal/store` | Messages: idempotent header insert, UID sets, flag updates, removal |
| T-0014 | `cmd/mailctl` | `account add`, `account list`, `account rm` |

- **Done when:** every card is merged and `make check` is green.

## Phase 3 — Sync core (planner)

- `internal/imapx`: a session wrapper (capabilities, ENABLE CONDSTORE, ID,
  LIST with SPECIAL-USE and role fallback, ESEARCH, STATUS, IDLE with
  wake-ups) and `MAILD_IMAP_TRACE` recording with AUTH lines redacted.
- `internal/sync`: the account actor and scheduler, the mailbox reconcile
  pass, UIDVALIDITY reset handling, the IDLE loop, STATUS polling, body
  fetch, and pending-op replay for flags, move and delete.
- `internal/thread`: incremental JWZ threading over `thread_refs`.
- `internal/view`: snapshots, recompute and per-connection deltas;
  `rpcserver` gains a per-connection notifier.
- Engine domains `message`, `view` and `sync`; maild starts one actor per
  account and restarts actors when accounts or passwords change.
- Tests: imapxtest for the no-CONDSTORE paths, a fault-injecting connection
  and a fake clock, Dovecot integration tests for CONDSTORE and IDLE.
- **Done when:** `mailctl` can sync the seeded test1 account, and the
  integration tests for new mail, flag changes and expunges pass.

## Phase 4 — Executor batch 2

- `mailctl sync`, `mailboxes`, `ls`, `show`, `search`, `flag` and `mv`.
- `tools/mailgen`: a deterministic generator of N messages (threads, MIME
  variety) and `make mailtest-seed N=50000`, which loads them with
  `doveadm`.
- The MIME corpus: at least 150 fixtures (encodings, charsets, nested and
  broken structures) with golden JSON, under `internal/mimex/testdata`.
- **Done when:** the cards are merged and `make check` is green.

## Phase 5 — Exit evidence

- **Done when**, each shown by an integration test or a recorded run:
  1. test5 seeded with 50,000 messages syncs completely; `mailctl ls INBOX`
     matches the server's count, and maild's RSS stays under 300 MB.
  2. A flag change, an expunge and a new delivery in INBOX made by another
     IMAP client reach a subscribed client as events within 5 seconds.
  3. `kill -9` of maild during the initial sync, then a restart, ends with
     local state equal to the server's: no duplicates, nothing missing.
  4. At least 150 MIME fixtures pass their golden tests.
  5. `mailctl search` over the 50,000 messages answers in under 150 ms.

## Later / ideas

- QRESYNC in the fork (SELECT parameter, VANISHED parsing) for Dovecot and
  Fastmail.
- NOTIFY (Dovecot) to replace polling of non-INBOX mailboxes.
- PREVIEW (RFC 8970) to skip partial fetches on servers that offer it.

## Open questions

- **Sync window default** (all mail versus the last N days for headers):
  decide from the 50,000-message timings in Phase 5.

## References

- Implements: [design/sync.md](../design/sync.md), [design/storage.md](../design/storage.md), [specs/rpc-protocol.md](../specs/rpc-protocol.md)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
