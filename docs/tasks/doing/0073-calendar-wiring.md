---
id: "0073"
title: Wire the Calendar module into the main window
milestone: M4.5
size: L
touch:
  - app/src/app/CalendarModule.tsx
  - app/src/app/useCalendar.ts
  - app/src/app/MainWindow.tsx
  - app/src/app/ToolbarContainer.tsx
  - app/src/app/SidebarContainer.tsx
  - app/src/app/PeopleModule.tsx
given:
  - app/src/app/CalendarModule.test.tsx
  - app/src/app/PeopleModule.test.tsx
acceptance: make ui-vitest F="src/app"
---
# T-0073: Wire the Calendar module into the main window

## Goal

Ctrl+2 (and the module bar) shows the Calendar module: the calendars
sidebar, the day, week or month view of the selected date, and the event
pane, fed by maild (or MockTransport) and kept current by its events,
with the calendar toolbar and keys.

## Read first

- `docs/specs/pim-ui.md`: "Modules", "Calendar module" and all its
  subsections, especially "Calendar behavior".
- `app/src/app/MainWindow.tsx`, `PeopleModule.tsx`, `usePeople.ts`
  (`useRefresh`: the pattern to follow), `ToolbarContainer.tsx`,
  `SidebarContainer.tsx`.
- The components in `app/src/features/calendar/` (T-0071), the toolbar's
  calendar mode, the calendar commands and `useUI`'s calendar state
  (T-0072), and `app/src/lib/calendarDates.ts` (T-0070).
- `app/src/rpc/mock/fixture.ts` (`fixtureCalendar`): what the tests see.
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

### `useCalendar.ts`

- **`useCalendarFrame()`**: `{ timeZone, locale, weekStart, date, today,
  now }` — the zone from `Intl.DateTimeFormat().resolvedOptions()`, the
  locale `navigator.language`, `localeWeekStart(locale)`, `today(zone,
  now)`, `date` = `useUI().calendarDate` or today when it is "", and
  `now` a `Date` that a timer renews every minute. The toolbar container
  and the module use it.
- **`useCalendar()`** loads, while the module is shown, as `usePeople`
  does (cancel stale answers; refetch on events after 100 ms):
  - the calendars: `account.collections({ kind: "calendar" })`, refetched
    on `account.changed`, into `CalendarSection`s titled by each account's
    email (accounts from `useMail`), read-only when the calendar or the
    account is, and a `colors` map by calendar ID;
  - the view's occurrences: `calendar.range` over
    `viewRange(view, date, weekStart)` with `timeZone`, refetched on
    `calendar.changed`;
  - the small month's busy days: `busyDates` of a `calendar.range` over
    `monthGrid(date, weekStart)`, refetched likewise;
  - the selected event: `calendar.event({ id, recurrenceId })` for
    `calendarSelected` (no `recurrenceId` key when it is ""), refetched on
    `calendar.changed`; a failure (gone) clears the selection.

### `CalendarModule.tsx`

- Panes as the spec says: the sidebar (`CalendarSidebar`, then
  `ModuleBar`) at `ui.sidebarWidth` with the sidebar `Splitter` (the
  `resizeSidebar`/`endDrag` widths MainWindow already passes People);
  the view, `TimeGrid` for day (one day) and week (seven), `MonthGrid`
  for month (`monthGrid(date, weekStart)`, `lines` fitted to the cell
  height with a `ResizeObserver`, 4 when unmeasured, at least 2); the
  `EventPane`, 320 wide. Hidden sidebar: no sidebar pane.
- Callbacks: selecting an occurrence → `selectOccurrence(key, its
  date in the zone)` (`zoned(start).date`, or `startDate` for all-day);
  a date (small month, month cell) → `setCalendarDate`; show a day →
  `setCalendarDate` and `setCalendarView("day")`; toggling a calendar →
  `account.setCollection({ id, enabled })`; a person in the pane → the
  contact card as the reader opens it (`ContactCardContainer`'s way), or
  nothing if that needs files outside `touch`.
- `CalendarHandle` (forwardRef): `focus(pane)` and `scroll(hours)` (the
  time grid's scroller; find it in the view pane, e.g. by a
  `data-scroller` attribute you set on a wrapper or with
  `querySelector(".overflow-y-auto")`).
- Clicking in a pane sets `calendarFocus` to it.

### `MainWindow.tsx`

- `module === "calendar"` renders `CalendarModule`; `showCalendar` sets
  the module (as `showPeople` does); the module bar lists `["mail",
  "calendar", "people"]` everywhere (`SidebarContainer`,
  `PeopleModule`, `CalendarModule`).
- In the Calendar module the keys do: `dayView`/`weekView`/`monthView` →
  `setCalendarView`; `today` → `setCalendarDate(today)`;
  `previousPeriod`/`nextPeriod` → `step(view, date, ∓1)`; `left`/`right`
  → ±1 day; `previous`/`next` (↑ ↓) → ±7 days in the month view, else
  `scroll(∓1)`; `open` (Enter) → the day view; `escape` →
  `selectOccurrence(null)`; `nextPane`/`previousPane` cycle sidebar,
  view ("list") and pane ("reader") through `calendarFocus` as People
  does. Other commands are not handled there (Ctrl+N, Delete, …).
  `focusSearch` does nothing in Calendar.

### `ToolbarContainer.tsx`

In Calendar: `mode="calendar"`, `calendar={{ view, title:
viewTitle(view, date, weekStart, locale), paneWidth: 320 }}`, and the
commands `today`, `previousPeriod`, `nextPeriod` and `calendarView` as the
keys do. Mail and People are unchanged.

## Tests (given, do not edit)

- `app/src/app/CalendarModule.test.tsx`: the module through `MainWindow`
  with MockTransport (the fixture's clock 2026-10-08 12:00 UTC; tests run
  in UTC; `calendarDate` set to 2026-10-08).
- `app/src/app/PeopleModule.test.tsx`, amended: the module bar now has
  Calendar (not Tasks).

## Gotchas

- Keep `MainWindow.tsx` readable: put the calendar key handling in a
  `calendarCommand(command, …)` function beside `moduleCommand`, as
  People's is.
- The time grid scrolls to 7:00 on mount; do not remount it on every
  refetch (keep keys stable).
- Every existing app test must keep passing (`make ui-check`).

## Out of scope

Upcoming rows on contact cards and the person pane (T-0074), reminders,
search in Calendar, creating or editing events, and every file not under
`touch`.

## Done when

`make accept T=0073` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
