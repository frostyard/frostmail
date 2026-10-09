---
id: "0071"
title: Build the Calendar module's components
milestone: M4.5
size: L
touch:
  - app/src/features/calendar/MiniMonth.tsx
  - app/src/features/calendar/CalendarSidebar.tsx
  - app/src/features/calendar/TimeGrid.tsx
  - app/src/features/calendar/MonthGrid.tsx
  - app/src/features/calendar/EventPane.tsx
  - app/src/features/calendar/eventStyle.ts
given:
  - app/src/features/calendar/fixtures.ts
  - app/src/features/calendar/MiniMonth.test.tsx
  - app/src/features/calendar/CalendarSidebar.test.tsx
  - app/src/features/calendar/TimeGrid.test.tsx
  - app/src/features/calendar/MonthGrid.test.tsx
  - app/src/features/calendar/EventPane.test.tsx
acceptance: make ui-vitest F="src/features/calendar"
---
# T-0071: Build the Calendar module's components

## Goal

The presentational pieces of the Calendar module: the small month, the
calendars sidebar, the day and week time grid, the month grid and the
event pane. They take data and callbacks by props; T-0072 wires them to
maild.

## Read first

- `docs/specs/pim-ui.md`: "Calendar module" and its subsections (the
  measurements, colors and behavior below come from there), and "Rules".
- `app/src/lib/calendarDates.ts`, `eventLayout.ts`, `eventText.ts` (done
  in T-0070): use them for every date, layout and text decision.
- The stubs in `app/src/features/calendar/`: keep their exported types
  and props exactly; replace the bodies and the "Task T-0071 builds it"
  lines.
- `app/src/features/people/PeopleSidebar.tsx`, `PersonPane.tsx` and
  `Avatar.tsx`: the style of a feature component, the sidebar's section
  titles, and the avatar to reuse.
- `app/src/styles/app.css`: the theme's utility names (`bg-accent`,
  `text-tertiary`, `border-separator`, …).
- The given tests and `fixtures.ts`.
- `docs/tasks/EXECUTOR.md`

## Contract

The tests pin these; the spec gives the rest of the look.

- **Colors.** Every occurrence element (a time-grid block, an all-day bar,
  a month line or pill) sets the CSS custom property `--event-color` to
  `calendarColor(colors.get(calendarId) ?? "")`, and its classes use it
  (e.g. `border-l-[var(--event-color)]`, a fill of
  `color-mix(in srgb, var(--event-color) 18%, var(--bg-window))`, and the
  color itself when selected). Put the shared class logic in
  `eventStyle.ts`. Cancelled occurrences get `opacity-60` and their title
  `line-through`; `needsaction` and `tentative` answers get
  `border-dashed` (a 1px dashed border in the color, `bg-window` fill).
- **MiniMonth:** a `grid` named by the shown month and year (`month`
  long, `year`) with seven `columnheader`s (narrow weekday names from
  `weekStart`) and 42 day `button`s (`aria-label` the full date, text the
  day number; `aria-pressed` the selection; `aria-current="date"` today).
  Other months' days have `text-tertiary`; the selected day
  `bg-selection-sidebar`, or `bg-accent` with `text-accent-contrast` when
  it is today. A busy day has a dot element with a `data-busy` attribute.
  "Previous Month"/"Next Month" buttons page the shown month (state); a
  new `selected` from props shows its month again.
- **CalendarSidebar:** the `MiniMonth`, then per section with calendars a
  `group` named by its title (`aria-label`; the title shown as Mail's
  sidebar shows account titles) holding one `button` with
  `role="checkbox"` per calendar: `aria-checked` its `enabled`, text the
  name, the 14px square swatch (no text), and a `Lock` icon with
  `aria-label="Read-only"` when read-only. Clicking calls
  `onToggle(id, !enabled)`. Sections without calendars are left out.
