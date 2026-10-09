---
id: "0074"
title: Show upcoming events on contact cards and the person pane
milestone: M4.5
size: M
touch:
  - app/src/features/calendar/UpcomingList.tsx
  - app/src/lib/eventText.ts
  - app/src/features/people/PersonPane.tsx
  - app/src/features/people/ContactPopover.tsx
  - app/src/app/PeopleModule.tsx
  - app/src/app/usePeople.ts
  - app/src/app/ContactCardContainer.tsx
  - app/src/app/useCalendar.ts
given:
  - app/src/lib/eventText.upcoming.test.ts
  - app/src/features/calendar/UpcomingList.test.tsx
  - app/src/app/UpcomingLinks.test.tsx
acceptance: make ui-vitest F="src/lib/eventText.upcoming.test.ts src/features/calendar src/features/people src/app"
---
# T-0074: Show upcoming events on contact cards and the person pane

## Goal

`people.card` already answers a person's next occurrences
(`ContactCard.upcoming`). Show them under "Upcoming" after Recent Mail on
the reader's contact card and in the People module's person pane; clicking
one opens it in the Calendar module.

## Read first

- `docs/specs/pim-ui.md`: "Person pane" (its Upcoming line), "Contact
  card", "Calendar behavior" ("From elsewhere").
- `app/src/features/people/PersonPane.tsx` and `ContactPopover.tsx`
  (their Recent Mail sections: Upcoming looks the same), their containers
  `app/src/app/PeopleModule.tsx`, `usePeople.ts` (`usePersonExtras`) and
  `ContactCardContainer.tsx`.
- `app/src/app/useCalendar.ts` (`useCalendarFrame`, the calendars query),
  `app/src/lib/eventText.ts`, `calendarDates.ts` (`zoned`),
  `app/src/features/calendar/eventStyle.ts`.
- The stubs `UpcomingList.tsx` and `upcomingWhen` (keep their
  signatures), and the given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

- **`upcomingWhen(o, timeZone, locale, now)`**: the occurrence's date in
  the zone (`zoned(start)`; for all-day, today when `startDate <= today <
  endDate`, else `startDate`). Today: the start time (`hour: "numeric",
  minute: "2-digit"`), or "Today" for all-day. One to six days ahead: the
  short weekday ("Fri"). Otherwise the short month and day ("Oct 15").
  Format the dates from their UTC midnight with `timeZone: "UTC"`.
- **`UpcomingList`**: nothing when there are no occurrences; else a
  `section` labelled by its `h3` "Upcoming" (the same classes as Recent
  Mail's heading), and a 32-high `button` per occurrence with
  `aria-label` "<title>, <when>", `--event-color` set to
  `calendarColor(colors.get(calendarId) ?? "")`, showing a 6px dot in that
  color, the title (truncated, "No Title" when empty) and `when` at the
  right in the list date style; clicking calls `onOpen(occurrence)`.
- **`PersonPane`** and **`ContactPopover`** gain an optional `upcoming?:
  ReactNode` prop rendered after Recent Mail (their existing tests must
  keep passing).
- **`useCalendar.ts`** exports:
  - `useCalendarColors()`: a map of calendar ID to color from
    `account.collections({ kind: "calendar" })`, refetched on
    `account.changed`.
  - `openOccurrence(occurrence, timeZone)`: set the module to Calendar and
    `selectOccurrence({ eventId, recurrenceId }, its date)`, with the date
    `startDate` for all-day ones, else `zoned(start, timeZone).date`. The
    current view is kept.
- **Containers:** `usePeople` keeps the person's `card.upcoming`;
  `PeopleModule` and `ContactCardContainer` pass `<UpcomingList …>` with
  `useCalendarFrame()`'s zone, locale and now, and `onOpen` calling
  `openOccurrence` (the contact card also closes).

## Tests (given, do not edit)

`app/src/lib/eventText.upcoming.test.ts`,
`app/src/features/calendar/UpcomingList.test.tsx`,
`app/src/app/UpcomingLinks.test.tsx` (through `MainWindow` with
MockTransport; the fixture's Lunch with Ann is on Friday 2026-10-09, the
day after the clock).

## Gotchas

- Every existing test in `src/features/people` and `src/app` must keep
  passing.
- `noUncheckedIndexedAccess` is on; no `any`.

## Out of scope

The reminder window, Calendar module changes, and every file not under
`touch`.

## Done when

`make accept T=0074` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
