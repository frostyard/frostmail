---
id: "0113"
title: Apply Rules in the message list's context menu
milestone: M5
size: S
touch:
  - app/src/app/ListContainer.tsx
  - app/src/app/commands.ts
given:
  - app/src/app/ListMenu.rules.test.tsx
acceptance: make ui-vitest F="src/app/ListMenu src/app/commands"
---
# T-0113: Apply Rules in the message list's context menu

## Goal

The list's context menu offers Apply Rules, which runs the enabled rules on
the chosen messages now, wherever they are, as Mail.app's Message > Apply
Rules does.

## Read first

- `docs/specs/ui.md`: "Context menu", the Apply Rules group.
- `app/src/app/ListContainer.tsx`: `menuItems` and `onMenuSelect`.
- `app/src/app/commands.ts`: the commands the menu calls.
- `app/src/data/stores.ts`: `useMail`'s `rules`.
- The given test; `docs/tasks/EXECUTOR.md`.

## Contract

- **`commands.ts`**: `applyRules(client, ids)` calls `rule.apply {ids}`;
  it does nothing for no IDs.
- **The menu:** when at least one rule in `useMail`'s `rules` is enabled,
  the menu ends with a separator and an item "Apply Rules" (id
  `applyRules`, no shortcut), disabled when the account is read-only, as
  the other items are. With no enabled rule there is neither the item nor
  its separator. Choosing it calls `applyRules` on the menu's messages; a
  failure is logged with `console.warn`, as compose's are.

## Tests (given, do not edit)

`ListMenu.rules.test.tsx`. `ListMenu.test.tsx` (T-0103's, whose fixture has
no rules, so its menu is unchanged) and every other app test must keep
passing.

## Gotchas

- Read `rules` with a selector (`useMail((s) => ...)`) and add what the
  menu depends on to `useMemo`'s dependencies, or the item will not appear
  when rules load after the list.
- No keyboard shortcut: Ctrl+Alt+L locks the screen on most Linux desktops.

## Out of scope

The Rules pane, maild, the mock, and every file not under `touch`.

## Done when

`make accept T=0113` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
