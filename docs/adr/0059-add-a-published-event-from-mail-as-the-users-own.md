# 0059 — Add a published event from mail as the user's own event

- **Status:** Proposed
- **Date:** 2026-11-12

## Context

Mail often carries an event that asks for no answer: a ticket, a booking, a
class schedule, an event someone shares as a file. Its `text/calendar` part
has `METHOD:PUBLISH` or no `METHOD` (RFC 5546 §3.2.1; `internal/itip` reads
both as `publish`). Apple Mail and Calendar offer to add such an event to a
calendar. Frostmail's invitation card shows it under the badge "Event" with
Show in Calendar and nothing else
([specs/pim-ui.md](../specs/pim-ui.md#invitation-card)): the user cannot
keep it without typing it again.

`calendar.respond` already stores an accepted invitation that the calendars
do not hold: the message's source without `METHOD`, under a new href in the
account's default calendar ([ADR-0022](0022-mail-the-reply-the-server-will-not-send.md),
[design/pim.md](../design/pim.md), Invitations). A published event differs
from an invitation in what its lines mean once a server holds them:

- **`ORGANIZER` and `ATTENDEE`.** A sender may write them although RFC 5546
  gives a published event no attendees. On a server that schedules (Google,
  iCloud; RFC 6638), an object with an organizer is a scheduling object:
  the server may treat the user as an attendee of someone else's event,
  refuse the user's changes to it, or send messages for it.
- **`VALARM`.** maild shows `DISPLAY` and `AUDIO` alarms and leaves
  `EMAIL` alarms to the server ([design/pim.md](../design/pim.md),
  Calendars). An alarm a sender wrote would ring, or have the user's server
  send mail to an address the sender chose, without the user setting it.
- **`UID`.** `calendar.invitation` finds the stored copy of a message's
  event by its `UID` in the message's account.

## Decision

- **A new method, `calendar.addFromMessage {messageId, calendarId?}`,**
  adds a message's published event to a calendar. It refuses every other
  method (`conflict`): an invitation is answered, not added.
- **The copy is the user's own event.** maild stores the message's
  iCalendar source without its `METHOD` line and, in every `VEVENT`,
  without the `ORGANIZER` and `ATTENDEE` lines and the `VALARM` blocks
  (`itip.Published`). Everything else stays as sent: the `UID`, the times
  and their zones, the recurrence and its overrides, the title, the
  location and the notes. The user sets alerts in the event editor.
- **One event.** A file may publish several events. The copy holds the one
  the card shows, the first `VEVENT` without a `RECURRENCE-ID` (else the
  first), with the `VEVENT`s of its `UID` (a series' overrides) and the
  file's time zones. The events of other `UID`s and any other component
  (`VTODO`, `VJOURNAL`, `VFREEBUSY`) are left out: a calendar resource
  holds one `UID` (RFC 4791 §4.1), and a server refuses one that holds
  more. A part that holds several calendar objects gives its first: the
  copy is that one object's bytes, and nothing before or after it is
  stored. maild checks the copy is one calendar object that holds that
  `UID` alone before it writes.
- **Where it goes:** the calendar named, which must take writes, else the
  default calendar of the message's account, found as `calendar.respond`
  finds it. No calendar that takes writes is a `conflict`.
- **Once.** An event whose `UID` a calendar of any account holds gets no
  second copy (`conflict`). maild checks this in the transaction that
  stores the copy, so two adds at the same time store one. The calendar
  named may belong to another account than the message's, so for a
  published event `calendar.invitation` looks for the copy in every
  account. `Invitation.canAdd` tells the card when the method would
  succeed with no calendar named: a published event, not held, in an
  account that is not read-only and has a calendar that takes writes.
- **The card** shows **Add to Calendar** when `canAdd`, and afterwards the
  line "This event is in your calendar." with Show in Calendar selecting it.

## Consequences

- A ticket or booking is kept with one click, and found again from its
  message by its `UID`.
- The copy names nobody. A reader of the calendar does not see who sent the
  event; the message still does. The user can change or delete the copy
  like any event they made.
- A sender's reminder is lost: the user adds an alert if they want one.
- From a file of several events the user gets the first. The card shows
  no other, so the others cannot be added; splitting such a file into a
  card for each event is not built.
- An update the sender publishes later (the same `UID`, a higher
  `SEQUENCE`) does not change the copy. The card says the calendar has the
  event; applying a published update is not built.
- The copy's `UID` is the sender's. A server that already holds that `UID`
  in a calendar maild has not synced, or in another collection, may refuse
  the `PUT`; maild then reports it as it reports any refused write
  (`calendar.refused`). This is not checked against Google or iCloud.
- An invitation the user is not invited to (a forwarded `REQUEST`) still
  has no button: adding it would need the same stripping and a decision on
  whether a forwarded invitation is the user's to keep.

## Alternatives considered

- **Store the source as sent, as `calendar.respond` does:** keeps the
  organizer, but hands a scheduling server an object it may schedule, and
  keeps the sender's alarms.
- **Keep `ORGANIZER` with `SCHEDULE-AGENT=CLIENT`:** tells an RFC 6638
  server not to schedule, but the copy then reads as someone else's event,
  which the editor shows as an invitation with only the alerts open.
- **Build the event from fields through `calendar.create`:** needs no new
  method, but makes a new `UID`, so the message no longer finds its event,
  and loses what `EventFields` cannot carry (overrides, `RDATE`).
- **Keep `DISPLAY` alarms and drop only `EMAIL`:** still rings at a time
  the sender chose.

## References

- Shapes: [design/pim.md](../design/pim.md) (Invitations),
  [specs/pim-ui.md](../specs/pim-ui.md#invitation-card),
  [specs/rpc-api.md](../specs/rpc-api.md) (`calendar.addFromMessage`)
- Builds on: [ADR-0022](0022-mail-the-reply-the-server-will-not-send.md),
  [ADR-0019](0019-answer-invitations-through-the-server.md)
- RFC 5546 §3.2.1 (`PUBLISH`), RFC 6638 (scheduling)
