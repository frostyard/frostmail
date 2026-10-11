---
id: "0108"
title: Smart mailboxes in the sidebar, their sheet, and Save from search
milestone: M5
size: L
touch:
  - app/src/lib/mailboxTree.ts
  - app/src/data/stores.ts
  - app/src/features/sidebar/Sidebar.tsx
  - app/src/features/search/SearchField.tsx
  - app/src/features/organize/SmartSheet.tsx
  - app/src/app/SmartSheetContainer.tsx
  - app/src/app/useSmartCounts.ts
  - app/src/app/SidebarContainer.tsx
  - app/src/app/ToolbarContainer.tsx
  - app/src/app/MainWindow.tsx
given:
  - app/src/lib/mailboxTree.smart.test.ts
  - app/src/data/stores.smart.test.ts
  - app/src/features/sidebar/Sidebar.smart.test.tsx
  - app/src/features/search/ScopeBar.save.test.tsx
  - app/src/features/organize/SmartSheet.test.tsx
  - app/src/app/SmartMailboxes.test.tsx
acceptance: make ui-vitest F="src/lib/mailboxTree.smart src/data/stores.smart src/features/sidebar/Sidebar.smart src/features/search/ScopeBar.save src/features/organize/SmartSheet src/app/SmartMailboxes"
---
# T-0108: Smart mailboxes in the sidebar, their sheet, and Save from search

## Goal

Smart mailboxes appear in their own sidebar section with unread counts,
are made and edited in a modal sheet built on T-0107's condition editor,
deleted from a row's menu, and can be saved from a search. Selecting one
lists it through `ViewQuery.smartMailboxId`.

## Read first

- `docs/specs/ui.md`: "Sidebar" (Smart Mailboxes) and "Behavior"
  (Search, the Save button).
- `docs/specs/organize-ui.md`: "The smart mailbox sheet".
- `docs/design/organize.md`: "Smart mailboxes".
- `app/src/features/organize/ConditionEditor.tsx` and
  `app/src/lib/conditions.ts` (T-0107): `newCondition`.
