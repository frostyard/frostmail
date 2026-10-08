---
id: "0030"
title: Draw the sidebar
milestone: M2
size: M
touch:
  - app/src/features/sidebar/Sidebar.tsx
given:
  - app/src/features/sidebar/Sidebar.test.tsx
acceptance: make ui-vitest F=src/features/sidebar/Sidebar.test.tsx
---
# T-0030: Draw the sidebar

## Goal

The sidebar shows Favorites and each account's mailboxes as Mail.app does:
collapsible sections, indented rows with role icons and unread counts, the
selection, sync activity, and arrow-key navigation (`docs/specs/ui.md`,
Sidebar). `buildSidebar` (T-0027) already computes the rows; draw them.

## Read first

- `app/src/features/sidebar/Sidebar.tsx`: `SyncIndicator`, `SidebarProps`,
  the stub.
- `app/src/lib/mailboxTree.ts`: `SidebarSection`, `SidebarItem`,
  `SidebarIcon`.
- `docs/specs/ui.md`, "Sidebar".
- `app/src/styles/app.css`: colors and the `sidebar-*` type roles.
- The given test `app/src/features/sidebar/Sidebar.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names; replace the stub's doc comment and the file's
"Task T-0030 …" sentence with a description of the sidebar.

**Root:** a `div` with `role="tree"`, `aria-label="Mailboxes"`,
`tabIndex={0}`, classes `pt-2 outline-none`, and the `onKeyDown` handler
below.

**Section** (one per `sections` entry, in order): a header `button`
(26px high, full width, left-aligned, `pl-3`, `text-sidebar-section
text-secondary`, class `group`) showing the title, then the sync indicator
for `sync[section.accountId]` if any, then a Lucide `ChevronDown` (12px,
`opacity-0 group-hover:opacity-100`, rotated `-rotate-90` while collapsed).
It has `aria-expanded` (`"true"` while open) and toggles the section; keep
the collapsed section keys in state. Below it, while open, a `div` with
`role="group"` holding the rows.

**Sync indicator:** `syncing`: a Lucide `LoaderCircle` (12px,
`animate-spin`, `aria-label="Syncing"`). `error`: a Lucide `CircleAlert`
(12px, `aria-label="Sync error"`) inside a `span` whose `title` is the
message. `idle` or missing: nothing.

**Row:** a `div` with `role="treeitem"`, `tabIndex={-1}`,
`data-key={item.key}`, `aria-level={item.depth + 1}`,
`aria-selected={item.key === selectedKey}`, `aria-disabled="true"` when not
selectable (no attribute otherwise); classes `mx-2 flex h-7 items-center
gap-2 rounded-md pr-1`, and `style={{ paddingLeft: 4 + 16 * item.depth }}`
(the 8px inset plus 4px makes the spec's 12px). Inside:

- the icon: the Lucide component for `item.icon` (`inbox` Inbox, `file`
  File, `send` Send, `shield-alert` ShieldAlert, `trash-2` Trash2,
  `archive` Archive, `flag` Flag, `folder` Folder), 16px, `text-accent`,
  with `data-icon={item.icon}`;
- the label in a `span` with `text-sidebar-row truncate flex-1`;
- when `unread > 0`, the count in a `span` with `text-[12px] text-secondary
  tabular-nums`.

Selected row: `bg-accent text-accent-contrast` (icon and count
`text-accent-contrast` too) when `focused`, else `bg-selection-sidebar`.
Clicking a selectable row calls `onSelect(item.key)`; non-selectable rows
ignore clicks and show `text-secondary`.

**Keyboard** (`onKeyDown` on the root). The candidates are the selectable
rows of open sections, in display order. ArrowDown / ArrowUp: the next /
previous candidate after / before the selected one (the first candidate on
ArrowDown when nothing selected is a candidate; nothing past the ends).
Home / End: the first / last candidate. For these four keys call
`onSelect(key)` when the target differs from the selection, and always
`preventDefault()` and `stopPropagation()`, so the window's keymap does not
act on them too. Leave every other key alone.

## Tests (given, do not edit)

`app/src/features/sidebar/Sidebar.test.tsx`.

## Gotchas

- Lucide icons are `aria-hidden` by default, so a row's accessible name is
  its label followed by its count.
- Build the candidate list from the same data you render; collapsed
  sections contribute no candidates.

## Out of scope

Building the rows (`buildSidebar`), choosing the source and focus handling
(the planner's `SidebarContainer`), and every file except
`app/src/features/sidebar/Sidebar.tsx`.

## Done when

`make accept T=0030`, `make check` and `make ui-check` pass, and only
`app/src/features/sidebar/Sidebar.tsx` changed.
