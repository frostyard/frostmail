---
id: "0091"
title: Polish Tasks and the To-Do bar
milestone: M4.5
size: S
touch:
  - app/src/lib/eventText.ts
  - app/src/app/ToDoBarContainer.tsx
  - app/src/features/toolbar/Toolbar.tsx
  - app/src/app/ToolbarContainer.tsx
  - app/src/features/tasks/TaskPane.tsx
given:
  - app/src/lib/eventText.next.test.ts
  - app/src/features/toolbar/Toolbar.tasks-create.test.tsx
  - app/src/features/tasks/TaskPane.list.test.tsx
acceptance: make ui-vitest F="src/lib/eventText.next.test.ts src/features/toolbar src/features/tasks src/app"
---
# T-0091: Polish Tasks and the To-Do bar

## Goal

Three fixes the first screenshots showed: the To-Do bar's Upcoming lists
cancelled events, the toolbar's New Task does nothing where no task can
be made, and the task pane's List line wraps.

## Read first

- `docs/specs/pim-ui.md`: "Tasks toolbar", "Task pane" (the List row),
  "To-Do bar" (Upcoming).
- The stub `nextOccurrences` in `app/src/lib/eventText.ts`;
  `app/src/app/ToDoBarContainer.tsx` (its Upcoming filter, which this
  replaces); `app/src/features/toolbar/Toolbar.tsx` (`ToolbarTasks`,
  `TasksSegment`), `app/src/app/ToolbarContainer.tsx`;
  `app/src/features/tasks/TaskPane.tsx` (`Field`, the List row).
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

- **nextOccurrences** (keep its signature): the occurrences in their
  order whose status is not `cancelled` and whose end is after `now` (an
  all-day one ends at `dayStart(endDate, timeZone)`), at most `limit`.
  `ToDoBarContainer` uses it for Upcoming (limit 5).
- **ToolbarTasks** gains `canCreate?: boolean` (absent means true); when
  false the New Task button is `disabled`. `ToolbarContainer` passes
  false for Flagged Mail and for a list that is read-only (its section's
  list `readOnly` in the tasks data).
- **TaskPane's List row:** the `dd`'s first child is one element with
  the class `truncate`, its `title` "<list> · <account>", holding the
  name and the tertiary " · <account>".

## Tests (given, do not edit)

`app/src/lib/eventText.next.test.ts`,
`app/src/features/toolbar/Toolbar.tasks-create.test.tsx`,
`app/src/features/tasks/TaskPane.list.test.tsx`; the other tests must keep
passing.

## Out of scope

Everything else, and every file not under `touch`.

## Done when

`make accept T=0091` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
