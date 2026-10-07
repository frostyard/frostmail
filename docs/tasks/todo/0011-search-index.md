---
id: "0011"
title: Index messages for full-text search
milestone: M1
size: S
touch:
  - internal/store/search.go
given:
  - internal/store/search_test.go
acceptance: go test ./internal/store -run 'TestSearchQuery|TestIndexAndSearch|TestReindexReplaces|TestRemoveFromIndex' -count=1
---
# T-0011: Index messages for full-text search

## Goal

Search runs on SQLite FTS5. Sync writes one index entry per message, and a
user's search text is turned into a safe MATCH expression. Implement the
three stubs in `internal/store/search.go`.

## Read first

- `internal/store/search.go`: `SearchDoc` and the stubs.
- `internal/store/migrations/0001_init.sql`: the `messages_fts` table
  (contentless, `contentless_delete = 1`, columns `subject`, `from_text`,
  `to_text`, `body_text`, `attachment_names`; rowid is the message ID).
- `internal/store/store.go`: `Tx`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the signatures; extend the doc comments with the rules below.

- **`IndexMessage(ctx, id, doc)`:** `DELETE FROM messages_fts WHERE rowid = ?`,
  then `INSERT INTO messages_fts (rowid, subject, from_text, to_text,
  body_text, attachment_names) VALUES (?, ?, ?, ?, ?, ?)`. Both through `t`.
- **`RemoveFromIndex(ctx, ids)`:** delete each ID's row; a missing row or an
  empty slice is fine.
- **`SearchQuery(input)`:** split `input` on whitespace (`strings.Fields`).
  Drop terms that contain no letter or digit (`unicode.IsLetter`,
  `unicode.IsDigit`). Turn each remaining term into an FTS5 prefix phrase:
  double every `"` in it, wrap it in `"`, append `*`. Join with single spaces
  (FTS5 reads juxtaposition as AND). No terms gives `""`.
- Wrap database errors with context (`fmt.Errorf("index message %d: %w", id, err)`).
- Delete the `errNotImplemented` uses in this file only; the variable
  itself is in `message.go` and other stubs still use it.

## Tests (given, do not edit)

`internal/store/search_test.go`: TestSearchQuery (10 cases), TestIndexAndSearch
(diacritics, prefixes, every column, operators typed as words),
TestReindexReplaces, TestRemoveFromIndex.

## Gotchas

- Quoting every term is what keeps `OR`, `NOT`, `-x` and `"` from being read
  as FTS5 syntax; never pass user text to MATCH unquoted.
- A contentless table cannot be updated in place; delete and insert.

## Out of scope

Ranking, a search language with fields (M4), and every file except
`internal/store/search.go`.

## Done when

`make accept T=0011` and `make check` pass, and only
`internal/store/search.go` changed.
