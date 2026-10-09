# 0019 — Answer invitations through the server's scheduling, else by mail

- **Status:** Accepted
- **Date:** 2026-10-08

## Context

An invitation reaches the user twice: as mail with a `text/calendar` part
(iMIP, `METHOD:REQUEST`), and, on servers that schedule (Google, iCloud),
as an event the server already put in the user's calendar with the same
`UID`. Answering means changing the user's `PARTSTAT` and telling the
organizer. Servers that implement CalDAV scheduling (RFC 6638,
`calendar-auto-schedule` in the `DAV` header, or a provider known to do it)
send the reply themselves when the calendar copy changes; servers that do
not leave it to the client, which mails an iTIP `REPLY` (RFC 5546, 6047).
Doing both sends the organizer two replies; doing neither sends none.

## Decision

- **One event per UID.** An invitation in the reader is shown as the
  calendar's event with that `UID` when the account's calendar has one, and
  from the mail's `text/calendar` part otherwise. The reader replaces the
  raw `.ics` attachment with a card: the time in the user's zone, the
  organizer and attendees, conflicts and adjacent events from the instances
  table, and Accept, Maybe and Decline.
- **Answer through the server when it schedules.** If the event is in a
  calendar of a server that schedules, the answer is a conditional `PUT` of
  the event with the user's `PARTSTAT` changed (ADR-0018's surgical patch),
  and the server tells the organizer.
- **Otherwise by mail.** If the server does not schedule, or the event is
  not on the server, maild builds the iTIP `REPLY` and sends it through the
  outbox (with its undo delay) to the organizer, and an accepted event is
  put in the account's default calendar.
- **Updates and cancellations** (`SEQUENCE` above the stored one, `METHOD:
  CANCEL`) update the card and, on a server that does not schedule, the
  calendar copy.
- **Read-only accounts** show invitations but do not answer them.

## Consequences

- Answering works on Google, iCloud and plain CalDAV servers, and sends one
  reply each way.
- Whether Google and iCloud schedule for a change made over CalDAV is a
  provider fact to verify in the trial and record in the provider profile;
  a wrong guess shows up as zero or two replies, which the trial checks.
- Proposing a new time (`COUNTER`) and delegating are left for later.

## Alternatives considered

- **Always mail the reply:** works everywhere, but duplicates the server's
  own reply on Google and iCloud.
- **Only through the server:** fails silently on servers without
  scheduling and for invitations whose event is not on the server.

## References

- Shapes: [design/pim.md](../design/pim.md),
  [design/send.md](../design/send.md)
- Builds on: [ADR-0017](0017-sync-contacts-calendars-and-tasks.md),
  [ADR-0018](0018-keep-contacts-events-and-tasks-as-sent.md)
