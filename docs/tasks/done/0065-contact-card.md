---
id: "0065"
title: Open a contact card from names in the reader
milestone: M4.5
size: M
touch:
  - app/src/features/people/ContactPopover.tsx
  - app/src/features/reader/MessageHeader.tsx
  - app/src/app/ContactCardContainer.tsx
  - app/src/app/ReaderContainer.tsx
given:
  - app/src/features/people/ContactPopover.test.tsx
  - app/src/features/reader/MessageHeader.addresses.test.tsx
  - app/src/app/ContactCard.test.tsx
acceptance: make ui-vitest F="src/features/people/ContactPopover.test.tsx src/features/reader src/app/ContactCard.test.tsx"
---
# T-0065: Open a contact card from names in the reader

## Goal

Outlook's contact card, in Mail.app's look: clicking a sender's or
recipient's name in the reader shows who they are, the mail with them,
and what to do next: write to them, add them to contacts, or open them in
People (ADR-0020).

## Read first

- `docs/specs/pim-ui.md`: "Contact card", and "Person pane" (the Recent
  Mail rows the card repeats).
- The stubs: `app/src/features/people/ContactPopover.tsx` (props,
  `AddState`), `app/src/app/ContactCardContainer.tsx`.
- `app/src/features/reader/MessageHeader.tsx` (`formatAddressList`, the
  sender line), `app/src/app/ReaderContainer.tsx` (where headers render).
- `app/src/features/people/Avatar.tsx`, `PersonPane.tsx` (Recent Mail),
  `app/src/lib/format.ts`.
- `app/src/data/stores.ts`: `setModule`, `setPeopleBook`,
  `clearPeopleSearch`, `selectPerson`; `app/src/app/compose.ts`
  (`composeTo`); `app/src/app/openMessage.ts` (`revealMessage`);
  `app/src/app/usePeople.ts` (how People loads cards and photos).
- `app/src/rpc/mock/people.ts`: `people.card` and `people.add` in the mock.
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

- **`MessageHeader`** gains `onAddress?: (address: Address, at: { x:
  number; y: number }) => void`. Without it the header is unchanged. With
  it, the sender's name is a `button type="button"` (keeping
  `text-reader-sender`, plain text look) whose accessible name is the
  address (`aria-label`) and whose text is `displayName(from)`; the To and
  Cc lines are `"To: "` / `"Cc: "` followed by a button per address for the
  first three, separated by `", "` text, then `" & N more"` text when there
  are more (the same text `formatAddressList` makes). A click calls
  `onAddress(address, { x: rect.left, y: rect.bottom })` with the button's
  `getBoundingClientRect()`.
- **`ContactPopover`**: a `role="dialog"` `div`, `w-[320px]`, `fixed` at
  `style.left`/`style.top` from `at` (moved left so it stays inside the
  window when `at.x + 320` would pass `window.innerWidth`), `bg-window`,
  1px `border-separator`, rounded 10, a shadow, 16px padding. Its
  accessible name (`aria-label`) and heading are the card's `name`, else
  the address's name, else the address. Under the name, the address
  (`text-secondary`) and the person's organization when there is one; a
  48px `Avatar` with `photo`. Buttons: **Message** (`onCompose`); **Add to
  Contacts** while `card.canAdd` and `add` is `"idle"` or an error
  (`onAdd`); `Adding…` text while adding and `Added` text once added,
  instead of the button; an error as a `p role="alert"` with its message;
  **Open in People** when `card.person` (`onOpenPerson(person.id)`). A
  "Recent Mail" `section` (labeled, `h3`) with a `button` per message as in
  `PersonPane` (`onOpenMessage(id)`), absent when there is none or the card
  is pending. Escape (a `keydown` on `document`) and a `mousedown` outside
  the dialog call `onClose`; a `mousedown` inside does not.
- **`ContactCardContainer`**: on mount, `people.card({ email: address })`
  and, when the person `hasPhoto`, `people.photo`; renders `ContactPopover`
  with `now = new Date()`.
  - Message: `composeTo(client, address, name)` without awaiting the window
    (failures to `console.warn`), then `onClose`.
  - Add to Contacts: `people.add({ email, name })` with the card's name, else
    the address's name, left out when empty; `adding`, then `added` and a
    fresh `people.card`, or `{ error: message }`.
  - Open in People: `setModule("people")`, `setPeopleBook("all")`,
    `clearPeopleSearch()`, `selectPerson(id)`, `onClose`.
  - A recent message: `revealMessage(client, id)` and `onClose`.
- **`ReaderContainer`** passes `onAddress` to each `MessageHeader` and
  renders one `ContactCardContainer` at a time (opening another replaces
  it; keyed by the address so its state resets).

## Tests (given, do not edit)

`app/src/features/people/ContactPopover.test.tsx`,
`app/src/features/reader/MessageHeader.addresses.test.tsx`,
`app/src/app/ContactCard.test.tsx` (through `MainWindow` and
`MockTransport`). The existing `MessageHeader.test.tsx` must keep passing.

## Gotchas

- happy-dom's `getBoundingClientRect` is all zeros: the tests expect
  `{ x: 0, y: 0 }`.
- The reader shows a whole conversation, so several headers may carry the
  same address; each opens the card for its own button.
- Do not let the card's Escape also clear Mail's search: the card handles
  Escape only while it is open.

## Out of scope

Upcoming events (Calendar, Phase 3), names in the message list and
compose, editing contacts, and every file not under `touch`.

## Done when

`make accept T=0065` and `make ui-check` pass (taskrun runs them), and only
the files under `touch` changed.
