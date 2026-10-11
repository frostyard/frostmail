---
id: "0112"
title: The Rules pane in the settings window
milestone: M5
size: M
touch:
  - app/src/features/settings/RulesPane.tsx
  - app/src/app/SettingsWindow.tsx
given:
  - app/src/features/settings/RulesPane.test.tsx
  - app/src/app/SettingsWindow.rules.test.tsx
  - app/src/app/SettingsWindow.general.test.tsx # T-0106's, with the Rules tab
acceptance: make ui-vitest F="src/features/settings/RulesPane src/app/SettingsWindow"
---
# T-0112: The Rules pane in the settings window

## Goal

A Rules tab in the settings window lists maild's rules in the order they
run and adds, edits, turns on and off, duplicates, reorders and removes
them, through the rule sheet (T-0111), as Mail.app's Rules pane does.

## Read first

- `docs/specs/settings-ui.md`: "Window" (the tabs) and "Rules pane",
  including its Behavior (container).
- `app/src/app/SettingsWindow.tsx`: `TABS`, `GeneralPaneContainer`, and
  `confirmRemove` (how removing an account asks first).
- `app/src/features/organize/RuleSheet.tsx` (`RuleSheet`, `newRuleDraft`,
  `RuleDraft`) and `app/src/app/SmartSheetContainer.tsx` (how a sheet's
  container saves, keeps busy, shows the error, and computes `today`).
- `app/src/data/stores.ts`: `useMail`'s `rules`, kept current by the
  session from `rule.changed`; `app/src/rpc/gen/api.ts`: `Rule` and the
  `rule.*` params.
- `app/src/features/settings/labels.ts` (`BUTTON`, `ALERT`).
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`RulesPane`** (`RulesPane.tsx`), props `rules: Rule[]`, `error?`,
  `onToggle(rule, enabled)`, `onAdd()`, `onEdit(rule)`,
  `onDuplicate(rule)`, `onRemove(rule)`, `onMove(rule, position)`: the
  spec's list (a `ul` named "Rules"; a row's checkbox "Enable <name>"; its
  name `button` with `aria-pressed`; the problem `role="img"` named by the
  problem with it as `title`; "No Rules" when empty), and the buttons Add
  Rule, Edit, Duplicate, Remove, Move Up, Move Down. It keeps the
  selection itself; `onMove` gets the new position (the selected rule's
  index ± 1).
- **The tab:** Rules, with Lucide's `ListFilter`, between Signatures and
  Sign-In.
- **The container** (in `SettingsWindow.tsx`), as the spec's Behavior says:
  `rule.create {name, conditions, actions}` from a New Rule sheet,
  `rule.update {id, name, conditions, actions}` from an Edit Rule sheet,
  `rule.update {id, enabled}` from a checkbox, Duplicate as
  `rule.create {name: "<name> Copy", conditions, actions, enabled}` then
  `rule.move {id: <the copy's>, position: <the original's> + 1}`, Remove
  after `window.confirm('Remove the rule "<name>"?')` (Tauri's `ask` in
  Tauri, as `confirmRemove` does), Move Up and Down as `rule.move`. Pane
  errors go to the pane's `error`, cleared by the next success; sheet
  errors to the sheet, which stays open.

## Tests (given, do not edit)

`RulesPane.test.tsx`, `SettingsWindow.rules.test.tsx`, and
`SettingsWindow.general.test.tsx`, T-0106's test with "Rules" among the
tabs. Every other app test must keep passing.

## Gotchas

- The list re-renders from `useMail`'s `rules` after `rule.changed`; do
  not keep a copy of the rules in the container.
- The tab test finds tabs as the buttons with `aria-pressed`; the Rules
  pane's name buttons have it too, but only while the Rules pane is open.
- Calls go out exactly as the contract says: no extra keys (for example no
  `enabled` on `rule.create` from the sheet).

## Out of scope

The rule sheet itself (T-0111), maild, the mock, and every file not under
`touch`.

## Done when

`make accept T=0112` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
