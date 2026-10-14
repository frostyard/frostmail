---
id: "0144"
title: Event writes in MockTransport
milestone: M5.5
size: M
touch:
  - app/src/rpc/mock/calendar.ts
  - app/src/rpc/mock/calendarEdits.ts
given:
  - app/src/rpc/mock/calendar.edit.test.ts
acceptance: make ui-vitest F="src/rpc/mock/calendar"
---
# T-0144: Event writes in MockTransport

## Goal

The app's tests run against `MockTransport`. Give its calendar domain the
writes M5.5 adds (`calendar.create`, `update`, `delete` and `restore`;
schema/rpc/calendar.yaml), behaving as maild's do (ADR-0030 to ADR-0032),
so the event editor, the scope sheet and the event pane's Edit and Delete
can be built against it. The mock keeps its own simple model of a series:
daily or weekly, `count` times, now with overrides and excluded
occurrences.

## Read first

- `schema/rpc/calendar.yaml`: the four methods, `EventFields`,
  `EventScope`, `EventWrite`; `app/src/rpc/gen/api.ts` has their types.
- ADR-0030, ADR-0031 and ADR-0032; `docs/design/pim.md`: "Editing".
- `app/src/rpc/mock/calendar.ts`: `MockEvent`, `occurrences`,
  `MockCalendar` (`dispatch`, `range`, `event`, `respond`).
- `app/src/rpc/mock/peopleEdits.ts` (T-0134): how the people domain's
  writes and restores are made; these follow it.
- `app/src/rpc/mock/fixture.ts`: `CALENDARS` (Work 201, the default;
  Home 202; the read-only Holidays 203) and the events.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

### The model (`calendar.ts`)

- Export `MockOverride`: a `Partial` of `CalendarEvent`'s `summary`,
  `location`, `description`, `allDay`, `start`, `end`, `startDate`,
  `endDate`, `timeZone`, `alarms` and `transparent`. `MockEvent` gains
  `overrides?: Record<string, MockOverride>` and `excluded?: string[]`,
  both by recurrence ID.
- A series' n-th occurrence (from 0) has the recurrence ID of its
  original start: for a timed event, the event's `start` plus n days (n
  weeks when weekly) as an ISO string (`2026-10-20T10:00:00.000Z`); for an
  all-day one, `startDate` plus n days or weeks (`2026-10-20`). Export
  that as `occurrenceKey(mock, n)` from `calendarEdits.ts`.
- `occurrences` leaves out excluded IDs and lays each override over its
  occurrence: an override with its own times uses them, else the
  occurrence's own. `calendar.event` with a `recurrenceId` returns the
  override's values too.

### The writes (`calendarEdits.ts`)

A class `MockEventEdits`, made by `MockCalendar` with its data,
collections, `emit` and a way to read an event (`this.event`).
`MockCalendar.dispatch` answers the four methods through it. Every write
emits `calendar.changed {accountId}` (for a move, of both calendars'
accounts), keeps a restore under a new ID, and returns `EventWrite`
(`event` absent after a delete).

- **Fields.** Times come whole or not at all: any of `allDay`, `start`,
  `end`, `timeZone`, `startDate`, `endDate` needs `allDay`, and then
  `start` before `end` with a `timeZone` `Intl` knows, or valid
  `startDate` before `endDate` (stored with `start` and `end` at their
  UTC midnights and `timeZone` ""). An `rrule` is `""` or has
  `FREQ=DAILY` or `FREQ=WEEKLY` (the mock's `every`), `COUNT` times (10
  when absent); any other frequency is `invalidParams`. It sets
  `recurrence` (`RRULE:<rule>`, or "") and `recurring`. `summary`,
  `location`, `description`, `transparent` and `alarms` replace theirs.
  Anything else invalid is `invalidParams`.
- **Writable:** an event is found by ID (`notFound`); a read-only event,
  or one whose calendar is read-only or disabled, is `conflict`, as is a
  write into such a calendar. A calendar that does not exist is
  `notFound`.
- **`calendar.create {calendarId?, fields}`**: into the calendar named,
  else the default writable calendar (else the first writable one). It
  needs its times. The event has a new ID (one more than the highest,
  at least 301), `uid` `mock-<id>`, and empty or default values under the
  fields: status `confirmed`, no attendees or alarms, not read-only.
- **`calendar.update {id, recurrenceId?, scope?, fields}`**:
  - An invitation (an organizer who is not the user) takes only `alarms`;
    anything else is `invalidParams`, as is `future` for it.
  - A single event takes the fields; an `rrule` makes it a series.
  - A series needs a `recurrenceId` and a `scope` (`invalidParams`
    otherwise), and the ID must name one of its occurrences that is not
    excluded (`notFound`).
  - `this` merges the fields into that occurrence's override (an `rrule`
    is `invalidParams`); the result is the occurrence.
  - `all` changes the series from that occurrence: new times move the
    series so that this occurrence gets them (its start less n steps is
    the first's), and the overrides and excluded IDs move with their
    occurrences; a new `rrule` drops those past its count (all of them
    for `""`). The result is the event.
  - `future` from the first occurrence is `all`. Otherwise the series
    ends before the occurrence (its `count` becomes n, and its overrides
    and excluded IDs from there go) and a new series with a new ID and
    `uid` starts at it with the rest of the count, those overrides and
    excluded IDs, and the fields applied as `all` from its first
    occurrence. The result is the new series.
  - `fields.calendarId` moves the event (a series only with `all`;
    otherwise `invalidParams`) to that writable calendar, and its
    account.
- **`calendar.delete {id, recurrenceId?, scope?}`**: a single event,
  `all`, or `future` from the first occurrence removes the event; `this`
  excludes the occurrence and drops its override; `future` ends the
  series before it. An invitation's series is deleted only with `all`.
- **`calendar.restore {restoreId}`** puts back every event a write
  changed as that write found it, at its old place, and removes what the
  write created (a new event, a split's new series). A restore runs once;
  an unknown or used ID is `notFound`. It returns null.

## Tests (given, do not edit)

`app/src/rpc/mock/calendar.edit.test.ts`: create, refusals, a change and
its restore, an invitation's alerts, one occurrence, a whole series
moved, a split and its restore, deletes and their restores, a move.

## Gotchas

- Recurrence IDs of one shape compare as strings in time order.
- Keep each write's restore as deep copies (`structuredClone`).
- `calendar.test.ts` (T-0072's) must still pass.

## Out of scope

The app's UI; every file not under `touch`.

## Done when

`make accept T=0144` and `make ui-check` pass, and only the files under
`touch` changed.
