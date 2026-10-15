---
id: "0159"
title: The event editor's Invitees field
milestone: M5.5
size: L
touch:
  - app/src/lib/eventDraft.ts
  - app/src/features/calendar/InviteesField.tsx
  - app/src/features/calendar/EventEditor.tsx
  - app/src/app/EventWindow.tsx
given:
  - app/src/lib/eventDraft.invitees.test.ts
  - app/src/features/calendar/InviteesField.test.tsx
  - app/src/app/EventWindow.invitees.test.tsx
acceptance: make ui-vitest F="src/lib/eventDraft src/features/calendar src/app/EventWindow src/app/Notify"
---
# T-0159: The event editor's Invitees field

## Goal

The user can invite people to an event they organize, in a calendar
whose server tells invitees (ADR-0035): the event editor gets an
Invitees row that lists each invitee with their answer and an optional
mark, and adds more with address completion as compose's recipient
fields do (docs/specs/pim-ui.md, "Event editor window", the invitees
row). A save sends the invitees as `EventFields.attendees`, and since
T-0158 the notify sheet asks before maild tells them.

## Read first

- `docs/specs/pim-ui.md`: "Event editor window" (the calendar and
  invitees rows, Footer, Save) and "Sending to invitees".
- `docs/adr/0035-invite-through-the-providers-scheduling.md`.
- `schema/rpc/calendar.yaml`: `Invitee`, `EventFields.attendees`,
  `Attendee`; `schema/rpc/account.yaml`: `Collection.invites`;
  `schema/rpc/address.yaml`: `address.suggest`.
- `app/src/lib/eventDraft.ts` (`EventDraft`, `draftOf`, `newDraft`,
  `validate`, `changedFields`), `app/src/features/calendar/EventEditor.tsx`
  (`CalendarChoice`, `CalendarSelect`, `Row`), `app/src/app/EventWindow.tsx`
  (`loadEvent`, `LoadedEditor`).
- `app/src/features/compose/RecipientField.tsx`: the input the field
  reuses; `app/src/lib/addressParse.ts`: `isValidAddress`.
- `app/src/features/calendar/EventPane.tsx`: how answers are drawn.
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

### `lib/eventDraft.ts`

- `DraftInvitee`: `{email, name, role: "required" | "optional", answer?:
  PartStat}`. `EventDraft.invitees?: DraftInvitee[]`, absent when the
  event invites nobody, so drafts and tests made before stay as they
  are. `DraftErrors` gains `invitees`.
- `draftOf` sets `invitees` from the event's attendees without
  `isUser` (role `optional` stays, anything else is `required`, with
  the answer), only when there are any. `newDraft` leaves it out.
- `validate` adds `invitees: "Check the invitees' addresses"` when an
  invitee's address is not valid (`isValidAddress`), along with any
  error of the times.
- `changedFields` sends `attendees` when the invitees changed by address
  (case-insensitive) and role, in any order, or for a new event that
  has some: each `{email, name (only when not empty), role}`, in the
  draft's order. A name alone does not count as a change.
- `sendsInvitees(original, draft)`: whether a save writes invitees: a
  new event with some, or a changed list that is not empty.

### `InviteesField` (new, `features/calendar/InviteesField.tsx`)

- Props: `invitees`, `onChange(next)`, `suggest(prefix)`, `available`
  (the chosen calendar invites), `disabled?`.
- A list named "Invitees" (only when there are invitees) of rows as the
  spec draws them: the name, else the address (tooltip "Name
  <address>", or "<address> is not a valid address" and `text-flag-1`
  for one that is not), the answer icon labelled "Accepted", "Declined"
  or "Maybe", an "Optional" toggle (`aria-pressed`) and a Remove button
  named "Remove <name or address>".
- Under it, when `available` and not `disabled`, a `RecipientField`
  labelled "Add Invitee" whose value is always empty: what it commits
  is added as `{email, name, role: "required"}` unless that address is
  already invited (case-insensitive), invalid ones included so they can
  be seen and removed; its suggestions leave out those already invited.
- Not `available`: no input, the line "Invitations aren't available in
  this calendar.", and the rows' buttons disabled. `disabled` (a
  read-only or invited event): no input, the buttons disabled.

### `EventEditor` and `EventWindow`

- `CalendarChoice.invites?: boolean` (the collection's `invites`);
  `EventEditor` takes `suggest?` and draws the Invitees row (label
  "Invitees") after the calendar row, `available` from the chosen
  calendar's `invites`, `disabled` as the other fields; `errors.invitees`
  shows under it.
- `EventWindow`: choices carry `invites`; an event whose draft has
  invitees, in a calendar that invites, offers only its own calendar;
  `suggest` is `address.suggest {prefix, limit: 8}`; Save is also
  disabled while `sendsInvitees` and the chosen calendar does not
  invite.

## Tests (given, do not edit)

`app/src/lib/eventDraft.invitees.test.ts`,
`app/src/features/calendar/InviteesField.test.tsx` and
`app/src/app/EventWindow.invitees.test.tsx`; the existing eventDraft,
EventEditor, EventWindow and Notify tests must still pass.

## Gotchas

- The earlier given tests build `EventDraft` and `CalendarChoice`
  literals without the new fields, and one compares `draftOf`'s result
  exactly: keep both fields optional and leave `invitees` out when
  empty.
- `RecipientField` turns text that does not parse into an address
  string as typed; keep it as an invalid invitee rather than dropping
  it.
- The mock's calendars do not invite unless a test turns them on.

## Out of scope

The event pane (it already shows the people); maild; every file not
under `touch`.

## Done when

`make accept T=0159` and `make ui-check` pass, and only the files under
`touch` changed.
