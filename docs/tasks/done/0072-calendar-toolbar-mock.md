---
id: "0072"
title: Calendar keys, state, toolbar and mock
milestone: M4.5
size: L
touch:
  - app/src/rpc/mock/calendar.ts
  - app/src/rpc/mock/mock.ts
  - app/src/rpc/mock/people.ts
  - app/src/data/stores.ts
  - app/src/lib/keymap.ts
  - app/src/features/toolbar/Toolbar.tsx
given:
  - app/src/lib/keymap.calendar.test.ts
  - app/src/features/toolbar/Toolbar.calendar.test.tsx
  - app/src/data/stores.calendar.test.ts
  - app/src/rpc/mock/calendar.test.ts
acceptance: make ui-vitest F="src/lib src/features/toolbar src/data src/rpc/mock"
---
# T-0072: Calendar keys, state, toolbar and mock

## Goal

Everything the Calendar module's containers (T-0073) stand on: its keys,
its window state, the toolbar's calendar mode, and MockTransport answering
the calendar domain from the fixture's events, so the app runs and is
tested without maild.

## Read first

- `docs/specs/pim-ui.md`: "Calendar module", "Calendar toolbar" and
  "Calendar behavior".
- `app/src/lib/keymap.ts` and `keymap.modules.test.ts`; `app/src/data/stores.ts`.
- `app/src/features/toolbar/Toolbar.tsx`, `Toolbar.test.tsx` and
  `Toolbar.people.test.tsx` (how modes change the segments).
- `app/src/rpc/mock/mock.ts`, `people.ts` (`MockPeople`: the pattern for
  a domain, and `account.collections`), `fixture.ts` (`CALENDARS` and
  `fixtureCalendar`, already written: read the events), and `calendar.ts`
  (the data types, already written).
- `app/src/lib/calendarDates.ts` (`dayStart`, `addDays`, `zoned`).
- `schema/rpc/calendar.yaml` and `account.yaml` (`setCollection`): what
  maild answers, and `internal/engine/calendar.go` for its rules.
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

### Keys (`keymap.ts`)

Add the commands `dayView`, `weekView`, `monthView` (Ctrl+Alt+1, 2, 3),
`today` (Ctrl+T), `previousPeriod` and `nextPeriod` (Ctrl+← and Ctrl+→),
`left` and `right` (← and →, no modifiers). In a text field only the
Ctrl+Alt ones apply (the existing rule already says so).

### State (`stores.ts`)

Add to `UIState` and its initial state, not persisted:

```ts
calendarView: CalendarView;            // "week"
calendarDate: string;                  // "" means today
calendarSelected: OccurrenceKey | null; // null
calendarFocus: Pane;                   // "list" (the view)
```

and to `UIActions`: `setCalendarView(view)`, `setCalendarDate(date)`,
`selectOccurrence(key | null, date?)` (sets the date too when given) and
`setCalendarFocus(pane)`. `CalendarView` comes from
`lib/calendarDates.ts`, `OccurrenceKey` from
`features/calendar/TimeGrid.tsx` (types only).

### Toolbar (`Toolbar.tsx`)

- `mode` gains `"calendar"`, and `ToolbarProps` an optional
  `calendar?: ToolbarCalendar` with
  `{ view: CalendarView; title: string; paneWidth: number }` (export the
  interface). `ToolbarCommand` gains `{ kind: "today" }`,
  `{ kind: "previousPeriod" }`, `{ kind: "nextPeriod" }` and
  `{ kind: "calendarView"; view: CalendarView }`.
- In calendar mode the segments are `sidebar` (the toggle only, as in
  People), `calendar` (filling the space: **Today**, a text button 28
  high; ‹ and › named "Previous Day/Week/Month" and "Next …" by the view;
  the title 15/20 600; and at its right a `radiogroup` named "View" of
  three `radio` buttons Day, Week, Month with `aria-checked`, a segmented
  control on a `bg-badge` track with the chosen one `bg-window` and a
  shadow) and `pane` (`paneWidth + 1` px wide: Settings and the window
  controls). The search field is not shown. Every segment keeps
  `data-tauri-drag-region`. Mail and People are unchanged.

### Mock (`calendar.ts`, `mock.ts`, `people.ts`)

`MockCalendar` (in `calendar.ts`) answers, with `dispatch(method, params)`
returning `NOT_HANDLED` (from `compose.ts`) for other methods:

- **`calendar.range`** `{from, to, timeZone?, calendarIds?}`: `from` and
  `to` are `YYYY-MM-DD`, `to` 1 to 400 days after `from`, the zone an IANA
  name (try `Intl.DateTimeFormat` with it; default UTC): otherwise an
  `RPCError` with `ErrorCode.invalidParams`. The occurrences of events in
  enabled calendar collections (and `calendarIds` when given), as maild
  would return them:
  - a `MockEvent` without `every` is one occurrence (`recurrenceId` "");
    with `every` it is `count` occurrences a day or a week apart (in UTC),
    each with `recurrenceId` its start as
    `YYYY-MM-DDTHH:MM:SS.000Z` (`toISOString()`), or `YYYY-MM-DD` for an
    all-day one, and `recurring` true;
  - timed ones overlap `[dayStart(from, zone), dayStart(to, zone))`
    (`start < to && end > from`, or a zero-length one starting in it);
    all-day ones overlap the dates (`startDate < to && endDate > from`),
    their `start`/`end` the midnights of their dates in the zone
    (ISO strings);
  - fields copied from the event (`eventId` is its `id`); `answer` only
    when the event has one;
  - sorted by start, all-day first on a tie, then by `eventId`.
- **`calendar.event`** `{id, recurrenceId?}`: the event (a copy), or with
  a `recurrenceId` of one of its occurrences, that occurrence's
  `recurrenceId`, `start`, `end` (and dates); an unknown `id` or
  `recurrenceId` is `ErrorCode.notFound`.
- **`account.setCollection`** `{id, enabled?, isDefault?}` on any
  collection: update it in the shared collections (`isDefault` true makes
  the others of its account and kind not default); emit the domain event
  when `enabled` changed (`calendar.changed` with `accountId` for
  calendars, `people.changed` for address books), then `account.changed`
  with `id` the account; return a copy. Unknown: `notFound`.
- **`upcoming(email)`**: the occurrences (UTC) with the address as the
  organizer or an attendee, from `now` to 30 days later, at most 5.

In `mock.ts`, build a `MockCalendar` from `data.calendar` (empty events
when absent) and the `MockPeopleData` collections, and dispatch to it
after compose and before people. In `people.ts`, `MockPeople` takes a
function giving `upcoming(email)` and fills `ContactCard.upcoming` with
it (`[]` without one).

## Tests (given, do not edit)

`app/src/lib/keymap.calendar.test.ts`,
`app/src/features/toolbar/Toolbar.calendar.test.tsx`,
`app/src/data/stores.calendar.test.ts`, `app/src/rpc/mock/calendar.test.ts`.
The fixture's D is 2026-10-08 in the mock test.

## Gotchas

- The existing toolbar, keymap, store and mock tests must keep passing.
- `noUncheckedIndexedAccess` is on; no `any`.
- Do not change `fixture.ts` or the types in `calendar.ts`.

## Out of scope

The Calendar module's container, `MainWindow`, `ToolbarContainer`, the
contact card's Upcoming rows, and every file not under `touch`.

## Done when

`make accept T=0072` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
