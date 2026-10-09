---
id: "0069"
title: Index events and serve calendar.range
milestone: M4.5
size: L
touch:
  - internal/store/events.go
  - internal/pimsync/calendar.go
  - internal/pimsync/index.go
  - internal/pimsync/dav.go
  - internal/pimsync/ops.go
  - internal/pimsync/pimsync.go
  - internal/engine/calendar.go
  - internal/engine/people.go
given:
  - internal/store/events_test.go
  - internal/pimsync/calendar_test.go
  - internal/engine/calendar_test.go
acceptance: go test ./internal/store ./internal/pimsync ./internal/engine -count=1
---
# T-0069: Index events and serve calendar.range

## Goal

pimsync stores calendar objects as sent but indexes nothing from them yet.
Index each object's events (with attendees and alarms), expand them into
the instances window (a year back, two years ahead: ADR-0018), move the
window as days pass, and serve `calendar.range`, `calendar.event` and the
contact card's upcoming events from that index.

## Read first

- `docs/design/pim.md`: "Calendars", "Time"; ADR-0018.
- `internal/store/migrations/0007_pim.sql`: `events`, `event_attendees`,
  `alarms`, `instances`, `instance_window` (their comments say how values
  are stored).
- `internal/store/events.go`: the types and stubs.
- `internal/store/people.go`, `collections.go`, `objects.go`: query style.
  Reads use `d.db`; writes run in a `*Tx`.
- `internal/calendar`: `Parse` and `Options` (event.go), `Expand` and
  `Occurrence` (expand.go), `ZoneNamed` (zone.go).
- `internal/pimsync`: `pimsync.go` (`Manager`, `Pass`, `pass`,
  `serviceOnce`, `Config.Now` and `Config.Local`), `index.go` (`indexDAV`),
  `dav.go` and `ops.go` (callers of `indexDAV`), and `calendar.go` (stubs).
- `schema/rpc/calendar.yaml` (`Occurrence`, `CalendarEvent`, `Attendee`,
  `range`, `event`); `schema/rpc/people.yaml` (`ContactCard.upcoming`).
- `internal/engine/calendar.go` and `people.go` (`Card`).
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

### Store (`internal/store/events.go`)

Keep the types and signatures; replace the stubs and remove
`errEventsNotYet`.

- Dates: an all-day row's `start_at` and `end_at` are `YYYY-MM-DD` (its
  `Start`/`End` are UTC midnights); a timed row's are `FormatTime`.
  `instance_window` holds `YYYY-MM-DD` and reads back as UTC midnights.
- **`IndexEvents(objectID, events)`** deletes the object's events (their
  attendees, alarms and instances go by cascade) and inserts these in
  order, attendees and alarms by position; it returns the new IDs in the
  same order. `AlarmRow.Offset` is stored in seconds.
- **`ReplaceInstances(objectID, rows)`** deletes the instances of the
  object's events and inserts these. A row whose `EventID` is not one of
  the object's events is an error.
- **`InstanceWindow`**: `ok` is false (and the error nil) when none is
  stored. **`SetInstanceWindow`** stores or replaces it.
