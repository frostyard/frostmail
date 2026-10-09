---
id: "0088"
title: Answer calendar.invitation
milestone: M4.5
size: L
touch:
  - internal/engine/invitation.go
  - internal/engine/calendar.go
  - internal/store/events.go
given:
  - internal/engine/invitation_test.go
acceptance: go test ./internal/engine -count=1
---
# T-0088: Answer calendar.invitation

## Goal

`calendar.invitation {messageId}`: the invitation in a message's
iCalendar part, matched against the account's stored events, with what
the reader's invitation card shows: the event, who sent it, the user's
answer, whether they can answer, and the calendar around it (ADR-0019).

## Read first

- `docs/adr/0019-answer-invitations-through-the-server.md`;
  `docs/design/pim.md`, "Invitations"; `schema/rpc/calendar.yaml`
  (`Invitation`, `invitation`).
- `internal/itip` (`FromMessage`, `Parse`, `Message.Main`, the methods).
- `internal/engine/calendar.go`: `eventUserEmails`, `calendarEventAPI`,
  `eventAnswer`, `occurrences` (stored occurrences in a zone), and the
  `Invitation` stub you replace (move it to `invitation.go`).
- `internal/engine/messages.go` (`messages.raw`: a stored body without
  the sync engine), `internal/pimsync/calendar.go` (`EventRow`: a parsed
  event as the store keeps it).
- `internal/store/events.go`: `Events`, `eventColumns`, `eventJoins`,
  `eventScan`, `parseEventTimes`.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

- **store:** `func (d *DB) EventsByUID(ctx context.Context, accountID
  int64, uid string) ([]EventRow, error)`: the events with that UID in
  the account's calendars (enabled or not), by ID, read as `Events`
  reads them (calendar, account and read-only set; no attendees or
  alarms needed).
- **Invitation:**
  1. The message's summary (`DB.Summaries`); none: `notFound`.
  2. Its raw body (`messages{c.Deps}.raw`), then `itip.FromMessage`:
     `ErrNoInvitation` is `notFound` ("message N has no invitation").
  3. `itip.Parse` with `calendar.Options{Local: time.Local, UserEmails:
     eventUserEmails(account)}`; an error is `invalidParams` ("the
     invitation cannot be read: …").
  4. `main := msg.Main()`. `Event` is `calendarEventAPI` of
     `pimsync.EventRow(main)` with the message's account ID (ID 0,
     calendar 0, not read-only).
  5. The stored event is the first of `EventsByUID(account, main.UID)`
     whose `RecurrenceID` equals `main.RecurrenceID`. When there is one:
     `EventID` is its ID, `Outdated` is its `Sequence` above `main`'s,
     and `Answer` its PartStat when not "". Otherwise `Answer` is
     `main.PartStat` (nil when "").
  6. `From`: for a reply, the first attendee (with their answer and
     `isUser`); otherwise the event's organizer as `calendarEventAPI`
     gives it.
  7. `CanRespond`: a request, the user invited (`main.PartStat` not
     ""), not outdated, the account not read-only, the stored event (if
     any) not read-only, and the event not cancelled.
  8. For a timed event, the occurrences of the event's day in maild's
     zone (`time.Local`; `c.occurrences` over that day), leaving out the
     stored event's own (every event ID with the UID), all-day ones,
     cancelled ones and ones the user declined:
     - `Conflicts`: those overlapping [start, end) that are not
       transparent, in start order;
     - `Adjacent`: of those not overlapping, the one ending last at or
       before the start, then the one starting first at or after the
       end; either may be missing.
     An all-day event has no conflicts and no adjacent occurrences.
     Both lists are empty, never null.

## Tests (given, do not edit)

`internal/engine/invitation_test.go`: invitations stored and not,
updated in the calendar after the mail, cancelled, not addressed to the
user, a reply, a read-only account, and the errors.

## Gotchas

- `messages.raw` already reads a stored body without the sync engine.
- `api.Invitation.Conflicts` and `Adjacent` are `[]api.Occurrence`;
  initialize them to empty slices.
- Keep functions under 60 lines: split the invitation into steps.

## Out of scope

`calendar.respond` (the planner's), the reader's card (T-0089), and
every file not under `touch`.

## Done when

`make accept T=0088` and `make check` pass (taskrun runs them), and only
the files under `touch` changed.
