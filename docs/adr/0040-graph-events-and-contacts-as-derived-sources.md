# 0040 — Store Graph's events and contacts as iCalendar and vCard derived from its JSON

- **Status:** Accepted
- **Date:** 2026-10-15

## Context

Microsoft accounts sync contacts and calendars over Microsoft Graph
([ADR-0039](0039-microsoft-accounts-over-imap-smtp-and-graph.md)). Everything
downstream of sync reads iCalendar and vCard: the index (events,
instances, alarms, contacts, people), recurrence expansion, the
invitation card, reminders, verify, and editing. Edits are patches of the
stored source with re-application after a 412 (ADR-0018, ADR-0030), in
three recurring scopes (ADR-0031), with undo that restores what a change
replaced (ADR-0032). Google Tasks, the one JSON backend, stores Google's
JSON and indexes it on its own path. It has no recurrence or invitations.

Plan 0010's Phase 1 probed the user's Outlook.com account on 2026-10-15:

- **IDs:** a personal account's default item IDs equal its immutable IDs.
  Events carry an `iCalUId`, a `changeKey`, and `@odata.etag` (the change
  key, quoted).
- **Writes:** a `PATCH` with a stale `If-Match` gets 412
  (`ErrorIrresolvableConflict`). Creation answers 201 with the new ID.
- **Recurring series:** a series is a `seriesMaster` with a recurrence
  pattern, a range and its zone. Its `cancelledOccurrences` lists the
  deleted occurrences (`OID.<master>.<date>`), and `exceptionOccurrences`
  lists the moved ones as events of their own. A moved occurrence keeps
  following the master: renaming the series renamed it too.
- **Delta:** `calendarView/delta` works per calendar, over a window fixed
  by the first request. It returns masters, single events, exceptions in
  full, and bare occurrences. A deletion comes back as
  `@removed: deleted` with only the ID, once per series. Contacts delta
  works on the default folder, the same way.
- **Times:** Graph keeps an IANA or a Windows zone name as written, and
  returns times in the zone `Prefer: outlook.timezone` names. All-day
  events are midnight to midnight. An event has one reminder
  (`isReminderOn`, `reminderMinutesBeforeStart`).
- **Photos:** a contact photo is a separate resource. A PNG comes back as
  JPEG.
- **Calendars:** Outlook.com has a default calendar, read-only Birthday
  and holiday calendars, and a family group calendar that can be edited.

## Decision

- **Collections:** each Graph calendar and contact folder, with the
  default contact folder as the account's address book. The href is the
  Graph ID; the name, color (`hexColor`, else the named color) and
  read-only flag (`canEdit`) come from Graph.
- **Objects:** one per single event, one per series (its master with its
  exceptions), and one per contact. The href is the Graph ID, the ETag is
  `@odata.etag`, and the UID is `iCalUId`. Every request asks for
  immutable IDs.
- **The stored source is derived.** maild turns Graph's JSON into
  iCalendar (`VEVENT`s, with `RRULE` from the pattern, `EXDATE` from the
  cancelled occurrences, overrides with `RECURRENCE-ID` from the
  exceptions, `VALARM`, attendees with their `PARTSTAT`) and into vCard
  4.0, deterministically. The rest of maild reads it as it reads a DAV
  source. Graph's JSON is kept beside it, for restores and for fields
  Frostmail does not show. Golden tests cover both directions.
- **Sync** stores each collection's delta link as its sync token:
  - **Calendars:** `calendarView/delta` over the instance window and a
    margin, rebased when the window passes the margin.
  - **Series:** any change to a series, its exceptions or its
    occurrences re-reads its master with its exceptions and cancellations,
    and replaces the object.
  - **Deletions:** a removed ID deletes its object.
  - **Contacts:** contacts delta per folder. Photos are fetched when a
    contact with a photo changes.
  - **Expired links:** an expired delta link resyncs the collection.
- **Writes** replay `pim_ops` as Graph requests built from the edits in the
  op's payload, not from the source:
  - **Edits:** a `PATCH` of the changed fields with `If-Match`.
  - **Creations:** a `POST`, after which the object moves to the returned
    ID (as with a DAV server's `Location`).
  - **This Event:** a `PATCH` or `DELETE` of the occurrence, found through
    the master's `instances`.
  - **All Future Events:** ends the series' range and creates the new
    series, as on DAV.
  - **All Events:** a `PATCH` of the master.
  - **A 412:** re-read the object and derive it again. The edit is
    re-applied when it still fits (ADR-0030).
  - **Contact photos:** `PUT .../photo/$value`.
- **Undo:** an edit is undone by patching back the old values. A deleted
  object is restored by posting its stored JSON again, without the
  read-only fields, under a new ID.
- **Invitations go through Outlook's scheduling.** Graph's
  `accept`, `tentativelyAccept` and `decline` (with `sendResponse`)
  answer an invitation, found by `iCalUId`. Invitees added to an event the
  user organizes are told by Outlook, as Google and iCloud do (ADR-0035).
- **Limits:**
  - **Reminders:** one per event. The editor offers one for Outlook
    calendars.
  - **Recurrence:** rules Graph's patterns cannot express are read-only.
  - **Notes:** an edited description replaces an HTML body with text.
  - **Zones:** Windows zone names are mapped to IANA with the existing
    table, and times are written with IANA names.

## Consequences

- The Calendar, People, invitations, reminders, the editor, undo and
  verify work on Outlook accounts without a second path through them.
- The derivation is the risky part: a field it gets wrong is wrong
  everywhere. It needs golden tests over recorded Graph JSON, and verify
  compares the derived source against a fresh one.
- A write never sends a derived source back, so what Frostmail does not
  model (online-meeting details, categories, attachments) survives edits.
- Events wholly outside the delta window (over a year back or two ahead)
  are not synced. `calendar.range` beyond it misses Outlook's single
  events there.
- A series edit costs an extra read of its master. Edits to occurrences
  cost a lookup of their IDs.
- A restored event or contact gets a new ID. A restored event with
  invitees invites them again, as a restored DAV object would.

## Alternatives considered

- **Store Graph's JSON and index it directly,** as for Google Tasks: no
  derivation, but a second indexer, expander, patcher and verify for
  events and contacts, and every later feature built twice.
- **Use Graph's occurrences as the instances:** no expansion of patterns,
  but the window would bound the views, and the editor would still need
  the rule as `RRULE`.
- **Write derived iCalendar back** (no field mapping for writes): Graph
  takes no iCalendar, and a round trip would drop what Frostmail does not
  model.

## References

- Shapes: [design/pim.md](../design/pim.md),
  [plans/0010](../plans/0010-m7-microsoft-and-windows.md)
- Builds on: [ADR-0017](0017-sync-contacts-calendars-and-tasks.md),
  [ADR-0018](0018-keep-contacts-events-and-tasks-as-sent.md),
  [ADR-0030](0030-edit-contacts-and-events-as-patches.md),
  [ADR-0031](0031-edit-a-recurring-event-in-three-scopes.md),
  [ADR-0032](0032-undo-restores-what-a-change-replaced.md),
  [ADR-0035](0035-invite-through-the-providers-scheduling.md),
  [ADR-0039](0039-microsoft-accounts-over-imap-smtp-and-graph.md)
