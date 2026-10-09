---
id: "0076"
title: Build the reminder window
milestone: M4.5
size: L
touch:
  - app/src/lib/reminderText.ts
  - app/src/features/calendar/ReminderPanel.tsx
  - app/src/app/reminders.ts
  - app/src/app/RemindersWindow.tsx
  - app/src/app/App.tsx
  - app/src/app/MainWindow.tsx
  - app/src/rpc/mock/calendar.ts
given:
  - app/src/lib/reminderText.test.ts
  - app/src/features/calendar/ReminderPanel.test.tsx
  - app/src/rpc/mock/reminders.test.ts
  - app/src/app/RemindersWindow.test.tsx
  - app/src/app/ReminderRaise.test.tsx
acceptance: make ui-vitest F="src/lib/reminderText.test.ts src/features/calendar src/rpc/mock src/app"
---
# T-0076: Build the reminder window

## Goal

maild fires reminders and sends `calendar.reminders` while the app is
connected (`internal/reminders`). Show them in a small window of their own,
raised over the app, with Snooze, Dismiss, Dismiss All, and opening the
occurrence in Calendar.

## Read first

- `docs/specs/pim-ui.md`: "Reminder window" (all of it).
- `schema/rpc/calendar.yaml`: `Reminder`, `reminders`, `snooze`,
  `dismiss`, the `reminders` event.
- The stubs `app/src/lib/reminderText.ts`,
  `app/src/features/calendar/ReminderPanel.tsx`, `app/src/app/reminders.ts`
  and `app/src/app/RemindersWindow.tsx`: keep their exported names, types
  and signatures.
- `app/src/app/settings.ts` and `App.tsx` (how the settings window is
  opened and routed: the reminder window works the same way; the Rust
  command `open_reminders` exists), `app/src/app/openMessage.ts` (how the
  main window watches requests), `MainWindow.tsx` (`useWindowEvents`).
- `app/src/app/useCalendar.ts` (`useCalendarFrame`, `useCalendarColors`,
  `openOccurrence`), `app/src/lib/calendarDates.ts`, `eventText.ts`,
  `app/src/features/calendar/eventStyle.ts`,
  `app/src/features/menu/ContextMenu.tsx` (the Snooze menu).
- `app/src/rpc/mock/calendar.ts` (`MockReminder`, already in the types)
  and `fixture.ts` (`fixtureCalendar`'s two reminders, seeded with
  `reminders: true`).
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

- **`reminderText.ts`**: `reminderWhen` as the spec's "When" says (days
  in the zone; "Mon, Oct 12" is `weekday`/`month` short and `day`,
  formatted from the date's UTC midnight in UTC; times `hour: "numeric",
  minute: "2-digit"` in the zone). `snoozeChoices(now, zone)`: 5, 10 and 15
  minutes and 1 hour after `now`, and "Tomorrow" at 9:00 the next day in
  the zone (`dayStart` of tomorrow plus nine hours).
- **`ReminderPanel`**: the spec's window content: the title strip with
  an `h1` "Reminders" (`data-tauri-drag-region`) and a Close button; a
  `ul` named "Reminders" with an `li` per reminder setting
  `--event-color` (`calendarColor(colors.get(calendarId) ?? "")`), whose
  title is a `button` named by the title ("No Title") calling `onOpen`,
  the when-and-where line, a "Snooze" button opening a `ContextMenu` of
  `snoozeChoices(now, timeZone)` (choosing one calls `onSnooze(id,
  until)`), and a "Dismiss" button calling `onDismiss([id])`; the footer's
  "Dismiss All" (two or more) calls `onDismiss(all ids)`. Escape on the
  panel calls `onClose`.
- **`reminders.ts`**:
  - `openReminders()`: in Tauri `invoke("open_reminders")`, else
    `window.open(`${location.pathname}#/reminders`, "reminders")`.
  - `openOccurrenceInMain(request)`: in Tauri `emitTo("main",
    "open-occurrence", request)`; else post `{ kind: "open-occurrence",
    ...request }` on a `BroadcastChannel("frostmail")`.
  - `watchReminders(client)`: ask `calendar.reminders` once and open the
    window when it has any; on each `calendar.reminders` event with
    `count > 0`, open it. Returns the cleanup.
  - `watchOccurrenceRequests()`: listen for both (`listen("open-occurrence")`
    in Tauri; the `BroadcastChannel` always) and call
    `openOccurrence({ eventId, recurrenceId, allDay: false, startDate:
    date, start: … }`-equivalent — set the module to Calendar and
    `selectOccurrence({ eventId, recurrenceId }, date)`. Returns the
    cleanup.
- **`MainWindow`**: `useWindowEvents` also runs `watchReminders(client)`
  and `watchOccurrenceRequests()`.
- **`RemindersWindow`**: lists `calendar.reminders` (refetch on the
  `calendar.reminders` and `calendar.changed` events), with
  `useCalendarFrame()` (a `now` renewed every 30 seconds for the times)
  and `useCalendarColors()`; Snooze and Dismiss call `calendar.snooze` /
  `calendar.dismiss` and drop the rows at once; when the list becomes
  empty after it had rows, and on Escape (window keydown) or Close, it
  closes the window: `getCurrentWindow().close()` in Tauri, else
  `window.close()`. Opening a title calls `openOccurrenceInMain` with the
  occurrence's date in the zone (`startDate` for all-day).
- **`App.tsx`**: the reminder window (`isRemindersWindow()`) shows
  `RemindersWindow`, after compose and before settings.
- **Mock (`calendar.ts`)**: `calendar.reminders`: the data's reminders
  not dismissed and not snoozed past `now`, as `Reminder`s from their
  events (summary, location, calendar, `allDay`, `start`/`startDate` of
  the occurrence named by `recurrenceId`, else the event's), `dueAt` the
  snooze's end when snoozed, else `dueAt`; sorted by `dueAt`.
  `calendar.snooze {ids, until}` and `calendar.dismiss {ids}` change them
  (empty `ids`: `invalidParams`) and emit `calendar.reminders` with the
  count `calendar.reminders` now returns.

## Tests (given, do not edit)

`app/src/lib/reminderText.test.ts`,
`app/src/features/calendar/ReminderPanel.test.tsx`,
`app/src/rpc/mock/reminders.test.ts`, `app/src/app/RemindersWindow.test.tsx`,
`app/src/app/ReminderRaise.test.tsx`.

## Gotchas

- Every existing app test must keep passing. The fixture seeds its two
  reminders only with `mockData({ reminders: true })`, so other tests that
  render `MainWindow` never raise the reminder window (happy-dom's
  `window.open` would load a page).
- `emitTo` and `listen` come from `@tauri-apps/api/event`; guard with
  `isTauri()` as `openMessage.ts` does.
- `noUncheckedIndexedAccess` is on; no `any`.

## Out of scope

maild's scheduler and notifications (done), the Rust command (done), and
every file not under `touch`.

## Done when

`make accept T=0076` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
