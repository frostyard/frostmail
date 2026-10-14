# Design: contacts, calendars and tasks

Status: built in [M4.5](../plans/0007-m4.5-people-calendar-tasks.md);
editing events and contacts comes in
[M5.5](../plans/0009-m5.5-edit-events-and-contacts.md). Rationale:
[ADR-0017](../adr/0017-sync-contacts-calendars-and-tasks.md) (protocols),
[ADR-0018](../adr/0018-keep-contacts-events-and-tasks-as-sent.md) (storage),
[ADR-0022](../adr/0022-mail-the-reply-the-server-will-not-send.md)
(invitations), [ADR-0020](../adr/0020-one-window-for-mail-calendar-people-and-tasks.md)
(one window), and for editing
[ADR-0030](../adr/0030-edit-contacts-and-events-as-patches.md) (patches
and refusals), [ADR-0031](../adr/0031-edit-a-recurring-event-in-three-scopes.md)
(recurring scopes), [ADR-0032](../adr/0032-undo-restores-what-a-change-replaced.md)
(undo), [ADR-0033](../adr/0033-write-new-events-with-their-zones-vtimezone.md)
(time zones) and
[ADR-0034](../adr/0034-invite-from-the-organizers-copy-and-ask-before-telling-invitees.md)
(invitations as organizer). Contracts: `schema/rpc/{people,calendar,tasks}.yaml` and
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
  off, except those the user checks when adding the account; turning one
  on discovers it at once with the account's credentials, or, for a
  Google account whose grant lacks the scope, at the first pass after the
  user signs in. Discovery sends
  credentials only to the start's origin, the provider's domains, or the
  start's registrable domain (`davx`'s trust rule).
