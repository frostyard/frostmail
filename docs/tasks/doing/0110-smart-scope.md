---
id: "0110"
title: A smart mailbox as the notification scope
milestone: M5
size: S
touch:
  - app/src/features/settings/GeneralPane.tsx
  - app/src/app/SettingsWindow.tsx
given:
  - app/src/features/settings/GeneralPane.smart.test.tsx
  - app/src/app/SettingsWindow.smart.test.tsx
acceptance: make ui-vitest F="src/features/settings/GeneralPane.smart src/app/SettingsWindow.smart"
---
# T-0110: A smart mailbox as the notification scope

## Goal

The General pane's "New message notifications" also offers each smart
mailbox, so that only new mail a smart mailbox lists notifies, as Mail.app
allows.

## Read first

- `docs/specs/settings-ui.md`: "General pane", the notifications bullet.
- `docs/design/organize.md`: "Notifications".
- `app/src/features/settings/GeneralPane.tsx` and
  `app/src/app/SettingsWindow.tsx` (`GeneralPaneContainer`).
- `app/src/data/stores.ts`: `useMail`'s `smarts`; `app/src/rpc/gen/api.ts`:
  `SmartMailbox`, `Settings.notifySmartId`, `SettingsSetParams`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`GeneralPane`**: a prop `smarts?: SmartMailbox[]`. When there are
  any, the notifications `select` ends with an `optgroup` labeled "Smart
  Mailboxes" holding an option per smart mailbox, valued `smart:<id>`,
  labeled with its name. The `select` shows `smart:<notifySmartId>` when
  `notifyScope` is `smart`. Choosing a smart mailbox calls
  `onChange({ notifyScope: "smart", notifySmartId: <id> })`; the other
  choices are as now.
- **`GeneralPaneContainer`**: passes `useMail`'s `smarts`.

## Tests (given, do not edit)

`GeneralPane.smart.test.tsx` and `SettingsWindow.smart.test.tsx` (the
settings window against the mock maild). Every other app test must keep
passing, `GeneralPane.test.tsx` included.

## Gotchas

- The mock refuses `notifyScope: "smart"` without `notifySmartId`; send
  both in one call.

## Out of scope

maild, the mock, and every file not under `touch`.

## Done when

`make accept T=0110` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
