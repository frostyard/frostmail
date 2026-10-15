---
id: "0157"
title: MockTransport's invitees and notifyRequired
milestone: M5.5
size: M
touch:
  - app/src/rpc/mock/calendarEdits.ts
  - app/src/rpc/mock/calendar.ts
  - app/src/rpc/mock/mock.ts
given:
  - app/src/rpc/mock/calendar.invite.test.ts
acceptance: make ui-vitest F="src/rpc/mock src/app/Calendar src/app/EventWindow src/app/Undo"
---
# T-0157: MockTransport's invitees and notifyRequired

## Goal

maild now takes invitees on events the user organizes, in calendars
whose server tells them (`Collection.invites`), and refuses a write that
would tell invitees unless it says `notify: true` (ADR-0035;
docs/design/pim.md, "Invitations as organizer"). MockTransport, which
the app's tests and the browser preview run against, does the same, so
the cards after this one can test the Invitees field and the
confirmations.

## Read first

- `docs/adr/0035-invite-through-the-providers-scheduling.md`: the
  decision, and what tells whom.
- `docs/design/pim.md`: "Invitations as organizer" (Editing).
- `schema/rpc/calendar.yaml`: `Invitee`, `EventFields.attendees`,
  `InviteNotice`, `NoticeKind`, `notify` on `create`, `update`, `delete`
  and `restore`; `schema/rpc/errors.yaml`: `notifyRequired`;
  `schema/rpc/account.yaml`: `Collection.invites`.
- `app/src/rpc/mock/calendarEdits.ts` (`MockEventEdits`, `fields`,
  `patch`, `keep`), `app/src/rpc/mock/calendar.ts` (`MockCalendar`),
  `app/src/rpc/mock/mock.ts` (where `MockCalendar` is made, and the
  accounts), `app/src/rpc/mock/fixture.ts` (event 302, the calendars,
  the user).
- The given test, and `calendar.edit.test.ts` for the harness.
- `docs/tasks/EXECUTOR.md`

## Contract

- **Fields:** `fields` accepts `attendees`, an array of objects with an
  `email` string and an optional `name` string and `role` (`"required"`
  or `"optional"`); anything else is `invalidParams`.
- **Invitees** (create and update, when `attendees` is given), for an
  event the user organizes (no organizer, or one with `isUser`):
  - Addresses are trimmed and lowercased; one that is not an address
    (an `@` with text around it and a dot after, no spaces, `,`, `;`,
    `:`, `<` or `>`) is `invalidParams`. The user's own address (the
    calendar's account's `email`) and repeats are left out.
  - The event's attendees become the user's own lines (`isUser`), as
    they were, then the invitees in the order given: `{email, name,
    role, answer, isUser: false}`. An invitee who was already invited
    keeps their `answer` (and their `name` when none is given); a new
    one has `answer: "needsaction"`, `role` defaulting to `"required"`,
    `name` to `""`.
  - With invitees and no organizer, the event gets the user as
    organizer: `{email, name: displayName, role: "chair", answer:
    "accepted", isUser: true}` from the calendar's account.
  - Invitees in a calendar whose `invites` is false are `invalidParams`;
    an empty list is fine anywhere. Invitations (the user is not the
    organizer) stay alarms-only, so `attendees` is `invalidParams` there.
- **Moves:** in a calendar that invites, `calendarId` for another
  calendar is `invalidParams` when the event has invitees or the update
  gives some.
- **Notices.** In a calendar that invites, each write works out what it
  tells invitees, comparing the stored events it replaces (before) with
  the ones it leaves (after), counting only events the user organizes
  and only attendees without `isUser`:
  - nothing after, or a delete of any scope: `cancellations` to the
    invitees before;
  - nothing before (a create, or a restore that brings a deleted event
    back): `invitations` to the invitees after;
  - otherwise, when what invitees see changed (`summary`, `location`,
    `description`, the times, `recurrence`, `status`, an occurrence's
    such fields, an excluded occurrence, or a split): `updates` to the
    invitees before and those added;
  - else invitees added and none removed: `invitations` to those added;
    removed and none added: `cancellations` to those removed; both:
    `updates` to those added and removed;
  - no count, no notice.
  - A restore compares the stored events now with what it puts back.
- **Asking.** With a notice, a write without `notify` throws
  `RPCError(ErrorCode.notifyRequired, message, notice)` (the notice as
  the error's `data`, `{kind, count}`), and `notify: false` throws
  `invalidParams`; only `notify: true` writes. Nothing is written or
  announced before that, so a refused restore can be used again.
  Without a notice, `notify` is ignored.
- `calendar.restore` takes its whole params (`restoreId` and `notify`).

## Tests (given, do not edit)

`app/src/rpc/mock/calendar.invite.test.ts`; the existing mock, Calendar,
EventWindow and Undo tests must still pass.

## Gotchas

- The fixture's calendars have `invites: false`, as on an IMAP
  account, so the earlier tests that edit event 302 (which has
  invitees) write without asking. The given test turns Work and Home on.
- `MockCalendar` does not know the accounts today: give it a way to
  read them from `MockTransport`, keeping its other callers working.
- Check for the notice after building the changed copy and before
  `keep` and the change to `data.events`.

## Out of scope

The app's Invitees field and confirmations (later cards); maild; every
file not under `touch`.

## Done when

`make accept T=0157` and `make ui-check` pass, and only the files under
`touch` changed.
