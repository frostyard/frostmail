---
id: "0068"
title: Expand events into occurrences
milestone: M4.5
size: L
touch:
  - internal/calendar/expand.go
given:
  - internal/calendar/expand_test.go
acceptance: go test ./internal/calendar -count=1
---
# T-0068: Expand events into occurrences

## Goal

The Calendar module, contact cards, invitations and reminders read
occurrences, never rules (ADR-0018). Write `calendar.Expand`: the
occurrences of one calendar object's events in a range, from RRULE, RDATE
and EXDATE in each event's own zone (a 9:00 meeting stays at 9:00 across a
DST change), with overrides replacing the occurrences they name.

## Read first

- `docs/design/pim.md`: "Calendars"; ADR-0018 ("Expand occurrences into a
  window").
- `internal/calendar/event.go` (done in T-0067): `Event` (`Start`, `End`,
  `AllDay`, `Zone`, `Recurrence`, `RecurrenceID`), `Parse`,
  `RecurrenceKey`; `zone.go`: how a `TZID` resolves (reuse it for RDATE and
  EXDATE values with a `TZID`).
- `internal/calendar/expand.go`: `Occurrence` and the stub.
- `github.com/teambition/rrule-go` (v1.8.2, in `go.mod`):
  `StrToROptionInLocation`, `NewRRule`, `RRule.Between`.
- RFC 5545 §3.8.5 (RDATE, EXDATE, RRULE) and §3.8.4.4 (RECURRENCE-ID).
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep `Occurrence` and the signature; replace the stub's comment and
remove `errNotYet`'s use and the `var _ = rrule.…` line.

**`Expand(events, from, to)`** works on the events of one object, grouped
by UID: per UID, the event with `RecurrenceID` "" is the master; the others
are overrides. It returns the occurrences that **overlap** `[from, to)` —
`start < to && end > from`, or for a zero-length one `from <= start < to`
— ordered by start, then by `Occurrence.Event`. The error is for the
future; nothing here returns one.

- **An event with an empty `Recurrence`** is one occurrence: its start and
  end, `RecurrenceID` "" (an override without a master keeps its own
  `RecurrenceID`).
- **A series** (a master with `Recurrence`): its original starts are
  `DTSTART` (always the first, RFC 5545), every start of each `RRULE`, and
  each `RDATE` value, less each `EXDATE` value, compared by
  `RecurrenceKey`. Read `Recurrence` with `contentline` (its lines are
  content lines). RRULEs expand with rrule-go from `Start` in `Zone`
  (`StrToROptionInLocation(value, Zone)`, `Dtstart` set to
  `Start.In(Zone)`), so wall times keep across DST; only starts before `to`
  and after `from` minus the event's duration matter, so an endless series
  costs only the range. An `RRULE` rrule-go cannot read is skipped.
  `RDATE` and `EXDATE` values are comma-separated `DATE` (all-day),
  `DATE-TIME` in UTC (`Z`), or in the property's `TZID` (resolved as
  DTSTART's is), else in `Zone`; `PERIOD` values are skipped.
- Each original start `s` becomes an occurrence `[s, s + (End - Start))`
  with `RecurrenceID` `RecurrenceKey(s, AllDay)` and the master's index —
  unless an override has that `RecurrenceID`: then the override stands
  instead, at its own times, wherever it moved. Overrides count by their
  own times: one moved into the range shows even when its original start
  is outside it, and one moved out does not. Cancelled overrides are kept;
  their status is the views' to show.
- Times in `Occurrence` are UTC instants (UTC midnights for all-day, whose
  `AllDay` is set from the event).

## Tests (given, do not edit)

`internal/calendar/expand_test.go`: TestSingleEvents, TestSeriesAcrossDST,
TestExceptionsAndOverrides, TestRDates, TestAllDaySeries,
TestEndlessSeries, TestOrphansAndOdd. They build events with `Parse`.

## Gotchas

- rrule-go's `Between(after, before, inc)` compares instants; pass
  `inclusive` and widen `after` slightly so a start exactly at `from` minus
  the duration is kept.
- rrule-go returns times in `Dtstart`'s location: convert to UTC.
- A `DTSTART` the rule does not produce (RFC 5545 says it still counts) must
  be added, without duplicating one the rule does produce.
- Keep functions under 60 lines.

## Out of scope

Storing instances, the window, `calendar.range` (T-0069), RANGE=THISANDFUTURE
(treated as an override of one occurrence), and every file not under
`touch`.

## Done when

`make accept T=0068` and `make check` pass, and only the files under
`touch` changed.
