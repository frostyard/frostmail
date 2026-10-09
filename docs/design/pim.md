# Design: contacts, calendars and tasks

Status: designed for [M4.5](../plans/0007-m4.5-people-calendar-tasks.md);
nothing here is built yet. Rationale:
[ADR-0017](../adr/0017-sync-contacts-calendars-and-tasks.md) (protocols),
[ADR-0018](../adr/0018-keep-contacts-events-and-tasks-as-sent.md) (storage),
[ADR-0019](../adr/0019-answer-invitations-through-the-server.md)
(invitations), [ADR-0020](../adr/0020-one-window-for-mail-calendar-people-and-tasks.md)
(one window). Contracts: `schema/rpc/{people,calendar,tasks}.yaml` and
`specs/pim-ui.md`, written in Phase 1.

## Overview

maild syncs an account's address books, calendars and task lists into the
same store as its mail, and the app shows them as four modules of one
window. The integration lives in maild's store: invitations, contact cards,
the To-Do bar and reminders are queries across mail, people, events and
tasks.

```
CardDAV ─┐                                  ┌─ Mail ─┬─ invitation cards
CalDAV  ─┼─► davx ─┐                        │        └─ To-Do bar
Tasks   ─┴─► gtasks┴─► actor ─► store ─► RPC┼─ Calendar
IMAP    ──► imapx ───► actor ─┘   │          ├─ People ─ contact cards
                                  └─ reminders (notify)  └─ Tasks ─ flagged mail
```

## Design

### Services and sync

- **Services.** `account_services` holds an account's contacts, calendar
  and tasks services: whether each is on, where its discovery starts (the
  provider profile's URL, one the user gave, or for other accounts the
  server of the domain's `_carddavs._tcp` or `_caldavs._tcp` SRV record,
  then the domain), and the home set discovery found. New services start
  off; turning one on in Settings discovers it at once with the account's
  credentials, or, for a Google account whose grant lacks the scope, at
  the first pass after the user signs in again. Discovery sends
  credentials only to the start's origin, the provider's domains, or the
  start's registrable domain (`davx`'s trust rule).
