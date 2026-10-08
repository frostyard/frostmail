---
id: "0032"
title: Draw the toolbar and title bar
milestone: M2
size: M
touch:
  - app/src/features/toolbar/Toolbar.tsx
given:
  - app/src/features/toolbar/Toolbar.test.tsx
acceptance: make ui-vitest F=src/features/toolbar/Toolbar.test.tsx
---
# T-0032: Draw the toolbar and title bar

## Goal

The window has no system title bar; the toolbar is the title bar, split
into three segments aligned with the panes below it, with Mail.app's
actions, the search field and our own window controls (`docs/specs/ui.md`,
Layout: Toolbar). Implement `Toolbar`; the planner's container decides what
each command does.

## Read first

- `app/src/features/toolbar/Toolbar.tsx`: `ToolbarCommand`, `MoveTarget`,
  `ToolbarSelection`, `ToolbarProps`, the stub.
- `app/src/features/menu/ContextMenu.tsx` (T-0033): the menu used for Flag
  and Move.
- `app/src/lib/flags.ts`: `FLAG_NAMES`.
- `docs/specs/ui.md`, "Layout: Toolbar" and "Keyboard map".
- The given test `app/src/features/toolbar/Toolbar.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names; replace the stub's doc comment and the file's
"Task T-0032 …" sentence with a description of the toolbar.

**Root:** a `div` with `role="toolbar"`, `aria-label="Toolbar"`,
`data-tauri-drag-region`, classes `flex h-[52px] shrink-0 items-center
border-b border-separator bg-toolbar`. Its `onDoubleClick` calls
`onCommand({ kind: "toggleMaximize" })` only when the event target itself
has the `data-tauri-drag-region` attribute (empty space, not a button).

**Segments:** three direct children with `data-segment` = `"sidebar"`,
`"list"`, `"reader"`, each with `data-tauri-drag-region` and `flex h-full
items-center`:

- `sidebar` (`gap-1 px-2 shrink-0`; `style.width = sidebarWidth + 1` px
  when `sidebarWidth > 0`, no width otherwise): **Toggle Sidebar** (Lucide
  `PanelLeft`) and **Get Mail** (Lucide `RefreshCw`, with `animate-spin` on
  the icon while `syncing`).
- `list` (`px-3 shrink-0`, `style.width = listWidth + 1` px): a `div`
  with `data-tauri-drag-region` and `min-w-0 flex-1 flex flex-col`
  holding the title (`text-toolbar-title truncate`) and the subtitle
  (`text-toolbar-subtitle text-secondary truncate`); then **New Message**
  (Lucide `SquarePen`, always disabled until M3).
- `reader` (`gap-1 px-2 min-w-0 flex-1`): **Delete** (`Trash2`),
  **Archive** (`Archive`), **Reply** (`Reply`), **Reply All** (`ReplyAll`),
  **Forward** (`Forward`), **Flag** (`Flag`), the mark button (`MailOpen`
  labeled **Mark as Read** when `selection.seen` is false, `Mail` labeled
  **Mark as Unread** when true), **Move** (`FolderInput`), a `flex-1`
  spacer with `data-tauri-drag-region`, the `search` node, then
  **Minimize** (`Minus`), **Maximize** (`Square`) or **Restore** (`Copy`)
  by `maximized`, and **Close** (`X`).

**Buttons:** `type="button"`, `aria-label` = the bold name above,
`title` = the name, plus ` (shortcut)` for: Toggle Sidebar `Ctrl+Alt+S`,
Get Mail `Ctrl+Shift+N`, Delete `Delete`, Archive `Ctrl+Alt+A`, Flag
`Ctrl+Shift+L`, Mark `Ctrl+Shift+U`. Classes `flex size-7 items-center
justify-center rounded-md text-secondary hover:bg-selection-inactive
disabled:opacity-40`; icons 16px.

**Disabled:** Delete, Flag, Mark and Move when `selection.count` is 0;
Archive when the count is 0 or `canArchive` is false; Move also when
`moveTargets` is empty; Reply, Reply All, Forward and New Message always.

**Commands** (`onCommand`): Toggle Sidebar `toggleSidebar`; Get Mail
`getMail`; Delete `delete`; Archive `archive`; the mark button
`toggleRead`; Minimize `minimize`; Maximize/Restore `toggleMaximize`; Close
`close`.

**Flag menu:** clicking Flag (`aria-haspopup="menu"`) opens a
`ContextMenu` at the button's bottom-left corner
(`getBoundingClientRect()`): items `flag:1` … `flag:7` labeled by
`FLAG_NAMES`, each with `checked: selection.flagColor === N` (so
`ContextMenu` shows them as `menuitemcheckbox`), a separator, and
`flag:0` "Clear Flag", disabled when `selection.flagColor` is 0. Choosing
`flag:N` calls `onCommand({ kind: "flag", color: N })`; the menu closes.

**Move menu:** clicking Move (`aria-haspopup="menu"`) opens a
`ContextMenu` the same way with one item per target, id
`move:<mailboxId>`, label `" ".repeat(depth) + label`; choosing one
calls `onCommand({ kind: "move", mailboxId })`.

## Tests (given, do not edit)

`app/src/features/toolbar/Toolbar.test.tsx`.

## Gotchas

- Keep which menu is open (and where) in one `useState`; close it on the
  menu's `onClose`.
- Buttons must not carry `data-tauri-drag-region`, or they would drag the
  window instead of clicking.

## Out of scope

What commands do, window state and the search field's behavior (planner
containers, T-0035), and every file except
`app/src/features/toolbar/Toolbar.tsx`.

## Done when

`make accept T=0032`, `make check` and `make ui-check` pass, and only
`app/src/features/toolbar/Toolbar.tsx` changed.
