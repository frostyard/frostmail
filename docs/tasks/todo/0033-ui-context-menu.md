---
id: "0033"
title: Add the context menu component
milestone: M2
size: M
touch:
  - app/src/features/menu/ContextMenu.tsx
given:
  - app/src/features/menu/ContextMenu.test.tsx
acceptance: make ui-vitest F=src/features/menu/ContextMenu.test.tsx
---
# T-0033: Add the context menu component

## Goal

The webview's own context menu is disabled; list rows and the toolbar's
Flag and Move buttons open the app's menu instead (`docs/specs/ui.md`,
Behavior: Context menu). Build `ContextMenu`: a positioned popup with items,
separators, check marks, shortcuts, submenus, keyboard navigation and
outside-click closing.

## Read first

- `app/src/features/menu/ContextMenu.tsx`: `MenuItem`, `ContextMenuProps`,
  the stub.
- The given test `app/src/features/menu/ContextMenu.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported types and names; replace the stub's doc comment and the
file's "Task T-0033 …" sentence with a description of the component.

**Structure.** Render with `createPortal(…, document.body)`. The root
element has `role="menu"`, `tabIndex={-1}`, and inline style
`position: fixed`, `left`, `top` in px. It receives focus when it mounts
(`useEffect` + `focus()`).

- `item`: an element with `role="menuitem"` containing a 14px check column
  (a Lucide `Check` when `checked` is true), the optional `icon`, the label
  in a `span`, and the `shortcut` (if any) in a right-aligned `span` with
  `text-secondary`. `aria-disabled="true"` when disabled;
  `aria-checked="true"` only when `checked` is true (no attribute
  otherwise).
- `separator`: an element with `role="separator"` (1px `bg-separator`,
  4px vertical margin).
- `submenu`: a `role="menuitem"` with `aria-haspopup="menu"` and
  `aria-expanded` (`"true"` while its submenu is open), showing the label
  and a Lucide `ChevronRight`. Its submenu is another `role="menu"`
  element (no portal needed), positioned `fixed` next to the item's right
  edge, top aligned with the item.
- Styling: `bg-window`, 1px `border-separator`, `rounded-md`, `shadow-lg`,
  4px vertical padding, `min-w-[180px]`; rows 24px high, 12px horizontal
  padding, 13px text; a highlighted row is `bg-accent text-accent-contrast`;
  disabled rows are `text-tertiary`.

**Position.** After mount (`useLayoutEffect`), measure the root and clamp:
`left = max(0, min(x, innerWidth - width - 4))`, the same for `top` with
heights.

**Highlight.** Each open menu level has at most one highlighted index.
Only the highlighted row of the **deepest open** menu has
`data-highlighted="true"`; a row whose submenu is open has
`data-open="true"` instead. Nothing is highlighted when the menu opens.
Hovering an enabled row highlights it; hovering a submenu row
(`onMouseEnter`) also opens its submenu.

**Keyboard** (`onKeyDown` on the root, which keeps focus; act on the
deepest open menu):

- ArrowDown / ArrowUp: highlight the next / previous enabled row (skip
  separators and disabled rows), wrapping around.
- ArrowRight, Enter or Space on a submenu row: open it and highlight its
  first enabled row.
- ArrowLeft: close the deepest submenu (its parent row becomes the
  highlighted row again); nothing at the root level.
- Enter or Space on an item row: `onSelect(id)` then `onClose()`.
- Escape: close the deepest submenu, or call `onClose()` at the root level.

**Mouse.** Clicking an enabled item calls `onSelect(id)` then `onClose()`;
clicking a disabled item or a separator does nothing. A `mousedown`
anywhere outside every open menu calls `onClose()`; listen on `document`
in a `useEffect` and remove the listener on unmount.

## Tests (given, do not edit)

`app/src/features/menu/ContextMenu.test.tsx`.

## Gotchas

- Keep the open submenu path and highlights as state: for example an array
  of `{ items, highlighted, openIndex }` levels.
- `getAllByRole("menuitem")` sees rows of open submenus too; the test
  expects four rows while no submenu is open.
- The outside check must accept presses inside submenus: test
  `element.contains(event.target)` for every open menu element.

## Out of scope

Where menus open and what their items do (the planner's containers), and
every file except `app/src/features/menu/ContextMenu.tsx`.

## Done when

`make accept T=0033`, `make check` and `make ui-check` pass, and only
`app/src/features/menu/ContextMenu.tsx` changed.
