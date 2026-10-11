---
id: "0121"
title: Drag messages to a mailbox
milestone: M5
size: M
touch:
  - app/src/features/list/MessageRow.tsx
  - app/src/app/ListContainer.tsx
  - app/src/features/sidebar/Sidebar.tsx
  - app/src/app/SidebarContainer.tsx
  - app/src/app/commands.ts
given:
  - app/src/features/list/MessageRow.drag.test.tsx
  - app/src/features/sidebar/Sidebar.drop.test.tsx
  - app/src/app/DragDrop.test.tsx
acceptance: make ui-vitest F="src/features/list src/features/sidebar src/app/DragDrop src/app/ListMenu"
---
# T-0121: Drag messages to a mailbox

## Goal

Drag messages from the list onto a mailbox in the sidebar to move them
there, or hold Alt (or Ctrl) to copy them, as in Mail.app.

## Read first

- `docs/specs/ui.md`: "Sidebar", the Drag and drop bullet.
- `app/src/features/list/MessageRow.tsx`, `app/src/app/ListContainer.tsx`
  (`selected`, `selectedSummaries`), `app/src/features/sidebar/Sidebar.tsx`
  (`Row`, the tree's handlers), `app/src/app/SidebarContainer.tsx`,
  `app/src/app/commands.ts` (`moveMessages`, `copyMessages`).
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`commands.ts`** exports `DRAG_TYPE = "application/x-frostmail-messages"`.
- **`MessageRow`**: an optional prop `onDragStart(id, event)`; the row is
  `draggable` exactly when it is given, and its `dragstart` calls it.
- **`ListContainer`**: passes `onDragStart`, which puts in the event's
  `dataTransfer` under `DRAG_TYPE` the JSON `{ accountId, ids }`: the
  selection when the row is in it, else the row; `accountId` is the first
  message's. `effectAllowed` is `"copyMove"`.
- **`Sidebar`**: optional props `canDrop(key)`, `onDrop(key, data, copy)`
  and `dragType`. `dragover` on a row that `canDrop` takes, while the drag
  carries `dragType` (in `dataTransfer.types`), calls `preventDefault`,
  sets `dropEffect` (`"copy"` or `"move"`) and draws that row with
  `bg-selection-inactive` (not when it is the selected row); `dragleave`
  and `drop` clear that. `drop` on such a row calls `onDrop(key,
  dataTransfer.getData(dragType), copy)`, where `copy` is true exactly
  when Alt or Ctrl is held. Other rows and other drags do nothing.
- **`SidebarContainer`**: `canDrop` takes `mailbox:<id>` and
  `favorite:<id>` rows; `onDrop` parses the JSON and, when the target
  mailbox is of the messages' account and that account is writable, calls
  `copyMessages` (copy) or `moveMessages` (with the list's source), else
  does nothing.

## Tests (given, do not edit)

`MessageRow.drag.test.tsx`, `Sidebar.drop.test.tsx`, `DragDrop.test.tsx`.
Every other app test must keep passing.

## Gotchas

- `event.altKey` and `event.ctrlKey` may be `undefined` on a drag event in
  the test DOM: compare with `=== true`.
- `getData` is only readable on `drop`; during `dragover` look at `types`.

## Out of scope

Dragging mailboxes, maild, the mock, and every file not under `touch`.

## Done when

`make accept T=0121` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
