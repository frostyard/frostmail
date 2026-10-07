---
id: "0034"
title: Map key presses to commands
milestone: M2
size: S
touch:
  - app/src/lib/keymap.ts
given:
  - app/src/lib/keymap.test.ts
acceptance: make ui-vitest F=src/lib/keymap.test.ts
---
# T-0034: Map key presses to commands

## Goal

The window has one keyboard listener that turns key presses into commands
for the focused pane (`docs/design/app.md`, Window). Implement `commandFor`,
the pure mapping from a key press to a `Command`, following the keyboard map
in `docs/specs/ui.md`.

## Read first

- `app/src/lib/keymap.ts`: `Command`, `KeyInput` and the stub.
- `docs/specs/ui.md`, "Keyboard map".
- The given test `app/src/lib/keymap.test.ts`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the types and the signature; replace the stub's doc comment and the
file's "Task T-0034 …" sentence with the rules below.

Bindings. A binding matches when the key matches and **all four modifiers
match exactly** (Ctrl, Alt, Shift as listed, Meta never). Letter keys match
case-insensitively (compare `key.toLowerCase()`); Space is the key `" "`.

| Binding | Command |
| --- | --- |
| ArrowUp / ArrowDown | previous / next |
| Shift+ArrowUp / Shift+ArrowDown | extendPrevious / extendNext |
| Home / End | first / last |
| Ctrl+A | selectAll |
| Space / Shift+Space | pageDown / pageUp |
| Delete, Backspace | delete |
| Ctrl+Alt+A | archive |
| Ctrl+Shift+U | toggleRead |
| Ctrl+Shift+L | toggleFlag |
| Ctrl+Alt+F, Ctrl+F | focusSearch |
| Escape | escape |
| Ctrl+Shift+N | getMail |
| Ctrl+1 | allInboxes |
| Ctrl+Alt+S | toggleSidebar |
| Tab / Shift+Tab | nextPane / previousPane |
| ContextMenu, Shift+F10 | contextMenu |

Any press with Meta returns null. Anything not in the table returns null.

**In a text field** (`inTextField` true) only these still apply: Escape,
Tab, Shift+Tab, Ctrl+1, and bindings that combine Ctrl with Alt or with
Shift. Every other press returns null, so the field keeps arrows, Space,
Delete, Backspace, Ctrl+A and Ctrl+F.

Write the table as a constant array of `{ key, ctrl, alt, shift, command }`
entries (several entries may share a command) and search it.

## Tests (given, do not edit)

`app/src/lib/keymap.test.ts`.

## Gotchas

- With Shift held, letter keys arrive uppercase (`"U"`); without it,
  lowercase. Both must match.
- `Ctrl+Shift+A` is not `Ctrl+A`: modifiers must match exactly.

## Out of scope

Dispatching commands (the planner's window listener), and every file except
`app/src/lib/keymap.ts`.

## Done when

`make accept T=0034`, `make check` and `make ui-check` pass, and only
`app/src/lib/keymap.ts` changed.
