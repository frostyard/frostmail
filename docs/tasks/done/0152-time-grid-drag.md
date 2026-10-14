---
id: "0152"
title: Dragging in the day and week views
milestone: M5.5
size: L
touch:
  - app/src/features/calendar/TimeGrid.tsx
given:
  - app/src/features/calendar/TimeGrid.drag.test.tsx
acceptance: make ui-vitest F=src/features/calendar/TimeGrid
---
# T-0152: Dragging in the day and week views

## Goal

In the day and week views (docs/specs/pim-ui.md, "Changing events in the
views"), dragging an occurrence moves it in 15-minute steps, across
days; dragging its bottom edge changes its end; dragging across empty
time draws a new event. `TimeGrid` stays presentational: it reports a
drop to its owner with the new range and writes nothing (a later card
wires the writes).

## Read first

- `docs/specs/pim-ui.md`: "Changing events in the views".
- `app/src/features/calendar/TimeGrid.tsx`: `TimeGridProps`, `EventBlock`,
  the day columns (`fieldset`s, 48 px an hour).
- `app/src/lib/eventDrag.ts` (T-0151): `stepsOf`, `daysOf`, `minuteAt`,
  `moved`, `resized`, `drawn`, `PX_PER_MINUTE`, `THRESHOLD`, `Range`.
- `app/src/features/calendar/eventStyle.ts`; `app/src/lib/eventText.ts`:
  `timeRange`.
- The given test, and `TimeGrid.test.tsx` (T-0071's), which must still
  pass.
- `docs/tasks/EXECUTOR.md`

## Contract

- **New optional props:** `canMove(occurrence)` (whether it may be
  dragged; without it nothing moves), `onMove(occurrence, range)` (an
  occurrence dropped at a new range, moved or resized) and
  `onDraw(range)` (a range drawn across empty time; without it nothing
  is drawn).
- **Pressing:** a primary-button `pointerdown` on a movable occurrence's
  block starts a press: a **resize** when it lands on the block's bottom
  6px handle (an element with a `data-resize` attribute, `aria-hidden`,
  only on movable blocks), else a **move**. A `pointerdown` on a day
  column's empty space (the `fieldset` itself) starts a **draw** there.
  The press records the pointer, the day's index, and the day column's
  `getBoundingClientRect()` (its width and top).
- **Following:** while a press is on, the grid follows `pointermove`
  and `pointerup` on `window`. A press becomes a drag once the pointer
  has moved `THRESHOLD` pixels or more on either axis, and stays one:
  - a move's range is `moved` by `daysOf(dx, width)` (kept within the
    days shown) and `stepsOf(dy)`, in the grid's `timeZone`;
  - a resize's is `resized` by `stepsOf(dy)`;
  - a draw's is `drawn` on the day pressed, from the step it began on
    (`minuteAt(y - top)`) to the step under the pointer.
- **Preview:** while dragging, an element with a `data-drag-preview`
  attribute shows the range in the day column it starts on, at 60%
  opacity, as tall as the range (0.8 px a minute, at least 18): the
  occurrence's summary (or "New Event" for a draw) and
  `timeRange(start, end, timeZone, locale)` under it. It does not take
  pointer events.
- **Dropping** (`pointerup` of a drag): a draw calls `onDraw(range)`; a
  move or resize calls `onMove(occurrence, range)`, unless the range has
  the same instants as the occurrence's (compare them as instants:
  maild's have no milliseconds), which is a click. After a drop the
  click the browser sends next does not select the occurrence; a press
  that never became a drag, or a drag back to where it began, selects it
  as a click does.
- **Escape** during a press ends it, writing nothing; the key is taken
  in the capture phase on `window` and does not reach the window's other
  listeners.

## Tests (given, do not edit)

`app/src/features/calendar/TimeGrid.drag.test.tsx`.

## Gotchas

- The test DOM has no layout; the test stubs `getBoundingClientRect` so
  every element is 100 wide and starts at the page's top-left.
- React's synthetic `onClick` follows `pointerup` on the same block:
  remember that a drop just happened, briefly, to skip it.

## Out of scope

What a drop writes, and the month view and the all-day strip (later
cards); every file not under `touch`.

## Done when

`make accept T=0152` and `make ui-check` pass, and only the files under
`touch` changed.
