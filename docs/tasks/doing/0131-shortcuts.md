---
id: "0131"
title: Redirect, Print, Raw Source and All Headers shortcuts
milestone: M5
size: M
touch:
  - app/src/lib/keymap.ts
  - app/src/app/MainWindow.tsx
  - app/src/app/ListContainer.tsx
  - app/src/app/ReaderContainer.tsx
given:
  - app/src/app/Shortcuts.test.tsx
acceptance: make ui-vitest F="src/app/Shortcuts src/app/ReaderMore src/app/Redirect src/app/Undo src/lib/keymap"
---
# T-0131: Redirect, Print, Raw Source and All Headers shortcuts

## Goal

The last of Mail.app's mail shortcuts, with Ctrl for ⌘: Redirect
(Ctrl+Shift+E), Print (Ctrl+P), Raw Source (Ctrl+Alt+U) and Show All
Headers (Ctrl+Shift+H), shown in their menus.

## Read first

- `docs/specs/ui.md`: the keyboard map, "More Actions", and the context
  menu's first group.
- `app/src/lib/keymap.ts` (`BINDINGS`, `allowedInTextField`),
  `app/src/app/MainWindow.tsx` (`mailCommand`),
  `app/src/app/ListContainer.tsx` (the `ListHandle`'s `command`, the
  Redirect sheet) and `app/src/app/ReaderContainer.tsx` (`ReaderHandle`,
  `Conversation`, `ConversationMessage`'s `moreAction`).
- The given test; `docs/tasks/EXECUTOR.md`.

## Contract

- **`keymap.ts`**: commands `redirect` (Ctrl+Shift+E), `print` (Ctrl+P),
  `rawSource` (Ctrl+Alt+U) and `allHeaders` (Ctrl+Shift+H).
- **`MainWindow.tsx`** (`mailCommand`), whatever pane has focus:
  - `redirect`: the list's `command("redirect")`.
  - `print`: with exactly one message selected, `window.print()`;
    otherwise not handled.
  - `rawSource` / `allHeaders`: with exactly one message selected, the
    reader handle's `more("source")` / `more("headers")`; otherwise not
    handled.
- **`ListContainer.tsx`**: `command("redirect")` opens the Redirect sheet
  for the last selected message, as the menu's Redirect… does, unless
  there is none or its account is read-only; the menu's Redirect… shows
  the shortcut "Ctrl+Shift+E".
- **`ReaderContainer.tsx`**: `ReaderHandle` gains `more(action: "headers"
  | "source")`, which runs that More Actions item (Show or Hide All
  Headers, Raw Source…) on the selected message's article, as choosing
  it in its menu does; nothing for other articles, and nothing again
  when a message is shown later. The More Actions items show their keys:
  Show/Hide All Headers "Ctrl+Shift+H", Raw Source… "Ctrl+Alt+U",
  Print… "Ctrl+P".

## Tests (given, do not edit)

`Shortcuts.test.tsx`. Every other app test must keep passing,
`ReaderMore.test.tsx`, `Redirect.test.tsx`, `Undo.test.tsx` and the
keymap tests included.

## Gotchas

- The selected message is one article among its conversation's; the
  shortcut acts on that one only.
- A request kept in the reader must not run again when another message
  is shown: count requests, and remember the last one an article ran.
- Biome checks hook dependencies; add what a handle's callback reads.

## Out of scope

Redo, maild, the mock, and every file not under `touch`.

## Done when

`make accept T=0131` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
