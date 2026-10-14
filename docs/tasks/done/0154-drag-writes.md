---
id: "0154"
title: What a drag writes
milestone: M5.5
size: M
touch:
  - app/src/app/CalendarModule.tsx
given:
  - app/src/app/CalendarModule.drag.test.tsx
acceptance: make ui-vitest F=src/app/CalendarModule
---
# T-0154: What a drag writes

## Goal

T-0152 and T-0153 made the calendar views report drags. Now Calendar
writes them (docs/specs/pim-ui.md, "Changing events in the views"): a
moved, resized or shifted occurrence becomes one `calendar.update` in
the event's zone, after the scope sheet for an occurrence of a series; a
range drawn across empty time opens the event editor on a new event.
Each write's restore goes on Calendar's undo list (`pimUndo`).

## Read first

- `docs/specs/pim-ui.md`: "Changing events in the views", "Scope sheet".
- `app/src/app/CalendarModule.tsx`: `CalendarView` (the props it gives
  `TimeGrid` and `MonthGrid`), the scope sheet T-0146 wired for deletes.
- `app/src/features/calendar/TimeGrid.tsx` and `MonthGrid.tsx`:
  `canMove`, `onMove`, `onDraw`, `onMoveDays`.
- `app/src/lib/eventDrag.ts`: `canMove`, `dropFields`, `shiftedDays`,
  `Range`.
- `app/src/app/eventWindow.ts` (`openEvent`), `app/src/app/pimUndo.ts`
  (`rememberRestore`).
- The given test, and the other `CalendarModule` tests, which must still
  pass.
- `docs/tasks/EXECUTOR.md`

## Contract

- **Who may drag:** an occurrence in a writable calendar (one of the
  sections' calendars that is not read-only) that the user is not invited
  to: `canMove(o, writable)`. The day and week views get `canMove`,
  `onMove`, `onDraw` and `onMoveDays`; the month view `canMove` and
  `onMoveDays`.
- **A drop** (`onMove(o, range)`, or `onMoveDays(o, days)`) first reads
  the event: `calendar.event {id: o.eventId, recurrenceId?}` (no
  `recurrenceId` when the occurrence's is empty). Its fields are
  `dropFields(range, zone)`, or `shiftedDays(o, days, appZone, zone)`,
  where `zone` is the event's `timeZone`, else the app's (`data.timeZone`).
  - For an occurrence of a series (the event is `recurring` and has a
    `recurrenceId`), the scope sheet asks first (`action="change"`,
    `first` from the event); its answer sends `calendar.update {id,
    recurrenceId, scope, fields}`, and Cancel writes nothing.
  - Otherwise `calendar.update {id, fields}` at once.
  - A successful update's `restoreId` goes to
    `rememberRestore({module: "calendar", restoreId})`. Failures are
    logged with `console.warn`.
- **A drawn range** (`onDraw(range)`) opens
  `openEvent({kind: "new", allDay: false, start, end, startDate: "",
  endDate: ""})` and writes nothing.

## Tests (given, do not edit)

`app/src/app/CalendarModule.drag.test.tsx`.

## Gotchas

- The test stubs `getBoundingClientRect` (every element 100 wide), as
  the grids' own drag tests do.
- The view refetches after `calendar.changed`; the moved occurrence
  shows in its new place without more work.

## Out of scope

Ctrl+Z, and the other writes' undo entries (T-0155); every file not
under `touch`.

## Done when

`make accept T=0154` and `make ui-check` pass, and only the files under
`touch` changed.
