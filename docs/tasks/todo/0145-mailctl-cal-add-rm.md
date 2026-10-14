---
id: "0145"
title: mailctl cal add and rm
milestone: M5.5
size: S
touch:
  - cmd/mailctl/cal.go
  - cmd/mailctl/cal_edit.go
given:
  - cmd/mailctl/cal_edit_test.go
acceptance: go test ./cmd/mailctl -run 'TestCal|TestPim' -count=1
---
# T-0145: mailctl cal add and rm

## Goal

Make events writable from the command line, over the calendar writes
maild now has (`calendar.create`, `calendar.delete`;
schema/rpc/calendar.yaml, ADR-0030, ADR-0031). `cal agenda --ids` names
each event, and each occurrence of a series, so `rm` can take them.

## Read first

- `cmd/mailctl/cal.go`: `newCalCmd`, `cal ls`, `cal agenda`,
  `agendaRange`, `printAgenda`.
- `cmd/mailctl/people_edit.go` (T-0138): the same kind of commands for
  contacts; `pimID` and `pimIDFlag` in `people.go`.
- `schema/rpc/calendar.yaml`: `EventFields`, `EventScope`, `EventWrite`,
  `create` and `delete`.
- The given test, and `pim_test.go`'s `pimFixture` (calendars Work,
  writable, and Home, read-only), `mustRun`, `cells`.
- `docs/tasks/EXECUTOR.md`

## Contract

- **`cal agenda --ids`** ends each event's line with two spaces and
  `[<event ID>]`, or `[<event ID> <recurrence ID>]` for an occurrence of
  a series (`Occurrence.eventId`, `recurrenceId`), after the summary,
  location and `[cancelled]`. Without `--ids` nothing changes.
- **`cal add`** (in a new `cal_edit.go`) calls `calendar.create`:
  - `--summary` (required, not blank), `--location`, `--notes`
    (`description`), `--repeat` (the `rrule` value as given), and the
    repeatable `--alert MINUTES` (`alarms`). Only flags given become
    fields, but the summary always is.
  - **Timed:** `--start` and `--end`, each `YYYY-MM-DD HH:MM` or
    `YYYY-MM-DDTHH:MM`, read in `--zone` (an IANA name). Without
    `--end`, the event is an hour long; an end not after the start is an
    error. Without `--zone`, the local zone's IANA name: `TZ` (without a
    leading `:`) when `time.LoadLocation` takes it, else the name after
    `zoneinfo/` in the target of `/etc/localtime`, else `UTC`. The
    fields are `allDay` false, `start`, `end` and `timeZone`.
  - **All-day:** `--date YYYY-MM-DD` and `--end-date` (the last day,
    inclusive; default `--date`; not before it). The fields are `allDay`
    true, `startDate`, and `endDate` the day after the last.
  - Exactly one of `--start` and `--date`; neither or both is an error.
  - `--calendar ID` sets `calendarId` (else maild's default calendar).
  - It prints `added <summary> (event <id>)` from the result's event.
- **`cal rm EVENT_ID`** calls `calendar.delete` and prints `deleted event
  <id>`. `--occurrence RECURRENCE_ID` and `--scope this|future|all` go
  together: either without the other, or another scope, is an error
  before dialing.
- With `--json`, `add` and `rm` print the `EventWrite` instead
  (`clix.OutputJSON`).
- Flag and time errors end the command before it dials; maild's errors
  (an unknown event, a read-only calendar, a rule that does not parse)
  end it with an error, and so does an ID that is not a number.

## Tests (given, do not edit)

`cmd/mailctl/cal_edit_test.go`: `TestCalWriteCommands`; `pim_test.go`'s
calendar tests must still pass.

## Gotchas

- `cobra`'s `Flags().Changed` tells a flag given from its default.
- `IntSliceVar` takes `--alert 15 --alert 30`.

## Out of scope

Editing events from the CLI (the app's editor does), and every file not
under `touch`.

## Done when

`make accept T=0145` and `make check` pass, and only the files under
`touch` changed.
