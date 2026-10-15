---
id: "0156"
title: Asking before invitees hear
milestone: M5.5
size: M
touch:
  - app/src/lib/notify.ts
  - app/src/features/calendar/NotifySheet.tsx
given:
  - app/src/lib/notify.test.ts
  - app/src/features/calendar/NotifySheet.test.tsx
acceptance: make ui-vitest F="src/lib/notify src/features/calendar/NotifySheet"
---
# T-0156: Asking before invitees hear

## Goal

maild refuses a calendar write that would tell invitees unless the call
says `notify: true`: the error is `notifyRequired`, and its data is an
`InviteNotice` saying what goes out (invitations, updates or
cancellations) and to how many (docs/specs/pim-ui.md, "Sending to
invitees"; ADR-0035). This card makes the pieces the app asks with: a
helper that reads the notice and repeats the write after a yes, and the
notify sheet. Later cards use them in the editor, the views and undo.

## Read first

- `docs/specs/pim-ui.md`: "Sending to invitees (`NotifySheet`)".
- `docs/adr/0035-invite-through-the-providers-scheduling.md`: why there
  is no Don't Send.
- `schema/rpc/errors.yaml` (`notifyRequired`) and `schema/rpc/calendar.yaml`
  (`InviteNotice`, `NoticeKind`, `notify`); `app/src/rpc/gen/api.ts`
  (`ErrorCode`, `InviteNotice`, `NoticeKindValues`).
- `app/src/rpc/transport.ts`: `RPCError` and its `data`.
- `app/src/features/calendar/ScopeSheet.tsx`: the dialog this sheet
  matches (frame, focus, Tab, Escape, keys kept from the window).
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

### `lib/notify.ts` (new)

- `noticeOf(err: unknown): InviteNotice | null`: for an `RPCError` whose
  `code` is `ErrorCode.notifyRequired` and whose `data` has a `kind`
  among `NoticeKindValues` and a `count` that is a whole number of at
  least 1, `{kind, count}` (nothing else from `data`); otherwise `null`.
- `noticeTitle(kind)`: "Send Invitations?", "Send Updates?" or "Send
  Cancellations?".
- `noticeText(count)`: "The invitee will get it by email." for 1, else
  "<count> invitees will get it by email."
- `withNotify<T>(send: (notify?: true) => Promise<T>, ask: (notice) =>
  Promise<boolean>): Promise<T | null>`: calls `send(undefined)`. When it
  rejects with a notice (`noticeOf`), awaits `ask(notice)`: true returns
  `send(true)`, false returns `null` without sending again. Any other
  rejection, from either call, passes through unchanged.

### `NotifySheet` (new, `features/calendar/NotifySheet.tsx`)

- Props: `notice: InviteNotice`, `onSend()`, `onCancel()`.
- A modal `role="dialog"` (`aria-modal="true"`), 420 wide, over a dimmed
  window, as `ScopeSheet` is. Its title, which is its accessible name, is
  `noticeTitle(notice.kind)`, and under it the text
  `noticeText(notice.count)`.
- Buttons at the right, in order: **Cancel**, then **Send** (primary,
  focused when the sheet opens). They call `onCancel` and `onSend`.
  Escape cancels; Tab and Shift+Tab move between the two buttons and stay
  in the sheet; key events do not reach the window. Focus returns where
  it was when the sheet closes.
- `useNotify()` returns `{ask, sheet}`: `ask(notice)` shows the sheet and
  returns a promise that settles with true for Send and false for Cancel
  or Escape, then hides it. `sheet` is the element the caller renders
  (`null` while nothing is asked).

## Tests (given, do not edit)

`app/src/lib/notify.test.ts` and
`app/src/features/calendar/NotifySheet.test.tsx`.

## Gotchas

- `ErrorCode` is a generated `const` object, not a TypeScript `enum`.
- A `<output>` element's role is `status`, which the hook's test reads.

## Out of scope

Calling `withNotify` from the editor, the views or undo (T-0159); every
file not under `touch`.

## Done when

`make accept T=0156` and `make ui-check` pass, and only the files under
`touch` changed.
