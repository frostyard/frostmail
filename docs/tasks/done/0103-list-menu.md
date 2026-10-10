---
id: "0103"
title: Spam, Copy to and one-step Flag in the message list's menu
milestone: M4.5
size: M
touch:
  - app/src/app/commands.ts
  - app/src/lib/keymap.ts
  - app/src/app/MainWindow.tsx
  - app/src/app/ListContainer.tsx
given:
  - app/src/app/commands.menu.test.ts
  - app/src/lib/keymap.junk.test.ts
  - app/src/app/ListMenu.test.tsx
acceptance: make ui-vitest F="src/app/commands src/lib/keymap src/app/ListMenu"
---
# T-0103: Spam, Copy to and one-step Flag in the message list's menu

## Goal

The row context menu gains Mark as Spam (Not Spam in the junk mailbox),
Copy to (maild's new `message.copy`) and a one-step Flag, in the order the
spec gives. Ctrl+Shift+J marks the selection as spam. On a read-only
account only Reply, Reply All and Forward stay enabled.

## Read first

- `docs/specs/ui.md`: "Behavior", the "Context menu" bullet, and the
  keyboard map's Ctrl+Shift+J.
- `app/src/app/ListContainer.tsx`: `menuItems` and `onMenuSelect`.
- `app/src/features/menu/ContextMenu.tsx`: `MenuItem`.
- `app/src/app/commands.ts`: `archiveOf`, `moveMessages`, `toggleFlag`.
- `app/src/lib/keymap.ts` and `MainWindow.tsx`'s `mailCommand`.
- `app/src/rpc/gen/api.ts`: `client.message.copy`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`commands.ts`** exports:
  - `junkOf(accountId, mailboxes)`: the account's mailbox with role
    `junk`.
  - `interface SpamTarget { notSpam: boolean; to: Mailbox | undefined }`
    and `spamTarget(model, ids, mailboxes)`, where the account is the
    first loaded row's:
    - with no rows: `{ notSpam: false, to: undefined }`;
    - with every row in the account's junk mailbox: `{ notSpam: true, to:
      <the account's inbox> }`;
    - otherwise `{ notSpam: false, to: <junk, or undefined> }`.
  - `toggleSpam(client, model, ids, source, mailboxes)`:
    `moveMessages` to `spamTarget(...).to`.
  - `moveTargets(message | undefined, mailboxes)`: the message's account's
    mailboxes, in the given order, except those in its `mailboxIds`; `[]`
    without a message.
  - `copyTargets(message | undefined, mailboxes)`: `moveTargets`, but
    when the account has a mailbox with `label` (Gmail), only those with
    `label` whose role is not `flagged`.
  - `copyMessages(client, ids, to | undefined)`:
    `client.message.copy({ ids, mailboxId: to.id })`; nothing without `to`
    or ids.
- **`keymap.ts`:** a command `"junk"` bound to Ctrl+Shift+J (works in a
  text field, as every Ctrl+Shift binding does).
- **`mailCommand`:** `"junk"` runs `toggleSpam` on the selection with the
  store's `source`.
- **`ListContainer`'s `menuItems`:**
  - Item ids and shortcuts, in this order:
    - `reply` "Reply" Ctrl+R, `replyAll` "Reply All" Ctrl+Shift+R,
      `forward` "Forward" Ctrl+Shift+F;
    - separator;
    - `archive` "Archive" Ctrl+Alt+A (only when `archiveMailbox` finds
      one, as now), `delete` "Delete" Delete, `spam` "Mark as Spam" or,
      when `notSpam`, "Not Spam", Ctrl+Shift+J, disabled without a `to`;
    - separator;
    - submenu `move` "Move to" (items `move:<id>` labeled by path,
      `moveTargets` of the first row), submenu `copy` "Copy to" (items
      `copy:<id>`, `copyTargets`), each disabled when empty;
    - separator;
    - `toggleFlag` "Flag", or "Unflag" when the first row is flagged,
      Ctrl+Shift+L;
    - submenu `flagColor` "Flag Color": the 7 colors (`flag:1`…`flag:7`,
      the first row's checked), a separator, `flag:0` "Clear Flag"
      (disabled when nothing is flagged), as the old `flag` submenu was;
    - `read` "Mark as Read" or "Mark as Unread" Ctrl+Shift+U, as now.
  - When the first row's account is read-only (`useMail`'s `accounts`),
    every item and submenu after Forward is disabled.
  - `onMenuSelect` adds:
    - `spam`: `toggleSpam`;
    - `toggleFlag`: `toggleFlag`;
    - `copy:<id>`: `copyMessages` to that mailbox.

## Tests (given, do not edit)

`commands.menu.test.ts`, `keymap.junk.test.ts` and `ListMenu.test.tsx`
(the window against the mock maild). `commands.test.ts`,
`keymap.test.ts` and every other app test must keep passing.

## Gotchas

- `MenuItem` ids must be unique within a menu: the colors' submenu is now
  `flagColor`, and the toggle is `toggleFlag`.
- Cards T-0101 and T-0102 change other parts of `ListContainer.tsx` and
  `MainWindow.tsx` at the same time; touch only the menu, the command and
  the lines they need.

## Out of scope

maild, the mock, the toolbar's menus, and every file not under `touch`.

## Done when

`make accept T=0103` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
