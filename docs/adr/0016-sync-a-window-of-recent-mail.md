# 0016 — Sync a window of recent mail

- **Status:** Accepted
- **Date:** 2026-10-08

## Context

The first sync fetches the headers and a 2 KB preview of every message in
every mailbox. The user's real Gmail account holds 688,585 messages in All
Mail; the sync stored about 500 a minute at about 4.2 KB each, so the first
sync would take about 23 hours, use 2–3 GB of database, and download about
2.9 GB. Gmail limits IMAP downloads to 2,500 MB a day per account and locks
IMAP for every client of the account for up to a day when it is exceeded.
About 115 messages a day arrive, so a year of that mail is about 42,000
messages. design/sync.md already said headers would be "backfilled down to
the configured window", but nothing configured or applied one.

## Decision

Each account has a sync window, `syncDays`: maild keeps the messages whose
INTERNALDATE falls within the last `syncDays` days (0, the default, keeps
everything). Every reconcile pass that searches the server (the generic
pass and the Gmail pass) asks for `UID SEARCH SINCE <date>` instead of every
UID, so the server set S is the window: messages in it that the store
lacks are fetched, newest first; stored messages outside it are removed
like messages deleted on the server, so mail that ages out leaves the store
(and stays on the server) at the next pass that searches. Flags are still
fetched for the stored messages. `account.verify` compares the same window.
Changing `syncDays` clears the account's mailbox sync state, so the next
pass searches again: a wider window backfills, a narrower one drops the
mail that left it. The user set the window to 365 days for their Gmail
account; the iCloud and test accounts keep everything.

## Consequences

- A big account syncs in about an hour and stays within Gmail's daily
  limit; the store holds the window's mail, and the counts, the list and
  local search cover only it.
- Mail older than the window is reachable only on the server until server
  search (`UID SEARCH`, `X-GM-RAW` on Gmail) is added to the search UI.
- A message with an old INTERNALDATE stays out of the store even when it
  moves into the inbox: Gmail keeps INTERNALDATE when a message is restored
  or relabeled.
- Aging is lazy: a message leaves the store at the first pass after it
  ages out that searches the server, which on Gmail is any pass after a
  change.
- Every pass that searches pays a `SEARCH SINCE` on the server instead of
  `SEARCH ALL`; on Gmail that is the cheaper of the two for a big account.

## Alternatives considered

- **Sync everything, throttled under Gmail's limit:** two to three days for
  the first sync and a store three times the size the search and list
  budgets were measured at (200,000 messages).
- **A window by count (the newest N):** the user thinks in time, and a
  count shifts with every message that arrives.
- **Headers for everything, previews only for recent mail:** halves the
  download but still exceeds Gmail's daily limit on this account and keeps
  the store's size.

## References

- Shapes: [design/sync.md](../design/sync.md),
  [design/accounts.md](../design/accounts.md),
  [specs/settings-ui.md](../specs/settings-ui.md)
- Builds on: [ADR-0012](0012-gmail-labels-as-memberships.md)
