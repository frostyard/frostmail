# 0025 — maild keeps Send Later, Remind Me and Undo Send

- **Status:** Accepted
- **Date:** 2026-10-10

## Context

Mail.app can send a message later, at a chosen time, from a Send Later
mailbox where it can be edited until then (P-506). It can bring a message
back to the top of the inbox at a chosen time, Remind Me (P-306, P-412).
Its Undo Send delay is a setting: Off, 10, 20 or 30 seconds (P-505).

`draft.send` builds the message, stores it as a blob, and queues an
outbox row whose `send_at` is now plus the undo delay, 10 seconds unless
`FROSTMAIL_UNDO_DELAY` says otherwise ([ADR-0010](0010-compose-and-send.md)).
The account's sender sends rows as they fall due, including rows that fell
due while maild was stopped. The draft stays until the server accepts the
message, so `outbox.cancel` can return it. maild's reminder scheduler
checks calendar alarms once a minute and when kicked. Views list messages
newest first by arrival date.

## Decision

- **Send Later is an outbox row due later.** `draft.send` takes an
  optional `sendAt`. A time past the undo delay queues the row as
  scheduled, due then, and builds the message with that time as its
  `Date`. `outbox.reschedule` gives a scheduled row a new time and
  rebuilds it with the new `Date`. Editing is `outbox.cancel`, which
  returns the draft, then a new send. A Send Later mailbox lists the
  scheduled rows of every account. A row that fell due while maild was
  stopped goes out at its next start, as any due row does. Scheduled rows
  are not in their undo window, so the app shows no undo toast for them.
- **Undo Send's delay is a setting:** 0, 10, 20 or 30 seconds, 10 by
  default ([ADR-0026](0026-preferences-live-in-maild.md)). 0 queues the row
  due at once. `FROSTMAIL_UNDO_DELAY` still overrides it, for tests.
- **Remind Me is a time on a message.** `message.remind` sets or clears
  it. maild's reminder scheduler, which already wakes each minute and on
  a kick, also fires due message reminders. One transaction clears the
  reminder, gives the message its new place in the lists (the time the
  reminder fired, used instead of its arrival date), and moves it back to
  its account's inbox if it left (an op, as `message.move`; a label edit on
  Gmail). maild then notifies. A reminder that fell due while maild was
  stopped fires at its next start. Deleting the message drops its
  reminder; read-only accounts cannot set one.

## Consequences

- Scheduled mail and reminders happen with the app closed, and late
  rather than never when the computer was off.
- Views order by a list date (the arrival date until a reminder fires).
  That is a new column with its own index, kept by every insert and by
  the reminder.
- A scheduled message's `Date` is the chosen time even when it leaves
  late. RFC 5322 makes `Date` the time the message was ready to send,
  which is the chosen time.
- Reminders and scheduled sends are maild's, on this machine. Mail.app on
  another Mac does not see them, and they do not see Mail.app's.

## Alternatives considered

- **Build the scheduled message when it goes:** the sender would need the
  compose path and could fail at send time on a draft that has changed.
  Building at queue time keeps one sending path.
- **A reminder as a task or calendar alarm:** it would show in Tasks, not
  in the inbox, which is Mail.app's point.
- **Mark the reminded message unread instead of moving it up:** it would
  stay where it was, often far down the list.

## References

- Shapes: [design/send.md](../design/send.md),
  [design/organize.md](../design/organize.md) (Remind Me),
  [specs/compose-ui.md](../specs/compose-ui.md),
  [specs/parity.md](../specs/parity.md) (P-306, P-412, P-505, P-506)
- Builds on: [ADR-0010](0010-compose-and-send.md),
  [design/pim.md](../design/pim.md) (the reminder scheduler)
