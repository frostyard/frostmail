---
id: "0126"
title: Forward as Attachment and Redirect in the list's menu
milestone: M5
size: M
touch:
  - app/src/features/compose/RedirectSheet.tsx
  - app/src/app/ListContainer.tsx
  - app/src/app/commands.ts
given:
  - app/src/features/compose/RedirectSheet.test.tsx
  - app/src/app/Redirect.test.tsx
  - app/src/app/ListMenu.test.tsx # T-0118's, with the two items after Forward
acceptance: make ui-vitest F="src/features/compose src/app/Redirect src/app/ListMenu src/app/commands"
---
# T-0126: Forward as Attachment and Redirect in the list's menu

## Goal

Two more ways to pass a message on, as in Mail.app: Forward as Attachment
(a forward with the message attached whole) and Redirect (the message
sent on as it is, from a small sheet that asks whom to).

## Read first

- `docs/specs/ui.md`: "Behavior", the context menu's first group and the
  Redirect sheet bullet; `docs/design/send.md`: "Replies and forwards".
- `app/src/rpc/gen/api.ts`: `DraftKind` (`attached`),
  `message.redirect`, `address.suggest`.
- `app/src/app/ListContainer.tsx` (`menuItems`, `onMenuSelect`, the
  reminder `TimeSheet`), `app/src/app/commands.ts` (`ComposeAction`,
  `compose`), `app/src/features/compose/RecipientField.tsx`,
  `app/src/features/later/TimeSheet.tsx` (the sheet frame and its Tab
  loop), `app/src/lib/addressParse.ts` (`isValidAddress`).
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`commands.ts`**: `ComposeAction` gains `"forwardAttachment"`, which
  `compose` starts as a draft of kind `attached` of the last message.
- **`RedirectSheet`** (`RedirectSheet.tsx`), props `subject`,
  `suggest(prefix)`, `onRedirect(to: Address[]): Promise<void>`,
  `onCancel()`:
  - A dialog named "Redirect" (TimeSheet's frame, `w-[420px]`), the
    subject under the title ("(no subject)" when blank), a
    `RecipientField` labeled "To" whose input has focus when the sheet
    opens, Cancel, and Redirect.
  - Redirect is disabled until there is an address and every address is
    valid, and while `onRedirect` is pending; it calls `onRedirect` with
    the addresses. A rejection shows its message in a `role="alert"`
    paragraph under the field and the sheet stays.
  - Escape cancels, except while the field's suggestion list is open
    (the input's `aria-expanded` is `"true"`), when it only closes the
    list. Key events stop at the dialog; Tab loops inside it.
- **`ListContainer`**:
  - After Forward: "Forward as Attachment" (enabled on a read-only
    account) and "Redirect…" (disabled on one), no shortcuts.
  - Forward as Attachment runs `compose(client, "forwardAttachment",
    ids)`.
  - Redirect… opens the sheet for the last message of the menu's set
    (its subject); `onRedirect` calls `message.redirect {id, to}` and
    closes the sheet once it resolves. Suggestions come from
    `address.suggest {prefix, limit: 8}`.

## Tests (given, do not edit)

`RedirectSheet.test.tsx`, `Redirect.test.tsx`, and `ListMenu.test.tsx`
(T-0118's with the two items). Every other app test must keep passing.

## Gotchas

- The context menu's `onClose` focuses the list; open the sheet from
  `onSelect` and it still gets focus, since it focuses its input in an
  effect. Restore focus to what had it when the sheet closes.
- `RecipientField` turns typed text into tokens on Enter; an invalid
  token (`bob`) keeps Redirect disabled.

## Out of scope

Shortcuts for these items (a later card), the reader, maild, the mock,
and every file not under `touch`.

## Done when

`make accept T=0126` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
