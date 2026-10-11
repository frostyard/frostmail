---
id: "0119"
title: The sidebar's mailbox menu and the mailbox sheet
milestone: M5
size: L
touch:
  - app/src/features/sidebar/MailboxSheet.tsx
  - app/src/app/SidebarContainer.tsx
  - app/src/lib/mailboxTree.ts
given:
  - app/src/features/sidebar/MailboxSheet.test.tsx
  - app/src/lib/mailboxTree.mailboxes.test.ts
  - app/src/app/Mailboxes.test.tsx
acceptance: make ui-vitest F="src/features/sidebar src/lib/mailboxTree src/app/Mailboxes src/app/SmartMailboxes"
---
# T-0119: The sidebar's mailbox menu and the mailbox sheet

## Goal

Mail.app's mailbox commands from the sidebar: right click a mailbox to make
one inside it, rename, move or delete it, use it for a role, erase Trash or
Junk, or add it to Favorites. An account's header gets a New Mailbox
button.

## Read first

- `docs/specs/ui.md`: "Sidebar", the bullets Mailboxes and The mailbox
  sheet.
- `docs/design/organize.md`: "Mailboxes" (what maild does and refuses).
- `app/src/app/SidebarContainer.tsx` (the smart mailbox menu to keep),
  `app/src/features/sidebar/Sidebar.tsx` (`onAdd`, `onContextMenu`),
  `app/src/lib/mailboxTree.ts` (`buildSidebar`, sections' `addLabel`).
- `app/src/features/organize/SmartSheet.tsx`: the sheet frame to follow.
- `app/src/app/SettingsWindow.tsx`: `confirmRemove` (Tauri's `ask`, else
  `window.confirm`).
- `app/src/rpc/gen/api.ts`: `mailbox.*`, `Settings.favorites`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`buildSidebar`**: an account's section has `addLabel: "New Mailbox"`
  unless the account is read-only.
- **`MailboxSheet`** (`MailboxSheet.tsx`) exports `MailboxSheetMode`
  (`"new" | "rename" | "move"`), `MailboxChoice` (`{id, path}`),
  `MailboxSheetProps` (`mode`, `name`, `parentId?`, `locations`, `busy?`,
  `error?`, `onSave({name, parentId?})`, `onCancel`) and `MailboxSheet`:
  the spec's sheet, titled "New Mailbox", "Rename Mailbox" or "Move
  Mailbox". "Name:" labels a text input (not when moving), "Location:" a
  `select` with "Top Level" (`""`) then each location by path (not when
  renaming), starting at `parentId`. `onSave` gets the trimmed name and
  `parentId` only when a location other than Top Level is chosen (never
  when renaming). The first field has focus when it opens.
- **The container** (`SidebarContainer.tsx`):
  - A mailbox row's (`mailbox:<id>`) context menu as the spec lists it,
    with the item labels the given test uses.
  - The account header's add button opens the sheet to make a top-level
    mailbox.
  - Calls go out exactly as the spec writes them, with no `parentId` key
    for the top level.
  - Add to Favorites and Remove from Favorites call `settings.set
    {favorites}` with the mailbox appended or removed (the `favorites`
    setting, `[]` when absent).
  - The smart mailbox menu stays as it is.

## Tests (given, do not edit)

`MailboxSheet.test.tsx`, `mailboxTree.mailboxes.test.ts`, `Mailboxes.test.tsx`.
Every other app test must keep passing, `SmartMailboxes.test.tsx` included.

## Gotchas

- `Settings.favorites` is optional: an empty list is absent.
- The submenu "Use This Mailbox For" is a `submenu` `MenuItem`; its items
  use `checked` for the mailbox's role.
- A read-only account's items are disabled, not hidden (but Add to
  Favorites stays enabled: favorites are a preference, not a change on the
  server).

## Out of scope

The Favorites rows in the sidebar and their own menu (T-0120), drag and drop
(T-0121), maild, the mock, and every file not under `touch`.

## Done when

`make accept T=0119` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