- `app/src/lib/mailboxTree.ts` (`buildSidebar`, `SidebarExtras`,
  `SidebarSection`), `app/src/data/stores.ts` (`Source`, `sourceKey`,
  `sourceFromKey`, `sourceQuery`, `useUI`, `useMail`'s `smarts`).
- `app/src/features/sidebar/Sidebar.tsx`, `app/src/app/SidebarContainer.tsx`,
  `app/src/app/useSidebarCounts.ts` (the counting pattern).
- `app/src/features/search/SearchField.tsx` (`ScopeBar`),
  `app/src/app/MainWindow.tsx` (`MailScope`), `ToolbarContainer.tsx`
  (`mailTitle`).
- `app/src/features/menu/ContextMenu.tsx`; `features/settings/labels.ts`.
- `app/src/rpc/gen/api.ts`: `SmartMailbox`, `client.smart.*`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`lib/mailboxTree.ts`**: `SidebarIcon` gains `"folder-cog"`;
  `SidebarSection` gains `addLabel?: string`; `SidebarExtras` gains
  `smarts?: SmartMailbox[]` and `smartCounts?: Record<number, ViewCount>`.
  Only when `smarts` is given (even empty), `buildSidebar` puts after
  Favorites a section `{ key: "smart", title: "Smart Mailboxes", addLabel:
  "New Smart Mailbox" }` with a row per smart mailbox, in order:
  `{ key: "smart:<id>", label: name, icon: "folder-cog", depth: 0, unread:
  smartCounts[id]?.unread ?? 0, selectable: true }`. Without `smarts` the
  sections are as before.
- **`data/stores.ts`**: `Source` gains `{ kind: "smart"; id: number }`
  (`smart:<id>` both ways; `sourceQuery` `{ smartMailboxId: id }` plus
  `threads` as the others). `UIState` gains `smartSheet: SmartSheetState
  | null` (`null` to start, not persisted) with
  `type SmartSheetState = { mode: "new"; initial?: SmartDraft } |
  { mode: "edit"; id: number }`, and actions `openSmartSheet(state)` and
  `closeSmartSheet()`.
- **`features/organize/SmartSheet.tsx`** (new): `interface SmartDraft {
  name: string; conditions: Conditions; includeTrash: boolean;
  includeSent: boolean }`, `newSmartDraft()` (the spec's start), and
  `SmartSheet(props: SmartSheetProps)` with `{ title; initial: SmartDraft;
  accounts; mailboxes; flagNames?; today; busy?; error?; onSave(draft);
  onCancel() }`, drawn as the spec says: a `role="dialog"`
  `aria-modal="true"` named by its `h2` title; "Smart Mailbox Name:"
  (`maxLength` 100); the `ConditionEditor`; the two checkboxes; maild's
  error in `role="alert"`; Cancel and OK. OK calls `onSave` with the draft,
  the name trimmed, and is disabled while the trimmed name is empty or
  `busy`. Cancel and Escape call `onCancel`.
- **`app/SmartSheetContainer.tsx`** (new): renders the sheet `useUI`'s
  `smartSheet` asks for (nothing for `null`, or for an `edit` of a smart
  mailbox not in `useMail`'s `smarts`); "New Smart Mailbox" or "Edit Smart
  Mailbox"; `today` as the local `YYYY-MM-DD`; OK calls `smart.create` or
  `smart.update` with the draft, then closes, and after a create makes
  `{ kind: "smart", id }` the source; a failure stays open with its
  message as `error`.
- **`app/useSmartCounts.ts`** (new): `useSmartCounts(smarts):
  Record<number, ViewCount>`, one `view.count` of `{ smartMailboxId }` per
  smart mailbox when mounted, when the list's IDs change, and 300 ms after
  the last `mailbox.changed`, `vip.changed`, `settings.changed` or
  `smart.changed`.
- **`Sidebar.tsx`**: the `folder-cog` icon (Lucide `FolderCog`); props
  `onAdd?(sectionKey)` and `onContextMenu?(key, x, y)`. A section with
  `addLabel` (and `onAdd`) shows a 14px `Plus` button named `addLabel`
  beside its header's toggle button (not inside it: a button may not hold
  a button), visible on hover and focus. A right click on a row calls
  `onContextMenu` with its key and the pointer's position.
- **`SidebarContainer`**: passes `smarts` and `useSmartCounts(smarts)`;
  `onAdd("smart")` opens the sheet `{ mode: "new" }`; a right click on a
  `smart:<id>` row opens a `ContextMenu` with "Edit Smart Mailbox…"
  (opens `{ mode: "edit", id }`) and "Delete Smart Mailbox"
  (`smart.delete`, and All Inboxes becomes the source when it was that
  one).
- **`ScopeBar`**: `onSave?: () => void`; with it, a button "Save" named
  "Save as Smart Mailbox", `text-accent`, at the bar's right.
- **`MailScope`**: passes `onSave`, which calls `smart.fromSearch({ text })`
  (plus `mailboxId` when the scope is the current source and it is a
  mailbox) and opens `{ mode: "new", initial: { name: <the search>,
  conditions, includeTrash: true, includeSent: true } }`; its source label
  is the smart mailbox's name for a smart source. `MainWindow` renders
  `SmartSheetContainer`.
- **`mailTitle`**: a smart source's title is its name, its subtitle "N
  messages".

## Tests (given, do not edit)

`mailboxTree.smart.test.ts`, `stores.smart.test.ts`,
`Sidebar.smart.test.tsx`, `ScopeBar.save.test.tsx`, `SmartSheet.test.tsx`
and `SmartMailboxes.test.tsx` (the window against the mock maild). Every
other app test must keep passing, `mailboxTree.test.ts` and
`Sidebar.test.tsx` included.

## Gotchas

- `stores.ts` may import the `SmartDraft` type from `SmartSheet.tsx`
  (`import type`); keep `SmartSheet.tsx` free of store imports.
- Give `SmartSheet` a `key` per sheet in the container, so an edit starts
  from that smart mailbox's values.
- `ui.search` and `ui.searchScope` are what Save sends; the mock's
  `smart.fromSearch` makes `content contains` conditions.
- `useSidebarCounts` stays as it is (its given tests pin its output); the
  smart mailboxes' counts are `useSmartCounts`'.

## Out of scope

maild, the mock, the filter bar's More (T-0109), the General pane's smart
scope (T-0110), and every file not under `touch`.

## Done when

`make accept T=0108` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
