---
id: "0089"
title: Show and answer invitations in the reader
milestone: M4.5
size: L
touch:
  - app/src/features/reader/InvitationCard.tsx
  - app/src/app/ReaderContainer.tsx
  - app/src/rpc/mock/calendar.ts
  - app/src/rpc/mock/fixture.ts
  - app/src/main.tsx
given:
  - app/src/features/reader/InvitationCard.test.tsx
  - app/src/app/Invitation.test.tsx
  - app/src/rpc/mock/invitation.test.ts
acceptance: make ui-vitest F="src/features/reader src/app/Invitation.test.tsx src/rpc/mock"
---
# T-0089: Show and answer invitations in the reader

## Goal

An invitation in mail shows as a card above the message, with Accept,
Maybe and Decline (ADR-0019). maild's `calendar.invitation` and
`calendar.respond` are built; this card builds the card, wires it into
the reader, and teaches MockTransport both methods.

## Read first

- `docs/specs/pim-ui.md`: "Invitation card" (the look and behavior), and
  "Rules".
- `schema/rpc/calendar.yaml` (`Invitation`, `invitation`, `respond`).
- The stub `app/src/features/reader/InvitationCard.tsx`: keep its exported
  names and props.
- `app/src/app/ReaderContainer.tsx` (`ConversationMessage`: where the
  card goes, `AttachmentStrip`), `app/src/features/reader/MessageHeader.tsx`.
- `app/src/lib/eventText.ts` (`dateText`, `timeRange`),
  `app/src/app/useCalendar.ts` (`useCalendarFrame`, `openOccurrence`, the
  `calendar.changed` refresh), `app/src/data/stores.ts`
  (`selectOccurrence`, `setModule`).
- `app/src/rpc/mock/calendar.ts` (`MockCalendar`: events, occurrences,
  `dispatch`) and `fixture.ts` (`fixtureCalendar`, event 303 "Lunch with
  Ann", the `reminders` option to copy), `app/src/main.tsx`.
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

- **invitationPart:** the first part whose `contentType` is
  `text/calendar` or `application/ics`, else the first whose filename
  ends in `.ics` (any case).
- **InvitationCard** (spec "Invitation card"): a `section` named
  "Invitation" (`aria-label`). The date tile's month is the short month
  in upper case as text (`toUpperCase`, not CSS), the day its own
  element. The title is its own element. The when line is one element
  with the whole text (`dateText` + " · " + `timeRange`, or + "All
  day"); the where-and-who line, the conflicts line, the adjacent line
  and the status line are each one element with their whole text (an
  icon beside the text is fine). The badge is its own element
  (`text-flag-1` for Cancelled). The answers are a `div` with
  `role="group"` and `aria-label="Answer"` holding `button`s named Accept,
  Maybe and Decline with `aria-pressed` (the pressed one `bg-accent
  text-accent-contrast`), all `disabled` while `busy`; they call
  `onAnswer` with `accepted`, `tentative`, `declined`. A reply's line is
  "<name or address> accepted|declined|said maybe". "Show in Calendar"
  is a `button` calling `onShowInCalendar`.
- **ReaderContainer:** in `ConversationMessage`, once the message is
  loaded and `invitationPart(message.parts)` finds a part, call
  `calendar.invitation({ messageId })`; show the card between the header
  and the body when it answers (nothing on error), and leave that part
  out of the `AttachmentStrip` while the card shows. Refetch it 100 ms
  after a `calendar.changed` event and after an answer. Answering calls
  `calendar.respond({ messageId, answer })` with `busy` set until it
  settles (log failures with `console.warn`). Show in Calendar selects
  the occurrence (`selectOccurrence({ eventId, recurrenceId: "" }, day)`
  when `eventId` is set, else just the date) and shows Calendar; the day
  is the event's `startDate` when all-day, else its start's date in the
  app's zone (`zoned`). Messages without such a part never call
  `calendar.invitation`.
- **MockTransport** (`calendar.ts`, `fixture.ts`):
  - `FixtureOptions.invitation` (default false) adds a message to the
    Inbox: from Ann Smith (`ann.smith@northwind.test`), subject
    "Invitation: Lunch with Ann", 5 minutes before `now`, unread, a short
    text body, and a part `{ path: "2", contentType: "text/calendar",
    filename: "invite.ics", disposition: "attachment", contentId: "",
    size: 900 }`; and `MockCalendarData.invitations` gains `{ messageId,
    eventId: 303 }` for it (`MockInvitation`, exported).
  - `calendar.invitation({ messageId })`: for a listed message, method
    `request`, `event` a copy of the stored event with `id` 0, `eventId`,
    `from` its organizer, `answer` the event's current answer,
    `canRespond` true unless the event is read-only, `outdated` false,
    `conflicts` the other timed occurrences of the event's UTC day that
    overlap it, are not transparent and not declined, and `adjacent` the
    one ending last at or before its start and the one starting first at
    or after its end that day. Any other message: `notFound`.
  - `calendar.respond({ messageId | eventId, answer })`: `answer` must be
    accepted, declined or tentative and one of the IDs given
    (`invalidParams`); an unknown event or message without an invitation
    is `notFound`. It sets the event's `answer` and the user's attendee
    answer, emits `calendar.changed` (`{ accountId }`), and returns the
    event (as `calendar.event` would).
  - `main.tsx`'s mock passes `invitation: true`.

## Tests (given, do not edit)

`app/src/features/reader/InvitationCard.test.tsx`,
`app/src/app/Invitation.test.tsx` and `app/src/rpc/mock/invitation.test.ts`.
The other tests must keep passing (the invitation message is opt-in so
their fixtures do not change).

## Gotchas

- `getByText` matches an element's own text: keep each tested line one
  element's text.
- `formatRange` puts thin spaces around its dash; the tests normalize
  them, don't replace them.
- Keep files under ~250 lines; split helpers inside the touched files.

## Out of scope

maild's side (done), the reminder window, and every file not under
`touch`.

## Done when

`make accept T=0089` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