- **TimeGrid:**
  - Header: one `button` per day, `aria-label` the full date, showing the
    short weekday and the day number; today's has `aria-current="date"`;
    clicking calls `onShowDay`.
  - All-day strip: `allDayLanes(occurrences, days)`; each bar a `button`
    named "<title>, all day" (`aria-label`), clicking calls `onSelect`.
    With seven days and more than three lanes, lanes 0 and 1 show and
    each day with bars in lanes 2 and up gets a `button` in lane 2 with
    text "N more" and `aria-label` "Show N more on <full date>", calling
    `onShowDay(date)`. With one day every lane shows.
  - Grid: each day is a `group` named by its full date (`aria-label`);
    nothing else is a `group`. Its blocks come from `timedLayout`:
    absolutely placed `button`s with `style.top` `${startMinute * 0.8}px`,
    `style.height` `${max((end - start) * 0.8, 18)}px`, `style.left`
    `${column / columns * 100}%` and a width of `100 / columns`% less 4px;
    `aria-label` "<title>, <timeRange>[, <location>]" (`timeRange` from
    eventText, in the zone); `aria-pressed` true for the `selected`
    occurrence (same `eventId` and `recurrenceId`); clicking calls
    `onSelect(occurrence)`. Inside: the title, and when the block is at
    least 36px high the time range and location.
  - Hour labels in the gutter for hours 1 to 23
    (`Intl.DateTimeFormat(locale, { hour: "numeric", timeZone: "UTC" })`
    of that hour on any day: "9 AM").
  - Now: in today's column only, an element with `data-now` and
    `style.top` at `zoned(now).minutes * 0.8`px (the gutter time beside
    it); none when today is not shown. The containers re-render with a
    new `now` every minute.
  - The grid scrolls (`overflow-y-auto`) with 1152px of hours; on mount
    scroll it so 7:00 is at the top.
- **MonthGrid:** a `grid` named by the selected date's month and year, a
  `row` of seven `columnheader`s (short weekday names) and six `row`s of
  seven `gridcell`s (`aria-label` the full date, `aria-selected` for
  `selectedDate`, `aria-current="date"` today). Each cell shows its day
  number (the 1st: "Oct 1", short month and day) — `text-tertiary`
  outside the selected date's month, today's on `bg-accent` — then
  `monthItems` as `button`s: all-day ones named "<title>, all day", timed
  ones "<title>, <start time>" (`hour: "numeric", minute: "2-digit"` in
  the zone), with the dot, time and title. When a day has more than
  `lines` items it shows `lines - 1` and a `button` with text "N more"
  and `aria-label` "Show N more on <full date>" (`onShowDay`). Clicking
  an item calls `onSelect` (and not `onSelectDate`: stop propagation);
  clicking a cell calls `onSelectDate`, double-clicking it `onShowDay`.
- **EventPane:** with no event, "No Event Selected". Otherwise:
  - an `h2` with the summary, or "No Title"; the location; "Cancelled"
    when cancelled;
  - `dateText` and, for timed events, `timeRange` in the app's zone, each
    its own element; "All day" for all-day ones; when the event's
    `timeZone` is set and differs from the app's, "<zone>: <timeRange in
    that zone>";
  - a `dl` of `dt`/`dd` rows: Repeats (`describeRecurrence`, when there is
    a recurrence), Calendar (swatch, name, " · ", account), Alerts (one
    `alarmText` per line, when there are alarms), Your answer
    (`answerText`, when `answer` is set);
  - people: a `ul` named "Organizer" (`aria-label`) with the organizer,
    and a `ul` named "Invitees" with the attendees, each list only when
    it has someone, under visible headings; each `li` holds the
    `Avatar` (size 28), a `button` with the name (else the email) that
    calls `onPerson(email, name, button)`, " (you)" for the user,
    " (optional)" for optional ones, and the answer icon (`Check`, `X` or
    `CircleHelp` from lucide-react with `aria-label` Accepted, Declined or
    Maybe; nothing otherwise);
  - the description in an element with `whitespace-pre-wrap`, its text
    as is (no links, no `<br>`).

## Tests (given, do not edit)

`app/src/features/calendar/{MiniMonth,CalendarSidebar,TimeGrid,MonthGrid,EventPane}.test.tsx`
and their shared `fixtures.ts`. Names and texts are compared after
replacing the thin and narrow no-break spaces `Intl` uses with spaces.

## Gotchas

- React sets a custom property with `style={{ "--event-color": c } as
  CSSProperties}`.
- Accessible names that differ from the visible text need `aria-label`
  (day buttons, cells, blocks, bars, "N more").
- Format dates for labels from their UTC midnight with `timeZone: "UTC"`;
  only occurrences' instants use the app's zone.
- `noUncheckedIndexedAccess` is on. Keep components under ~150 lines;
  split helpers inside the touched files.

## Out of scope

Containers, stores, the toolbar, keys, data fetching (T-0072), and every
file not under `touch`.

## Done when

`make accept T=0071` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
