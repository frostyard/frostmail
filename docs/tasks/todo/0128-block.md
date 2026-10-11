---
id: "0128"
title: Block a sender from the contact card, and the blocked banner
milestone: M5
size: M
touch:
  - app/src/data/stores.ts
  - app/src/data/session.tsx
  - app/src/features/people/ContactPopover.tsx
  - app/src/app/ContactCardContainer.tsx
  - app/src/features/reader/MessageHeader.tsx
  - app/src/app/ReaderContainer.tsx
given:
  - app/src/features/people/ContactPopover.block.test.tsx
  - app/src/features/reader/BlockedBanner.test.tsx
  - app/src/app/Block.test.tsx
acceptance: make ui-vitest F="src/features/people src/features/reader src/app/Block src/app/ContactCard src/app/Unsubscribe src/app/RemindMe"
---
# T-0128: Block a sender from the contact card, and the blocked banner

## Goal

Block a sender, as in Mail.app: from the contact card, Block Contact;
their mail then says so in the reader, with Unblock. maild keeps the
list and keeps their mail from notifying.

## Read first

- `docs/specs/ui.md`: "Reader", the Blocked banner bullet;
  `docs/specs/pim-ui.md`: "Contact card" (the VIP button is the model);
  `docs/design/organize.md`: "Mute and block".
- `app/src/rpc/gen/api.ts`: `block.list`, `block.add`, `block.remove`,
  `BlockedSender`, the `block.changed` event.
- `app/src/data/stores.ts` and `app/src/data/session.tsx` (how `vips`
  loads and follows `vip.changed`), `app/src/app/ContactCardContainer.tsx`
  (`toggleVip`), `app/src/features/people/ContactPopover.tsx`,
  `app/src/features/reader/MessageHeader.tsx` (`ReminderBanner`) and
  `app/src/app/ReaderContainer.tsx` (`ConversationMessage`).
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **Store:** `useMail` gains `blocked: BlockedSender[]` (initially `[]`).
  The session loads it with `block.list` when it loads the VIPs, and
  again on `block.changed`; a failure leaves it as it was.
- **`ContactPopover`**: optional props `blocked`, `blockBusy`,
  `onBlock()`. With `onBlock`, a button after the VIP button, styled as
  it: "Block Contact", or "Unblock Contact" when `blocked`; disabled
  while `blockBusy`.
- **`ContactCardContainer`**: `blocked` is whether the store's list holds
  the card's address, lowercased. `onBlock` calls `block.add` or
  `block.remove` with `{addresses: [the card's address]}`, busy
  meanwhile; a failure only logs. The button follows the store.
- **`BlockedBanner`** (exported from `MessageHeader.tsx`), prop
  `onUnblock()`: a `role="status"` banner drawn as `ReminderBanner` is,
  "This message is from a blocked sender." and at the right a button
  "Unblock" calling `onUnblock`.
- **`ReaderContainer`**, per message: when the sender's address,
  lowercased, is in the store's blocked list, the banner after the
  reminder banner (and its time sheet), before the unsubscribe banner;
  Unblock calls `block.remove {addresses: [that address]}`.

## Tests (given, do not edit)

`ContactPopover.block.test.tsx`, `BlockedBanner.test.tsx`,
`Block.test.tsx`. Every other app test must keep passing, the VIP,
contact card and reader tests included.

## Gotchas

- A conversation can hold several messages from the blocked sender;
  each gets its banner.
- maild lowercases blocked addresses; the message's address may not be.

## Out of scope

The settings window's blocked list (a later card), maild, the mock, and
every file not under `touch`.

## Done when

`make accept T=0128` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