- **Credentials.** iCloud and generic servers use the account's password
  (iCloud's app-specific one). Google uses the account's OAuth token: the
  authorization URL asks for `mail.google.com` plus the scopes of the
  services turned on (calendar, CardDAV, Tasks). Adding an account turns
  the chosen services on before its first sign-in, so one consent covers
  mail and all of them; turning one on later asks the user to sign in
  again, and Settings offers one sign-in for every service waiting.
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
  network failure retries after 1, 5, 15 and 60 minutes. A server that
  refuses a task change or an answer (412, or another 4xx) fails it, and
  the server's version stands (a refused creation is removed locally).
  An edit of a contact or event is re-applied after a 412 when it still
  fits (Editing, below). A server may store a new object under a name of its
  own and answer 201 with a `Location` (Google's CalDAV names it from its
  UID, which for an accepted invitation is the organizer's): the local
  object moves there, or gives way to the copy a pass already brought,
  and without an ETag in the answer it is read back from there.

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
    last check are recorded as fired (`reminders.fired_at`) and announced,
    and so are snoozes that ended. Each check is recorded
    (`reminders_checked`), so after a start the first check looks back to
    the last one before maild stopped, at most a day; a database that
    never checked looks back nowhere. An alarm that was due before its
    event was synced, or before Frostmail first checked, never reminds.
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

- `internal/itip` finds a message's iCalendar part (the first
  `text/calendar`, else an `.ics` attachment), reads its `METHOD` and
  events, and writes answers: the iTIP `REPLY` (RFC 5546) and the
  `PARTSTAT` patch of a stored copy (ADR-0018).
- `calendar.invitation {messageId}` resolves the message's `UID` against
  the account's events (ADR-0022) and returns the card: the event as the
  message has it, the stored copy's ID, the user's answer (the stored
  copy's when there is one), whether the user can answer (a request to
  them, not older than the stored copy, in a writable account and
  calendar), the busy occurrences it overlaps and the ones just before and
  after it that day.
- `calendar.respond {messageId | eventId, answer, recurrenceId?,
  comment?}`:
  - **A stored copy** (the event, or the message's `UID` in a calendar)
    gets the user's `PARTSTAT` patched and its `put` queued on the ETag it
    was read with. A message older than the copy (`SEQUENCE`) is refused.
  - **Only the message:** an accepted or tentative invitation goes into the
    account's default calendar (the invitation without `METHOD`, a `put`
    that creates it); a declined one is not stored. A default the user did
    not choose (Calendar's "Use as Default Calendar", `default_chosen`)
    first follows the one the server names (RFC 6638's
    `schedule-default-calendar-URL` on the scheduling inbox), asked for
    then, with a 10 s limit; before, the first calendar listed was the
    default, which on iCloud can be a shared one.
  - **The reply** (ADR-0022): a stored copy on a provider that schedules
    (`providers.DAV.Schedules`: Google, iCloud) is answered by its `put`
    alone, unless the copy carries `SCHEDULE-AGENT=CLIENT` on `ORGANIZER`;
    a new copy is answered by its `put` alone only where the provider also
    schedules for copies a client creates (`SchedulesCopies`: Google). In
    every other case maild mails the organizer the `REPLY` (`text/calendar;
    method=REPLY`, "Accepted: <summary>") through the outbox with the undo
    delay, and the copy it writes gets `SCHEDULE-AGENT=CLIENT` on
    `ORGANIZER` so the server sends no second one. A declined or unstored
    answer is always mailed. The `REPLY` carries no `SCHEDULE-*`
    parameters. Undo cancels the mail; the calendar keeps the answer.
  - One occurrence of a series is answered only when the series has a
    VEVENT for it (an override); otherwise `invalidParams`.
  - Read-only accounts and calendars, cancellations and replies are
    refused with `conflict`.
- Whether a provider schedules is in its profile, else not.

### People in mail

- Contact cards: `people.card {email}` returns the person, their addresses
  and phones, the last messages with them (a search), and their upcoming
  events (instances with them as attendee).
- Autocomplete ranks contacts first, then seen addresses, by how recently
  and often the user wrote to them.
- "Add to Contacts" creates a vCard 3.0 (Google and iCloud both accept it)
  in the account's default address book with `dav.put` and
  `If-None-Match: *`.

### Editing

M5.5 ([plan 0009](../plans/0009-m5.5-edit-events-and-contacts.md)) makes
contacts and events writable through `people.create`, `update`, `delete`,
`setPhoto` and `restore`, and `calendar.create`, `update`, `delete` and
`restore`.

- **Patches.** A method checks the object is writable (`conflict` for a
  read-only account, address book or calendar), applies the fields to
  the stored source (`vcardx.Patch` and `NewCard`; `calendar.PatchEvent`
  and `NewEvent`), stores the result and its index, records the restore,
  and queues the op, all in one transaction with `people.changed` or
  `calendar.changed`. Then it kicks the account's pass. An event's
  instances are expanded again in that transaction, so the views move at
  once.
- **The op's payload.** An edit's `put` holds `{"changes": [fields…],
  "base": {"<component>/<PROPERTY>": [lines…]}}`. A component is `VCARD`,
  or `VEVENT` with the occurrence's `RECURRENCE-ID` (`VEVENT` alone for
  the master). The base holds each touched property's unfolded lines as
  the source had them before the first change; a list change also counts
  the card's `X-ABLabel` lines. A second edit while the op waits appends
  its fields, and its base for properties not yet in it; an edit to a
  contact whose creation waits adds nothing, since the creation writes
  the source as it is then. A restore's put is `{"whole": true}`, which
  is never applied again, and a contact's delete carries `{"name": …}`
  for the app should the server refuse it.
- **After a 412**, the pass fetches the object (`GET`). When every base
  entry matches the server's lines, it applies the changes in order to
  the server's source, stores that with the server's ETag, and puts it
  again: three attempts, then as below. Otherwise it stores the server's
  version, drops the op, and emits `people.refused` or `calendar.refused`
  with reason `changed`. A 404 emits reason `deleted` and removes the
  local object; other 4xx answers emit `refused`. A contact delete
  refused with 412 lists the address book again, which brings the
  server's version back, and emits `changed`.
- **Restores** (`pim_restores`, a row per write with an opaque ID, the
  account and when; `pim_restore_objects`, a row per object it touched,
  with the collection, href, UID and the source before, or NULL when the
  write created the object). `restore` queues a `put` of each kept
  source on the object's current ETag, deletes the created ones, and
  drops the row. A deleted object whose delete still waits gets that
  delete cancelled and its source put back on the server's ETag; one
  whose delete was written is created again under a new name with
  `If-None-Match: *`, keeping its UID, since Google refuses the deleted
  name (found by Phase 2's check, 2026-10-14). Rows
  older than a day are dropped by the pass. The tables come with Phase
  2's migration.
- **Contacts.** `vcardx.Patch` keeps the card's version, its groups and
  every parameter the fields do not set. List fields map onto the lines
  of their property in order, with Apple's `itemN.X-ABLabel` labels kept
  as groups (ADR-0030). A new contact is a vCard 3.0, `<uid>.vcf` in the
  address book. `setPhoto` writes `PHOTO` in the card's version: a JPEG
  or PNG of at most 256 KB, which the app has scaled to at most 512 × 512.
- **Events.** `calendar.PatchEvent` changes a single event or a series'
  master; `calendar.PatchOccurrence` and `DeleteOccurrence` change one
  occurrence of a series in its scope (ADR-0031):
  - **This Event** patches the occurrence's override, made from the
    master when it has none; a delete adds an `EXDATE` and drops the
    override.
  - **All Events** patches the master from the occurrence edited. The
    editor sends only the fields that changed, and new times are the
    occurrence's: the series moves as that occurrence does, each time
    keeping its days and clock from the occurrence's original start in the
    series' zone, so a 09:00 series moved to 10:00 is at 10:00 on both
    sides of a DST change. Its `EXDATE`s, `RDATE`s, overrides and an
    `UNTIL` move with it. An override takes each changed value it still
    shared with the old master, and the new times when its own were its
    occurrence's; what was changed for it alone stays. A new rule drops
    the overrides and `EXDATE`s that no longer name an occurrence.
  - **All Future Events** splits the series: the old object ends a second
    (or a day) before the occurrence, and a new object under a new UID
    starts there with the change, the rule's remaining `COUNT` or its
    `UNTIL`, and the later `RDATE`s, `EXDATE`s and overrides. Its creation
    is a put of its own. From the first occurrence it is All Events, and
    a delete deletes the object (`calendar.ErrWholeSeries`);
    `CalendarEvent.first` tells the app which occurrence that is.

  A new event is `<uid>.ics` in its calendar, with the generated
  `VTIMEZONE` of its zone (`calendar.VTimezone`, ADR-0033), `DTSTAMP`,
  `CREATED`, `LAST-MODIFIED` and `SEQUENCE:0`. Alerts are `VALARM`s with
  `ACTION:DISPLAY` and a relative `TRIGGER`: the fields replace the
  event's `DISPLAY` and `AUDIO` alarms (the ones Calendar shows) and keep
  its `EMAIL` alarms, which are the server's. An event put's payload is
  `{"event": [edits…], "base": {…}}`, each edit the occurrence, scope and
  fields (or a delete of occurrences). For a single event the base keys
  are `VEVENT/<PROPERTY>` for the properties the edits touch; for a series
  they are every VEVENT's (`VEVENT;RECURRENCE-ID=<key>/<PROPERTY>`)
  touched properties and its shape (`DTSTART`, `DTEND`, `DURATION`,
  `RRULE`, `RDATE`, `EXDATE`, `RECURRENCE-ID`), and `VEVENTS`, the list of
  its VEVENTs. A 412 applies the edits again with the calendar package
  when the base fits. A move to another calendar is a `put` there and a
  `delete` here, whose payload names the put (`{"after": <op id>}`): the
  delete waits while the put is queued; when the server refuses the put,
  the delete is dropped, the old calendar is listed in full again, which
  brings the event back, and `calendar.refused` says so.
- **The app** (docs/specs/pim-ui.md): the event pane's Edit opens the
  event editor window (`open_event`; `#/event/<id>` in a browser), and
  its Delete deletes through the scope sheet for an occurrence of a
  series. The editor edits a draft (`lib/eventDraft.ts`), sends
  `calendar.create`, or `calendar.update` with the changed fields after
  the scope sheet, and turns repeat presets and the Custom Repeat sheet
  into `RRULE`s (`lib/repeat.ts`).
- **Invitations as organizer** (ADR-0034, Phase 5 of the plan): the
  organizer is the account's default identity. The provider fact
  `SchedulesOrganizer` chooses between the server and maild's iTIP
  `REQUEST` and `CANCEL` (`internal/itip`) through the outbox. The app
  asks first, and the methods take `notify`. Answers that come back as
  mail are applied to the organizer's copy.

### Tasks

- Google Tasks: lists, tasks with notes, a due date (Google keeps the date
  only), completion, and subtasks (`parent`); a task created from a message
  in Gmail carries a link to its thread, which Frostmail opens by
  `X-GM-THRID`.
- CalDAV tasks (VTODO) on servers that have them, in the same table.
- iCloud has none Frostmail can reach (ADR-0017): since iOS 13 Reminders
  keeps its lists where only Apple's apps reach them. iCloud's CalDAV
  still lists the lists left from before the upgrade, frozen and named
  with "⚠️"; a reminder made today never appears in them (probed
  read-only on the user's account, 2026-10-10: the lists' `getctag`
  stayed the same across a new reminder). The provider profile says so
  (`DAV.NoTasks`), `account.services` returns it as the tasks service's
  `reason`, and Settings shows it under the account's services.
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
  by the server's answer. A new Google task waits as `local-<uuid>` until
  its insert names it; its subtasks and later changes follow the new ID.
  CalDAV tasks change through `put` and `delete` ops whose source
  `internal/calendar` patches (`SUMMARY`, `DESCRIPTION`, `DUE`, `STATUS`,
  `COMPLETED`, `PERCENT-COMPLETE`), with golden tests; a new one is
  `<uid>.ics` in its list, with `RELATED-TO` for a parent. A put waiting
  for an object writes its source as it is when it runs, so edits made
  before the pass add no ops. Deleting a task deletes its subtasks: Google
  takes them with the parent's `tasks.delete`; CalDAV gets a delete per
  object, on its ETag. A task the server never had is only dropped, with
  its waiting ops.

### Testing

- `internal/davtest`: an in-process CalDAV and CardDAV server with memory
  collections, sync tokens, ETags and hooks for other clients' changes, as
  `imapxtest` is for IMAP; `davx` is tested against it.
- A fake Tasks server (`httptest`) for `internal/gtasks`.
- Golden tests for every patch function (ADR-0018): source in, source out.
- Radicale in the `frostmail-mailtest` container for integration tests
  (`make engine-it`).
- Recordings of Google's and iCloud's DAV and Google's Tasks sessions
  (`MAILD_DAV_TRACE`, `internal/httprec`), scrubbed by `tools/davrec` and
  replayed in `internal/pimsync/replay_test.go` (testing.md, DAV and Tasks
  recordings).

## Operational notes

- `mailctl verify` grows a check per service: hrefs and ETags on the
  server against the store, changing nothing. It asks for what it finds
  missing locally and leaves out what the server cannot serve (Google's
  listing keeps objects its sync calls deleted).
- Google's sync-collection also calls some objects deleted that its
  listing and a multiget do serve and Google Calendar shows: on the
  user's account in 2026-10, two single occurrences from 2020 stored
  without their series. Sync follows sync-collection, so they stay
  missing locally and verify lists them. A single occurrence accepted
  now syncs normally; a Google-only listing check on changed calendars
  would fetch them if more turn up.
- A collection whose objects fail to parse keeps the source and its
  `parse_error`, so a parser fix can rebuild the index without a resync.
- Reminders for a series far in the future come from the instances window;
  the window moves daily with the first pass of the day.
