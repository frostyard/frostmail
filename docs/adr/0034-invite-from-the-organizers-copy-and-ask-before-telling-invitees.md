# 0034 — Invite from the organizer's copy, and ask before telling invitees

- **Status:** Superseded by [0035](0035-invite-through-the-providers-scheduling.md)
- **Date:** 2026-10-14

## Context

[ADR-0022](0022-mail-the-reply-the-server-will-not-send.md) decided how
the user answers an invitation. M5.5 lets the user organize events with
invitees, so Frostmail must now send invitations, updates and
cancellations. Servers that schedule (RFC 6638) send them on their own
when the organizer's copy is written with `ATTENDEE`s. Whether Google
and iCloud do so for a client's write over CalDAV is not yet verified;
M4.5 found that iCloud does not schedule for every copy a client writes.
As with answers, the ways this fails are two messages or none.
`SCHEDULE-AGENT=CLIENT` on an `ATTENDEE` tells a server to leave that
invitee to the client, and `SCHEDULE-AGENT=NONE` to send nobody anything.

Outlook asks whether to send an update; Google Calendar asks whether to
email guests; Apple Calendar sends without asking. On 2026-10-14 the user
chose that Frostmail asks.

## Decision

- **The organizer** is the default identity of the account whose
  calendar holds the event. An event with invitees must be in an account
  that sends mail, and it moves only within that account
  ([ADR-0030](0030-edit-contacts-and-events-as-patches.md)).
- **The server or the mail, per provider.** A new provider fact,
  `SchedulesOrganizer`, says that a provider tells invitees about events
  a client writes. It is false until plan 0009's Phase 5 verifies it.
  - Where it is true, maild writes the `ATTENDEE`s without
    `SCHEDULE-AGENT`, and the server tells them.
  - Everywhere else, every `ATTENDEE` carries `SCHEDULE-AGENT=CLIENT`,
    and maild mails the iTIP message (RFC 5546, 6047) itself, from the
    organizer's identity, through the outbox with its undo delay: one
    message per method per change, to the invitees it concerns.
- **What tells whom:**

  | Change | Message |
  | --- | --- |
  | Creating an event with invitees; adding invitees | `REQUEST`, to them |
  | Changing what invitees see (title, location, times, recurrence, notes) | `REQUEST`, to all |
  | Removing invitees | `CANCEL`, to them |
  | Deleting the event, an occurrence or the rest of a series | `CANCEL`, to all |

  `SEQUENCE` rises for the changes RFC 5545 counts as significant:
  `DTSTART`, `DTEND`, `DURATION`, `RRULE`, `RDATE`, `EXDATE` and
  `STATUS`. The user's alerts, availability and calendar tell nobody.
  For one occurrence, the message carries its `RECURRENCE-ID`. A split
  series ([ADR-0031](0031-edit-a-recurring-event-in-three-scopes.md))
  sends a `REQUEST` for the shortened series and another for the new one.
- **Invitees' answers.** Where maild mails the invitations, the
  answers come back as mail: maild applies an attendee's `REPLY` to the
  organizer's copy (their `PARTSTAT`, as a patch and a `put`), and the
  event pane shows it. Where the server sends them, the server applies
  the answers.
- **Frostmail asks first.** Before a change that would tell invitees,
  the app asks: "Send updates to the invitees?", with Send, Don't Send
  and Cancel ("Send invitations?" for a new event, "Send
  cancellations?" for a delete). Cancel abandons the change. The answer
  is `notify` on `calendar.create`, `update`, `delete` and `restore`; a
  call that would tell invitees without it is `invalidParams`, so no path
  sends without asking.
  - **Don't Send** writes the change and tells nobody. Where maild mails,
    it simply sends nothing; `SEQUENCE` still rises, so a later update
    supersedes it. Where the server schedules, Don't Send is offered only
    once Phase 5 has verified how that provider can be kept quiet
    (`SCHEDULE-AGENT=NONE` for the write); until then the sheet there
    offers Send and Cancel.
- **Invited events** (the user is an attendee, not the organizer) change
  only as ADR-0022 says, plus the user's own alerts, which tell nobody.

## Consequences

- No path sends invitations, updates or cancellations without the
  user's yes, and each invitee gets one message per change.
- Until Phase 5 verifies a provider, Frostmail sends every invitation
  itself, even on Google. Invitees get Frostmail's mail rather than
  Google's, and their answers arrive as mail that maild applies.
- Applying `REPLY`s adds a write that reacts to incoming mail. It runs
  only for events the user organizes, on an `ATTENDEE` already in the
  event.
- Don't Send can leave invitees with an outdated copy. That is the
  user's choice, and the next update fixes it.

## Alternatives considered

- **Always send, as Apple does:** fewer questions, but the user chose to
  be asked, and a drag that moves a meeting should not mail people
  unannounced.
- **Always through the server:** two messages or none on a provider
  that behaves differently from what was assumed, which is exactly what
  M4.5 found on iCloud.
- **Always by mail, everywhere:** predictable, but it fights Google's
  own invitations once Google is verified to send them.

## References

- Shapes: [design/pim.md](../design/pim.md),
  [design/send.md](../design/send.md),
  [specs/pim-ui.md](../specs/pim-ui.md)
- Builds on: [ADR-0022](0022-mail-the-reply-the-server-will-not-send.md),
  [ADR-0030](0030-edit-contacts-and-events-as-patches.md),
  [ADR-0031](0031-edit-a-recurring-event-in-three-scopes.md)
