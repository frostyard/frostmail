---
id: "0105"
title: The VIP star and Add to VIPs
milestone: M5
size: M
touch:
  - app/src/lib/vips.ts
  - app/src/features/list/MessageRow.tsx
  - app/src/app/ListContainer.tsx
  - app/src/features/reader/MessageHeader.tsx
  - app/src/app/ReaderContainer.tsx
  - app/src/features/people/ContactPopover.tsx
  - app/src/app/ContactCardContainer.tsx
given:
  - app/src/lib/vips.test.ts
  - app/src/features/list/MessageRow.vip.test.tsx
  - app/src/features/reader/MessageHeader.vip.test.tsx
  - app/src/features/people/ContactPopover.vip.test.tsx
  - app/src/app/VipStar.test.tsx
acceptance: make ui-vitest F="src/lib/vips src/features/list/MessageRow.vip src/features/reader/MessageHeader.vip src/features/people/ContactPopover.vip src/app/VipStar"
---
# T-0105: The VIP star and Add to VIPs

## Goal

A VIP sender's mail carries a star in the message list and in the
reader's header, and the contact card makes its address a VIP, or stops
it being one, the way Mail.app's star by a sender does. VIPs come from
`useMail`'s `vips`, which maild keeps and the store reloads on
`vip.changed`.

## Read first

- `docs/specs/ui.md`: "Message list" (Line 1) and "Reader" (Header).
- `docs/specs/pim-ui.md`: "Contact card" (Actions).
- `docs/design/organize.md`: "VIPs".
- `app/src/features/list/MessageRow.tsx` and `ListContainer.tsx` (where
  rows are made).
- `app/src/features/reader/MessageHeader.tsx` and `ReaderContainer.tsx`
  (`ConversationMessage`).
- `app/src/features/people/ContactPopover.tsx` and
  `app/src/app/ContactCardContainer.tsx`.
- `app/src/data/stores.ts`: `useMail`'s `vips`; `app/src/rpc/gen/api.ts`:
  `Vip`, `client.vip.add` and `remove`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`lib/vips.ts`** (new): `vipSet(vips: Vip[]): ReadonlySet<string>`, the
  addresses lowercased; `isVip(address: string, set): boolean`, comparing
  the address trimmed and lowercased.
- **`MessageRow`**: a prop `vip?: boolean`. When true, line 1 starts with a
  Lucide `Star`, size 10, filled (`fill-current`), `role="img"`,
  `aria-label="VIP"`, in `text-secondary`, or `text-accent-contrast` on a
  selected row in a focused list, before the sender's name.
- **`ListContainer`**: each row's `vip` is `isVip(row.from.address,
  vipSet(vips))`, the set built once per change of `useMail`'s `vips`.
- **`MessageHeader`**: a prop `vip?: boolean`. When true, a Lucide `Star`,
  size 12, filled, `role="img"`, `aria-label="VIP"`, in `text-secondary`,
  follows the sender's name (the button or the span), 4px from it, before
  the date.
- **`ReaderContainer`**: each message's header gets `vip` for its sender,
  from `useMail`'s `vips`.
- **`ContactPopover`**: props `vip?: boolean`, `vipBusy?: boolean` and
  `onVip?: () => void`. With `onVip`, the actions row ends with a button
  "Add to VIPs", or "Remove from VIPs" when `vip`, styled as Add to
  Contacts, disabled while `vipBusy`, calling `onVip`. Without `onVip`, no
  button.
- **`ContactCardContainer`**: `vip` is `isVip(address.address, ...)`;
  `onVip` calls `client.vip.remove` when `vip`, else `client.vip.add`, with
  `{ personId }` when the card has a person, else `{ addresses: [address] }`,
  and holds `vipBusy` until the call settles. The button turns over when
  the store's `vips` reload after `vip.changed`.

## Tests (given, do not edit)

`vips.test.ts`, `MessageRow.vip.test.tsx`, `MessageHeader.vip.test.tsx`,
`ContactPopover.vip.test.tsx` and `VipStar.test.tsx` (the window against the
mock maild). Every other app test must keep passing, including
`MessageRow.test.tsx`, `MessageHeader.test.tsx` and
`ContactPopover.test.tsx`.

## Gotchas

- The header's sender is `flex-1` today, which would push the star to the
  date: wrap the name and the star in one `flex-1` span.
- Lucide icons need `role="img"` to be found by their `aria-label`.
- Do not fetch `vip.list` in the components: `useMail` already holds it.

## Out of scope

maild, the mock, the sidebar's VIPs (T-0104), and every file not under
`touch`.

## Done when

`make accept T=0105` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
