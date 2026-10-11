---
id: "0116"
title: Send Later in the compose window
milestone: M5
size: M
touch:
  - app/src/features/compose/ComposeToolbar.tsx
  - app/src/app/ComposeWindow.tsx
given:
  - app/src/features/compose/ComposeToolbar.later.test.tsx
  - app/src/app/ComposeWindow.later.test.tsx
  - app/src/features/compose/ComposeToolbar.test.tsx # T-0045's, with Send Later after Send
acceptance: make ui-vitest F="src/features/compose src/app/ComposeWindow"
---
# T-0116: Send Later in the compose window

## Goal

A Send Later button beside Send opens a menu of times (tonight, tomorrow
morning, or one chosen in the time sheet). maild then sends the message
at that time, dated then, with the app open or not.

## Read first

- `docs/specs/compose-ui.md`: "Layout" (the Send Later button) and "Send
  Later".
- `app/src/features/compose/ComposeToolbar.tsx` and
  `app/src/app/ComposeWindow.tsx` (`send`, `onCommand`).
- `app/src/features/toolbar/Toolbar.tsx`: how the main toolbar opens a
  `ContextMenu` under a button (`openMenu`).
- `app/src/lib/later.ts` (`sendChoices`, `LaterChoice`) and
  `app/src/features/later/TimeSheet.tsx`.
- `app/src/lib/calendarDates.ts` (`appLocale`).
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`ComposeToolbar`**: `ComposeCommand` gains `"sendLater"`. New optional
  props `laterChoices?: LaterChoice[]` (none by default) and
  `onSendAt?(at: Date)`. Right after Send, a button named "Send Later"
  (title "Send Later", `aria-haspopup="menu"`, 16 × 28, a 12px
  `ChevronDown` in `--accent`, disabled with Send) opens a `ContextMenu`
  under it: one item per choice (its label), a separator, and "Send
  Later…". A choice calls `onSendAt(choice.at)`; "Send Later…" calls
  `onCommand("sendLater")`.
- **`ComposeWindow`**: passes `sendChoices(new Date(), <system zone>,
  appLocale(navigator.language))`. `onSendAt(at)` sends as Send does with
  `draft.send {id, sendAt: at.toISOString()}`; `"sendLater"` opens the
  `TimeSheet` titled "Send Later", starting at the last choice's time
  (tomorrow 8:00), whose OK sends at the chosen time and whose Cancel
  closes it. Send without a time still calls `draft.send {id}` with no
  `sendAt` key.

## Tests (given, do not edit)

`ComposeToolbar.later.test.tsx`, `ComposeWindow.later.test.tsx`, and
`ComposeToolbar.test.tsx`, T-0045's with "Send Later" after "Send". Every
other app test must keep passing.

## Gotchas

- The system zone is `Intl.DateTimeFormat().resolvedOptions().timeZone`.
- The toolbar stays presentational: no client calls in it.

## Out of scope

The main window's Send Later section and undo toast (T-0117), maild, the
mock, and every file not under `touch`.

## Done when

`make accept T=0116` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
