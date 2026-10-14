---
id: "0150"
title: New Event, Ctrl+N, and the calendar view's Enter and Delete
milestone: M5.5
size: S
touch:
  - app/src/features/toolbar/Toolbar.tsx
  - app/src/app/ToolbarContainer.tsx
  - app/src/app/MainWindow.tsx
given:
  - app/src/app/CalendarModule.keys.test.tsx
acceptance: make ui-vitest F="src/app/CalendarModule src/features/toolbar"
---
# T-0150: New Event, Ctrl+N, and the calendar view's Enter and Delete

## Goal

Make events creatable from Calendar, and the view's keys edit and delete
(docs/specs/pim-ui.md, "Calendar toolbar" and "Calendar behavior"): a
**New Event** button in the toolbar and Ctrl+N open the event editor on
a new event on the selected date; Enter opens the selected occurrence in
the editor; Delete and Backspace delete it as the pane's Delete does.

## Read first

- `docs/specs/pim-ui.md`: "Calendar toolbar", "Calendar behavior" (Keys),
  "Event editor window" (Opening).
- `app/src/features/toolbar/Toolbar.tsx`: `ToolbarCommand`,
  `ToolbarCalendar`, `CalendarSegment`, `ToolbarButton`.
- `app/src/app/ToolbarContainer.tsx`: `useToolbarCommand` and the
  calendar props; `app/src/app/MainWindow.tsx`: `calendarCommand`, the
  key handler, the `ToolbarContainer` element.
- `app/src/app/CalendarModule.tsx`: `CalendarHandle.edit()` and
  `remove()` (T-0146).
- `app/src/app/eventWindow.ts`: `openEvent`, `newEventAt`;
  `app/src/app/useCalendar.ts`: `CalendarData` (`sections`, `today`,
  `now`, `timeZone`).
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

- **Toolbar:** `ToolbarCommand` gains `{kind: "newEvent"}` and
  `ToolbarCalendar` an optional `canCreate`. The calendar segment begins
  with a `ToolbarButton` labeled "New Event" with the shortcut "Ctrl+N"
  (title "New Event (Ctrl+N)") and a 16px `Plus`, disabled when
  `canCreate` is false, sending `newEvent`. The rest of the segment stays.
- **ToolbarContainer:** a new optional prop `onNewEvent`. In Calendar,
  `newEvent` calls it, and `canCreate` is whether it is given.
- **MainWindow:**
  - A calendar can take a new event when some section has a calendar
    that is not read-only. `ToolbarContainer` gets `onNewEvent` only
    then.
  - A **new event** opens `openEvent(newEventAt(date, today, now,
    timeZone))` with the selected date (`calendarDate`, else today) and
    the calendar data's `today`, `now` and `timeZone`; a failure is logged
    with `console.warn`.
  - In `calendarCommand`: `compose` (Ctrl+N) starts a new event when a
    calendar can take one; `delete` (Delete, Backspace) calls the
    calendar's `remove()` when an occurrence is selected, and is
    otherwise not handled; `open` (Enter, with the view focused) calls
    `edit()` when an occurrence is selected and it returns true, and
    otherwise shows the selected date's day as before.

## Tests (given, do not edit)

`app/src/app/CalendarModule.keys.test.tsx`; the existing CalendarModule
and Toolbar tests must still pass.

## Gotchas

- The test's week is in the past for its clock, so a new event on one of
  its days starts at 9:00 AM (`newEventAt`).
- In the tests the app is not in Tauri; `openEvent` opens a tab with
  `window.open`, which they spy on.

## Out of scope

The editor window (T-0149); undo (Phase 4); every file not under
`touch`.

## Done when

`make accept T=0150` and `make ui-check` pass, and only the files under
`touch` changed.
