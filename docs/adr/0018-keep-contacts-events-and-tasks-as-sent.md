# 0018 — Keep each contact, event and task as the server sent it

- **Status:** Accepted
- **Date:** 2026-10-08

## Context

vCard and iCalendar carry far more than a client shows: vendor properties
(`X-APPLE-…`, `X-GOOGLE-…`), parameters, alarms per device, attachments,
structured locations, and properties from future revisions. A client that
parses an object into its own model and writes that model back loses
whatever the model lacks, and every other client of the account sees the
loss. Calendars add recurrence: one stored event can stand for thousands of
occurrences, some of them overridden by objects of their own
(`RECURRENCE-ID`), and the views ask for whatever falls in a week.

## Decision

- **Store the source.** Every contact (vCard), event or task (iCalendar),
  and Google task (JSON) is stored byte for byte as the server sent it,
  with its href and ETag. Parsed columns (names, addresses, times, the
  attendee list, the user's answer) are an index over that source, rebuilt
  from it, never the other way round.
- **Edit surgically.** A change Frostmail makes (an answer to an
  invitation, a new contact, a completed task) is a patch applied to the
  stored source: the properties it touches are replaced, everything else is
  written back as it was. A property Frostmail does not understand is never
  dropped. Writes are conditional on the stored ETag; a server that answers
  412 is refetched, and the patch is applied again or the user decides.
- **Expand occurrences into a window.** Each event's occurrences (RRULE,
  RDATE and EXDATE, with overrides by `RECURRENCE-ID`) are materialized
  into an instances table for a window around today (a year back, two
  ahead), extended as the user navigates; views and reminders read
  instances, never rules.
- **Time zones are IANA.** Times are stored in UTC with their IANA zone;
  Windows zone names (Outlook's invitations) map to IANA through a table
  generated from CLDR's `windowsZones.xml`, pinned and checksum-verified
  (frostyard/core ADR-0023); a zone that is neither is evaluated from the
  object's own `VTIMEZONE`. All-day events are dates, not times. tzdata is
  built into maild (`time/tzdata`), so the sandbox's copy does not matter.

## Consequences

- Edits in Frostmail are safe for every other client of the account, and a
  sync can always rebuild the index after a parser fix.
- Storage holds source and index both; contacts and events are small, so
  the cost is a few megabytes for a large calendar.
- Writing is harder than serializing a struct: each kind of change needs a
  patch function with golden tests (source in, source out).
- Occurrences beyond the window are computed on demand (a search far in
  the past), which is slower than reading the table.

## Alternatives considered

- **Parse into a model and serialize it back:** simple, and the common
  cause of other clients losing data.
- **Expand recurrence at query time only:** no table to keep in step, but
  every view and every reminder check would expand every series.

## References

- Shapes: [design/pim.md](../design/pim.md),
  [design/storage.md](../design/storage.md)
- Builds on: [ADR-0017](0017-sync-contacts-calendars-and-tasks.md)
