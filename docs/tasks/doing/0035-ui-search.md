---
id: "0035"
title: Add the search field and scope bar
milestone: M2
size: S
touch:
  - app/src/features/search/SearchField.tsx
given:
  - app/src/features/search/SearchField.test.tsx
acceptance: make ui-vitest F=src/features/search/SearchField.test.tsx
---
# T-0035: Add the search field and scope bar

## Goal

Searching filters the list as the user types, after a short pause, and a
scope bar chooses between all mailboxes and the current one
(`docs/specs/ui.md`, Toolbar and Behavior: Search). Build the two
presentational components; the container decides what a search opens.

## Read first

- `app/src/features/search/SearchField.tsx`: the props and the stubs.
- `docs/specs/ui.md`: "Toolbar" (the search field) and "Behavior" (Search).
- The given test `app/src/features/search/SearchField.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported types and names; replace the stubs' doc comments and the
file's "Task T-0035 …" sentence with what each component does.

**`SearchField`**

- A wrapper `div` (220px wide, 28px high, `rounded-md`,
  `bg-selection-inactive`, items centered, 6px horizontal padding) holding:
  a 14px Lucide `Search` icon in `text-tertiary`; an `input` with
  `type="text"`, `role="searchbox"`, `aria-label="Search"`,
  `placeholder="Search"`, transparent background, no border or outline,
  `flex-1`, 13px text, its `ref` set to `inputRef`; and, only while `value`
  is not empty, a `button` with `aria-label="Clear search"` showing a 14px
  Lucide `CircleX` in `text-tertiary`.
- Every input change calls `onChange(newValue)` at once and (re)starts a
  250 ms timer; when it fires it calls `onSearch(newValue.trim())`.
- Enter cancels the timer and calls `onSearch(value.trim())` at once.
- Escape, or a click on the clear button, cancels the timer and calls
  `onClear()`.
- The timer is cleared when the component unmounts. Keep it in a `useRef`.

**`ScopeBar`**

- A 28px-high row (`bg-toolbar`, 1px `border-separator` bottom, 12px left
  padding, 8px gap, items centered): the text `Search:` (12px,
  `text-secondary`), then one `button` per scope showing its label (12px,
  `rounded`, 8px horizontal padding, 20px high), with
  `aria-pressed="true"` and `bg-selection-inactive text-primary` when its
  key is `selected`, `aria-pressed="false"` and `text-secondary` otherwise.
- Clicking a scope calls `onSelect(scope.key)`.

## Tests (given, do not edit)

`app/src/features/search/SearchField.test.tsx`.

## Gotchas

- Use `type="text"` with `role="searchbox"`, not `type="search"`: WebKit
  draws its own cancel button on search inputs.
- Handle Enter and Escape in `onKeyDown` on the input.
- The field is controlled: render `value` from props and never keep a copy
  of the text in state.

## Out of scope

Opening search views (the planner's container), and every file except
`app/src/features/search/SearchField.tsx`.

## Done when

`make accept T=0035`, `make check` and `make ui-check` pass, and only
`app/src/features/search/SearchField.tsx` changed.
