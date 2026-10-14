# 0031 — Edit a recurring event in three scopes

- **Status:** Accepted
- **Date:** 2026-10-14

## Context

A recurring event is one object: a master VEVENT with its rule (`RRULE`,
`RDATE`, `EXDATE`) and any number of overrides, VEVENTs with the same UID
and a `RECURRENCE-ID` naming the occurrence they replace. Changing one
occurrence, the rest of the series from one occurrence on, or the whole
series each write something different. Google Calendar and Outlook offer
all three; Apple Calendar offers "only this event" and "future events",
which from the first occurrence means all of them. iCalendar's own way to
change a series from an occurrence on (`RANGE=THISANDFUTURE`) is rarely
supported by servers or other clients.

## Decision

The app asks for the scope whenever the user changes or deletes an
occurrence of a series. The answer is This Event, All Future Events or
All Events, and `calendar.update` and `calendar.delete` take it as
`scope`.

- **This Event.** A change writes an override. If the occurrence already
  has one, it is patched. Otherwise a new VEVENT is added to the object:
  a copy of the master without `RRULE`, `RDATE` and `EXDATE`, with
  `RECURRENCE-ID` set to the occurrence's original start (in the form of
  the master's `DTSTART`: the same `TZID`, a date, or UTC), and the
  occurrence's own times with the change applied. A delete adds the
  occurrence's start to the master's `EXDATE` (same form) and removes its
  override.
- **All Future Events,** from an occurrence other than the first, splits
  the series:
  - The old series ends before the occurrence. The rule gets an `UNTIL`
    a second before the occurrence's start (the day before, for all-day
    events), replacing any `COUNT`. Its `RDATE`s, `EXDATE`s and overrides
    from the occurrence on are removed from the old object.
  - A new object with a new UID starts at the occurrence: the master with
    the change applied, the rule's original end (its `UNTIL`, or its
    `COUNT` less the occurrences before), and the `RDATE`s, `EXDATE`s and
    overrides that were removed from the old one.

  A delete is the first half alone. From the first occurrence, All
  Future Events is All Events.
- **All Events** patches the master.
  - When the change moves the start by an offset, every
    `RECURRENCE-ID`, `EXDATE` and `RDATE` moves by the same offset, so
    each still names its occurrence.
  - An override's own `DTSTART` and `DTEND` move too when they were the
    occurrence's original times. Times the user had changed for that
    occurrence stay.
  - Every other property follows the master into an override when the
    override still had the master's old value. A value changed for that
    one occurrence stays.
  - When the rule itself changes (frequency, days, interval), the
    overrides and `EXDATE`s that no longer name an occurrence are
    dropped.

  A delete deletes the object.
- An event the user is invited to, rather than organizing, has no
  scopes: only the user's answer
  ([ADR-0022](0022-mail-the-reply-the-server-will-not-send.md)) and
  alerts change, on the occurrence or the series.

## Consequences

- What the user sees matches Google Calendar's and Outlook's choices,
  and the objects written match what Apple and Google write, so other
  clients show the same occurrences.
- A split series is two UIDs. Invitations to it
  ([ADR-0034](0034-invite-from-the-organizers-copy-and-ask-before-telling-invitees.md))
  update the old event and invite to the new one, which is what Google
  sends.
- Moving the start of a long series with many overrides rewrites them
  all, in one object.
- The offset rule is about the start only. A change of the duration
  alone moves no `RECURRENCE-ID`.

## Alternatives considered

- **Apple's two scopes:** the app's sheet would be simpler, but renaming
  a whole series from a later occurrence would mean finding its first
  one.
- **`RANGE=THISANDFUTURE` overrides:** a single object, but servers and
  other clients ignore or mishandle it.
- **Dropping overrides when All Events moves the start:** what some
  clients do, and it silently throws away the user's changes to single
  occurrences.

## References

- Shapes: [design/pim.md](../design/pim.md),
  [specs/pim-ui.md](../specs/pim-ui.md)
- Builds on: [ADR-0018](0018-keep-contacts-events-and-tasks-as-sent.md),
  [ADR-0030](0030-edit-contacts-and-events-as-patches.md)
