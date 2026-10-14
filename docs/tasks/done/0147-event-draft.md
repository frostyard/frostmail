---
id: "0147"
title: The event editor's draft
milestone: M5.5
size: M
touch:
  - app/src/lib/eventDraft.ts
given:
  - app/src/lib/eventDraft.test.ts
acceptance: make ui-vitest F=src/lib/eventDraft
---
# T-0147: The event editor's draft

## Goal

The event editor (docs/specs/pim-ui.md, "Event editor window") edits a
draft: the event's fields as its form shows them, dates and wall times in
the event's own zone. `lib/eventDraft.ts` makes the draft from an event or
a new event's first times, checks it, moves its start, and turns it back
into the `EventFields` a save sends: everything for a new event, and only
what changed for an existing one (ADR-0030), the times whole (ADR-0033).

## Read first

- `docs/specs/pim-ui.md`: "Event editor window" (starts and ends, time
  zone, repeat, Save).
- `schema/rpc/calendar.yaml`: `CalendarEvent`, `EventFields` (a timed
  event has `start`, `end` and `timeZone`; an all-day one `startDate` and
  `endDate`, the day after the last).
- `app/src/lib/calendarDates.ts`: `zoned`, `addDays`, `instantAt`.
- `app/src/lib/repeat.ts`: `followStart`.
- The stub `app/src/lib/eventDraft.ts`: keep `EventDraft`, `DraftErrors`
  and every export's signature; replace the bodies and `notYet`.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

Times are `HH:MM` (24-hour) and dates `YYYY-MM-DD`.

- **`ruleOf(recurrence)`**: the value of the `RRULE:` line among an
  event's recurrence lines (joined by `\n`), trimmed; `""` without one.
- **`draftOf(event, appZone)`**: `summary`, `location`, `description`,
  `allDay`, `calendarId`, `transparent` and a copy of `alarms` as given;
  `rrule` from `ruleOf(event.recurrence)`.
  - A timed event's times are read in its `timeZone`, else `appZone`
    (which is then the draft's `timeZone`): `startDate`/`startTime` and
    `endDate`/`endTime` are `start` and `end` as `zoned` gives them.
  - An all-day event keeps `startDate`, shows its last day as `endDate`
    (the event's `endDate` less a day), and gets `startTime` `09:00`,
    `endTime` `10:00` and `timeZone` `appZone`, ready for a change to
    timed.
- **`newDraft(times, calendarId, appZone)`**: the draft of an event with
  those times (instants, or the first day and the day after the last) in
  `appZone`, in that calendar, and nothing else set.
- **`instants(d)`**: a timed draft's start and end, its dates and times
  read in its zone (`instantAt`).
- **`validate(d)`**: `{start: "Enter a start"}` when the start date (or,
  timed, its time) is not a valid shape, `{end: "Enter an end"}` the same
  for the end, and `{timeZone: "Choose a time zone"}` for a timed draft
  without one; any of these returns at once. Otherwise
  `{end: "The end is before the start"}` when a timed end is not after
  its start, or an all-day end date is before its start date (one day is
  fine). A valid draft gives `{}`.
- **`moveStart(d, startDate, startTime = d.startTime)`**: the draft with
  that start and the same length: an all-day draft's end date moves by
  the same days; a timed draft's end is the new start plus the old
  length in time (so across a DST change the wall clock may differ). Its
  `rrule` is `followStart(d.rrule, d.startDate, startDate)`.
- **`changedFields(original, d)`**:
  - For a new event (`original` null): `summary`, `location` and
    `description` when not empty, the times, `rrule` when not empty,
    `alarms` when there are any, and `transparent` when true. Not
    `calendarId` (calendar.create takes it apart).
  - For an existing event: each of `summary`, `location`, `description`,
    `rrule`, `transparent` that differs, `alarms` when the list differs,
    and `calendarId` when it changed.
  - **The times** are sent whole, when new or when any of `allDay`,
    `startDate`, `endDate` changed, or, for a timed draft, `startTime`,
    `endTime` or `timeZone`: `{allDay: false, start, end, timeZone}` as
    ISO instants from `instants`, or `{allDay: true, startDate, endDate}`
    with `endDate` the day after the last. An all-day draft's hidden times
    and zone are no change.

## Tests (given, do not edit)

`app/src/lib/eventDraft.test.ts`.

## Gotchas

- `zoned` gives minutes after midnight; pad hours and minutes to two
  digits.
- Pure functions: no `Date.now()`, no locale.

## Out of scope

The editor's form and window (later cards); every file not under
`touch`.

## Done when

`make accept T=0147` and `make ui-check` pass, and only the files under
`touch` changed.
