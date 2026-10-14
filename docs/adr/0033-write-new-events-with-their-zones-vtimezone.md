# 0033 — Write new events with their zone's VTIMEZONE

- **Status:** Accepted
- **Date:** 2026-10-14

## Context

An event that repeats across a daylight-saving change has to be written
in its zone, not in UTC, or its occurrences shift by an hour twice a
year. iCalendar names a zone with `TZID` and requires a `VTIMEZONE` for
every `TZID` an object uses (RFC 5545 §3.6.5). Google and Apple write IANA
names with a `VTIMEZONE` and read others' that way. maild already reads
zones by their IANA name, falling back to an object's own `VTIMEZONE`
([ADR-0018](0018-keep-contacts-events-and-tasks-as-sent.md)), and has
tzdata built in. Go's `time` exposes a zone's transitions one at a time
(`Time.ZoneBounds`), not its rules.

## Decision

- **Times are in a zone.** A new event, and a changed time, is written
  in the editor's zone (the app's own unless the user picked another):
  `DTSTART;TZID=<IANA name>` and `DTEND` the same way. All-day events are
  `VALUE=DATE` with no zone. Frostmail writes no floating times.
- **maild generates the `VTIMEZONE`** (`calendar.VTimezone`) from its
  built-in tzdata. Its `TZID` is the IANA name, and it carries
  `X-LIC-LOCATION` with the same name. It walks the zone's transitions
  with `ZoneBounds` from the year before the event's first start to 2037:
  - When the transitions of that span follow one yearly rule each for
    standard and daylight time (the same month, the same n-th or last
    weekday, the same local time), it writes a `STANDARD` and a
    `DAYLIGHT` component with that `RRULE`, starting at the first.
  - When they do not, each transition is its own `RDATE`.
  - A zone without transitions in the span is one `STANDARD` with its
    offset.
- **A patch keeps the object's zones.** An event that already has a
  `VTIMEZONE` for its `TZID` keeps it as sent. A new `TZID` adds the
  generated one. A `VTIMEZONE` that nothing uses any more is left in
  place.
- Golden tests cover a zone with daylight time in each hemisphere
  (Europe/Berlin, America/New_York, Australia/Sydney), one without
  (Asia/Kolkata), a half-hour shift (Australia/Lord_Howe), one whose rule
  ended (America/Sao_Paulo, 2019), and UTC.

## Consequences

- Recurring events keep their wall-clock time across daylight-saving
  changes in every client, whether that client reads the IANA name or
  the `VTIMEZONE`.
- The generator stays correct only as long as maild's tzdata is, which
  follows the Go release.
- Whether Google and iCloud keep Frostmail's `VTIMEZONE` or replace it
  with their own is a provider fact, checked in plan 0009's Phase 3. It
  does not change what maild reads.

## Alternatives considered

- **UTC times:** valid and simple, but a weekly 9:00 meeting moves to
  8:00 or 10:00 in half the year.
- **`TZID` without a `VTIMEZONE`:** most servers accept it, but it breaks
  RFC 5545, and clients without IANA names cannot place the event.
- **Embedding a published `VTIMEZONE` set** (tzurl's): pinned,
  checksum-verified data, but a second copy of tzdata to keep in step
  with Go's.

## References

- Shapes: [design/pim.md](../design/pim.md)
- Builds on: [ADR-0018](0018-keep-contacts-events-and-tasks-as-sent.md),
  [ADR-0030](0030-edit-contacts-and-events-as-patches.md)
