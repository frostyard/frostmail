---
id: "0122"
title: The list's Sort menu, Conversations and Contact Photos
milestone: M5
size: M
touch:
  - app/src/features/list/SortMenu.tsx
  - app/src/data/stores.ts
  - app/src/app/ToolbarContainer.tsx
  - app/src/app/MainWindow.tsx
given:
  - app/src/features/list/SortMenu.test.tsx
  - app/src/data/stores.sort.test.ts
  - app/src/app/SortList.test.tsx
acceptance: make ui-vitest F="src/features/list src/data src/app/SortList src/app/ListFilter"
---
# T-0122: The list's Sort menu, Conversations and Contact Photos

## Goal

Sort the message list by date, from, to, subject, size, flags, unread or
attachments, either way, and turn conversations and contact photos on or
off, from a Sort menu beside the filter bar, as Mail.app's sort control does.

## Read first

- `docs/specs/ui.md`: "Message list", the Sort and view options bullet.
- `app/src/data/stores.ts`: `UIState`, `initialUI`, `partialize`,
  `listQuery`; `app/src/app/ToolbarContainer.tsx`: `useListQuery`;
  `app/src/app/MainWindow.tsx`: where `FilterBar` renders.
- `app/src/features/list/FilterBar.tsx` (the look) and
  `app/src/features/menu/ContextMenu.tsx` (`checked` items).
- `app/src/rpc/gen/api.ts`: `ViewSort`, `ViewQuery.sort` and `ascending`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`stores.ts`:**
  - `UIState` gains `sort: ViewSort` (`"date"`), `ascending: boolean`
    (`false`) and `contactPhotos: boolean` (`false`), all three persisted
    with the layout.
  - `UIActions` gains `setSort(sort)`, which also sets `ascending` to the
    field's usual direction (true for `from`, `to` and `subject`, else
    false); `setAscending`, `setConversations` and `setContactPhotos`.
  - `listQuery` takes optional `sort` and `ascending`. It adds `{sort}`,
    plus `ascending: true` when ascending, unless the sort is date
    descending, the default (and then neither key). This applies to every
    branch, a search included.
- **`useListQuery`** passes the store's sort and direction.
- **`SortMenu`** (`features/list/SortMenu.tsx`) exports `SORTS`,
  `SortMenuProps` (`sort`, `ascending`, `conversations`, `contactPhotos`,
  `onSort`, `onAscending`, `onConversations`, `onContactPhotos`) and
  `SortMenu`:
  - A button whose text is "Sort by <Field>", with `aria-haspopup="menu"`
    and a 12px `ChevronDown`, opens a `ContextMenu` with the spec's
    entries. The checked ones use `checked`.
  - Choosing Ascending or Descending calls `onAscending(true or false)`.
  - Conversations and Contact Photos call their handler with the opposite
    of the current value.
- **`MainWindow`**: the filter bar and the Sort menu share one row, the
  Sort menu at its right, outside the "Filter messages" toolbar.

## Tests (given, do not edit)

`SortMenu.test.tsx`, `stores.sort.test.ts`, `SortList.test.tsx`. Every
other app test must keep passing, the FilterBar tests included.

## Gotchas

- The filter bar's own tests count its buttons: keep the Sort button out
  of its toolbar element.
- The persisted layout is read on start; give the new fields defaults so
  an older stored layout still loads.

## Out of scope

Drawing contact photos (T-0123), maild, the mock, and every file not under
`touch`.

## Done when

`make accept T=0122` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
