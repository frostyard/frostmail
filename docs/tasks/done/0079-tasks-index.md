---
id: "0079"
title: Read VTODOs and index tasks
milestone: M4.5
size: M
touch:
  - internal/calendar/todo.go
  - internal/store/tasks.go
given:
  - internal/calendar/todo_test.go
  - internal/store/tasks_test.go
acceptance: go test ./internal/calendar ./internal/store -count=1
---
# T-0079: Read VTODOs and index tasks

## Goal

Tasks come from Google (JSON) and from CalDAV task lists (VTODO). Read a
VTODO into the fields Frostmail keeps, and store and query the `tasks`
index that both feed (docs/design/pim.md, Tasks).

## Read first

- `docs/design/pim.md`: "Tasks" (the index and the order), "Storage".
- `internal/store/migrations/0007_pim.sql`: `tasks`, `collections`,
  `account_services`.
- `internal/calendar/event.go` and `zone.go`: `timeReader` (it reads
  DATE, UTC, TZID and floating values), `propertyValue`, `propertyText`.
- `internal/store/events.go` and `people.go`: query style, how shown
  collections are selected (enabled, and the account's service on).
- The stubs `internal/calendar/todo.go` and `internal/store/tasks.go`
  (keep every exported name and signature), and the given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

### `calendar.ParseTodo(raw, opts)`

The first VTODO of the first VCALENDAR; an error when there is no
VCALENDAR or no VTODO, or the source does not parse.

- `UID`, `Summary` and `Description` (text unescaped), `SortOrder`
  (`X-APPLE-SORT-ORDER`, trimmed).
- `ParentUID`: the first `RELATED-TO` whose `RELTYPE` is `PARENT` or
  absent.
- `Due`: a DATE as it is; a DATE-TIME (UTC, TZID or floating, resolved as
  events' times are, floating in `opts.Local`) as its date in
  `opts.Local` (nil: `time.Local`).
- `Completed` when `STATUS:COMPLETED` or a `COMPLETED` time exists;
  `CompletedAt` that time in UTC.

### Store (`internal/store/tasks.go`)

- **`IndexTask(objectID, r)`** inserts or replaces the object's row with
  every stored field (`CompletedAt` as `FormatTime` or NULL; `GmThrID` 0
  as NULL). **`RemoveTask`** deletes it (none is not an error).
- **`Tasks(f)`**: tasks of `tasklist` collections that are enabled, of
  accounts whose tasks service is on; `ListID` keeps one list;
  without `Completed`, completed tasks are left out; `DueBefore` keeps
  tasks with a due date before it. Each row has `ListID`, `AccountID`,
  `ReadOnly` (the list or the account is) and `ParentID` (the object of
  the task in the same list whose UID is `ParentUID`; 0 when none),
  whether or not the parent passed the filter.
- **Order:** lists by collection position, then ID. In a list, the
  top-level tasks — those without a parent, and those whose parent is not
  among the rows returned — by position (text; tasks without one after
  those with one), then title, then ID; each followed by its subtasks in
  the same order. (Google has one level of subtasks.)
- **`Task(id)`**: one row as above, whatever its list's state;
  `ErrNotFound` when there is none.

## Tests (given, do not edit)

`internal/calendar/todo_test.go`, `internal/store/tasks_test.go`.

## Gotchas

- Order in Go after one query; compute `ParentID` in SQL (a subquery) or
  from the rows.
- Keep functions under 60 lines.

## Out of scope

Syncing tasks, the tasks API, and every file not under `touch`.

## Done when

`make accept T=0079` and `make check` pass, and only the files under
`touch` changed.
