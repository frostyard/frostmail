# 0055 — maild streams its background activity as typed entries with no free text

- **Status:** Proposed
- **Date:** 2026-11-10

## Context

On 2026-11-10 the owner asked for ChairLift's console in Frostmail's
desktop app ([ADR-0054](0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md)),
"to show background activity": what maild does while the user reads.
ChairLift's console (frostui's `Console` and `ConsoleView`) shows lines,
each with a time, a level and a source, filtered by level and by text.

What maild offers clients today is state, not activity:

- `sync.progress` carries an account's `SyncStatus` (phase, the mailbox
  being synced, done and total, an error's text, the last full sync).
- Durable `*.changed` and `*.removed` events say what changed in the store,
  `outbox.changed` what the Outbox holds, and the transient
  `message.announced`, `calendar.reminders` and `*.refused` events what the
  user must see.
- Nothing says that an IDLE woke, that bodies were fetched, that flag
  changes were pushed, that a message went out or failed, that rules ran,
  that a CalDAV, CardDAV, Graph or Google Tasks pass ran, or that an OAuth
  token was refreshed.

maild records that activity only in its journal: about 44 `slog` calls in
`internal/mailsync`, `internal/pimsync`, `internal/oauth` and
`internal/engine` write free-text messages with `err` values. Those errors
carry whatever the server said (a `NO` response, an SMTP reply, an HTTP
body) and can name addresses, hrefs and user names.

