---
id: "0139"
title: Patch an event or a series' master, and write new events
milestone: M5.5
size: L
touch:
  - internal/calendar/eventpatch.go
given:
  - internal/calendar/eventpatch_test.go
acceptance: go test ./internal/calendar -count=1
---
# T-0139: Patch an event or a series' master, and write new events

## Goal

`calendar.update` and `calendar.create` change events by patching their
source, which pimsync then writes to the server (ADR-0030; docs/design/
pim.md, Editing). Patch a single event, or the master of a series, while
leaving every other byte as the server sent it (ADR-0018). New times are
written in their zone, with that zone's VTIMEZONE (ADR-0033). Recurring
scopes (an override, a split) are T-0140's.

## Read first

- ADR-0018, ADR-0030, ADR-0033; `docs/design/pim.md`: "Editing".
- `internal/contentline`: `Parse` (properties' and components' `Start`
  and `End`, a component's `Children`), `Prop.Encode`, `Apply` and `Edit`,
  `EscapeText`, `LineEnding`.
- `internal/calendar/event.go` (`Parse`, which the tests read results
  back with: times, alarms, recurrence), `vtimezone.go` (`VTimezone`),
  and `todopatch.go` (the same kind of patch for VTODOs).
- The stub `internal/calendar/eventpatch.go`: keep `EventChange`,
  `PatchEvent` and `NewEvent` as declared; replace the bodies and
  `errEventPatchNotYet`.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

### `PatchEvent(src, c, now)`

- **Which event:** the first VEVENT without `RECURRENCE-ID` in the first
  VCALENDAR. A source that does not parse, has no VCALENDAR or has no
  such VEVENT is an error. When `c` sets nothing, `src` comes back
  unchanged.
- **Lines:** each property `c` sets replaces that property's first line
  in the VEVENT and removes its repeats; one the VEVENT lacks is
  inserted. Insertions go before the VEVENT's first sub-component (its
  first VALARM), or before `END:VEVENT` when it has none, in this order:
  `SUMMARY`, `LOCATION`, `DESCRIPTION`, `DTSTART`, `DTEND`, `RRULE`,
  `TRANSP`, `SEQUENCE`, `DTSTAMP`, `LAST-MODIFIED`. A property set to
  `""` is removed. Lines are written with `Prop.Encode` in the source's
  own line ending. Everything else, other VEVENTs (overrides) included,
  stays byte for byte.
- **Text:** `SUMMARY`, `LOCATION`, `DESCRIPTION` are escaped
  (`EscapeText`).
- **Timing:** when any of `AllDay`, `Start`, `End`, `Zone`, `StartDate`,
  `EndDate` is set, the whole timing is given, or it is an error:
  - **timed** (`AllDay` false): `Start`, `End` (after `Start`) and `Zone`
    (`UTC`, or a name `time.LoadLocation` takes; not "" or `Local`).
    In `UTC` it is `DTSTART:20261016T140000Z`; in a zone,
    `DTSTART;TZID=<zone>:<wall time there>`, and `DTEND` the same way;
  - **all-day** (`AllDay` true): `StartDate` and `EndDate`, valid
    `YYYY-MM-DD` dates with the end after the start:
    `DTSTART;VALUE=DATE:20261016`, and `DTEND` the same way.

  The new lines replace `DTSTART` and `DTEND` whole, parameters
  included, and any `DURATION` is removed. A zone other than UTC that the
  VCALENDAR has no VTIMEZONE for (by `TZID` text) gets
  `VTimezone(zone, Start, eol)`, inserted at the start of the
  VCALENDAR's first VEVENT.
- **`RRule`:** the `RRULE` value as given, checked with rrule-go's
  `StrToROption` (an error when it does not parse); `""` removes `RRULE`.
  `RDATE` and `EXDATE` stay.
- **`Transparent`:** `TRANSP:TRANSPARENT`, or `TRANSP:OPAQUE` when false.
- **`SEQUENCE`** becomes one more than before (0 when absent) when the
  timing or the rule changes.
- **`DTSTAMP` and `LAST-MODIFIED`** become `now` in UTC
  (`20261014T153000Z`) whenever `c` sets anything.
- **`Alarms`** removes the VEVENT's VALARMs whose `ACTION` is `DISPLAY`
  or `AUDIO`, and keeps the others (Google's `EMAIL`). One alarm per
  value is added before `END:VEVENT`:
  `BEGIN:VALARM`, `ACTION:DISPLAY`, `DESCRIPTION:<summary, escaped>`,
  `TRIGGER:<duration>`, `END:VALARM`. The summary is the new one when `c`
  sets it, else the event's, else `Reminder`.
  - The duration is minutes before the start: `-` when before, none when
    after, then `P`, whole days as `nD`, then, for the rest, `T` with
    `nH` and `nM` when not zero.
  - Examples: 15 is `-PT15M`, 90 `-PT1H30M`, 1440 `-P1D`, 2340
    `-P1DT15H`, −540 `PT9H`, and 0 is `PT0S`.

### `NewEvent(uid, c, now)`

A calendar object with CRLF lines, written with `Prop.Encode`, in this
order:
- `BEGIN:VCALENDAR`, `VERSION:2.0`, `PRODID:-//Frostyard//Frostmail//EN`;
- the zone's VTIMEZONE, for a timed event in a zone other than UTC;
- `BEGIN:VEVENT`, `UID`, then `DTSTAMP`, `CREATED` and `LAST-MODIFIED`
  (all `now`), and `SEQUENCE:0`;
- `DTSTART` and `DTEND` as above (the whole timing is required);
- `SUMMARY`, `LOCATION`, `DESCRIPTION` when set and not empty, `RRULE`
  when set and not empty (checked), `TRANSP` when set, and the alarms as
  above;
- `END:VEVENT`, `END:VCALENDAR`.

## Tests (given, do not edit)

`internal/calendar/eventpatch_test.go`: `TestPatchEventTextKeepsTheRest`,
`TestPatchEventMovesTheTimeInItsZone`,
`TestPatchEventInANewZoneAddsItsVTimezone`,
`TestPatchEventToAllDayAndToUTC`, `TestPatchEventRefusesWhatCannotBeWritten`,
`TestPatchEventAlarms`, `TestPatchEventRuleAndTransparencyOnTheMaster`,
`TestNewEvent`.

## Gotchas

- Collect the edits and `contentline.Apply` them once. An insertion at
  the offset where a rewritten line starts is applied first.
- The insertion point before `END:VEVENT` is the start of that line,
  found from the component's `End`.
- The master may come after an override in the object.
- Keep functions under 60 lines.

## Out of scope

Occurrences and scopes (T-0140), the calendar API's use of these (the
planner's), and every file not under `touch`.

## Done when

`make accept T=0139` and `make check` pass, and only the files under
`touch` changed.
