---
id: "0143"
title: The editor's repeat rules to and from RRULE
milestone: M5.5
size: M
touch:
  - app/src/lib/repeat.ts
given:
  - app/src/lib/repeat.test.ts
acceptance: make ui-vitest F=src/lib/repeat
---
# T-0143: The editor's repeat rules to and from RRULE

## Goal

The event editor (docs/specs/pim-ui.md, "Event editor window") offers
repeat presets, a Custom… sheet and an end (never, on a date, after a
count). `lib/repeat.ts` turns those into the `RRULE` value
`calendar.update` and `calendar.create` take (`EventFields.rrule`) and
reads them back from an event's rule. A rule the editor cannot show stays
as it is until the user changes it, so reading must say when it cannot.

## Read first

- `docs/specs/pim-ui.md`: "Event editor window" (repeat, end repeat, the
  repeat sheet).
- RFC 5545 §3.3.10 (RECUR) for `FREQ`, `INTERVAL`, `BYDAY`,
  `BYMONTHDAY`, `BYMONTH`, `COUNT`, `UNTIL`, `WKST`.
- `app/src/lib/calendarDates.ts`: `addDays`, `weekday`, `zoned`,
  `dayStart`.
- `app/src/lib/eventText.ts`: `describeRecurrence`, which reads rules
  for display (do not change it).
- The stub `app/src/lib/repeat.ts`: keep every export and its signature;
  replace the bodies and `notYet`.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

Rules are `RRULE` values without the `RRULE:` prefix: `NAME=VALUE` parts
joined by `;`, names and values read case-insensitively and written in
upper case. A part without `=`, with an empty value or a second `=`, a
repeated name, or no `FREQ` makes a rule unreadable. Dates are
`YYYY-MM-DD`; `start` is the event's first day.

- **`nthWeekday(start)`**: the start's weekday in its month as `BYDAY`
  writes it, `1` to `4` before the day (`3TH`), or `-1` for a fifth one
  (`-1TH`).
- **`repeatOf(rule, start)`**: the rule as the sheet shows it, or
  `undefined` when the sheet cannot (unreadable, or any part but `FREQ`,
  `INTERVAL`, `WKST`, `COUNT`, `UNTIL` and those below). `INTERVAL` is a
  positive integer (1 when absent). The end does not count.
  - `DAILY` with nothing else: `weekdays` `[]`, `byWeekday` false.
  - `WEEKLY`, with no `BYDAY` (the start's day) or a `BYDAY` of plain days
    (no ordinals): `weekdays` in `WEEKDAYS` order.
  - `MONTHLY` with nothing else, or a `BYMONTHDAY` equal to the start's
    day: `byWeekday` false. With a `BYDAY` naming the start's weekday in
    its month (`nthWeekday(start)`, or `-1<day>` when the start is in its
    month's last seven days): `byWeekday` true.
  - `YEARLY`, with nothing else or `BYMONTH` and `BYMONTHDAY` equal to the
    start's month and day (either may be absent).
- **`defaultRepeat(start)`**: weekly, every 1, on the start's day.
- **`ruleOf(r, start)`**: `FREQ=<frequency>`, then `;INTERVAL=<n>` when
  over 1, then for `WEEKLY` `;BYDAY=<days in WEEKDAYS order>` unless the
  days are none or just the start's day, and for `MONTHLY` with
  `byWeekday` `;BYDAY=<nthWeekday(start)>`. A rule the start's own day
  implies leaves it to `DTSTART`, so moving the series moves its day.
- **`presetOf(rule, start)`**: `never` for `""`; otherwise by
  `repeatOf`, ignoring the end: `daily` (every 1 day), `weekdays` (every
  week, Monday to Friday), `weekly` and `biweekly` (every 1 or 2 weeks on
  the start's day alone), `monthly` (every month, on its day), `yearly`
  (every year); anything else, or what `repeatOf` cannot read, is
  `custom`.
- **`presetRule(preset)`**: `""`, `FREQ=DAILY`,
  `FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR`, `FREQ=WEEKLY`,
  `FREQ=WEEKLY;INTERVAL=2`, `FREQ=MONTHLY`, `FREQ=YEARLY`.
- **`endOf(rule, timeZone)`**: `{kind: "count", count}` for a positive
  `COUNT`; for `UNTIL`, `{kind: "date", date}`: a date's own day, a UTC
  time's day in `timeZone` (`zoned`), a floating time's day as written;
  otherwise (none, or unreadable) `{kind: "never"}`.
- **`withEnd(rule, end, allDay, timeZone)`**: the rule without its
  `COUNT` and `UNTIL`, its other parts in their order, then `;COUNT=<n>`
  (at least 1), or `;UNTIL=<date as YYYYMMDD>` for an all-day event, or
  for a timed one `;UNTIL=<the last second of that date in timeZone, in
  UTC, as YYYYMMDDTHHMMSSZ>`. An unreadable rule, `""` included, comes
  back unchanged.
- **`followStart(rule, from, to)`**: when the start moves from one date
  to another, the parts naming the old start's day follow it: a `WEEKLY`
  `BYDAY` that is just the old day becomes the new one; a `MONTHLY`
  `BYDAY` that names the old start's weekday in its month becomes
  `-1<day>` when it was `-1` and the new date is in its month's last
  seven days, else `nthWeekday(to)`; a `BYMONTHDAY` equal to the old day
  becomes the new day; a `YEARLY` `BYMONTH` equal to the old month
  becomes the new month. Everything else stays, in order. The same date,
  or an unreadable rule, returns `rule`.

## Tests (given, do not edit)

`app/src/lib/repeat.test.ts`: `presetOf`, `presetRule`, `nthWeekday`,
`repeatOf and ruleOf`, `endOf and withEnd`, `followStart`.

## Gotchas

- `weekday` counts Sunday as 0; `WEEKDAYS` starts on Monday.
- The last second of a day in a zone: `dayStart` of the next day, less a
  second. Across a DST change the offset differs from the start's.
- A `Map` keeps parts in their order for writing back.
- Pure functions: no `Date.now()`, no locale.

## Out of scope

The editor and the sheet that use these (later cards);
`describeRecurrence`; every file not under `touch`.

## Done when

`make accept T=0143` and `make ui-check` pass, and only the files under
`touch` changed.
