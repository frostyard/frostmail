---
id: "0141"
title: Change a whole series from one of its occurrences
milestone: M5.5
size: L
touch:
  - internal/calendar/occurrence_all.go
  - internal/calendar/occurrence.go
given:
  - internal/calendar/occurrence_all_test.go
acceptance: go test ./internal/calendar -count=1
---
# T-0141: Change a whole series from one of its occurrences

## Goal

All Events (ADR-0031) changes a series' master from whichever occurrence
the user edited. When the change moves the occurrence, the whole series
moves the same way, and its `EXDATE`s, `RDATE`s and overrides move with
it so each still names its occurrence. Overrides take the change where
they still had the master's old value, and keep what the user changed
for them alone. When the rule changes, the overrides and `EXDATE`s that
no longer name an occurrence go. Every byte the change does not need to
touch stays as the server sent it (ADR-0018).

## Read first

- ADR-0018, ADR-0031 (All Events); `docs/design/pim.md`: "Editing".
- `internal/calendar/occurrence.go` (T-0140: `PatchOccurrence`, which
  already sends `ScopeAll` to `patchAll`; `occurrencePatch`,
  `readOccurrencePatch`, `timeProp`) and `eventpatch.go`
  (`patchEventComponent`, `eventTiming`, `eventPatchMaster`).
- `internal/calendar/event.go` (`readEvent`, `timeReader`,
  `VEventRecurrenceID`, `RecurrenceKey`) and `expand.go` (`Expand`,
  `recurrenceDates`, `eventZone`).
- The stub `internal/calendar/occurrence_all.go`: keep `patchAll`'s
  signature; replace its body and `errAllEventsNotYet`.
- The given test, and the fixtures it uses in `eventpatch_test.go` and
  `occurrence_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

`patchAll(src, p, c, now)` is `PatchOccurrence(src, id, ScopeAll, c, "",
now)` after `readOccurrencePatch` has found the occurrence: `p.start` is
its original start, `p.master` and `p.event` the master. `split` stays
nil.

### Checks

- `c`'s timing, when any of it is set, is checked as `PatchEvent` checks
  it (`eventTiming`), and `c.RRule`, when not `""`, with rrule-go's
  `StrToROption`. A failed check is an error, and nothing is written.
- A change that sets nothing returns `src` unchanged.

### Frames and the shift

- A series writes its times in a **frame**: dates (`;VALUE=DATE:`), UTC
  (`...Z`), wall times in a zone (`;TZID=<name>:`), or floating wall
  times. The old frame is the master's `DTSTART`'s; the new one is `c`'s
  (all-day; `UTC`; or the zone `c.Zone` names, written as
  `TZID=<c.Zone>`), or the old one when `c` sets no timing.
- `c`'s times are the edited occurrence's new times. The **shift** maps
  the occurrence's original start `o` (in the old frame) to `c`'s new
  start `n` (in the new frame), and every other time `t` the same way:
  read `t` and `o` as civil dates and wall clocks in the old frame (dates
  at 00:00 for an all-day series); the result is `n`'s date plus the days
  from `o`'s date to `t`'s, at `n`'s wall clock plus the difference of
  `t`'s clock from `o`'s, in the new frame. Into an all-day frame only
  the days count. So a weekly 09:00 series moved to 10:00 from a
  November occurrence is at 10:00 on every date, on either side of a DST
  change.
- The series' new length is `c.End − c.Start` (or the days from
  `StartDate` to `EndDate`); without timing, the old one.
- The series **moved** when the shifted master start differs from its
  start or the frame changed (all-day, UTC or `TZID`).

### The master

The master is patched with `patchEventComponent`, by `c` with its timing
replaced: the shifted master start and that plus the new length, in the
new frame. When the series moved and `c.RRule` is nil, an `UNTIL` in its
`RRULE` is shifted too, and the rule written back with only that part
changed: `UNTIL=<date>` in an all-day frame, else `UNTIL=<UTC time>Z`.

### EXDATE, RDATE and overrides

Each is decided on the source before any edit, and the edits applied in
one `contentline.Apply` ahead of the master's patch:

- **Members.** When the series moved or `c.RRule` is set, the changed
  series is expanded (its new start, length, frame and rule, and its
  shifted `RDATE`s, without `EXDATE`s) around the shifted times; an
  `EXDATE` value or override whose shifted time is not one of its
  occurrences no longer names one. With no rule, nothing is a member.
- **`EXDATE` and `RDATE` lines** (but `VALUE=PERIOD`): every value is
  shifted, and `EXDATE` values that name no occurrence are dropped. A
  line is rewritten in the new frame only when the series moved or a
  value was dropped, and removed when no value is left; otherwise its
  bytes stay.
- **Overrides** (the object's VEVENTs with a `RECURRENCE-ID`): one whose
  shifted `RECURRENCE-ID` names no occurrence is removed whole. When the
  series moved, the others' `RECURRENCE-ID` line is rewritten as its
  shifted time in the new frame.
- **What follows into an override:** each of `c`'s `Summary`,
  `Location`, `Description`, `Transparent` and `Alarms` whose value the
  override shared with the old master (as `readEvent` reads both: text,
  transparency, and the same alarms in order). When the series moved or
  its length changed, an override whose times were its occurrence's own
  (its start the original start, its length the master's old length) is
  moved to its shifted `RECURRENCE-ID` with the new length, in the new
  frame. An override's own values and times stay. Each override that
  takes anything is patched with `patchEventComponent`, after the
  master: parse the source again, and find the override by its new
  `VEventRecurrenceID`.

## Tests (given, do not edit)

`internal/calendar/occurrence_all_test.go`:
`TestPatchAllRenamesTheSeries`, `TestPatchAllMovesTheSeries`,
`TestPatchAllMovesUntil`, `TestPatchAllChangesTheRule`,
`TestPatchAllOfAnAllDaySeries`, `TestPatchAllToAllDay`,
`TestPatchAllOfNothingAndOfWhatCannotBe`.

## Gotchas

- Work out every decision on the first parse (the old master, old
  values, byte offsets), then apply: the list and override edits, then
  the master, then the overrides one by one.
- Civil arithmetic: carry dates and wall clocks as `time.Time` in UTC,
  and build the result with `time.Date` in the new zone, which
  normalizes overflowing days and seconds.
- `Expand` adds the master's start as an occurrence even when the rule
  would not; overrides of it stay.
- The patched master adds the new zone's VTIMEZONE (when the object has
  none), which the overrides' patches then find.
- Keep functions under 60 lines; a plan struct holding the decisions
  helps.

## Out of scope

All Future Events and its delete (T-0142); the calendar API's use of
these (the planner's); every file not under `touch`.

## Done when

`make accept T=0141` and `make check` pass, and only the files under
`touch` changed.
