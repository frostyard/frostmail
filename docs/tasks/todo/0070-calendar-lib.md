---
id: "0070"
title: Calendar dates, layout and text
milestone: M4.5
size: L
touch:
  - app/src/lib/calendarDates.ts
  - app/src/lib/eventLayout.ts
  - app/src/lib/eventText.ts
given:
  - app/src/lib/calendarDates.test.ts
  - app/src/lib/eventLayout.test.ts
  - app/src/lib/eventText.test.ts
acceptance: make ui-vitest F="src/lib/calendarDates.test.ts src/lib/eventLayout.test.ts src/lib/eventText.test.ts"
---
# T-0070: Calendar dates, layout and text

## Goal

The Calendar module's views and toolbar are drawn from a few pure
functions: date arithmetic on `YYYY-MM-DD` strings, an instant's day and
minutes in a time zone, where each occurrence goes in the day, week and
month views, and the words for recurrences, alerts, times and answers.
Write them, with no React and no dependencies beyond `Intl`.

## Read first

- `docs/specs/pim-ui.md`: "Calendar module" (all of it: the views use
  these functions).
- `app/src/rpc/gen/api.ts`: `Occurrence`, `PartStat`.
- `app/src/lib/peopleRows.ts`: the style of a lib module.
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

Dates are `YYYY-MM-DD` strings, compared as strings. Do date arithmetic in
UTC (`Date.UTC`, `getUTC*`) so the machine's zone never matters; reach
time zones only through `Intl.DateTimeFormat` with `timeZone`.

### `calendarDates.ts`

```ts
export type CalendarView = "day" | "week" | "month";
export function addDays(date: string, n: number): string;
/** 0 is Sunday. */
export function weekday(date: string): number;
/** The first day of date's week; weekStart 0 is Sunday. */
export function startOfWeek(date: string, weekStart: number): string;
/** The 42 days from the start of the week holding the month's 1st. */
export function monthGrid(date: string, weekStart: number): string[];
/** The days a view shows, [from, to). */
export function viewRange(view: CalendarView, date: string, weekStart: number): { from: string; to: string };
/** Where ‹ (-1) and › (1) go: a day, a week, or the same day of the next
 *  or previous month, clamped to that month's last day. */
export function step(view: CalendarView, date: string, direction: 1 | -1): string;
export function viewTitle(view: CalendarView, date: string, weekStart: number, locale: string): string;
/** An instant's date and minutes since midnight (wall clock) in a zone. */
export function zoned(instant: string | Date, timeZone: string): { date: string; minutes: number };
export function today(timeZone: string, now: Date): string;
/** The instant a date's midnight is in a zone. */
export function dayStart(date: string, timeZone: string): Date;
/** The locale's first day of the week (0 is Sunday); 0 when unknown. */
export function localeWeekStart(locale: string): number;
```

- `viewTitle`: day, the long date (`weekday`, `month` long, `day`,
  `year`); month, `month` long and `year`; week, the same when its seven
  days are in one month, else the short months joined by " – " with the
  year at the end ("Sep – Oct 2026"), or after each month across years
  ("Dec 2026 – Jan 2027"). Format in UTC from the date's UTC midnight.
- `zoned` reads `formatToParts` with `hourCycle: "h23"`.
- `dayStart` finds the instant whose `zoned` value is the date at minute
  0: start from the date's UTC midnight and correct by the difference two
  or three times (offsets change at DST).
- `localeWeekStart`: `new Intl.Locale(locale)`, then `getWeekInfo()` or
  the `weekInfo` property (both exist in engines today); `firstDay` is 1
  (Monday) to 7 (Sunday): map 7 to 0. Unknown or throwing: 0.

### `eventLayout.ts`

```ts
export interface TimedBlock {
  occurrence: Occurrence;
  date: string;          // the day this part is in
  startMinute: number;   // minutes since that day's midnight, wall clock
  endMinute: number;     // likewise; 1440 when it runs past midnight
  column: number;
  columns: number;       // columns in its cluster
  continuesBefore: boolean;
  continuesAfter: boolean;
}
export interface AllDayBar {
  occurrence: Occurrence;
  lane: number;
  first: number;         // index into days
  last: number;          // inclusive
  continuesBefore: boolean;
  continuesAfter: boolean;
}
export function timedLayout(occurrences: Occurrence[], days: string[], timeZone: string): TimedBlock[];
export function allDayLanes(occurrences: Occurrence[], days: string[]): AllDayBar[];
export function monthItems(occurrences: Occurrence[], date: string, timeZone: string): Occurrence[];
export function busyDates(occurrences: Occurrence[], days: string[], timeZone: string): Set<string>;
```

