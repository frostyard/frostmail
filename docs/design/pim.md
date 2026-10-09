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

- **Services.** `accounts` gains a services set (`mail`, `contacts`,
  `calendar`, `tasks`) and, per service, its endpoint (a principal URL, or
  Google Tasks). Provider profiles carry Gmail's and iCloud's endpoints;
  generic accounts use `/.well-known/caldav` and `carddav`, then SRV
  `_caldavs._tcp` and `_carddavs._tcp`. New services start off; the user
  turns them on in Settings, which runs discovery.
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
- **Writes.** Every change is a `pending_ops` row (`dav.put`, `dav.delete`,
  `tasks.patch`, …) holding the patched source and the ETag it expects,
  replayed like mail's ops: a 412 refetches the object, reapplies the patch
  and tries once more, then fails the op and keeps the server's version.

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
- `event_attendees`: event, email, name, role, `PARTSTAT`.
- `instances`: event, start and end in UTC, all-day flag, override event;
  materialized for a window (a year back, two ahead) by
  `internal/calendar` (rrule-go for RRULE, RDATE and EXDATE), extended as
  views ask for ranges beyond it.
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
- **Reminders.** maild keeps the next alarm per account and, when it is due,
  announces it: to the app's reminder window when the app is running, as a
  desktop notification otherwise (`internal/notify`, with Snooze and
  Dismiss actions). Only `DISPLAY` and `AUDIO` alarms; `EMAIL` alarms are
  the server's.

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
  the window is extended daily by the DAV loop.
