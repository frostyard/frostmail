# 0030 — Edit contacts and events as patches, and re-apply a refused change

- **Status:** Accepted
- **Date:** 2026-10-14

## Context

M4.5 writes three kinds of change to CardDAV and CalDAV: an answer to an
invitation (a `PARTSTAT` patch), a contact made by Add to Contacts, and
task edits. Each is a `pim_ops` row whose `put` writes the object's source
as it is when the op runs, on the ETag the change was made on. A server
that refuses the write (412 because another client changed the object, or
another 4xx) fails the op for good, and the next listing brings back the
server's version. For a tick on a task that is tolerable. M5.5 edits every
field of contacts and events, while the user's phone and the providers'
web apps edit the same objects. Losing an edit without a word, or writing
over another client's change, is not.

ADR-0018 already decided that every change is a patch to the stored
source. What is missing is the patches for contacts and events, and what
happens when the server refuses one.

## Decision

- **A change is a list of fields, and a patch applies it.** The API takes
  the fields that change (`ContactFields`, `EventFields`). maild applies
  them to the stored source with a patch function and stores the result
  at once, so the app shows the change before the server has it:
  - `vcardx.Patch(src, fields)` writes in the card's own version (3.0 or
    4.0). A list field (emails, phones, addresses, URLs) replaces the
    lines of that property in order: the n-th value rewrites the n-th
    line, keeping its group, its other parameters and its group's other
    lines; a label the card writes as an Apple group label
    (`itemN.X-ABLabel`) stays one. Extra values add lines. A known label
    becomes `TYPE`, and any other label becomes a new group with
    `X-ABLabel`. Missing values remove their lines, with their groups'
    other lines. `FN` follows the names unless the card's `FN` differed
    from them.
  - `vcardx.NewCard(uid, fields)` writes a vCard 3.0, which Google and
    iCloud both take.
  - `calendar.PatchEvent` and `calendar.NewEvent` do the same for
    events, in the scopes of
    [ADR-0031](0031-edit-a-recurring-event-in-three-scopes.md), with
    times as [ADR-0033](0033-write-new-events-with-their-zones-vtimezone.md)
    says.
  - **Photos** are `PHOTO`: `ENCODING=b;TYPE=JPEG` (or `PNG`) in 3.0, a
    `data:` URI in 4.0. The app scales the image to at most 512 × 512
    before it sends it, and maild refuses anything that is not JPEG or
    PNG or is over 256 KB.

  Each function has golden tests (source in, source out) over Google's,
  iCloud's and Nextcloud's objects.
- **The op carries the change.** A `put` an edit queues records in its
  payload the fields of each change, in order. It also records the
  *base*: for every property the changes touch (by component and name),
  the lines the source had when the first change was made. A later edit
  before the op runs adds its fields to the same op, and its base for any
  property not already recorded.
- **A 412 re-applies the change when it still fits.** maild fetches the
  server's object. When each touched property's lines on the server
  equal the base, nobody else changed what the user changed: maild
  applies the recorded fields to the server's source, stores the result
  with the server's ETag, and writes again (at most three times). When
  another client changed a touched property too, the server's version is
  stored and stands. The op is dropped, and maild emits `people.refused`
  or `calendar.refused` naming the contact or event, which the app shows.
- **Other refusals stand.** A 404 (deleted elsewhere), another 4xx, or a
  412 on a delete leaves the server's version (or its absence) in place
  and emits the same event. A refused creation is removed locally, as
  today.
- **Moving an event to another calendar** writes a new object with the
  same UID in the new calendar (`If-None-Match: *`). Once that write
  succeeds, the object is deleted from the old calendar on its ETag; the
  delete waits for the put. Contacts do not move between address books.
- **Read-only** accounts, address books and calendars refuse every write
  with `conflict` before anything is stored.

## Consequences

- Edits survive another client's change to other fields, and never
  silently overwrite a change to the same field: the user sees the
  conflict.
- The payload makes ops larger and stateful. A crash between two edits
  keeps both, because both are in the op.
- Every list property needs careful golden tests. Apple's grouped labels,
  Google's `TYPE` labels and vCard 4's parameters all have to round-trip
  unchanged where they were not edited.
- Moving an event is two writes. A crash between them leaves two copies
  for a pass, until the delete runs.

## Alternatives considered

- **Last write wins** (writing without `If-Match`): simple, but it
  overwrites whatever another client changed in the meantime.
- **A three-way merge of every property:** keeps more of both sides, but
  needs a merge rule per property, which the re-check of the touched
  properties makes unnecessary.
- **Asking the user on every 412:** more prompts. And most 412s come
  from another client changing something else, which re-applying handles
  without a question.

## References

- Shapes: [design/pim.md](../design/pim.md),
  [specs/pim-ui.md](../specs/pim-ui.md)
- Builds on: [ADR-0018](0018-keep-contacts-events-and-tasks-as-sent.md)