- **Credentials.** iCloud and generic servers use the account's password
  (iCloud's app-specific one). Google uses the account's OAuth token: the
  authorization URL asks for `mail.google.com` plus the scopes of the
  services turned on (calendar, CardDAV, Tasks), and turning one on asks
  the user to sign in again.
- **The DAV loop.** `internal/pimsync` runs one loop per account with a
  service on, beside mail's actor and independent of its connections, with
  its own HTTP client (`internal/davx`): list the collections under each home set
  (display name, color, supported components, `getctag`, `sync-token`), then
  per collection a `sync-collection` REPORT from the stored token (all
  objects on the first run), then a `multiget` for the changed hrefs in
  batches of 100. A server without sync-collection is compared by ETag
  (`PROPFIND` depth 1). An invalid token (`valid-sync-token` error) resyncs
  the collection. Each batch is one transaction with events.
- **The Tasks loop.** For Google accounts with tasks on: `tasklists.list`,
  then per list `tasks.list` with `updatedMin` (the stored high-water mark),
  `showDeleted` and `showHidden`.
- **When.** Every 5 minutes; at once on `sync.now`, on window focus (the app
  calls `sync.now` with a `pim` hint), and after a write. Read-only accounts
  sync and never write.
- **Writes.** Every change is a `pim_ops` row (`put`, `delete`,
  `tasks.insert`, `tasks.patch`, `tasks.delete`) naming the object, whose
  source is already patched locally, and the ETag the change was made on
  (`If-None-Match: *` for a creation). Each pass replays the due changes
  before it reads, and leaves objects with waiting changes alone. A
  network failure retries after 1, 5, 15 and 60 minutes; a server that
  refuses the change (412, or another 4xx) fails it, and the server's
  version stands (a refused creation is removed locally). Re-applying a
  patch to the server's newer version after a 412 comes with the first
  patches (Phase 4).

### Storage

- `collections`: account, service, href, name, color, kind (address book,
  calendar, task list), supported components, sync token, ctag, read-only,
  enabled, default for new objects.
- `objects`: collection, href, ETag, UID, kind, `raw` (the source bytes),
  `parse_error`. The source of truth for everything below.
- `contacts`: object, display name, sorting name, organization, photo blob,
  `people_id`; `contact_emails`, `contact_phones`; indexed for search.
- `people`: a person seen across address books, linked by normalized email,
  with the best display name and photo; contact cards and autocomplete read
  it. `addresses` (seen in mail, M3) joins on email.
- `events`: object, UID, `RECURRENCE-ID`, summary, location, start and end
  (UTC with their IANA zone, or dates for all-day), recurrence text, status,
  transparency, organizer, the user's `PARTSTAT`, sequence.
  An event keeps its ID while its object keeps its recurrence ID, across
  syncs, since the app's selection holds it.
- `event_attendees`: event, email, name, role, `PARTSTAT`.
- `instances`: event, start and end in UTC (dates for all-day), recurrence
  ID; under the override's event when one replaces the occurrence.
  Materialized for a window (a year back, two ahead: `instance_window`) by
  `internal/calendar` (rrule-go for RRULE, RDATE and EXDATE). The first
  pass of a new UTC day moves the window, re-expanding the stored events
  without fetching them. Ranges beyond it (`calendar.range` far ahead) are
  expanded on demand from the stored events, not stored.
- `alarms`: instance, trigger time, action; `reminders`: snoozed until,
  dismissed.
- `tasks`: object (CalDAV) or Google id, list, title, notes, due date,
  status, completed time, parent, position.

Flagged mail is not copied into `tasks`: the Tasks module and the To-Do bar
query flagged messages alongside tasks, and completing one clears the flag.

### Calendars

- **Time.** Stored in UTC with the IANA zone; Windows zone names map through
  the CLDR table (`internal/calendar/zones_gen.go`, generated by
  `tools/zonesgen` from a pinned `windowsZones.xml`); otherwise the object's
  `VTIMEZONE` is evaluated. Floating times use the local zone. tzdata is
  built in (`time/tzdata`).
- **Views** ask maild for instances in a range (`calendar.range`), joined
  with their events and calendars; `calendar.changed` events name the
  ranges that moved.
- **Bounds.** A rule generates at most 200,000 starts, counted from
  `DTSTART`, and rules repeating more than hourly read as single events, so
  a hostile invitation cannot stall indexing. RDATE and EXDATE values with
  a `TZID` that only the object's `VTIMEZONE` names are read in the event's
  zone.
- **Reminders.** Only `DISPLAY` and `AUDIO` alarms; `EMAIL` alarms are the
  server's.
  - *Triggers.* An occurrence's alarm triggers at its start plus the
    offset (its end with `RELATED=END`); an all-day occurrence's at its
    date's midnight in maild's zone. Offsets more than 30 days before or a
    day after are ignored. An absolute trigger counts for a single event or
    an override, not a series. A reminder is keyed by the collection, the
    UID, the occurrence's recurrence ID and the trigger, so it survives
    resyncs and an event moved to a new time reminds again.
  - *Firing.* maild's scheduler (`internal/reminders`) checks once a
    minute, at the minute: the alarms of shown calendars due since its
    last check (after a start, back at most a day) are recorded as fired
    (`reminders.fired_at`) and announced, and so are snoozes that ended.
  - *Announcing.* With the app connected (a client that said
    `frostmail-app` in `rpc.hello` and subscribed to events), maild sends
    `calendar.reminders {count}` and the app raises its reminder window.
    Otherwise each reminder is a desktop notification (`internal/notify`):
    the title as the summary, the time and place as the body, the actions
    Snooze (10 minutes) and Dismiss, and clicking it opens the app on its
    reminder window. Dismissing or snoozing in either place closes the
    notification.
  - *State.* `calendar.reminders` lists the fired reminders neither
    dismissed nor snoozed past now, oldest due first; `calendar.snooze`
    and `calendar.dismiss` change them.

### Invitations

- `mimex` finds `text/calendar` parts with a `METHOD`; the reader asks
  `calendar.invitation {messageId}`, which resolves the `UID` against the
  account's events (ADR-0019) and returns the card: times in the user's
  zone, organizer and attendees, the user's answer, conflicts and the
  adjacent events.
- `calendar.respond {messageId, answer}` patches `PARTSTAT` and either
  queues a `dav.put` (the server schedules) or builds an iTIP `REPLY` with
  `internal/compose` and queues it in the outbox (it does not), putting an
  accepted event in the default calendar.
- Whether a provider schedules is in its profile (Google, iCloud: to verify
  in the trial), else from the `DAV` header's `calendar-auto-schedule`.

### People in mail

- Contact cards: `people.card {email}` returns the person, their addresses
  and phones, the last messages with them (a search), and their upcoming
  events (instances with them as attendee).
- Autocomplete ranks contacts first, then seen addresses, by how recently
  and often the user wrote to them.
- "Add to Contacts" creates a vCard 3.0 (Google and iCloud both accept it)
  in the account's default address book with `dav.put` and
  `If-None-Match: *`.

### Tasks

- Google Tasks: lists, tasks with notes, a due date (Google keeps the date
  only), completion, and subtasks (`parent`); a task created from a message
  in Gmail carries a link to its thread, which Frostmail opens by
  `X-GM-THRID`.
- CalDAV tasks (VTODO) on servers that have them, in the same table.
- Create, edit, complete and delete are in scope from the start: a task
  list that cannot be ticked is not one.
- **Google lists and tasks.** A task list is a `tasklist` collection (its
  href the list's ID); a task is an `objects` row of kind `gtask` (its href
  the task's ID, `raw` the task's JSON as sent, the ETag its `etag`),
  through `internal/gtasks`. A list's sync token is the `updated` of the
  newest task seen; `updatedMin` is inclusive, so that task comes again and
  is stored again unchanged. A deleted task removes its object; a list gone
  from `tasklists.list` removes its collection.
- **CalDAV lists.** With tasks on, a DAV account's calendar collections
  that support VTODO are also `tasklist` collections (the same href). A
  task list stores every object of the collection, so ETags stay
  comparable, and indexes only its VTODOs; the calendar of the same href
  indexes only events. Tasks has its own `account_services` row and home.
- **The index.** One `tasks` row per task object: Google's ID or the
  VTODO's `UID`, the parent (`parent`, or `RELATED-TO` with `RELTYPE=PARENT`
  or none), title, notes, the due date (a VTODO's `DUE` date-time is read
  as its date in maild's zone; its time stays in the source), completion
  (`status: completed`, or `STATUS:COMPLETED` or a `COMPLETED` time) and
  when, the position (Google's `position`; Apple's `X-APPLE-SORT-ORDER`),
  and for a task made from Gmail the thread its `email` link names. A
  task's ID in the API is its object's ID.
- **Order.** Lists in collection order; in a list, parents by position
  (text), then those without one by title, each parent followed by its
  subtasks the same way.
- **Writes.** Google tasks change through `tasks.insert`, `tasks.patch` and
  `tasks.delete` ops whose payload is the change (`gtasks.Fields`, and the
  parent for an insert); the object's JSON is patched at once and replaced
  by the server's answer. CalDAV tasks change through `put` and `delete`
  ops whose source `internal/calendar` patches (`SUMMARY`, `DESCRIPTION`,
  `DUE`, `STATUS`, `COMPLETED`, `PERCENT-COMPLETE`), with golden tests.

### Testing

- `internal/davtest`: an in-process CalDAV and CardDAV server with memory
  collections, sync tokens, ETags and hooks for other clients' changes, as
  `imapxtest` is for IMAP; `davx` is tested against it.
- A fake Tasks server (`httptest`) for `internal/gtasks`.
- Golden tests for every patch function (ADR-0018): source in, source out.
- Radicale in the `frostmail-mailtest` container for integration tests
  (`make engine-it`).
- Recordings of Google's and iCloud's DAV sessions (`MAILD_DAV_TRACE`),
  scrubbed into replay scripts as `tools/imaprec` does for IMAP.

## Operational notes

- `mailctl verify` grows a check per service: hrefs and ETags on the
  server against the store, changing nothing.
- A collection whose objects fail to parse keeps the source and its
  `parse_error`, so a parser fix can rebuild the index without a resync.
- Reminders for a series far in the future come from the instances window;
  the window moves daily with the first pass of the day.
