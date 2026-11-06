# 0051 — maild erases old Trash and Junk mail when the user asks

- **Status:** Accepted
- **Date:** 2026-11-05

## Context

Mail.app can erase deleted messages and junk messages by itself, per
account: Never, after one day, after one week, or after one month
(Settings, Accounts, Mailbox Behaviors). Frostmail has only the manual
Erase Deleted Items and Erase Junk Mail (P-110,
[design/organize.md](../design/organize.md#mailboxes)): `mailbox.erase`
hides every stored message of a trash or junk mailbox at once and queues
one `expunge` op. Brian asked for the automatic form on 2026-11-05
(T929a): off by default, and never in the user's own folders.

What maild knows and does not know:

- IMAP keeps no time at which a message entered a mailbox. A move or a
  copy keeps the message's `INTERNALDATE`, which is its first arrival, so
  a two-year-old message deleted today has a two-year-old date in Trash.
- maild stores each message's place as a `message_mailbox` row, made when
  sync or a local op first puts it there, and has no time on that row.
- maild's reminder scheduler wakes every minute and at start
  ([ADR-0025](0025-maild-keeps-send-later-remind-me-and-undo-send.md),
  [design/organize.md](../design/organize.md#remind-me)).
- An account keeps only the mail of its sync window
  ([ADR-0016](0016-sync-a-window-of-recent-mail.md)); mail outside it is
  not in the store.

## Decision

- **Two settings per account**, as Mail.app has them:
  `Account.eraseTrashDays` and `Account.eraseJunkDays`, each 0 (Never,
  the default), 1, 7 or 30, set with `account.update` and
  `mailctl account set`. They live in `accounts`, beside `notify` and
  `syncDays` ([ADR-0026](0026-preferences-live-in-maild.md) keeps
  per-account settings there).
- **Age is time in the mailbox as maild saw it.** `message_mailbox` gains
  `added_at`, the time maild first saw the message in that mailbox: set
  when a row is made and whenever a row moves to another mailbox (a local
  move updates the row in place), kept while it stays. A message is
  old when its `added_at` is at least the setting's days ago. Rows that
  exist when the column arrives get the migration's time, so nothing is
  erased until a full period after the upgrade. `INTERNALDATE` is never
  used.
- **Only the role mailbox.** The erase pass looks only at the mailbox that
  has the `trash` or `junk` role for the account when it runs (the
  server's role, or the user's choice with Use This Mailbox For, P-109).
  It never touches a mailbox without one of those roles.
- **The UIDs are bound to `UIDVALIDITY`.** The `expunge` op records the
  mailbox's `UIDVALIDITY` when it is queued, and replays only when the
  server still has it; otherwise it erases nothing and the mailbox
  resyncs. maild replays ops before its first sync after a start, so a
  server whose UIDs were reset could otherwise see a stale UID erase a
  new message.
- **The same erase as the menu's.** The reminder scheduler runs the pass
  at maild's start and then once an hour. For each writable account with
  a setting above 0, one transaction hides the old members of the
  mailbox that `mailbox.erase` would take, queues one `expunge` op for
  them, and emits `message.changed` and `mailbox.changed`. At most 1,000
  messages per mailbox per pass; the next pass takes the rest.
- **No undo, no prompt at the time.** The setting is the user's consent,
  as in Mail.app. The settings form says that erased mail cannot be
  recovered.
- **Read-only accounts are never erased.** The setting is kept and does
  nothing while the account is read-only.

## Consequences

- Trash and Junk stay small with nobody looking, with the app closed,
  and late rather than never when the computer was off.
- A message erases at least the chosen days after maild first saw it in
  the mailbox. It can be later (the computer was off; a full resync after
  a `UIDVALIDITY` change makes new rows with a new `added_at`), never
  earlier.
- A message older than the sync window leaves the store when it reaches
  Trash, as other old mail does, so this setting never erases it; the
  server keeps it until the user erases Trash elsewhere. The settings
  form says so.
- If the user gives the Trash role to one of their own folders, that
  folder is now the Trash and its old mail is erased. The form names the
  mailbox it erases.
- `message_mailbox` grows one column, and every path that makes a
  membership sets it: sync, local moves and copies, Gmail labels.
- Gmail already erases its Trash and Spam after 30 days; the setting
  only makes it sooner there.

## Alternatives considered

- **Use `INTERNALDATE`:** a deleted old message would be erased within
  the hour, with no time to take it back from Trash.
- **Use `SEARCH BEFORE` on the server, to reach mail outside the
  window:** the same `INTERNALDATE` problem, and erasing what the user
  cannot see in Frostmail.
- **One global setting in `settings`:** Mail.app's is per account, and an
  account the user shares with another client may need Never.
- **"When quitting Mail"**, Mail.app's fifth choice: maild does not quit
  when the app does.

## References

- Shapes: [design/organize.md](../design/organize.md#erase-old-mail),
  [specs/settings-ui.md](../specs/settings-ui.md#accountform),
  [specs/parity.md](../specs/parity.md) (P-114),
  [plans/0014](../plans/0014-small-features-from-the-nexmail-review.md)
  (Phase 4)
- Builds on: [ADR-0025](0025-maild-keeps-send-later-remind-me-and-undo-send.md)
  (the scheduler), [ADR-0026](0026-preferences-live-in-maild.md),
  [ADR-0016](0016-sync-a-window-of-recent-mail.md)
