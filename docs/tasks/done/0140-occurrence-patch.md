---
id: "0140"
title: Change or delete one occurrence of a series
milestone: M5.5
size: M
touch:
  - internal/calendar/occurrence.go
  - internal/calendar/eventpatch.go
given:
  - internal/calendar/occurrence_test.go
acceptance: go test ./internal/calendar -count=1
---
# T-0140: Change or delete one occurrence of a series

## Goal

Editing an occurrence of a repeating event asks This Event, All Future
Events or All Events (ADR-0031). This card is This Event: changing one
occurrence writes an override (a VEVENT with `RECURRENCE-ID`), made from
the master when the occurrence has none, and deleting one adds an
`EXDATE` to the master. Every other byte stays as the server sent it
(ADR-0018). The other two scopes are T-0141's.

## Read first

- ADR-0018, ADR-0030, ADR-0031; `docs/design/pim.md`: "Editing".
- `internal/calendar/eventpatch.go` (T-0139's `PatchEvent`, whose rules a
  change to an override follows) and its test's fixtures, which the given
  test uses (`ev`, `same`, `sp`, `eventNow`, `nowStamp`, `googleEvent`,
  `appleSeries`).
- `internal/calendar/event.go` and `expand.go`: `Parse`, `Expand`,
  `Occurrence.RecurrenceID`, `RecurrenceKey`, `VEventRecurrenceID`.
- `internal/contentline`: `Parse`, `Prop.Encode`, `Apply`, `Edit`,
  `LineEnding`.
- The stub `internal/calendar/occurrence.go`: keep `Scope`, its
  constants, `PatchOccurrence` and `DeleteOccurrence` as declared; replace
  the bodies and `errOccurrenceNotYet`.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

### The series and the occurrence

- The source must parse and hold a VCALENDAR whose master (its first
  VEVENT without `RECURRENCE-ID`) repeats (`RRULE` or `RDATE`, as
  `Parse` reads it). Otherwise it is an error.
- `recurrenceID` names an occurrence as `Expand` does: its original start
  as `RecurrenceKey` writes it (`2026-10-22T13:00:00.000Z`, or
  `2026-10-26` for an all-day series). It is an error when it does not
  parse, or when the series has no such occurrence (`Expand` over a span
  around it does not yield it) and no override has that ID.
- The occurrence's override is the VEVENT whose `VEventRecurrenceID` is
  `recurrenceID`.
- `ScopeFuture` and `ScopeAll` return an error saying they come with
  T-0141, after the checks above.

### `PatchOccurrence(src, id, ScopeThis, c, newUID, now)`

- `split` is always nil; `newUID` is unused.
- `c.RRule` set (even to `""`) is an error: one occurrence does not
  repeat.
- **With an override:** the override is patched by `PatchEvent`'s rules
  (lines, text, timing, `SEQUENCE`, stamps, alarms, a new zone's
  VTIMEZONE). Refactor `PatchEvent` so the same code patches a given
  VEVENT; `PatchEvent`'s own behaviour does not change.
- **Without one:** a new override is inserted right after the master's
  `END:VEVENT` line, then patched as above. Its lines are the master's,
  in the master's order and bytes, with these changes:
  - `RRULE`, `RDATE` and `EXDATE` are left out;
  - `DTSTART` is the occurrence's original start, followed at once by a
    `RECURRENCE-ID` with the same value;
  - `DTEND` is the original start plus the master's length;
  - both written like the master's `DTSTART`: `;VALUE=DATE:20261026` for
    an all-day series, `20261022T130000Z` when it is in UTC, else
    `;TZID=<its TZID>:<wall time in its zone>`;
  - the master's sub-components (VALARMs) are copied after its
    properties, byte for byte;
  - it starts `BEGIN:VEVENT` and ends `END:VEVENT`, in the source's line
    ending.

### `DeleteOccurrence(src, id, ScopeThis, now)`

- An `EXDATE` for the original start, written like `DTSTART` above, is
  inserted in the master before its first sub-component (or before its
  `END:VEVENT` line when it has none).
- The master's `SEQUENCE` becomes one more (0 when absent, so 1), and its
  `DTSTAMP` and `LAST-MODIFIED` become `now`, as `PatchEvent` writes
  them.
- The occurrence's override, when it has one, is removed.
- Anything but `ScopeThis` and `ScopeFuture` is an error (deleting a
  whole series deletes its object, which is not a patch); `ScopeFuture`
  is T-0141's error, as above.

## Tests (given, do not edit)

`internal/calendar/occurrence_test.go`:
`TestPatchOccurrenceMakesAnOverride`,
`TestPatchOccurrencePatchesItsOverride`,
`TestPatchOccurrenceMovesOneOccurrence`,
`TestPatchOccurrenceOfAnAllDaySeries`, `TestDeleteOccurrence`,
`TestOccurrencesThatAreNot`.

## Gotchas

- Read the source with `time.Local` as `Options.Local`, as the stores do;
  the tests' series all carry zones or dates.
- The simplest override: build its text, `Apply` the insertion, parse the
  result again, and patch the new override in it.
- Collect a patch's edits and `contentline.Apply` them once; insertions at
  the same offset keep their order.
- Keep functions under 60 lines.

## Out of scope

This and Future Events, All Events, and deleting a series (T-0141); the
calendar API's use of these (the planner's); every file not under
`touch`.

## Done when

`make accept T=0140` and `make check` pass, and only the files under
`touch` changed.
