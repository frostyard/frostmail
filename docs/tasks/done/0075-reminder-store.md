---
id: "0075"
title: Store reminders and serve calendar.reminders
milestone: M4.5
size: L
touch:
  - internal/store/reminders.go
  - internal/engine/calendar.go
given:
  - internal/store/reminders_test.go
  - internal/engine/reminders_test.go
acceptance: go test ./internal/store ./internal/engine -count=1
---
# T-0075: Store reminders and serve calendar.reminders

## Goal

Alarms become reminders: find the alarms that came due, record them as
fired, list the ones to show, snooze and dismiss them. maild's reminder
scheduler (the planner's) calls the store; the app's reminder window calls
`calendar.reminders`, `calendar.snooze` and `calendar.dismiss`.

## Read first

- `docs/design/pim.md`: "Reminders".
- `internal/store/migrations/0007_pim.sql`: `alarms`, `instances`,
  `reminders`.
- `internal/store/reminders.go` (types and stubs), `events.go`
  (`eventColumns`, `eventJoins`, `visibleEvents`, `eventScan`,
  `parseEventTimes`: reuse them).
- `schema/rpc/calendar.yaml`: `Reminder`, `reminders`, `snooze`,
  `dismiss`; `internal/engine/calendar.go`.
- The given tests (the engine test uses `newCalendarEnv` from
  `calendar_test.go`).
- `docs/tasks/EXECUTOR.md`

## Contract

### Store (`internal/store/reminders.go`)

Keep the types and signatures; replace the stubs and `errRemindersNotYet`.
Times are stored with `FormatTime`; `TriggerAt` values are UTC.

- **Triggers.** For an occurrence (an `instances` row) and an alarm of its
  event: a relative alarm (`offset_seconds`) triggers at the occurrence's
  start plus the offset, or its end when `related_end`; for an all-day
  occurrence, at its start (or end) date's midnight in `local`. Offsets
  more than 30 days before or 1 day after are ignored. An absolute alarm
  (`trigger_at`) triggers then, only for occurrences whose recurrence ID is
  their event's own (single events and overrides): a series' absolute
  alarms are ignored. The key is the object's collection, the event's UID,
  the *occurrence's* recurrence ID, and the trigger.
- **`AlarmsDue(since, until, local)`**: the keys with a trigger in
  `(since, until]` of occurrences in shown calendars (as `visibleEvents`),
  without a `reminders` row, sorted by trigger, then collection, UID,
  recurrence ID, without duplicates. Find the candidates in SQL (instances
  starting before `until` + 30 days and ending after `since` − 1 day,
  joined to their alarms) and compute triggers in Go.
- **`FireReminders(keys, firedAt)`**: insert rows; a key that has one
  already is left alone.
- **`ActiveReminders(now)`**: rows not dismissed and not snoozed past
  `now` (`snoozed_until` null or ≤ now), whose occurrence still exists in
  a shown calendar (join by collection, UID and the instance's recurrence
  ID), with `Event` (no attendees or alarms) and `Instance`; `DueAt` is
  `SnoozedUntil` when set, else `TriggerAt`; sorted by `DueAt`, then key.
- **`SnoozesEnded(since, until)`**: keys of rows not dismissed whose
  `snoozed_until` is in `(since, until]`, by `snoozed_until`.
- **`SnoozeReminders(keys, until)`** sets `snoozed_until`;
  **`DismissReminders(keys, at)`** sets `dismissed_at`; both only on rows
  not dismissed, returning how many rows changed.

### Engine (`internal/engine/calendar.go`)

- A reminder's `id` is opaque: the key as
  `base64.RawURLEncoding` of the JSON array `[collectionId, uid,
  recurrenceId, triggerAt (FormatTime)]` (`encoding/json/v2`).
- **`calendar.reminders`**: `ActiveReminders(DB.Now())` as
  `[]api.Reminder` (never nil): `eventId` the instance's event,
  `recurrenceId` the occurrence's, `calendarId`, `summary`, `location`,
  `allDay`, `start` the occurrence's start (all-day: its date's UTC
  midnight) and `startDate` for all-day ones, `dueAt`.
- **`calendar.snooze {ids, until}`** and **`calendar.dismiss {ids}`**
  (`dismissed_at` = `DB.Now()`): `invalidParams` when `ids` is empty or
  any id does not decode; otherwise update in one transaction (unknown
  keys change nothing).

## Tests (given, do not edit)

`internal/store/reminders_test.go` (TestAlarmsDue,
TestReminderLifecycle) and `internal/engine/reminders_test.go`
(TestReminderAPI).

## Gotchas

- Compare instants with `Equal`, never `==`.
- The all-day midnight is `time.Date(y, m, d, 0, 0, 0, 0, local)` of the
  stored date, then `.UTC()`.
- Keep functions under 60 lines.

## Out of scope

The scheduler that fires reminders and the desktop notifications (the
planner's), the reminder window (a later card), and every file not under
`touch`.

## Done when

`make accept T=0075` and `make check` pass, and only the files under
`touch` changed.
