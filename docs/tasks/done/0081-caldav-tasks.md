---
id: "0081"
title: Sync CalDAV task lists
milestone: M4.5
size: M
touch:
  - internal/pimsync/dav.go
  - internal/pimsync/index.go
  - internal/pimsync/ops.go
  - internal/pimsync/pimsync.go
given:
  - internal/pimsync/caldav_tasks_test.go
acceptance: go test ./internal/pimsync -count=1
---
# T-0081: Sync CalDAV task lists

## Goal

Accounts whose tasks live on CalDAV (iCloud, Nextcloud, Radicale) sync
their task lists with the DAV pass that already serves address books and
calendars: calendars that support VTODO become `tasklist` collections whose
VTODOs fill the tasks index (docs/design/pim.md, Tasks: CalDAV lists).

## Read first

- `docs/design/pim.md`: "Services and sync", "Tasks".
- `internal/pimsync/dav.go`, `index.go`, `ops.go` and `pimsync.go`
  (`serviceOnce`'s tasks case).
- `internal/calendar/todo.go` (`ParseTodo`), `internal/store/tasks.go`
  (`IndexTask`, `RemoveTask`).
- The given test, and `dav_test.go`'s `newEnv`.
- `docs/tasks/EXECUTOR.md`

## Contract

- `syncDAV` takes the collection kind it stores
  (`syncDAV(ctx, c, kind, collectionKind, want)`); contacts pass
  `addressbook`, the calendar `calendar`. `indexDAV` and `deleteDAV` take
  the `store.Collection` instead of its ID, and `emitDAV` the collection's
  kind: `people.changed` for address books, `calendar.changed` for
  calendars, `tasks.changed` for task lists.
- `serviceOnce`'s tasks case, for an account that is not Google's: the
  tasks service's own home (`p.home(ctx, s, davx.Calendars)`), then
  `syncDAV(…, davx.Calendars, CollectionKindTasklist, want)` keeping the
  collections whose components include `VTODO`.
- **Indexing a task list's object:** `calendarMetadata`, then
  `PutObject` (every object is kept, so its ETag is known); a `vtodo`
  object is read with `calendar.ParseTodo` (`Options{Local: cfg.Local}`) and
  indexed with `IndexTask` (UID, parent UID, summary as title, description
  as notes, due, completion and when, and `SortOrder` as the position);
  any other object, or a VTODO that does not parse (record its
  `parse_error` and store it as `other`), has its task removed
  (`RemoveTask`). Task lists index no events, and calendars no tasks.
- **Replay:** a change in a task list is written with the calendar DAV
  kind under the tasks service (`replayOne`'s mapping), and a re-fetched
  object is indexed as its collection's kind.

## Tests (given, do not edit)

`internal/pimsync/caldav_tasks_test.go`; every existing pimsync test keeps
passing.

## Out of scope

Patching VTODO sources and the tasks API's CalDAV writes (later), and
every file not under `touch`.

## Done when

`make accept T=0081` and `make check` pass, and only the files under
`touch` changed.
