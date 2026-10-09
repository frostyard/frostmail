---
id: "0078"
title: Write the Google Tasks client
milestone: M4.5
size: M
touch:
  - internal/gtasks/gtasks.go
given:
  - internal/gtasks/gtasks_test.go
acceptance: go test ./internal/gtasks/... -count=1
---
# T-0078: Write the Google Tasks client

## Goal

Phase 4 syncs Google Tasks. pimsync needs a small client of the Tasks API
(v1): task lists, a list's tasks changed since a time, and the writes
Frostmail makes (insert, patch, delete, move), keeping each task's JSON as
sent (ADR-0018 keeps sources as sent).

## Read first

- `docs/design/pim.md`: "Tasks", "Testing".
- `internal/gtasks/gtasks.go`: the types, errors and stubs (keep every
  exported name and signature; replace the bodies and remove `errNotYet`).
- `internal/gtasks/gtaskstest/gtaskstest.go`: the in-process API the
  tests run against; it answers as Google's does.
- Google's reference, for the shapes: `tasklists.list`, `tasks.list`
  (`showCompleted`, `showHidden`, `showDeleted`, `updatedMin`,
  `maxResults`, `pageToken`), `tasks.insert` (`parent`, `previous`),
  `tasks.patch`, `tasks.delete`, `tasks.move`.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

- `New(base, opts)`: `base` is the API root with or without a trailing
  slash (`providers.GoogleTasksURL` has one). A nil `opts.HTTP` gets a
  client with a 30-second timeout.
- Every request asks `opts.Token` for the bearer token (a failure is
  returned wrapped) and sends `Authorization: Bearer …` and, when set,
  `User-Agent`.
- Answers: 401 is `ErrUnauthorized`, 404 `ErrNotFound`, any other non-2xx
  an `*APIError` with the status and Google's `error.message`.
- **`Lists`** follows `nextPageToken` (`maxResults=100`).
- **`Tasks(list, updatedMin)`** asks with `showCompleted=true`,
  `showHidden=true`, `showDeleted=true`, `maxResults=100`, and
  `updatedMin` (RFC 3339 in UTC) when it is not zero; it follows pages and
  keeps the API's order. Each `Task` is read from its JSON: `Completed`
  when `status` is `completed`, `CompletedAt` from `completed`, `Due` the
  first ten characters of `due` (the API sends a date at midnight UTC),
  `Updated`, links, and `Raw` the item's JSON exactly as it came
  (`jsontext.Value`).
- **`Insert`** posts the fields (and `parent`, `previous` when set) and
  returns the created task; **`Patch`** sends only the set fields: `Due`
  as `YYYY-MM-DDT00:00:00.000Z`, or JSON `null` when `""`; `Completed`
  true as `"status": "completed"`, false as `"status": "needsAction"`
  with `"completed": null`. **`Move`** posts to `…/tasks/{id}/move` with
  `parent` and `previous` when set. **`Delete`** expects 204.
- Path segments are escaped (`url.PathEscape`).

## Tests (given, do not edit)

`internal/gtasks/gtasks_test.go`: TestRead, TestWrite, TestErrors.

## Gotchas

- `encoding/json/v2` matches field names case-sensitively: tag every
  field (`json:"id"`), nested structs included.
- Close response bodies; keep functions under 60 lines.

## Out of scope

Syncing tasks into the store (a later card), OAuth, and every file not
under `touch`.

## Done when

`make accept T=0078` and `make check` pass, and only the files under
`touch` changed.
