---
id: "0101"
title: Filter the message list from a bar at its top
milestone: M4.5
size: M
touch:
  - app/src/features/list/FilterBar.tsx
  - app/src/data/stores.ts
  - app/src/app/ToolbarContainer.tsx
  - app/src/app/MainWindow.tsx
  - app/src/app/ListContainer.tsx
given:
  - app/src/features/list/FilterBar.test.tsx
  - app/src/data/stores.filter.test.ts
  - app/src/app/ListFilter.test.tsx
acceptance: make ui-vitest F="src/features/list src/data src/app/ListFilter"
---
# T-0101: Filter the message list from a bar at its top

## Goal

The user wants to narrow the message list to unread, flagged, or with
attachments without leaving it. A filter bar at the top of the list column
offers All, Unread, Flagged and Attachments; the choice goes into the
list's view query, which maild now filters on (`ViewQuery.unread`,
`flagged`, and the new `hasAttachments`). maild, and the mock, keep a row
that is read or unflagged while a filtered view is open, so reading a
message under Unread does not make it vanish.

## Read first

- `docs/specs/ui.md`: "Message list", the "Empty" and "Filter bar"
  bullets.
- `app/src/data/stores.ts`: `UIState`, `setSearchScope`, `listQuery`,
  `persist`'s `partialize`.
- `app/src/app/ToolbarContainer.tsx`: `useListQuery`.
- `app/src/app/MainWindow.tsx`: `MailPanes` (the list column).
- `app/src/app/ListContainer.tsx`: the empty state ("No Messages").
- `app/src/features/search/SearchField.tsx`: `ScopeBar`, a similar strip.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`app/src/features/list/FilterBar.tsx`** (new) exports:
  - `type ListFilter = "all" | "unread" | "flagged" | "attachments"`;
  - `LIST_FILTERS: { key: ListFilter; label: string }[]`, in that order,
    labeled All, Unread, Flagged, Attachments;
  - `emptyText(filter)`: "No Messages", "No Unread Messages", "No Flagged
    Messages", "No Messages with Attachments";
  - `FilterBar({ filter, onChange })`: a `div` with `role="toolbar"`,
    `aria-label="Filter messages"`, classes including `flex h-[32px]
    shrink-0 items-center gap-1 border-b border-separator bg-window px-3`;
    one `button type="button"` per filter with `aria-pressed`, classes
    including `h-[22px] rounded-md px-2 text-[12px] leading-4`, plus
    `bg-selection-inactive text-primary` when chosen and
    `bg-transparent text-secondary hover:text-primary` otherwise. A click
    calls `onChange(key)`.
- **`stores.ts`:** `UIState.listFilter: ListFilter` (initially `"all"`,
  not in `partialize`, so not remembered); `setListFilter(filter)` sets it
  and clears `selected` and `anchor`. `setSource` leaves it alone.
  `listQuery` takes an optional `listFilter` and adds `unread: true`,
  `flagged: true` or `hasAttachments: true` to every query it builds (the
  source's, and a search's in either scope); `"all"` or absent adds
  nothing. Import the type from `../features/list/FilterBar`, as the store
  already imports `OccurrenceKey`.
- **`useListQuery`:** passes the store's `listFilter`.
- **`MainWindow`'s list column:** a `flex h-full shrink-0 flex-col`
  column with the `FilterBar` (the store's filter and `setListFilter`) and
  under it the `ListContainer` in a `min-h-0 flex-1` box.
- **`ListContainer`'s empty state:** `emptyText(listFilter)` in the
  existing `text-empty text-secondary` style; under it, when the filter is
  not `"all"`, a `button type="button"` "Show All" (`border-0
  bg-transparent p-0 text-[12px] leading-4 text-accent`) that sets the
  filter to `"all"`. The column centers both.

## Tests (given, do not edit)

`FilterBar.test.tsx`, `stores.filter.test.ts` and `ListFilter.test.tsx`
(the window against the mock maild). Every other app test must keep
passing.

## Gotchas

- The view query decides which view opens: keep `listQuery`'s output a
  plain object built the same way every time.
- Do not change the mock (`app/src/rpc/mock`): it already filters on
  `hasAttachments` and keeps read rows.
- Cards T-0102 and T-0103 change other parts of `ListContainer.tsx` and
  `MainWindow.tsx` at the same time; touch only the list column, the empty
  state and the lines they need.

## Out of scope

maild, the toolbar's buttons and subtitle, the search scope bar, keyboard
shortcuts for filters, and every file not under `touch`.

## Done when

`make accept T=0101` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
