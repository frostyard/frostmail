---
id: "0039"
title: Store the outbox and its send states
milestone: M3
size: M
touch:
  - internal/store/outbox.go
given:
  - internal/store/outbox_test.go
acceptance: go test ./internal/store -run 'Outbox' -count=1
---
# T-0039: Store the outbox and its send states

## Goal

Sent mail waits in an outbox until its undo delay passes, then moves through
sending states; a worker must never move a message from a state it did not
expect, so a crash or a second worker cannot send twice
(`docs/design/send.md`, Outbox). Implement the store functions for outbox
rows: queueing, reading, the due query, and compare-and-set state changes.

## Read first

- `internal/store/outbox.go`: `OutboxItem` and the stubs.
- `internal/store/migrations/0003_drafts_outbox.sql`: the `outbox` table.
- `internal/store/drafts.go` (T-0037): `DeleteDraft`; the outbox's
  `draft_id` is cleared when its draft is deleted (`ON DELETE SET NULL`).
- `internal/store/store.go`: `ErrNotFound`, `ErrConflict`, `Tx.Now`.
- The given test `internal/store/outbox_test.go`, which builds on
  `draftFixture` from `drafts_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the type and signatures; replace each stub's doc comment (and the
"Task T-0039 …" sentence) with the rules below. None of these functions emits
events.

- **Storage:** `rcpt_json` and `to_json` hold `Recipients` and `To` as JSON;
  `draft_id` is NULL when `DraftID` is 0; times use `FormatTime` and
  `ParseTime`.
- **`QueueOutbox(ctx, o)`:** inserts with state `queued`, `attempts` 0, an
  empty `last_error`, and `created_at` = `updated_at` = `t.Now()`; `o.ID`,
  `State`, `Attempts`, `LastError` and the times are ignored. Returns the row
  as `GetOutbox` reads it.
- **`GetOutbox(ctx, id)`:** one row; missing wraps `ErrNotFound`.
- **`ListOutbox(ctx, accountID)`:** rows whose state is not `sent`, of the
  account (every account when 0), by ID.
- **`DueOutbox(ctx, accountID, now)`:** the account's `queued` rows with
  `send_at` at or before `now`, by `send_at`, then ID.
- **`OutboxInState(ctx, accountID, state)`:** the account's rows in that
  state, by ID.
- **State changes** are compare-and-set: each applies only if the row is in
  the expected state, sets `updated_at` = `t.Now()`, and otherwise returns an
  error wrapping `ErrNotFound` (no such row) or `ErrConflict` (another
  state):
  - `MoveOutbox(ctx, id, from, to)`: state `from` → `to`;
  - `RetryOutboxLater(ctx, id, sendAt, reason)`: `sending` → `queued`,
    `send_at` = `sendAt`, `attempts + 1`, `last_error` = `reason`;
  - `FailOutbox(ctx, id, reason)`: `sending` → `failed`, `attempts + 1`,
    `last_error` = `reason`;
  - `RequeueOutbox(ctx, id, sendAt)`: `failed` → `queued`, `send_at` =
    `sendAt` (attempts and the last error stay).
- **`CancelOutbox(ctx, id)`:** only a `queued` row: returns it as it was and
  deletes it. Missing wraps `ErrNotFound`; another state wraps
  `ErrConflict`.

Write each compare-and-set as one `UPDATE … WHERE id = ? AND state = ?`;
when it changes no row, tell `ErrNotFound` from `ErrConflict` with a second
query.

## Tests (given, do not edit)

`internal/store/outbox_test.go`: TestQueueAndGetOutbox,
TestDueAndListOutbox, TestMoveOutboxIsCompareAndSet,
TestRetryFailRequeueOutbox, TestCancelOutbox,
TestOutboxDraftLinkClearsWithTheDraft.

## Gotchas

- Share one row scanner between the single-row and list queries.
- `send_at` comparisons on stored text work because `FormatTime` has a fixed
  width; always format the value you compare with.

## Out of scope

The outbox worker, SMTP and the engine (planner), and every file except
`internal/store/outbox.go`.

## Done when

`make accept T=0039` and `make check` pass, and only
`internal/store/outbox.go` changed.
