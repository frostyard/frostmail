# 0035 — Invite through the provider's scheduling, and confirm before it sends

- **Status:** Accepted
- **Date:** 2026-10-14

## Context

[ADR-0034](0034-invite-from-the-organizers-copy-and-ask-before-telling-invitees.md)
decided that an event the user organizes tells its invitees through the
server only where a provider is verified to do so (`SchedulesOrganizer`).
Everywhere else, maild would mail the iTIP `REQUEST` or `CANCEL` itself,
with `SCHEDULE-AGENT=CLIENT` on each `ATTENDEE` so the server sends no
second copy. Plan 0009's Phase 5 probed both providers on the user's
accounts on 2026-10-14 (`tools/schedprobe`). It wrote organizer copies
over CalDAV with invitees on Google, iCloud and both, then changed and
deleted them:

| Step | Google | iCloud |
| --- | --- | --- |
| Create with invitees | Invites each one | Invites each one |
| `SCHEDULE-AGENT=CLIENT` or `NONE` on `ATTENDEE` | Strips it and invites anyway | Keeps it and invites anyway |
| Change the time | Updates every invitee | Updates every invitee |
| Add an alert, show as Free | Sends nothing | Sends nothing |
| Remove an invitee | Cancels for that invitee alone | Not probed (one invitee) |
| Delete | Cancels for every invitee | Cancels for every invitee |

Both providers schedule every organizer copy a client writes, and
neither can be kept quiet. maild's own invitations would reach every
invitee twice. iCloud also rewrites the account's own addresses as its
principal URL, with the address in an `EMAIL` parameter, and raises
`SEQUENCE` on changes of its own.

The user's accounts are Gmail and iCloud. On 2026-10-14 the user chose
to defer maild's mailed invitations to Later, since they would serve
only other CalDAV servers.

## Decision

- **Only where the server schedules.** An event takes invitees only in a
  calendar whose provider schedules organizer copies
  (`SchedulesOrganizer`: Google and iCloud, verified 2026-10-14). The
  calendar says so (`Collection.invites`). Elsewhere, an event with
  invitees is `invalidParams`, and the editor says why. maild mailing
  iTIP itself is Later.
- **maild writes, the server tells.** The organizer is the default
  identity of the calendar's account. maild writes `ORGANIZER` and the
  `ATTENDEE` lines without `SCHEDULE-AGENT`, and sends no invitation mail
  of its own. A new invitee's line has `CN` when known, `ROLE`
  (`REQ-PARTICIPANT` or `OPT-PARTICIPANT`), `PARTSTAT=NEEDS-ACTION` and
  `RSVP=TRUE`. An invitee who stays keeps their line, answer included.
  Invitees are one property in the scopes of
  [ADR-0031](0031-edit-a-recurring-event-in-three-scopes.md): All Events
  changes an override's invitees only when they were the master's.
- **What tells whom** is the servers' choice, which matches ADR-0034's
  table: creating an event with invitees, or adding invitees, invites
  them; a change to what invitees see (title, location, notes, times,
  recurrence, status) updates all of them; removing invitees cancels for
  them; deleting the event, an occurrence or the rest of a series
  cancels for all. The user's alerts and availability tell nobody.
- **Frostmail asks first.** A create, update, delete or restore that
  would tell invitees, sent without `notify: true`, fails with
  `notifyRequired`. The error's data is an `InviteNotice`: invitations,
  updates or cancellations, and how many invitees get them. The app then
  asks "Send Invitations?", "Send Updates?" or "Send Cancellations?" with
  **Send** and **Cancel**. Send repeats the call with `notify: true`, and
  Cancel abandons the change. maild alone decides what tells invitees,
  for every path: the editor, a drag, a delete and an undo.
- **No Don't Send** on Google and iCloud: their servers send whatever
  the copy says. `notify: false` is `invalidParams` there.
- **An event with invitees stays in its calendar.** A move is a put in
  one calendar and a delete in the other, which the servers announce as
  a new invitation and a cancellation.
- **Answers.** The servers apply invitees' answers to the organizer's
  copy, and maild shows them after its next pass.
- **Invited events** change only as
  [ADR-0022](0022-mail-the-reply-the-server-will-not-send.md) says, plus
  the user's own alerts.
- **iCloud's principals.** An `ORGANIZER` or `ATTENDEE` whose value is not
  a `mailto:` takes its address from its `EMAIL` parameter, so the user
  still organizes their own iCloud events and can answer as themselves.

## Consequences

- Each invitee gets one message per change, from their organizer's own
  provider, in the form they expect. Their answers come back through the
  server with no mail parsing in maild.
- The user cannot change a meeting quietly on Google or iCloud. Every
  change invitees see asks, and only Send writes it.
- Other CalDAV accounts cannot invite until maild mails invitations
  itself (Later). ADR-0034's `SCHEDULE-AGENT=CLIENT` path and its `REPLY`
  handling wait for that.
- `notifyRequired` makes the app's call a two-step exchange for events
  with invitees, and only for them.
- What a server sends for a given change is its decision. maild's
  notice can be wrong for a change the server treats differently, such
  as notes on one provider. That would be an extra question, or a message
  the user was not asked about.

## Alternatives considered

- **Mail the invitations, with `SCHEDULE-AGENT=CLIENT`** (ADR-0034's
  default): both servers ignore the parameter, so every invitee would get
  two copies.
- **Don't Send by writing the event without its invitees and adding them
  back later:** the servers would send cancellations, then invitations.
- **The app works out when to ask:** two copies of the rule, which drift.
  An undo would have to know what the restore writes.
- **Build maild's mailed invitations now** for other CalDAV servers: the
  user deferred it, since none of their accounts needs it.

## References

- Supersedes: [ADR-0034](0034-invite-from-the-organizers-copy-and-ask-before-telling-invitees.md)
- Shapes: [design/pim.md](../design/pim.md),
  [specs/pim-ui.md](../specs/pim-ui.md),
  [specs/rpc-api.md](../specs/rpc-api.md)
- Builds on: [ADR-0022](0022-mail-the-reply-the-server-will-not-send.md),
  [ADR-0030](0030-edit-contacts-and-events-as-patches.md),
  [ADR-0031](0031-edit-a-recurring-event-in-three-scopes.md)
- Evidence: plan 0009, Phase 5; `tools/schedprobe`
- RFC 6638 (CalDAV scheduling), RFC 7986 (`EMAIL`)
