---
id: "0080"
title: Sync Google Tasks
milestone: M4.5
size: L
touch:
  - internal/pimsync/gtasks.go
  - internal/pimsync/pimsync.go
given:
  - internal/pimsync/gtasks_test.go
acceptance: go test ./internal/pimsync -count=1
---
# T-0080: Sync Google Tasks

## Goal

A Google account with Tasks on syncs its task lists and tasks into the
store, where the tasks index (T-0079) serves them: lists as `tasklist`
collections, tasks as `gtask` objects with their JSON, changes since the
last pass only.

## Read first

- `docs/design/pim.md`: "Services and sync" (the Tasks loop, Writes) and
  "Tasks" (Google lists and tasks, the index).
- `internal/gtasks/gtasks.go` (done in T-0078) and `gtaskstest`.
- `internal/pimsync/pimsync.go` (`pass`, `service`, `serviceOnce`,
  `scope`, `Config.Tokens`) and `dav.go` (how a DAV pass stores objects,
  leaves pending ones alone and emits events).
- `internal/store`: `ReplaceCollections`, `SetCollectionSync`,
  `PendingHrefs`, `ObjectByHref`, `PutObject`, `DeleteObjects`,
  `IndexTask`; `providers.ForKind(...).DAV.Tasks`.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

- `serviceOnce`'s tasks case: when `providers.ForKind(acct.Kind).DAV.Tasks`
  is `"google"`, sync Google Tasks (in `gtasks.go`); otherwise do nothing
  yet (CalDAV task lists are a later card). The scope check stays first.
- `service` also retries once with a new token (`Tokens.Invalidate`) when
  the error is `gtasks.ErrUnauthorized`, as it does for DAV's.
- **The pass:** a `gtasks.Client` on the service's URL with
  `Config.HTTP`, `Config.UserAgent`, and `Config.Tokens.AccessToken` for the
  account. `Lists`, then `ReplaceCollections(kind tasklist)` with each
  list's ID as the href and title as the name (lists gone from the answer
  go, with their tasks). Then, for each enabled list, `Tasks` with
  `updatedMin` from the collection's sync token (RFC 3339, empty for all),
  and in one transaction per list:
  - a task with a pending change (`PendingHrefs`) is left alone;
  - a deleted task removes its object (`DeleteObjects`);
  - a task whose JSON equals the stored object's `raw` is left alone;
  - any other is stored (`PutObject`: kind `gtask`, href and UID the task's
    ID, `raw` its JSON) and indexed (`IndexTask`: UID, parent UID, title,
    notes, due, completion and when, position, and `GmThrID` from the first
    link of type `email`, whose last path segment is the thread's ID in
    hex);
  - the sync token becomes the newest `updated` seen (RFC 3339 nanoseconds,
    UTC), or stays;
  - `tasks.changed` (`api.TasksChanged`) is emitted when an object was
    stored or removed, and only then.

## Tests (given, do not edit)

`internal/pimsync/gtasks_test.go`: TestGoogleTasksSync,
TestGoogleTasksPendingLeftAlone, TestGoogleTasksAuth. The existing
pimsync tests keep passing.

## Gotchas

- `updatedMin` is inclusive: the newest task comes again each pass; it
  must not count as a change.
- Keep functions under 60 lines.

## Out of scope

Writing tasks (`tasks.*` ops: the planner's), CalDAV task lists, the tasks
API, and every file not under `touch`.

## Done when

`make accept T=0080` and `make check` pass, and only the files under
`touch` changed.
