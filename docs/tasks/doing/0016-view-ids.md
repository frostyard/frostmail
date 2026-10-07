---
id: "0016"
title: Query the message IDs of a view
milestone: M1
size: S
touch:
  - internal/store/read.go
given:
  - internal/store/view_ids_test.go
acceptance: go test ./internal/store -run 'TestViewIDs' -count=1
---
# T-0016: Query the message IDs of a view

## Goal

A view (`docs/specs/rpc-api.md`, view domain) is an ordered list of message
IDs that maild keeps for a client. Implement `ViewIDs`, the query that
builds that list from a `ViewFilter`, fast enough for 50,000-message
mailboxes.

## Read first

- `internal/store/read.go`: `ViewFilter` and the `ViewIDs` stub. Other stubs
  in that file belong to T-0017; leave them.
- `internal/store/search.go`: `SearchQuery` (turns typed text into an FTS5
  MATCH expression; `""` means nothing searchable).
- `internal/store/migrations/0001_init.sql`: `messages`, `message_mailbox`,
  `messages_fts` and their indexes.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the signature; extend the doc comment with these rules. Build one
`SELECT m.id FROM messages m` query with `d.db`, adding a condition (and its
argument) only for each filter that is set:

- always: `m.deleted = 0`;
- `AccountID != 0`: `m.account_id = ?`;
- `MailboxID != 0`: `EXISTS (SELECT 1 FROM message_mailbox mm WHERE
  mm.message_id = m.id AND mm.mailbox_id = ?)`;
- `Unread != nil`: `m.seen = ?` (0 when `*Unread` is true, 1 when false);
- `Flagged != nil`: `m.flagged = ?`;
- `q := SearchQuery(Text)` not empty: `m.id IN (SELECT rowid FROM
  messages_fts WHERE messages_fts MATCH ?)`.

Order by `m.internal_date DESC, m.id DESC`. Return the IDs (nil or empty
when none).

## Tests (given, do not edit)

`internal/store/view_ids_test.go`: TestViewIDsFilters (14 cases over a
seven-message fixture) and TestViewIDsLargeMailbox (50,000 messages, under
500 ms).

## Gotchas

- Use `EXISTS`, not a `JOIN`, for the mailbox filter: a message in two
  mailboxes (message 4) must appear once.
- Build the WHERE clause with a `[]string` of conditions joined by `" AND "`
  and a matching `[]any` of arguments; never format values into the SQL.

## Out of scope

The other stubs in `read.go` (T-0017), and every file except
`internal/store/read.go`.

## Done when

`make accept T=0016` and `make check` pass, and only
`internal/store/read.go` changed.
