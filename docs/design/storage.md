# Design: storage

Decided in [ADR-0003](../adr/0003-sqlite-store.md). Code: `internal/store`.

## Files

- `$FROSTMAIL_DATA_DIR/frostmail.db` (+ `-wal`, `-shm`): SQLite in WAL mode,
  `synchronous=NORMAL`, `foreign_keys=ON`, `busy_timeout=5000`, writes with
  `BEGIN IMMEDIATE`. Must be on a local filesystem.
- `$FROSTMAIL_DATA_DIR/blobs/<sha[:2]>/<sha>`: raw RFC 5322 messages, written
  to a temp file and renamed (M1).
- `$FROSTMAIL_CACHE_DIR/parts/`: decoded parts for `mailpart://`;
  disposable.

## Schema

`internal/store/migrations/0001_init.sql` is the source of truth. Times are
UTC text in `store.TimeFormat` (`2006-01-02T15:04:05.000Z`), which sorts
lexically. Highlights:

- `accounts`: server settings per account; `email` is unique without case.
  Credentials are not stored here (Secret Service, M4; a dev store in M1).
- `mailboxes`: one per server folder or Gmail label, with sync state
  (`uidvalidity`, `uidnext`, `highestmodseq`, `synced_low_uid`).
- `messages`: one per message per account (Gmail: per `gm_msgid`), with
  envelope fields, flags, `flag_color` (Apple's `$MailFlagBit0-2`), and
  `body_state` (`headers` until the body is fetched into the blob store).
- `message_mailbox`: memberships. Generic IMAP has exactly one row per
  message; Gmail has one per label, with the UID on the All Mail row.
  `(mailbox_id, uid)` is unique when `uid` is set.
- `threads`, `thread_refs`: thread rows and every Message-ID seen, mapped to a
  thread (JWZ containers, [sync.md](sync.md)).
- `parts`: the MIME structure from BODYSTRUCTURE.
- `messages_fts`: contentless FTS5 (`unicode61 remove_diacritics 2`,
  prefixes 2 and 3); rowid = `messages.id`.
- `outbox`, `pending_ops`: queued sends and offline actions;
  `pending_op_messages` names the messages each action covers, whose flags
  sync leaves alone until the action is replayed or fails.
- `settings`, `vips`, `smart_mailboxes`, `rules`, `message_reminders`
  (M5): preferences and what is built on them
  ([organize.md](organize.md)). Views order by `messages.list_date`, the
  arrival date until a Remind Me reminder fires, unless `ViewQuery.sort`
  names another key (from, to, subject, size, flags, unread,
  attachments, either way); ties keep the list date. With threads, a
  thread is its newest message's row and goes where that row's key puts
  it.
- `changes`: the durable event log; `seq` is AUTOINCREMENT, so it is never
  reused after `PruneChanges`.

## Writes and events

`DB.Tx(ctx, fn)` holds the write mutex, begins, runs `fn`, commits, then
passes the events `fn` emitted to `DB.OnCommit` (the broker) while still
holding the mutex, so delivery order equals commit order. `Tx.Emit` inserts
durable events into `changes` inside the transaction; a rollback drops them.
Store functions that write take `*Tx` and emit their own events; readers are
`*DB` methods.

## Errors

`ErrNotFound` and `ErrConflict` (from `IsUniqueViolation`, which checks the
SQLite extended code) are the store's sentinels; the engine maps them to
`api.ErrNotFound` and `api.ErrConflict`.
