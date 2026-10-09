---
id: "0067"
title: Read events from iCalendar objects
milestone: M4.5
size: L
touch:
  - internal/calendar/event.go
  - internal/calendar/zone.go
given:
  - internal/calendar/event_test.go
acceptance: go test ./internal/calendar -count=1
---
# T-0067: Read events from iCalendar objects

## Goal

maild keeps each calendar object as the server sent it (ADR-0018) and
indexes its events for the Calendar module, invitations and reminders.
Write `calendar.Parse`: the VEVENTs of an object (a single event, or a
series' master with its overrides) with their times resolved to instants
through IANA zones, Outlook's Windows zone names and VTIMEZONEs, all-day
dates, the people, the user's answer and the alarms. Expanding series is
the next card.

## Read first

- `docs/design/pim.md`: "Calendars"; ADR-0018 ("Time zones are IANA").
- `internal/calendar/event.go`: the types, their field comments and the
  stubs. `internal/calendar/zones.go`: `WindowsZone`.
- `internal/contentline/contentline.go`: `Parse`, `Component`
  (`ChildrenNamed`, `Prop`, `PropsNamed`), `Prop` (`Param`, `Text`,
  `Value`, `Encode`). Use it; do not parse content lines yourself.
- RFC 5545 §3.3.5 (DATE-TIME forms), §3.3.6 (DURATION), §3.6.6 (VALARM),
  §3.8.2 to §3.8.5 (DTSTART, DTEND, DURATION, RRULE, RDATE, EXDATE).
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the types and signatures; replace the stub comments with what the
code does and remove `errNotYet`. Put zone resolution in `zone.go`.

**`Parse(raw, opts)`**: `contentline.Parse` (errors wrapped with `%w`), the
first `VCALENDAR` (none is an error), then each `VEVENT` in order. A VEVENT
without a usable `DTSTART` is left out. A VCALENDAR without VEVENTs gives
no events and no error. `opts.Local` nil means `time.Local`.

**Times** (`DTSTART`, `DTEND`, `RECURRENCE-ID`):

- `VALUE=DATE`, or an 8-digit value: an all-day date, midnight UTC; the
  event is all-day when `DTSTART` is; `TZID` "", `Zone` `time.UTC`.
- A value ending in `Z`: UTC; `TZID` "UTC", `Zone` `time.UTC`.
- With a `TZID` parameter: the wall time in that zone (below), as a UTC
  instant; `TZID` the zone's name, `Zone` the zone.
- Otherwise floating: the wall time in `opts.Local`; `Floating` true, `TZID`
  "", `Zone` `opts.Local`.

**Zones**, the first that works: `time.LoadLocation(tzid)` (not for "" or
"Local"; `TZID` is the location's name); `WindowsZone(tzid)`; the object's
`VTIMEZONE` with that `TZID` (its `TZID` property's `Text()`), by its
`X-LIC-LOCATION` loaded with `time.LoadLocation`, else as a fixed zone at
the `TZOFFSETTO` of its `STANDARD` sub-component with the latest
`DTSTART` (or, with no `STANDARD`, its `DAYLIGHT` one), named like
`UTC+05:30` (sign, two-digit hours and minutes), which is also the `TZID`;
else UTC (`TZID` "UTC").

**End**: `DTEND`; else `DTSTART` plus `DURATION`; else for all-day one day
later and for a timed event the start itself.

**Fields**: `UID` trimmed; `SUMMARY`, `LOCATION`, `DESCRIPTION` with
`Text()`; `STATUS` `TENTATIVE` → `tentative`, `CANCELLED` → `cancelled`,
anything else or none → `confirmed`; `TRANSP:TRANSPARENT` → `Transparent`;
`SEQUENCE` as an integer (0 when missing or bad); `RECURRENCE-ID` read as
a time and formatted with `RecurrenceKey`; `Recurrence` the `RRULE`,
`RDATE` and `EXDATE` lines in source order, each as one unfolded line
(`Prop.Encode` without its line ending, or the source line unfolded), joined
by "\n".

**People**: `ORGANIZER` → `Organizer` with `Role` `chair`, `Name` its `CN`,
`PartStat` `accepted`. Each `ATTENDEE` → an `Attendee`: `Email` the value
without a case-insensitive `mailto:`, trimmed and lowercased; `Name` its
`CN`; `ROLE` `CHAIR` → `chair`, `OPT-PARTICIPANT` → `optional`,
`NON-PARTICIPANT` → `none`, else `required`; `PARTSTAT` `ACCEPTED`,
`DECLINED`, `TENTATIVE`, `DELEGATED` → the lowercase word, else
`needsaction`. An attendee with the organizer's email is not listed: it
gives the organizer its `PARTSTAT` (when it has one) and, when the
organizer has no `CN`, its name. `PartStat` is the first listed attendee's
whose email is in `opts.UserEmails`, else "".

**Alarms**: each `VALARM` with `ACTION` `DISPLAY` or `AUDIO` (lowercased in
`Action`) and a readable `TRIGGER`: with `VALUE=DATE-TIME`, `At` (UTC);
otherwise a duration (`[+-]P[nW][nD][T[nH][nM][nS]]`) in `Offset`, with
`RelatedEnd` for `RELATED=END`. Other actions and unreadable triggers are
left out.

**`RecurrenceKey(t, allDay)`**: `t.UTC()` as `2006-01-02` when all-day,
else as `2006-01-02T15:04:05.000Z` (`store.TimeFormat`; keep `calendar`
free of an import of `store`).

## Tests (given, do not edit)

`internal/calendar/event_test.go`: TestGoogleEvent, TestOutlookInvitation,
TestAllDay, TestUTCAndFloating, TestZones, TestOverrides, TestAlarms,
TestPeople, TestParseOddities, TestRecurrenceKey.

## Gotchas

- A quoted `TZID` parameter may hold commas; `Prop.Param` already unquotes
  it. A VTIMEZONE's `TZID` property escapes them (`\,`): compare its
  `Text()`.
- Read DTSTART in its zone before converting to UTC, so DST applies.
- Precompile the duration pattern at package level, or parse by hand.
- Keep functions under 60 lines: times, zones, people and alarms each read
  well on their own.

## Out of scope

Expanding RRULE, RDATE and EXDATE (T-0068), VTODO, the store, and every
file not under `touch`.

## Done when

`make accept T=0067` and `make check` pass, and only the files under
`touch` changed.
