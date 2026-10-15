---
id: "0153"
title: Dragging by days in the month view and the all-day strip
milestone: M5.5
size: M
touch:
  - app/src/features/calendar/MonthGrid.tsx
  - app/src/features/calendar/TimeGrid.tsx
given:
  - app/src/features/calendar/MonthGrid.drag.test.tsx
acceptance: make ui-vitest F="src/features/calendar/MonthGrid src/features/calendar/TimeGrid"
---
# T-0153: Dragging by days in the month view and the all-day strip

## Goal

In the month view and the day and week views' all-day strip
(docs/specs/pim-ui.md, "Changing events in the views"), dragging an
occurrence to another day moves it by whole days, keeping its times. Use
the platform's drag and drop, which cancels on Escape by itself. The
grids report a drop to their owner and write nothing; a later card wires
the writes.

## Read first

- `docs/specs/pim-ui.md`: "Changing events in the views".
- `app/src/features/calendar/MonthGrid.tsx`: `MonthCell`, the occurrence
  buttons in it.
- `app/src/features/calendar/TimeGrid.tsx`: `AllDayStrip` (its grid of
  day columns and bars), and the props T-0152 added (`canMove`).
- `app/src/app/DragDrop.test.tsx` (T-0121's): how the app's tests drive
  drag and drop with a stub `DataTransfer`.
- The given test, and `MonthGrid.test.tsx`, `TimeGrid.test.tsx` and
  `TimeGrid.drag.test.tsx`, which must still pass.
- `docs/tasks/EXECUTOR.md`

## Contract

The drag's data type is `application/x-frostmail-occurrence`, its value
`<eventId>:<recurrenceId>`, `effectAllowed` `move`. Drops of other data
(a message from Mail) are not taken: no `preventDefault` on their
`dragover` and nothing on their `drop`.

### `MonthGrid`

- New optional props `canMove(occurrence)` and
  `onMoveDays(occurrence, days)`. An occurrence's button is `draggable`
  only when both are given and `canMove` says yes; otherwise it is
  `draggable="false"`.
- `dragstart` remembers the occurrence and the cell's date it left (a
  multi-day occurrence shows in several cells; the one taken counts).
- Over a cell, `dragover` of our type calls `preventDefault` (the cell
  takes the drop), sets `dropEffect` to `move`, and marks the cell with
  a `data-drop-target` attribute (and a 2px inset accent ring); only the
  cell under the drag is marked.
- `drop` on a cell calls `onMoveDays(occurrence, days)` with the whole
  days from the cell it left to this one, unless that is 0, and clears
  the drag. `dragend` (a drop elsewhere, or Escape) clears the drag and
  the mark without a call; a later `drop` then does nothing.
- Export `daysBetween(from, to)`, the whole days between two dates.

### The all-day strip (`TimeGrid`)

- A new optional prop `onMoveDays(occurrence, days)`. A bar is
  `draggable` only when it and `canMove` are given and `canMove` says
  yes.
- The strip's grid of bars carries a `data-all-day` attribute.
- Drag events carry no position in every engine, so the day a bar was
  taken from is the column under the `pointerdown` before the drag:
  `Math.floor` of the pointer's x within the strip's
  `getBoundingClientRect()`, as a share of its width, times the days
  shown (kept within them); a drag without a press counts from the bar's
  first day.
- While a bar is dragged, the strip lays a row of drop cells over
  itself, one per day shown, each with a `data-drop-day` attribute set to
  its date. `dragover` of our type on a cell takes the drop as above
  (marking it); `drop` calls `onMoveDays(occurrence, days)` with the
  cell's day less the day taken, unless that is 0. The cells go away when
  the drag ends, dropped or not.

## Tests (given, do not edit)

`app/src/features/calendar/MonthGrid.drag.test.tsx`.

## Gotchas

- The test DOM's drag events have no `clientX`; that is why the strip
  reads the press.
- Biome asks for a reason on a plain element with drag handlers: a
  `biome-ignore lint/a11y/noStaticElementInteractions` comment saying the
  cells exist only for a pointer drag (the keyboard moves events in the
  editor).

## Out of scope

What a drop writes (T-0154); every file not under `touch`.

## Done when

`make accept T=0153` and `make ui-check` pass, and only the files under
`touch` changed.
