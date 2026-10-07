---
id: "0024"
title: Query views by mailbox role and by thread
milestone: M2
size: M
touch:
  - internal/store/read.go
given:
  - internal/store/threads_view_test.go
acceptance: go test ./internal/store -count=1
---
# T-0024: Query views by mailbox role and by thread

## Goal

The M2 app lists "All Inboxes" (every mailbox with the inbox role) and shows
one row per conversation, like Mail.app. Extend `ViewIDs` with the `Role`
and `Threads` filters, add `ThreadCount` to summaries, and implement
`ThreadMessageIDs`, which lists a conversation for the reader.

## Read first

- `internal/store/read.go`: `ViewFilter` (the new `Role` and `Threads`
  fields), `ViewIDs`, `Summary` (the new `ThreadCount` field),
  `summaryCols`, `scanSummary`, `Summaries`, `GetMessage`, and the
  `ThreadMessageIDs` stub.
- `internal/store/migrations/0001_init.sql`: `messages.thread_id` (NULL
  for a message without a thread), `threads.msg_count`, `mailboxes.role`.
- The given test `internal/store/threads_view_test.go` and the fixture it
  builds on, `viewFixture` in `internal/store/view_ids_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

**`ViewIDs`** keeps every existing condition and order. Add:

- `Role != ""`: `EXISTS (SELECT 1 FROM message_mailbox mm JOIN mailboxes mb
  ON mb.id = mm.mailbox_id WHERE mm.message_id = m.id AND mb.role = ?)`.
- `Threads`: of the messages matching every other condition, keep only the
  newest of each thread, by the same order the view uses
  (`m.internal_date DESC, COALESCE(m.date_hdr, '') DESC, m.id DESC`). A
  message with `thread_id` NULL is its own thread. The result keeps the
  view's order. Use a window function: `ROW_NUMBER() OVER (PARTITION BY
  COALESCE(m.thread_id, -m.id) ORDER BY …)` in a subquery, keeping rows
  numbered 1.

**`Summary.ThreadCount`** is the `msg_count` of the message's thread, or 1
when the message has no thread or the count is below 1. Every function that
returns a `Summary` fills it: `Summaries` and `GetMessage` (its `Summary`).
Read it in SQL, for example by adding
`COALESCE((SELECT t.msg_count FROM threads t WHERE t.id = messages.thread_id), 1)`
as a column; mind the table name or alias each query uses.

**`ThreadMessageIDs(ctx, threadID)`** returns the IDs of the thread's
messages with `deleted = 0`, oldest first: `internal_date ASC,
COALESCE(date_hdr, '') ASC, id ASC`. It returns a non-nil empty slice when
the thread exists but has no such messages, and an error wrapping
`ErrNotFound` when no thread has that ID. Replace the stub's doc comment's
last sentence ("Task T-0024 implements it; …") with these rules.

## Tests (given, do not edit)

`internal/store/threads_view_test.go`: TestViewIDsRolesAndThreads,
TestThreadMessageIDs, TestSummaryThreadCount, TestThreadViewLargeMailbox
(50,000 messages in 10,000 threads, under one second). The existing
`view_ids_test.go` and `read_test.go` must keep passing.

## Gotchas

- The thread filter applies after the other filters: a thread's row is its
  newest message *among the matching ones* ("threads within a mailbox"
  expects message 1, not 3, for thread 100).
- Keep building SQL from a `[]string` of conditions and a matching `[]any`
  of arguments; never format values into the SQL.
- `scanSummary` is shared: if you add a column to `summaryCols`, add the
  matching scan destination in `scanSummary` itself, and check every
  query that uses `summaryCols`.
- `ThreadMessageIDs` must tell a missing thread (ErrNotFound) from an empty
  one (empty slice): check the `threads` table.

## Out of scope

The engine and API (T-0025), and every file except `internal/store/read.go`.

## Done when

`make accept T=0024` and `make check` pass, and only
`internal/store/read.go` changed.
