---
id: "0082"
title: Patch VTODOs and write new ones
milestone: M4.5
size: M
touch:
  - internal/calendar/todopatch.go
given:
  - internal/calendar/todopatch_test.go
acceptance: go test ./internal/calendar -count=1
---
# T-0082: Patch VTODOs and write new ones

## Goal

The tasks API changes CalDAV tasks by patching their source, which pimsync
then puts back on the server (docs/design/pim.md, Tasks: Writes). Patch a
VTODO's title, notes, due date and completion while leaving every other
byte as the server sent it (ADR-0018), and write new tasks.

## Read first

- ADR-0018 ("keep sources as sent"); `docs/design/pim.md`: "Tasks".
- `internal/contentline`: `Parse` (each property's `Start`/`End` bytes,
  a component's `End`), `Apply` and `Edit`, `Fold`, `EscapeText`,
  `LineEnding`.
- `internal/calendar/todo.go` (`ParseTodo`, which the tests read the
  results back with) and the stub `todopatch.go` (keep its exported names
  and signatures; replace the bodies and `errPatchNotYet`).
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

### `PatchTodo(src, c, now)`

The first VTODO of the first VCALENDAR (an error when there is none, or
the source does not parse). Every property the change sets replaces the
first line of that name in the VTODO (removing any repeats), or, when the
VTODO has none, is inserted before its `END:VTODO` line (after any
VALARM, never inside one). Lines are folded at 75 octets with the source's
own line ending. Nothing else changes.

- Always: `DTSTAMP` and `LAST-MODIFIED` become `now` in UTC
  (`20261009T143000Z`).
- `Summary`: `SUMMARY:<escaped text>`.
- `Description`: `DESCRIPTION:<escaped text>`, or removed when "".
- `Due`: `DUE;VALUE=DATE:YYYYMMDD` (dropping a time and `TZID`), or
  removed when "".
- `Completed` true: `STATUS:COMPLETED`, `COMPLETED:<now>`,
  `PERCENT-COMPLETE:100`; false: `STATUS:NEEDS-ACTION`, and `COMPLETED` and
  `PERCENT-COMPLETE` removed.

### `NewTodo(uid, c, parentUID, now)`

CRLF lines, in this order: `BEGIN:VCALENDAR`, `VERSION:2.0`,
`PRODID:-//Frostyard//Frostmail//EN`, `BEGIN:VTODO`, `UID`, `DTSTAMP`,
`CREATED`, `LAST-MODIFIED` (all `now`), `SUMMARY`, `DESCRIPTION` (when set
and not empty), `DUE;VALUE=DATE` (when set and not empty), the status
lines (`STATUS:NEEDS-ACTION`, or the three completed ones),
`RELATED-TO;RELTYPE=PARENT:<parentUID>` (when set), `END:VTODO`,
`END:VCALENDAR`.

## Tests (given, do not edit)

`internal/calendar/todopatch_test.go`.

## Gotchas

- Edits must not overlap: collect them and `contentline.Apply` once.
- Insert at the start of the `END:VTODO` line, found from the component's
  `End`.
- Keep functions under 60 lines.

## Out of scope

The tasks API's use of these (the planner's), and every file not under
`touch`.

## Done when

`make accept T=0082` and `make check` pass, and only the files under
`touch` changed.
