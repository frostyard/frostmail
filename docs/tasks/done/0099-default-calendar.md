---
id: "0099"
title: Use a calendar as the default from Calendar's sidebar
milestone: M4.5
size: S
touch:
  - app/src/features/calendar/CalendarSidebar.tsx
  - app/src/app/useCalendar.ts
  - app/src/app/CalendarModule.tsx
given:
  - app/src/features/calendar/CalendarSidebar.default.test.tsx
  - app/src/app/CalendarModule.default.test.tsx
acceptance: make ui-vitest F="src/features/calendar src/app/CalendarModule"
---
# T-0099: Use a calendar as the default from Calendar's sidebar

## Goal

iCloud listed a shared calendar ("Maintenance") first, so it became the
user's default, and an invitation accepted in Frostmail went there. maild
now follows the server's own default when the user has not chosen one;
the user also needs to choose. A calendar row's context menu in Calendar's
sidebar gets `Use as Default Calendar`, which calls
`account.setCollection` with `isDefault: true` (the API already takes
it, and the mock handles it).

## Read first

- `docs/specs/pim-ui.md`: "Calendar sidebar (`CalendarSidebar`)", the
  "Default calendar" bullet.
- `app/src/features/calendar/CalendarSidebar.tsx`: `CalendarRow.isDefault`
  and the prop `onMakeDefault` are declared, not used.
- `app/src/features/menu/ContextMenu.tsx` (a checkable item is a
  `menuitemcheckbox`) and its use in
  `app/src/features/calendar/ReminderPanel.tsx`.
- `app/src/app/useCalendar.ts` (the sections built from
  `account.collections`, reloaded on `account.changed`) and
  `app/src/app/CalendarModule.tsx` (`onToggle` wiring).
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`CalendarSidebar`:** with `onMakeDefault`, right-clicking a row
  (preventing the browser's menu), or the `ContextMenu` key or Shift+F10
  on the focused row, opens a `ContextMenu` at the pointer (or under the
  row) with one item, id `default`, label `Use as Default Calendar`,
  `checked` when the row `isDefault`, `disabled` when it `isDefault` or is
  `readOnly`. Choosing it calls `onMakeDefault(id)`; the menu closes. The
  row's click still toggles it. Without `onMakeDefault` nothing changes.
- **`useCalendar`:** each row carries the collection's `isDefault`.
- **`CalendarModule`:** passes `onMakeDefault` calling
  `client.account.setCollection({ id, isDefault: true })`, logging a
  failure as `onToggle` does.

## Tests (given, do not edit)

`CalendarSidebar.default.test.tsx` and `CalendarModule.default.test.tsx`.
The other calendar tests must keep passing.

## Gotchas

- `ContextMenu` keeps the items it opened with; open a new one for each
  right-click.
- The sidebar rows are `button`s with `role="checkbox"`; don't change
  their names or roles.

## Out of scope

maild, the People and Tasks sidebars, and every file not under `touch`.

## Done when

`make accept T=0099` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
