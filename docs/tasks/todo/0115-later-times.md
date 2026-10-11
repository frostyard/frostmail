---
id: "0115"
title: Send Later and Remind Me times, and the time sheet
milestone: M5
size: M
touch:
  - app/src/lib/later.ts
  - app/src/features/later/TimeSheet.tsx
given:
  - app/src/lib/later.test.ts
  - app/src/features/later/TimeSheet.test.tsx
acceptance: make ui-vitest F="src/lib/later src/features/later"
---
# T-0115: Send Later and Remind Me times, and the time sheet

## Goal

What Send Later and Remind Me know about time: the menus' choices ("Send
9:00 PM Tonight", "Remind Me Tomorrow"), how a time is said ("Tomorrow at
8:00 AM"), and a small sheet that asks for a date and a time. T-0116 to
T-0118 build on them.

## Read first

- `docs/specs/ui.md`: "Times (M5)" (the functions and the time sheet).
- `app/src/lib/calendarDates.ts`: `zoned`, `today`, `addDays`, `dayStart`
  (`atLocal` works as `dayStart` does, at a wall-clock time).
- `app/src/lib/reminderText.ts`: how calendar reminders word times.
- `app/src/features/organize/SmartSheet.tsx`: the sheet frame to follow.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`app/src/lib/later.ts`** exports `LaterChoice` (`{ label: string; at:
  Date }`) and:
  - `atLocal(date, hour, minute, timeZone): Date`, the instant of a local
    `YYYY-MM-DD` at that wall-clock time, right across DST changes.
  - `whenText(at, now, timeZone, locale)`: "Today at …", "Tomorrow at …",
    else `Intl.DateTimeFormat(locale, { weekday: "short", month: "short",
    day: "numeric" })` of the date, with `year: "numeric"` when the year
    is not now's, then " at " and the time as `Intl.DateTimeFormat(locale,
    { hour: "numeric", minute: "2-digit", timeZone })`.
  - `sendChoices(now, timeZone, locale)`: `Send <time> Tonight` at today's
    21:00 while now is before it, then `Send <time> Tomorrow` at
    tomorrow's 8:00, the times written as `whenText` writes them.
  - `remindChoices(now, timeZone)`: "Remind Me in 1 Hour" (now + 1 h),
    "Remind Me Tonight" (today's 21:00, while now is before it), "Remind
    Me Tomorrow" (tomorrow's 8:00).
- **`TimeSheet`** (`app/src/features/later/TimeSheet.tsx`), props `title`,
  `initial: Date`, `now: Date`, `timeZone`, `onChoose(at: Date)`,
  `onCancel()`: the spec's sheet. The date input (`type="date"`, named
  "Date") and time input (`type="time"`, named "Time", `HH:MM`) start at
  `initial` in `timeZone`; OK gives `atLocal` of what they hold, and is
  disabled while that is not after `now` or either is empty. The first
  input has focus when it opens; Escape and Cancel call `onCancel`.

## Tests (given, do not edit)

`later.test.ts` and `TimeSheet.test.tsx`. Every other app test must keep
passing.

## Gotchas

- Compute in the given `timeZone`, never the machine's: the tests use
  America/New_York whatever the machine's zone is.
- `new Date("YYYY-MM-DD")` is UTC midnight; `dayStart` shows how to correct
  from there.

## Out of scope

The menus and sections that use these (T-0116 to T-0118), maild, the mock,
and every file not under `touch`.

## Done when

`make accept T=0115` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
