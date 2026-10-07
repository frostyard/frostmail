---
id: "0017"
title: Read message summaries, details and locations
milestone: M1
size: M
touch:
  - internal/store/read.go
given:
  - internal/store/read_test.go
acceptance: go test ./internal/store -run 'TestSummaries|TestGetMessage|TestMessageLocation|TestSetBody' -count=1
---
# T-0017: Read message summaries, details and locations

## Goal

`view.range` returns message summaries, `message.get` returns details, and
fetching a body needs to know which mailbox and UID to ask the server for.
Implement the four remaining stubs in `internal/store/read.go`.

## Read first

- `internal/store/read.go`: `Summary`, `MessageDetail`, `Location`, and the
  stubs `Summaries`, `GetMessage`, `MessageLocation`, `SetBody`. `ViewIDs`
  belongs to T-0016; leave it as it is.
- `internal/store/message.go` (`Address`, `Part`) and `internal/store/flags.go`
  (`Flags`).
- `internal/store/migrations/0001_init.sql`: `messages`, `message_mailbox`,
  `mailboxes`, `parts`.
- The given test `internal/store/read_test.go`, especially which empty
  values must be nil.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the signatures; extend the doc comments with these rules. Reads use
`d.db`; `SetBody` uses `t`.

- **Summary columns:** `id, account_id, thread_id` (NULL → 0), `subject`,
  `from_name, from_addr` → `From`, `date_hdr` (when NULL use
  `internal_date`; both through `ParseTime`) → `Date`, `preview`,
  `has_attachments`, `size`, and the flags `seen, flagged, answered,
  forwarded, draft, deleted, flag_color, keywords_json` → `Flags`
  (`Keywords` nil when the JSON array is empty). `MailboxIDs`: every
  `message_mailbox.mailbox_id` of the message, ascending.
- **`Summaries(ctx, ids)`:** one query with `WHERE id IN (?, ?, …)`, one more
  for the mailbox IDs; return summaries in the order of `ids`, skipping IDs
  that do not exist. No IDs: return an empty slice without querying.
- **`GetMessage(ctx, id)`:** the summary plus `to_json`, `cc_json`,
  `reply_to_json` (→ `[]Address`, nil when empty), `msgid_hdr`,
  `in_reply_to`, `refs_json` (→ `[]string`, nil when empty), `list_id`,
  `list_unsubscribe`, `blob_sha` (NULL → `""`), and `Parts` from the `parts`
  table ordered by `path` (nil when none). No row: `ErrNotFound`.
- **`MessageLocation(ctx, id)`:** the membership with a non-NULL `uid` and
  the lowest `mailbox_id`, joined to `mailboxes` for the path. None:
  `ErrNotFound`.
- **`SetBody(ctx, id, blobID)`:** `UPDATE messages SET blob_sha = ?,
  body_state = 'full' WHERE id = ?`; zero rows: `ErrNotFound`.
- Decode JSON columns with `encoding/json/v2`; wrap errors with context.
- Remove the `errNotImplemented` uses in this file only.

## Tests (given, do not edit)

`internal/store/read_test.go`: TestSummaries, TestGetMessage,
TestMessageLocation, TestSetBody.

## Gotchas

- `reflect.DeepEqual` tells nil from empty: decode JSON into a local slice,
  then set the field only `if len(x) > 0`.
- Write one `scanSummary` helper shared by `Summaries` and `GetMessage` so
  the column list is written once.
- Build `IN (?, ?, …)` with `strings.Repeat("?, ", n)` and the IDs as
  `[]any`.

## Out of scope

`ViewIDs` (T-0016), and every file except `internal/store/read.go`.

## Done when

`make accept T=0017` and `make check` pass, and only
`internal/store/read.go` changed.
