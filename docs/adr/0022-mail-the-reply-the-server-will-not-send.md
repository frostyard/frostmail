# 0022 — Mail the reply the server will not send

- **Status:** Accepted
- **Date:** 2026-10-10

## Context

[ADR-0019](0019-answer-invitations-through-the-server.md) answered an
invitation through the server's scheduling on Google and iCloud (a `PUT` of
the calendar copy with the user's `PARTSTAT`) and mailed an iTIP `REPLY`
only on servers that do not schedule, assuming both providers tell the
organizer whenever the attendee's copy changes. M4.5's Phase 5 tried it on
the user's accounts, answering in Frostmail invitations that had arrived
as mail and were not yet in the calendar, so Frostmail created the copy:

- Google (an invitation from another Google account): Google told the
  organizer; one reply.
- iCloud (an invitation from a Google account to the iCloud address):
  iCloud told nobody; the organizer's copy still said `NEEDS-ACTION` a
  quarter of an hour later. iCloud schedules for the copies its own
  scheduling delivered, not for one a client creates from an emailed
  invitation.

RFC 6638 §7.1 lets a client take scheduling over for an event:
`SCHEDULE-AGENT=CLIENT` on the `ORGANIZER` property of the attendee's copy
tells the server not to send messages for it. iTIP messages themselves
carry no `SCHEDULE-*` parameters.

## Decision

- **One event per UID**, the card, updates and cancellations, and
  read-only accounts are as in ADR-0019.
- **Through the server when it delivered the invitation.** A stored copy
  without `SCHEDULE-AGENT=CLIENT` on a server that schedules (the provider
  profile's `Schedules`) is answered by its `PUT` alone.
- **Through the server for a new copy only where verified.** An invitation
  not in the calendar is accepted by creating its copy; the server tells
  the organizer only on a provider whose profile says it schedules for
  copies a client creates (`SchedulesCopies`: Google yes, iCloud no).
- **Otherwise by mail, and the copy says so.** In every other case maild
  mails the `REPLY` through the outbox (with its undo delay), and the copy
  it writes carries `SCHEDULE-AGENT=CLIENT` on `ORGANIZER`, so a server
  that schedules after all (iCloud for some copies, Nextcloud's SabreDAV)
  does not send a second reply. A stored copy that already carries it was
  answered by mail before and is answered by mail again.
- The `REPLY` drops `SCHEDULE-*` parameters from the lines it copies.

## Consequences

- iCloud gets exactly one reply for an emailed invitation, and Google
  keeps its own. Plain CalDAV servers that schedule without saying so no
  longer duplicate maild's reply.
- An invitation answered by mail stays the client's to answer: changing
  the answer later mails again, even on Google.
- `SchedulesCopies` is a provider fact like `Schedules`; a new provider
  starts without it, so a wrong guess errs towards mail. Google's case is
  verified for an organizer on Google only; an external organizer is still
  to be checked.
- The copy differs from the server's own scheduling copies by one
  parameter, which other clients ignore or honour as RFC 6638 says.

## Alternatives considered

- **Always mail, always `SCHEDULE-AGENT=CLIENT`:** one rule everywhere,
  but it takes replies away from Google, whose own replies keep the
  organizer's Google Calendar in step without parsing mail.
- **Mail only on iCloud by kind:** fixes the observed case but leaves
  copies a client created on other scheduling servers to a guess.
- **Ask the user which way:** an implementation detail no user can answer.

## References

- Supersedes: [ADR-0019](0019-answer-invitations-through-the-server.md)
- Shapes: [design/pim.md](../design/pim.md) (Invitations),
  [design/send.md](../design/send.md)
- RFC 6638 §7.1 (`SCHEDULE-AGENT`), RFC 5546 (iTIP), RFC 6047 (iMIP)