Every function leaves out occurrences the user declined
(`answer === "declined"`).

- **In a day:** a timed occurrence is in a day when it overlaps
  `[dayStart(date), dayStart(next day))` — `start < dayEnd && end >
  dayStart`, or for a zero-length one `dayStart <= start < dayEnd`. An
  all-day one covers the dates `startDate <= date < endDate`.
- **`timedLayout`**, per day in order: the timed occurrences in it, with
  `startMinute` 0 when they began before the day (`continuesBefore`) and
  `endMinute` 1440 when they end after it (`continuesAfter`; ending
  exactly at the next midnight is neither). A block takes the room it is
  drawn in: `[startMinute, max(endMinute, startMinute + 22.5))` (18px at
  0.8px a minute). Sort the day's blocks by `startMinute`, then the longer
  first, then input order. Walk them: a block whose start is at or after
  the current cluster's latest room end starts a new cluster; within a
  cluster each block takes the lowest column whose last block's room has
  ended by its start, else a new column; `columns` is the cluster's
  column count. Return the days' blocks in day order, each day by
  `startMinute`, then `column`.
- **`allDayLanes`**: the all-day occurrences overlapping the days (as
  dates: `startDate < day after the last && endDate > first day`), as bars
  from the index of their first shown day to their last
  (`continuesBefore` when `startDate` is before the first day,
  `continuesAfter` when `endDate` is after the day after the last). Sort
  by `first`, then the longer first, then input order; each takes the
  lowest lane whose last bar ends before its `first`. Return them in that
  order.
- **`monthItems`**: the date's all-day occurrences (by `startDate`, then
  the longer first, then input order), then its timed ones (by start, then
  input order).
- **`busyDates`**: the days for which `monthItems` is not empty.

### `eventText.ts`

```ts
export function describeRecurrence(recurrence: string, locale: string): string;
export function alarmText(minutes: number): string;
export function timeRange(start: string, end: string, timeZone: string, locale: string): string;
export function dateText(
  e: { allDay: boolean; start: string; end: string; startDate: string; endDate: string },
  timeZone: string,
  locale: string,
): string;
export function answerText(answer: PartStat): string;
export function calendarColor(color: string): string;
```

- **`describeRecurrence`** reads the first `RRULE:` line of the
  recurrence text (lines are joined by `\n`; other lines are ignored);
  "" for "", "Custom" when there is no RRULE or it says more than this
  can: FREQ `DAILY`, `WEEKLY`, `MONTHLY` or `YEARLY`; INTERVAL n
  ("Every day" / "Every n days", week, month, year); COUNT ("…, n times",
  "…, once"); UNTIL ("…, until December 31, 2026": its first 8 digits as
  a date, long month, in UTC, in the locale); WKST (ignored); BYDAY on a
  weekly rule: plain days in the order given, "on Monday, Wednesday and
  Friday" ("Every weekday" for exactly MO,TU,WE,TH,FR every week); on a
  monthly rule one ordinal day ("on the second Tuesday": 1 first, 2
  second, 3 third, 4 fourth, -1 last); BYMONTHDAY on a monthly rule, one
  number ("on day 15"). Any other part or combination is "Custom". The
  words are English; only the UNTIL date uses the locale.
- **`alarmText`**: minutes before the start (negative: after). 0 is "At
  time of event"; whole days "N day(s)", else whole hours "N hour(s)",
  else "N minute(s)", then "before" or "after"; singular for 1.
- **`timeRange`**: `Intl.DateTimeFormat(locale, { hour: "numeric",
  minute: "2-digit", timeZone }).formatRange`.
- **`dateText`**: a timed event's start date in the zone, long (weekday,
  month, day, year); an all-day event's `startDate` the same way, or for
  several days `formatRange` (month long, day, year) of the first and the
  last day (`endDate` less one).
- **`answerText`**: Accepted, Declined, Maybe (tentative), Not answered
  (needsaction), Delegated.
- **`calendarColor`**: the color, or `var(--accent)` when empty.

## Tests (given, do not edit)

`app/src/lib/calendarDates.test.ts`, `eventLayout.test.ts`,
`eventText.test.ts`. They compare `Intl` output after replacing thin and
narrow no-break spaces with spaces.

## Gotchas

- `new Date("2026-10-08")` is UTC midnight, but `new Date(2026, 9, 8)`
  is local: use the first form or `Date.UTC`.
- `noUncheckedIndexedAccess` is on: indexing an array gives `T |
  undefined`.
- Keep functions short; no `any`.

## Out of scope

Components, containers, the toolbar, and every file not under `touch`.

## Done when

`make accept T=0070` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
