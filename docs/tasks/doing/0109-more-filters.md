---
id: "0109"
title: The filter bar's More menu
milestone: M5
size: M
touch:
  - app/src/features/list/FilterBar.tsx
  - app/src/data/stores.ts
given:
  - app/src/features/list/FilterBar.test.tsx
  - app/src/features/list/FilterBar.more.test.tsx
  - app/src/data/stores.filter.more.test.ts
  - app/src/app/ListFilter.more.test.tsx
acceptance: make ui-vitest F="src/features/list/FilterBar src/data/stores.filter.more src/app/ListFilter.more"
---
# T-0109: The filter bar's More menu

## Goal

The filter bar keeps All, Unread, Flagged and Attachments and gains a
fifth button, More, whose menu offers To Me, Cc Me and From VIPs, as
Mail.app's filter does. Each is a condition in the view's `filter`, so it
combines with any source, including those listed by their own conditions
(a VIP, a flag color, a smart mailbox).

## Read first

- `docs/specs/ui.md`: "Message list", the filter bar bullet.
- `app/src/features/list/FilterBar.tsx`: `ListFilter`, `LIST_FILTERS`,
  `emptyText`, `FilterBar`.
- `app/src/data/stores.ts`: `listQuery`.
- `app/src/features/menu/ContextMenu.tsx` (checkable items are
  `menuitemcheckbox`); the toolbar's Flag menu in
  `app/src/features/toolbar/Toolbar.tsx` opens one the same way.
- `app/src/rpc/gen/api.ts`: `ViewQuery.filter`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`FilterBar.tsx`**:
  - `ListFilter` gains `"toMe" | "ccMe" | "vips"`; `LIST_FILTERS` stays
    the four buttons.
  - `MORE_FILTERS = [{ key: "toMe", label: "To Me" }, { key: "ccMe",
    label: "Cc Me" }, { key: "vips", label: "From VIPs" }]`.
  - `emptyText`: "No Messages to You", "No Messages Cc'd to You", "No
    Messages from VIPs".
  - After the four buttons, a button named "More filters" with
    `aria-haspopup="menu"`, styled as the others (22 high, rounded 6),
    whose text is "More", or the chosen More filter's label, followed by a
    12px `ChevronDown`; `aria-pressed` and the pressed style while a More
    filter is chosen. Clicking opens a `ContextMenu` under it with the
    three filters as checkable items (the chosen one checked); choosing
    one calls `onChange` with its key.
- **`listQuery`**: for `toMe`, `ccMe` and `vips`, the query of the source
  or the search as now, plus `filter: { match: "all", conditions: [{
  field: "tome" | "ccme" | "vip", op: "is", value: "true" }] }`.

## Tests (given, do not edit)

`FilterBar.test.tsx` (T-0101's contract, revised: the strip now holds
More), `FilterBar.more.test.tsx`, `stores.filter.more.test.ts` and
`ListFilter.more.test.tsx` (the window against the mock maild). Every
other app test must keep passing, including `stores.filter.test.ts` and
`ListFilter.test.tsx`.

## Gotchas

- `FilterBar.test.tsx` replaces the file T-0101 left; taskrun copies it
  in when the task starts.
- Keep the menu's position from the More button's bounding box, as the
  toolbar's menus do.

## Out of scope

maild, the mock, the sidebar, and every file not under `touch`.

## Done when

`make accept T=0109` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
