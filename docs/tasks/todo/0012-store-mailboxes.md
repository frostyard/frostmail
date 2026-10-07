---
id: "0012"
title: Store mailboxes from LIST and their sync state
milestone: M1
size: M
touch:
  - internal/store/mailbox.go
given:
  - internal/store/mailbox_sync_test.go
acceptance: go test ./internal/store -run 'TestReplaceMailboxes|TestMailboxSyncState' -count=1
---
# T-0012: Store mailboxes from LIST and their sync state

## Goal

Each sync starts by listing the account's mailboxes. The store must mirror
that list exactly and keep each mailbox's position in the reconcile pass
(`docs/design/sync.md`). Implement the three stubs at the end of
`internal/store/mailbox.go`.

## Read first

- `internal/store/mailbox.go`: `Mailbox`, `ServerMailbox`, `SyncState`,
  `listMailboxes` (works with a `*Tx`), and the stubs, whose doc comments
  state the contract.
- `internal/store/migrations/0001_init.sql`: tables `mailboxes`,
  `message_mailbox`, `messages`, `messages_fts`.
- `internal/store/store.go`: `Tx`, `Tx.Emit`, `FormatTime`, `ParseTime`,
  `ErrNotFound`.
- `internal/store/account.go`: an example of scanning rows and emitting.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the signatures and doc comments. Replace the three
`errNotImplemented` returns in this file (the variable lives in
`message.go`; leave it).

**`ReplaceMailboxes(ctx, accountID, list)`**, all through `t`:

1. Load the account's stored mailboxes: `id, path, delimiter, name, role,
   attrs_json, selectable, subscribed`.
2. For each `ServerMailbox`: compute `name` (last component after
   `Delimiter`, or the whole path) and `attrs_json` (`json.Marshal` from
   `encoding/json/v2` of `Attrs`; nil becomes `[]`). If the path is new,
   `INSERT` it and emit `api.MailboxChanged{ID, AccountID}`. If it exists and
   any of delimiter, name, role, attrs_json, selectable or subscribed differs,
   `UPDATE` it and emit the same event. Otherwise do nothing.
3. For each stored path not in `list`: collect the IDs of messages whose only
   memberships are in that mailbox (they have no `message_mailbox` row in
   any other mailbox), delete the mailbox (memberships cascade), delete those
   messages and their `messages_fts` rows (`DELETE FROM messages_fts WHERE
   rowid = ?`), emit `api.MailboxChanged{ID, AccountID, Deleted: true}`, and,
   if any messages were deleted, one `api.MessageRemoved{AccountID, IDs}`
   with the IDs ascending.
4. Return `listMailboxes(ctx, t, accountID)`.

**`MailboxSyncState(ctx, mailboxID)`:** select `uidvalidity, uidnext,
highestmodseq, server_count, last_sync_at` with `d.db`. No row:
`ErrNotFound`. `uidvalidity` NULL: zero state, `ok` false. Otherwise read
NULL columns as 0 (`sql.NullInt64`), parse `last_sync_at` with `ParseTime`
when not NULL, and return `ok` true.

**`SetMailboxSyncState(ctx, mailboxID, s)`:** one `UPDATE` of those five
columns (`last_sync_at` NULL when `s.LastSyncAt.IsZero()`, else
`FormatTime`). Zero rows affected: `ErrNotFound`. No events.

## Tests (given, do not edit)

`internal/store/mailbox_sync_test.go`: TestReplaceMailboxesInserts,
TestReplaceMailboxesUpdatesAndDeletes, TestMailboxSyncState.

## Gotchas

- Read everything through `t` inside `ReplaceMailboxes`; `d.db` cannot see
  the transaction's uncommitted rows.
- `SyncState` holds `uint32` and `uint64`; convert explicitly when scanning
  and binding (SQLite integers are signed 64-bit; `HighestModSeq` fits).
- Close each `*sql.Rows` before running the next statement in the same
  transaction (collect rows into a slice first).
- An unchanged list must emit nothing: compare before updating.

## Out of scope

Choosing roles from SPECIAL-USE (the sync engine does that), and every file
except `internal/store/mailbox.go`.

## Done when

`make accept T=0012` and `make check` pass, and only
`internal/store/mailbox.go` changed.
