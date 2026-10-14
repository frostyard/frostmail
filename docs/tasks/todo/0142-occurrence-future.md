---
id: "0142"
title: Split a series at an occurrence, or end it there
milestone: M5.5
size: M
touch:
  - internal/calendar/occurrence_future.go
  - internal/calendar/occurrence.go
given:
  - internal/calendar/occurrence_future_test.go
acceptance: go test ./internal/calendar -count=1
---
# T-0142: Split a series at an occurrence, or end it there

## Goal

All Future Events (ADR-0031) changes an occurrence and those after it.
From any occurrence but the first, the series splits: the old object ends
before the occurrence, and a new object, under a new UID, starts at it
with the change applied. Deleting All Future Events is the first half
alone. From the first occurrence, a change is All Events and a delete
deletes the object. Every byte the split does not need to touch stays as
the server sent it (ADR-0018).

## Read first

- ADR-0018, ADR-0031 (All Future Events); `docs/design/pim.md`:
  "Editing".
- `internal/calendar/occurrence.go` (`PatchOccurrence` and
  `DeleteOccurrence`, which already send `ScopeFuture` here;
  `occurrencePatch`, `readOccurrencePatch`, `timeProp`),
  `occurrence_all.go` (T-0141's `patchAll`, which applies the change to
  the new series) and `eventpatch.go` (`eventPropertyEdits`,
  `eventRevisionFields`).
- `internal/calendar/expand.go`: `ruleStarts`, `recurrenceDates`,
  `eventZone`; `event.go`: `timeReader`, `RecurrenceKey`.
- The stub `internal/calendar/occurrence_future.go`: keep
  `ErrWholeSeries` and the two signatures; replace the bodies and
  `errFutureNotYet`.
- The given test, and the helpers it uses from `eventpatch_test.go`,
  `occurrence_test.go` and `occurrence_all_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

`o` is the occurrence's original start (`p.start`). It is the series'
**first** occurrence when it equals the master's start.

### `patchFuture(src, p, c, newUID, now)`

- From the first occurrence: `patchAll(src, p, c, now)`, and `split` is
  nil.
- Otherwise `newUID` must not be `""` (an error), and the result is both
  halves below. Any error from either is the call's error.

### The old object: `patched` (and `deleteFuture`)

The source with one `contentline.Apply`:

- The master's `RRULE` (when it has one) ends before `o`: its `COUNT` or
  `UNTIL` part is replaced by `UNTIL=<end>`, in that part's place (the
  first of them, the other removed), or appended after the other parts
  when it has neither. The end is the day before `o` (`20261101`) for an
  all-day series; a second before `o`, in UTC (`20261029T125959Z`), when
  `DTSTART` is in a zone or UTC; a second before in wall time when it is
  floating.
- The master's `SEQUENCE` becomes one more, and `DTSTAMP` and
  `LAST-MODIFIED` `now`, as `eventRevisionFields` writes them.
- `RDATE` and `EXDATE` values from `o` on are dropped (`VALUE=PERIOD`
  lines stay). A line that loses values is rewritten with the rest, in
  the master's form (`timeProp`, the values joined by commas), or
  removed when none is left; the others keep their bytes.
- Overrides whose `RECURRENCE-ID` is `o` or later are removed whole.

### The new object: `split`

The source with one `contentline.Apply`, then `c` applied by
`patchAll` from its first occurrence (`o`, found again with
`readOccurrencePatch`):

- The master's `DTSTART` is `o` and its `DTEND` (when it has one) `o`
  plus the master's length, both by `timeProp`.
- Its `RRULE` keeps its `UNTIL`; a `COUNT` becomes the count less the
  rule's starts before `o` (`ruleStarts` from the master's start to `o`).
  When none is left, the `RRULE` is removed.
- Its `UID` becomes `newUID`, and `CREATED`, `DTSTAMP` and
  `LAST-MODIFIED` become `now`.
- `RDATE` and `EXDATE` values before `o` are dropped, as above.
- Overrides before `o` are removed; the others' `UID` becomes `newUID`.
- Everything else (the VCALENDAR's properties, its VTIMEZONEs, the
  master's other lines and alarms) stays.

### `deleteFuture(src, p, now)`

From the first occurrence: `ErrWholeSeries` (the calendar API deletes the
object). Otherwise: the old object above.

## Tests (given, do not edit)

`internal/calendar/occurrence_future_test.go`:
`TestPatchFutureSplitsTheSeries`, `TestPatchFutureMovesTheRest`,
`TestPatchFutureOfAnAllDaySeries`, `TestPatchFutureKeepsUntil`,
`TestPatchFutureFromTheFirstIsAll`, `TestDeleteFuture`.

## Gotchas

- Build the new object first: it checks `c` (through `patchAll`), so a
  bad change fails before anything else is worked out.
- An override can come before the master in the object (Apple's).
- `eventPropertyEdits` replaces a property's first line and removes its
  repeats, which suits `RRULE`, `UID` and the stamps, but not `EXDATE`
  or `RDATE`, which may repeat.
- Keep functions under 60 lines.

## Out of scope

The calendar API's use of these (the planner's); every file not under
`touch`.

## Done when

`make accept T=0142` and `make check` pass, and only the files under
`touch` changed.