- **`Occurrences(f)`** joins instances to their events, objects and
  collections, keeping calendars (`kind = 'calendar'`) that are enabled, of
  accounts whose calendar service is enabled. Timed rows overlap
  `[From, To)` (`start < To AND end > From`, or a zero-length one with
  `From <= start < To`); all-day rows overlap `[FromDate, ToDate)` as
  dates (`start < ToDate AND end > FromDate`). `CalendarIDs` and
  `WithEmail` (the event's organizer, or an `event_attendees` row of that
  event) narrow it; `Limit > 0` caps it. Order: `start_at` text (a date
  sorts before that day's times), then all-day first, event ID,
  recurrence ID. `Instance.EventID` is set; `Event` has no attendees or
  alarms.
- **`Event(id)`**: the event with its attendees and alarms in position
  order, `CalendarID`, `AccountID`, and `ReadOnly` (the collection or the
  account is read-only); `ErrNotFound` when there is none.
- **`Events(f)`**: events of calendars without attendees and alarms,
  ordered by object ID, then event ID. `Visible` keeps what `Occurrences`
  keeps; `CalendarIDs` narrows.

### pimsync

- **`Window(now)`**: `now`'s UTC day at midnight, less 365 days and plus
  730 days (`AddDate`).
- **`Instances(events, local, from, to)`** converts the rows to
  `calendar.Event` (UID, RecurrenceID, AllDay, Start, End, Recurrence;
  `Zone` is UTC for all-day events, else `calendar.ZoneNamed(TZID,
  local)`), calls `calendar.Expand`, and maps each occurrence to an
  `InstanceRow` with `EventID` the row's `ID`. The rows are one object's
  events.
- **The window.** Before a pass indexes any calendar object (replay or
  the calendar service), it makes sure the stored window is
  `Window(cfg.Now())`. When it is missing or different, the Manager
  expands every stored event (`Events` with no filter, so hidden calendars
  are ready when shown again) over the new window, object by object
  (`Instances`, `ReplaceInstances`), and stores the window, in one
  transaction. Guard this with a Manager mutex, since passes of different
  accounts run at once. The pass then indexes with that window.
- **Indexing.** `indexDAV` becomes a `pass` method (update its callers in
  `dav.go` and `ops.go`). For a calendar object it keeps
  `calendarMetadata` and `PutObject`, then, in the same transaction: when
  the object is a `vevent`, `calendar.Parse` with `Local: cfg.Local` and
  `UserEmails` (the account's address and its identities' addresses,
  lowercased; read once per pass), maps each event to an `EventRow`
  (organizer email and name; attendees; alarms with `Offset` or `At`
  set), `IndexEvents`, sets the IDs, and stores `Instances` over the
  window with `ReplaceInstances`. Otherwise, or when `Parse` fails, it
  calls `IndexEvents` with no events, so an object that stopped being an
  event loses its old ones. Deleting objects and collections needs nothing
  more: the schema cascades.

### Engine

- **`calendar.range`**: `from` and `to` are `YYYY-MM-DD` days in
  `timeZone` (an IANA name; default `time.Local`); `to` is 1 to 400 days
  after `from`. Anything else, or an unknown zone, is `invalidParams`.
  The filter is `From`/`To` = midnight of `from`/`to` in the zone (UTC),
  `FromDate`/`ToDate` = the strings, and `CalendarIDs`.
  - When the stored window covers the range (both instants and both
    dates within it), read `store.Occurrences`.
  - Otherwise compute on demand: `store.Events` (`Visible`,
    `CalendarIDs`), `pimsync.Instances` per object (with `time.Local`)
    over a span covering both the instants and the dates, then keep what
    `Occurrences` would keep.
  - Map to `api.Occurrence`: `eventId` is the instance's event (the
    master, or the override that replaced it); `recurring` when the
    instance's recurrence ID is set; `answer` is the event's `PartStat`
    when set. All-day: `startDate`/`endDate` from the instance's dates,
    `start`/`end` their midnights in the zone. Return them sorted by
    `start`, all-day first on a tie, then by event ID: a non-empty slice
    or an empty one, never nil.
- **`calendar.event`**: `store.Event`, `notFound` when it is missing.
  With a `recurrenceId` other than the event's own: the event must be a
  series' master (`Recurrence` set) and the ID must parse as
  `calendar.RecurrenceKey` would write it (`2006-01-02T15:04:05.000Z`, or
  `YYYY-MM-DD` for all-day); the times are that start plus the event's
  duration. Otherwise `notFound`.
  - `timeZone` is the event's TZID unless it starts with `UTC`
    (`UTC`, or a fixed offset).
  - `recurring` when `Recurrence` or `RecurrenceID` is set.
  - All-day: `startDate`/`endDate` from the dates.
  - `organizer`: role `chair`, answer `accepted`.
  - `isUser` when the address is the account's or one of its identities'.
  - `alarms`: minutes before the start: `-Offset`;
    `-(Offset + duration)` when `RelatedEnd`; `start - At` when absolute.
  - `attendees` and `alarms` are never nil.
- **`people.card`**: `upcoming` holds the occurrences (as in
  `calendar.range`, zone `time.Local`) with the address as organizer or
  attendee, from `DB.Now()` to 30 days later, at most 5, in start order.

## Tests (given, do not edit)

- `internal/store/events_test.go`: TestIndexEvents, TestOccurrences,
  TestInstanceWindowAndEvents.
- `internal/pimsync/calendar_test.go`: TestPassIndexesEvents,
  TestWindowMoves, TestInstances.
- `internal/engine/calendar_test.go`: TestCalendarRange,
  TestCalendarRangeParams, TestCalendarEvent, TestContactCardUpcoming.
  The server's clock reads 2026-10-08 12:00 UTC (`rpctest.Options.Now`).

## Gotchas

- Placeholders: build `IN (?, ?, …)` from `len(CalendarIDs)`; never paste
  values into SQL.
- `Occurrences` compares stored text: format the filter's instants with
  `FormatTime`, and pass the date strings as they are.
- The pass must not refetch anything to move the window: it re-expands
  stored rows. Floating times (`TZID` "", not all-day) expand in
  `cfg.Local`.
- The window check happens before `replay` too: replay can index a fetched
  calendar object.
- `slices.SortStableFunc` for the engine's order. Keep functions under 60
  lines.

## Out of scope

Reminders, invitations, `calendar.respond`, tasks (VTODO indexing), the
Calendar UI, schema or migration changes, and every file not under
`touch`.

## Done when

`make accept T=0069` and `make check` pass, and only the files under
`touch` changed.
