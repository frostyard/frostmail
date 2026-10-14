# 0032 — Undo restores what a change replaced

- **Status:** Accepted
- **Date:** 2026-10-14

## Context

Mail's undo replays an inverse call that the app keeps
([ADR-0028](0028-mute-and-block-in-maild-undo-in-the-app.md)): moving a
message back, or setting its flags back. For contacts and events, an
inverse built from fields would lose what the app never sees (attendees'
answers, alarms of other devices, vendor properties). Several changes
have no inverse in fields at all: a delete, a split series
([ADR-0031](0031-edit-a-recurring-event-in-three-scopes.md)), or a move
between calendars.

## Decision

- **Every write keeps what it replaced.** Creating, changing, deleting
  or moving a contact or event, and setting a photo, records a
  `pim_restores` row. For each object the change touched, it holds the
  collection, href and UID, and the source as it was before, or nothing
  when the change created the object. The method returns its
  `restoreId`.
- **Restore writes it back.** `calendar.restore` and `people.restore`
  take a `restoreId`. They put each kept source back as the object's
  whole source, on its current ETag. A deleted object is created again
  with `If-None-Match: *`, and an object the change created is deleted.
  A restore is a change like any other: it is queued, written, and
  re-applied or refused as
  [ADR-0030](0030-edit-contacts-and-events-as-patches.md) says.
- **Kept for a day.** A restore older than that is dropped, and asking
  for it is `notFound`. The app's undo list lives in memory and holds
  `{method: "calendar.restore" | "people.restore", restoreId}`, as Mail's
  holds its inverse calls.
- **Invitees are told again.** A restore of an event the user organizes
  that tells invitees asks first and takes `notify`, as the change did
  ([ADR-0034](0034-invite-from-the-organizers-copy-and-ask-before-telling-invitees.md)).

## Consequences

- Every kind of change has an undo, and undo brings back exactly what
  was there, unknown properties included.
- Undo after another client changed the same object meanwhile puts the
  user's earlier version over that change. This is the user's own
  explicit step, made within the day.
- The store keeps a day of replaced sources. That is small for contacts
  and events, but a contact's photo counts.

## Alternatives considered

- **Inverse calls built by the app:** consistent with Mail, but they lose
  data and cannot express a delete, a split or a move.
- **Keeping the history on the server** (trash collections, versioning):
  Google and iCloud expose none over CalDAV and CardDAV.

## References

- Shapes: [design/pim.md](../design/pim.md),
  [specs/pim-ui.md](../specs/pim-ui.md)
- Builds on: [ADR-0028](0028-mute-and-block-in-maild-undo-in-the-app.md),
  [ADR-0030](0030-edit-contacts-and-events-as-patches.md)
