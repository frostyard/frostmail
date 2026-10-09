---
id: "0077"
title: Polish the month lines and the time grid's first scroll
milestone: M4.5
size: S
touch:
  - app/src/features/calendar/MonthGrid.tsx
  - app/src/features/calendar/TimeGrid.tsx
given:
  - app/src/features/calendar/CalendarPolish.test.tsx
acceptance: make ui-vitest F="src/features/calendar"
---
# T-0077: Polish the month lines and the time grid's first scroll

## Goal

The first screenshots showed month cells too narrow for "9:30 AM S…": the
title must come first and the time give way. And the time grid opened with
the 7 AM label cut in half.

## Read first

- `docs/specs/pim-ui.md`: "Month view" (Cells) and "Day and week views"
  (Grid), as amended.
- `app/src/features/calendar/MonthGrid.tsx` and `TimeGrid.tsx`.
- The given test and the existing tests in `src/features/calendar`.
- `docs/tasks/EXECUTOR.md`

## Contract

- **Month lines:** each cell is a size container (Tailwind v4's
  `@container` class). A timed line shows the dot, then the title
  (`min-w-0 flex-1 truncate`), then the start time (`shrink-0`,
  `text-secondary`, `tabular-nums`) with the classes `hidden
  @[9rem]:inline`, so cells narrower than 9rem show only the title. The
  line's `aria-label` is unchanged.
- **First scroll:** the time grid opens scrolled to `7 * 48 - 12` px.

## Tests (given, do not edit)

`app/src/features/calendar/CalendarPolish.test.tsx`; the existing
calendar tests keep passing.

## Out of scope

Everything else, and every file not under `touch`.

## Done when

`make accept T=0077` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
