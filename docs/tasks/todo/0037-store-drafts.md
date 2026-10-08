---
id: "0037"
title: Store drafts and their attachments
milestone: M3
size: M
touch:
  - internal/store/drafts.go
given:
  - internal/store/drafts_test.go
acceptance: go test ./internal/store -run 'Draft' -count=1
---
# T-0037: Store drafts and their attachments

## Goal

maild keeps every draft locally the moment a compose window changes it, and
copies it to the server's Drafts mailbox after a quiet period
(`docs/design/send.md`, Drafts). Implement the store functions for drafts:
create, read, list, update, delete, attachments, and the query that finds
drafts whose server copy is out of date.

## Read first

- `internal/store/drafts.go`: the types and the stubs.
- `internal/store/migrations/0003_drafts_outbox.sql`: `drafts` and
  `draft_attachments`.
- `internal/store/read.go`: `rowScanner`, `FormatTime`/`ParseTime` use, and
  how `GetMessage` loads a row and its children.
- `internal/store/store.go`: `Tx`, `Tx.Now`, `ErrNotFound`.
- The given test `internal/store/drafts_test.go` and the helpers it uses in
  `message_test.go` and `helpers_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the types and signatures; replace each stub's doc comment (and its
"Task T-0037 …" sentence) with the rules below. None of these functions emits
events (the engine does).

- **Storage:** `content_json` holds `DraftContent` as JSON (its struct tags);
  `refs_json` holds `References` as a JSON array (`[]` when empty);
  `source_id`, `server_uid` and `saved_at` are NULL when zero. Times are
  stored with `FormatTime` and read with `ParseTime`.
- **`CreateDraft(ctx, d)`:** inserts the draft with `created_at` and
  `updated_at` = `t.Now()` (UTC, truncated to the millisecond); `d.ID`,
  `ServerUID` and `SavedAt` are ignored. Returns the stored draft as
  `GetDraft` would read it (with the new ID).
- **`GetDraft(ctx, id)`:** the draft with its attachments in insertion
  order (by ID); `nil` attachments when there are none. A missing draft is an
  error wrapping `ErrNotFound`.
- **`ListDrafts(ctx, accountID)`:** drafts of the account (every account
  when 0), `updated_at` newest first, then higher ID first; each with its
  attachments.
- **`UpdateDraftContent(ctx, id, c)`:** replaces the content, sets
  `updated_at` = `t.Now()`, returns the draft as stored. Missing: wraps
  `ErrNotFound`.
- **`DeleteDraft(ctx, id)`:** returns the draft as it was (attachments and
  server copy included), then deletes it; its attachments go with it (the
  foreign key cascades). Missing: wraps `ErrNotFound`.
- **`AddDraftAttachment(ctx, draftID, a)`:** inserts the attachment and
  returns it with its new ID.
- **`RemoveDraftAttachment(ctx, draftID, attachmentID)`:** deletes the
  attachment only if it belongs to that draft; otherwise (or when missing)
  wraps `ErrNotFound`.
- **`DraftsToSave(ctx, accountID, quietBefore)`:** the account's drafts
  with `updated_at` at or before `quietBefore` whose server copy is missing
  or older than the content (`saved_at IS NULL OR saved_at < updated_at`),
  oldest `updated_at` first, then lower ID; each with its attachments.
- **`SetDraftServerCopy(ctx, id, uid, savedAt)`:** records the server copy's
  UID and when it was written. Missing: wraps `ErrNotFound`.

Tx methods use the transaction (`t.ExecContext`, `t.QueryContext`,
`t.QueryRowContext`); DB methods use `d.db`. Share the row scanning and the
attachment loading between them with small helpers that take either.

## Tests (given, do not edit)

`internal/store/drafts_test.go`: TestCreateAndGetDraft,
TestEmptyDraftLists, TestUpdateDraftContent, TestListDrafts,
TestDraftAttachments, TestDeleteDraft, TestDraftsToSave.

## Gotchas

- `encoding/json/v2` (the repository's JSON package) encodes nil slices as
  `[]`; reading back gives empty slices, which the tests accept.
- Comparing stored times as text works because `FormatTime` has a fixed
  width; keep using it for every value you compare in SQL.
- Load attachments after closing the drafts query's rows (SQLite in this
  store allows one open query per connection inside a transaction).

## Out of scope

The engine, the draft saver and the outbox (planner), and every file except
`internal/store/drafts.go`.

## Done when

`make accept T=0037` and `make check` pass, and only
`internal/store/drafts.go` changed.
