---
id: "0106"
title: The General pane and the flags' names in the menus
milestone: M5
size: M
touch:
  - app/src/features/settings/GeneralPane.tsx
  - app/src/app/SettingsWindow.tsx
  - app/src/features/toolbar/Toolbar.tsx
  - app/src/app/ToolbarContainer.tsx
  - app/src/app/ListContainer.tsx
given:
  - app/src/features/settings/GeneralPane.test.tsx
  - app/src/features/toolbar/Toolbar.flagnames.test.tsx
  - app/src/app/SettingsWindow.general.test.tsx
  - app/src/app/FlagNames.test.tsx
acceptance: make ui-vitest F="src/features/settings/GeneralPane src/features/toolbar/Toolbar.flagnames src/app/SettingsWindow.general src/app/FlagNames"
---
# T-0106: The General pane and the flags' names in the menus

## Goal

The settings window gains a General pane, first among its tabs, for
maild's preferences: which new mail notifies, the undo send delay, and
names for the seven flags. The list's Flag Color menu and the toolbar's
Flag menu then show those names, as the sidebar does.

## Read first

- `docs/specs/settings-ui.md`: "Window" (the tabs) and "General pane".
- `docs/specs/ui.md`: "Behavior", the context menu's flag names.
- `docs/design/organize.md`: "Settings".
- `app/src/app/SettingsWindow.tsx`: `TABS`, the panes and their
  containers.
- `app/src/features/settings/labels.ts`: `GRID`, `LABEL`, `FIELD`, `ALERT`.
- `app/src/lib/flags.ts`: `flagName`, `flagLabel`.
- `app/src/features/toolbar/Toolbar.tsx`: `flagItems`, `ToolbarProps`;
  `app/src/app/ToolbarContainer.tsx`.
- `app/src/app/ListContainer.tsx`: `menuItems`' Flag Color submenu.
- `app/src/data/stores.ts`: `useMail`'s `settings`, which the store loads
  and reloads on `settings.changed`; `client.settings.set`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`features/settings/GeneralPane.tsx`** (new), presentational:
  `interface GeneralPaneProps { settings: Settings | null; error?: string;
  onChange: (change: SettingsSetParams) => void }` and
  `GeneralPane(props)`:
  - A `GRID` of rows in 24px padding, labels in `LABEL`, fields in
    `FIELD`.
  - "New message notifications:", a `select` labeled by its `label`, with
    options `inbox` "Inbox Only", `vips` "VIPs", `contacts` "Contacts",
    `all` "All Mailboxes", showing `settings.notifyScope`; a choice calls
    `onChange({ notifyScope })`.
  - "Undo send delay:", a `select` with options `0` "Off", `10` "10
    Seconds", `20` "20 Seconds", `30` "30 Seconds", showing
    `settings.undoDelay`; a choice calls `onChange({ undoDelay: <number> })`.
  - "Flag names:", seven rows: a 12px filled Lucide `Flag` in
    `text-flag-<n>` (the seven classes listed whole) and a text input
    named "Flag <n> name", `maxLength` 40, placeholder `flagName(n)`,
    holding a draft of `settings.flagNames[n - 1]`. Leaving a field, or
    Enter in it, calls `onChange({ flagNames })` with all seven drafts
    trimmed, only when they differ from `settings.flagNames`. The drafts
    start over when `settings.flagNames` changes.
  - `error` shows in a `p` with `role="alert"` and `ALERT`.
  - While `settings` is `null`, every control is disabled.
- **`SettingsWindow`**: a tab "General" (Lucide `Settings2`) before
  Accounts; the window still opens on Accounts. The General pane's
  container passes `useMail`'s `settings`, calls `client.settings.set`
  with each change, and shows a failure's message as `error` until a
  change succeeds.
- **Flag names:** `ToolbarProps` gains `flagNames?: readonly string[]`, and
  `flagItems` labels each color `flagLabel(n, flagNames)`;
  `ToolbarContainer` passes `useMail`'s `settings?.flagNames` (subscribe
  to the store, so the menu follows a rename). `ListContainer`'s Flag
  Color submenu labels its colors the same way. Item ids, order and
  checks stay as they are.

## Tests (given, do not edit)

`GeneralPane.test.tsx`, `Toolbar.flagnames.test.tsx`,
`SettingsWindow.general.test.tsx` (the settings window against the mock
maild) and `FlagNames.test.tsx` (the main window). Every other app test
must keep passing, including `Toolbar.test.tsx`, `ListMenu.test.tsx` and
`SettingsWindow*.test.tsx`.

## Gotchas

- `useEffect(() => setNames(...), [settings?.flagNames])` needs no
  Biome suppression; an unneeded `biome-ignore` is itself a warning that
  fails `make ui-check`.
- The row's hover actions also have a button named "Flag"; the toolbar's
  is the one with `aria-haspopup="menu"`.
- Keep `FLAG_NAMES` exported: other code uses it.

## Out of scope

maild, the mock, the sidebar's color rows (T-0104), notifying for a smart
mailbox (Phase 3), and every file not under `touch`.

## Done when

`make accept T=0106` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