The events broker (`internal/events`) sends every event to every
subscribed connection and closes a connection that falls 1,024 behind,
which suits state that a client must not miss, not a high-volume feed that
may drop lines. Callers are `local` (the desktop and mailctl), `session`
(lodge's web build) and `agent` (`/mcp/mail`).

## Decision

- **A new API domain, `activity`** (protocol 1, additive), for the
  default callers `local` and `session`; agents get none of it.
- **An entry has no string field.** `ActivityEntry` holds only numbers,
  times and enums:
  - `seq` (per maild run), `at`, `level` (`debug`, `info`, `warn`,
    `error`);
  - `kind`, a closed enum: `connect`, `list`, `mailboxsynced`, `idle`,
    `fetched`, `pushed`, `draftsaved`, `sent`, `sendfailed`, `sendlater`,
    `rulesapplied`, `remindfired`, `announced`, `erased`, `pimsynced`,
    `pimrefused`, `tokenrefreshed`, `signinneeded`, `offline`, `failed`,
    `dropped` (the IDL's enum values are lowercase words);
  - `accountId`, `mailboxId` and `service` (the enum `ActivityServer`:
    `imap`, `smtp`, `caldav`, `carddav`, `graph`, `gtasks`, `oauth`), each
    when it applies;
  - counts (`n`, `added`, `changed`, `removed`, `bytes`) and `ms`, the
    duration;
  - for a failure, `errorClass` (`network`, `timeout`, `tls`, `auth`,
    `refused`, `quota`, `protocol`, `internal`), with `code`, a number (an
    SMTP reply code, an HTTP status), or `imapCode`, an enum of the RFC 5530
    response codes (`authenticationfailed`, `unavailable`, `overquota`,
    `limit`, `nonexistent`, …) with `other` for the rest.

  The client writes the line from the kind and the fields, and names the
  account and the mailbox from what it already shows. A message's
  subject, body and addresses, credentials, tokens and servers' texts
  cannot travel, because no field can hold them.
- **One way in.** `internal/activity` is the only package that makes an
  `api.ActivityEntry`, through typed constructors
  (`MailboxSynced(acct, mbox, added, changed, removed, d)`,
  `SendFailed(acct, err, retried)` and the like) that the sync, send,
  rule, reminder, erase, PIM and OAuth code calls. `activity.Classify(err)`
  maps an error to its class and code from its type (`net.Error`, TLS
  errors, the IMAP response code through `imapx.ResponseCode`, the SMTP
  reply code through `smtpx.ReplyCode`, the HTTP status of davx's,
  graph's and gtasks' errors, oauth's refused grants) and never reads its
  text. oauth, which imapx and smtpx import, reports refreshes through a
  callback that maild turns into entries. The journal keeps the free
  text, as today.
- **Bounded at every step.**
  - maild keeps the newest 2,000 entries in memory: not in SQLite, not
    replayed, gone at a restart.
  - It records one entry per mailbox pass, prefetch run, push batch,
    send, rule run, PIM service pass and token refresh, never one per
    message, and at most 20 debug or info entries of one kind a second per
    account; past that it counts them in one `dropped` entry. Warnings and
    errors are always kept, so a burst of mailbox passes cannot hide a
    failed send.
  - Each subscribed connection has a buffer of 256 entries. A connection
    that falls behind loses entries and then gets one `dropped` entry with
    their number; it is never closed for it.
- **Methods and event:** `activity.recent {limit}` returns the newest
  entries (at most 500), so a console opened late shows what came before;
  `activity.subscribe` and `activity.unsubscribe` start and stop the
  transient event `activity.added` on this connection, apart from
  `events.subscribe`. `mailctl activity [-f]` prints them with
  `activity.Line`, which the desktop's console uses too.
- **The desktop shows it in frostui's console**, closed by default, opened
  with Ctrl J, a top-bar button or the palette's Show Activity; the status
  bar shows the newest entry. The app adds lines of its own about its
  connection to maild, built the same way. The web build may use the same
  methods later; this ADR changes nothing there.

## Consequences

- The user sees maild's background work without the journal, on the
  desktop now and in the web build later.
- A line says less than the journal: a failure shows its class and code
  ("Work: send failed, refused (550)"), not the server's words. The
  journal and problem records keep the detail.
- Each emit point needs a test that it records its entry, and every new
  kind is an IDL change.
- Three tests hold the rule:
  - a test of the generated `api.ActivityEntry` fails when it gains a
    field of kind string;
  - a test of the module's source fails when a package other than
    `internal/activity` builds an `api.ActivityEntry` literal;
  - a canary test, end to end, syncs, flags, sends and signs in against
    the in-memory IMAP and SMTP servers with canaries in the subject, the
    body, the addresses, the SMTP server's refusal and the password, and
    fails when any recorded entry's JSON holds one; and at the
    constructors, every error type a server's words reach (davx's,
    graph's, gtasks', oauth's, network errors, plain text) carries a
    canary through every constructor that takes an error.
- maild holds about 2,000 small entries in memory, and does a little work
  per pass even with no client listening.
- A client learns nothing through activity that it could not already ask
  for: IDs, counts and timings of its own accounts.

## Alternatives considered

- **Tee maild's `slog` lines to the console:** free text whose errors
  carry servers' words, addresses and hrefs, and whose wording changes
  with any log edit.
- **Free text with a redactor:** a redactor misses what it does not know,
  such as an address inside a server's `NO` response; a closed vocabulary
  needs none.
- **Durable events in the changes log:** activity is not state; it would
  grow the log and replay noise to every reconnecting client.
- **Build the console in the app from `sync.progress` and the `*.changed`
  events:** shows state changes, but not sends, failures, rule runs, PIM
  passes or refreshes, and the web build could not share it.

## References

- Shapes: [plans/0015](../plans/0015-mygo-desktop.md),
  [specs/rpc-api.md](../specs/rpc-api.md) (generated, once the IDL lands),
  [specs/rpc-protocol.md](../specs/rpc-protocol.md)
- Builds on: [ADR-0002](0002-daemon-and-thin-clients.md),
  [ADR-0007](0007-rpc-contract.md),
  [ADR-0041](0041-maild-runs-as-a-lodge-app.md),
  [ADR-0049](0049-record-problems-in-the-local-journal.md),
  [ADR-0054](0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md)
