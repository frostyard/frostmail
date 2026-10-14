---
id: "0151"
title: The arithmetic of dragging events
milestone: M5.5
size: M
touch:
  - app/src/lib/eventDrag.ts
given:
  - app/src/lib/eventDrag.test.ts
acceptance: make ui-vitest F=src/lib/eventDrag
---
# T-0151: The arithmetic of dragging events

## Goal

Phase 4 lets the calendar views move and resize events by dragging
(docs/specs/pim-ui.md, "Changing events in the views"). Keep the
arithmetic out of the components, in pure functions: pixels to 15-minute
steps and whole days, a range moved or resized in the app's wall time,
a range drawn across empty time, who may drag what, and the
`EventFields` a drop writes.

## Read first

- `docs/specs/pim-ui.md`: "Changing events in the views".
- `app/src/features/calendar/TimeGrid.tsx`: the grid's scale (48 px an
  hour, so 0.8 px a minute).
- `app/src/lib/calendarDates.ts`: `zoned`, `instantAt`, `addDays`.
- `schema/rpc/calendar.yaml`: `Occurrence` (`answer` is set only when the
  user is invited), `EventFields`.
- The stub `app/src/lib/eventDrag.ts`: keep the constants, `Range` and
  every export's signature; replace the bodies and `notYet`.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

Instants are ISO strings (`toISOString()`); minutes count from the day's
midnight on the wall clock of the zone given.

- **`stepsOf(dy)`**: a vertical move of `dy` pixels as minutes, to the
  nearest whole `STEP` (`Math.round` of `dy / PX_PER_MINUTE / STEP`,
  times `STEP`).
- **`daysOf(dx, columnWidth)`**: a horizontal move as whole columns
  (`Math.round(dx / columnWidth)`); 0 when the width is not positive.
- **`minuteAt(y)`**: the step a pointer `y` pixels below a day's top is
  in (rounded down to a `STEP`), kept within the day's first and last
  steps (0 to 23:45).
- **`moved(range, days, minutes, timeZone)`**: the range's start read as
  a wall date and time in `timeZone`, moved by `days` and `minutes`
  (past midnight into the next day as needed), back to an instant
  (`instantAt`); the end keeps the range's length in time.
- **`resized(range, minutes, timeZone)`**: the end moved by `minutes` of
  wall time, but never less than one `STEP` after the start.
- **`drawn(date, from, to, timeZone)`**: the range drawn on `date` across
  two steps, in either order: from the earlier to the end of the later
  (the later plus one `STEP`, at most midnight), in `timeZone`.
- **`canMove(o, writable)`**: an occurrence may be dragged when its
  calendar is in `writable` and the user is not invited to it (no
  `answer`).
- **`dropFields(range, zone)`**: `{allDay: false, start, end, timeZone:
  zone}`.
- **`shiftedDays(o, days, appZone, zone)`**: what moving an occurrence by
  whole days writes, keeping its times: for an all-day one
  `{allDay: true, startDate, endDate}` each moved by `days`; for a timed
  one `dropFields(moved(…, days, 0, appZone), zone)`.

## Tests (given, do not edit)

`app/src/lib/eventDrag.test.ts`.

## Gotchas

- `Math.round(-2.5)` is `-2` in JavaScript, which the test expects.
- Pure functions: no `Date.now()`, no DOM.

## Out of scope

The views that use these (later cards); every file not under `touch`.

## Done when

`make accept T=0151` and `make ui-check` pass, and only the files under
`touch` changed.
