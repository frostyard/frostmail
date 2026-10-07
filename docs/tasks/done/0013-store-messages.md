---
id: "0013"
title: Store message headers, flags and removals by UID
milestone: M1
size: M
touch:
  - internal/store/message.go
given:
  - internal/store/message_test.go
acceptance: go test ./internal/store -run 'TestInsertHeaders|TestMailboxUIDs|TestUpdateFlags|TestRemoveUIDs' -count=1
---
# T-0013: Store message headers, flags and removals by UID

## Goal

The reconcile pass (`docs/design/sync.md`) stores headers for new UIDs,
updates flags the server changed, and removes UIDs the server expunged. All
three must be idempotent so a pass killed halfway can run again. Implement
the four stubs in `internal/store/message.go`.

## Read first

- `internal/store/message.go`: the types and the four stubs, whose doc
  comments state the contract.
- `internal/store/flags.go`: `Flags` (column mapping below).
- `internal/store/migrations/0001_init.sql`: `messages`, `message_mailbox`,
  `parts`, `messages_fts`.
- `internal/store/store.go`: `Tx`, `FormatTime`.
- `internal/mimex/subject.go`: `NormalizeSubject`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the signatures and doc comments. Remove the four `errNotImplemented`
returns, but keep the `errNotImplemented` variable: other stubs use it.

**Column mapping** for `Flags`: `seen`, `flagged`, `answered`, `forwarded`,
`draft`, `deleted` are 0 or 1; `flag_color` is `Color`; `keywords_json` is
`json.Marshal(Keywords)` (`encoding/json/v2`, so nil becomes `[]`).

**`InsertHeaders`**, all through `t`:

1. For each header, check `SELECT 1 FROM message_mailbox WHERE mailbox_id = ?
   AND uid = ?`. If found, skip it.
2. Otherwise `INSERT INTO messages` with `account_id`, `msgid_hdr`,
   `in_reply_to`, `refs_json` (JSON of `References`), `subject`,
   `subject_norm` (`mimex.NormalizeSubject(Subject)`), `from_name`,
   `from_addr`, `to_json`, `cc_json`, `bcc_json`, `reply_to_json` (JSON of the
   `[]Address`; `Address` has JSON tags `name` and `addr`), `date_hdr` (NULL
   when `Date.IsZero()`, else `FormatTime(Date)`), `internal_date`
   (`FormatTime`), `size`, `preview`, `has_attachments`, `list_id`,
   `list_unsubscribe`, `auth_results`, the flag columns, and
   `body_state = 'headers'`. Leave `thread_id` NULL.
3. `INSERT INTO message_mailbox (message_id, mailbox_id, uid, modseq)`.
4. `INSERT INTO parts` one row per `Part` (`message_id, path, content_type,
   charset, encoding, disposition, filename, content_id, size`).
5. Return the new IDs in input order (nil if none were inserted).

**`MailboxUIDs`**: `SELECT uid FROM message_mailbox WHERE mailbox_id = ? AND
uid IS NOT NULL ORDER BY uid` with `d.db`; return `[]uint32{}` when empty.

**`UpdateFlags`**: for each update, find `message_id` by `(mailbox_id, uid)`;
skip if missing. Set the membership's `modseq`. Read the message's flag
columns; if any differs from the update's `Flags`, write them all and append
the ID to the result.

**`RemoveUIDs`**: for each UID, find `message_id` by `(mailbox_id, uid)`;
skip if missing. Delete that membership. If the message has no other
`message_mailbox` row, delete it from `messages` (parts cascade) and from
`messages_fts` (`DELETE FROM messages_fts WHERE rowid = ?`), and add its ID to
the result. Return the IDs sorted ascending.

Wrap database errors with context, for example
`fmt.Errorf("insert headers uid %d: %w", h.UID, err)`.

## Tests (given, do not edit)

`internal/store/message_test.go`: TestInsertHeadersStoresEverything,
TestInsertHeadersIsIdempotent, TestMailboxUIDs, TestUpdateFlags,
TestRemoveUIDs.

## Gotchas

- Bind `uint32` and `uint64` values as `int64(...)`; scan UIDs into `int64`
  and convert.
- Compare keywords as their JSON strings when deciding whether flags
  changed.
- Do not query with `d.db` inside the `Tx` methods; it cannot see the
  transaction's own rows.

## Out of scope

Threading (`thread_id`), search indexing, and events (all done by the sync
engine in the same transaction); every file except
`internal/store/message.go`.

## Done when

`make accept T=0013` and `make check` pass, and only
`internal/store/message.go` changed.
