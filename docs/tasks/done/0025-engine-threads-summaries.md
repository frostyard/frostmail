---
id: "0025"
title: Serve summaries, thread messages and thread views
milestone: M2
size: S
touch:
  - internal/engine/messages.go
  - internal/engine/threads.go
  - internal/engine/views.go
given:
  - internal/engine/read_api_test.go
acceptance: go test ./internal/engine -count=1
---
# T-0025: Serve summaries, thread messages and thread views

## Goal

The app refreshes changed list rows with `message.summaries`, shows a
conversation with `thread.messages`, and opens views of every inbox and of
conversations through the view query's `role` and `threads` fields. The
store side exists (T-0024); connect it to the API.

## Read first

- `docs/specs/rpc-api.md`: `message.summaries`, `thread.messages`,
  `ViewQuery` (`role`, `threads`), `MessageSummary.threadCount`.
- `internal/engine/messages.go`: `Get` and `toAPISummary`; the
  `Summaries` stub.
- `internal/engine/threads.go`: the `Messages` stub.
- `internal/engine/views.go`: `views.Open` and `views.Range`.
- `internal/store/read.go`: `Summaries`, `ThreadMessageIDs`, `ViewFilter`,
  `Summary.ThreadCount`.
- `internal/engine/engine.go`: `apiError`.
- `docs/tasks/EXECUTOR.md`

## Contract

- **`toAPISummary`** also copies `ThreadCount`.
- **`messages.Summaries`**: `m.DB.Summaries(ctx, p.IDs)`, each converted
  with `toAPISummary`; always a non-nil slice (empty for no IDs).
- **`threads.Messages`**: `t.DB.ThreadMessageIDs(ctx, p.ID)`, then
  `t.DB.Summaries` of those IDs, converted the same way. A missing thread
  is `apiError(err, fmt.Sprintf("thread %d", p.ID))`, which maps
  `store.ErrNotFound` to the API's notFound.
- **`views.Open`** sets `f.Role = string(*q.Role)` when `q.Role` is set and
  `f.Threads = *q.Threads` when `q.Threads` is set.
- Replace each stub's doc comment ending ("Task T-0025 implements it; …")
  with a sentence saying what the method does.

## Tests (given, do not edit)

`internal/engine/read_api_test.go`: TestMessageSummaries,
TestThreadMessages, TestViewRoleAndThreads. They run over the socket with
`rpctest` and reuse `createParams` from `accounts_test.go`.

## Gotchas

- `api.MessageSummary` gained `ThreadCount`; `views.Range` already uses
  `toAPISummary`, so the row count comes from there.
- Keep the empty-slice result: JSON `[]`, never `null`.

## Out of scope

`message.render` and `message.part` (`render.go`), and every file outside
the touch list.

## Done when

`make accept T=0025` and `make check` pass, and only the files under touch
changed.
