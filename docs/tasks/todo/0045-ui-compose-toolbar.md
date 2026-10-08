---
id: "0045"
title: Build the compose toolbar and format bar
milestone: M3
size: S
touch:
  - app/src/features/compose/ComposeToolbar.tsx
given:
  - app/src/features/compose/ComposeToolbar.test.tsx
acceptance: make ui-vitest F=src/features/compose/ComposeToolbar.test.tsx
---
# T-0045: Build the compose toolbar and format bar

## Goal

The compose window has its own title bar with Send, the draft's title and
its actions, and a format bar that switches text styles in the editor
(`docs/specs/compose-ui.md`). Build both presentational components; the
container wires them to maild and to the TipTap editor.

## Read first

- `app/src/features/compose/ComposeToolbar.tsx`: the types and the stubs.
- `docs/specs/compose-ui.md`: "Toolbar" and "Format bar" under Layout,
  "Formatting" under Behavior.
- `app/src/features/toolbar/Toolbar.tsx`: the main window's toolbar; copy
  its button style (`BUTTON_CLASS`) and window controls.
- The given test `app/src/features/compose/ComposeToolbar.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported types and names; replace the stubs' doc comments and the
file's "Task T-0045 …" sentence with what each component does.

**`ComposeToolbar`**

- A `div` with `role="toolbar"`, `aria-label="Compose"`,
  `data-tauri-drag-region`, classes `flex h-[52px] shrink-0 items-center
  gap-1 border-b border-separator bg-toolbar px-2`, holding in order:
  1. Send: Lucide `Send` (16px), `title="Send (Ctrl+Enter)"`, disabled
     unless `canSend`; the main toolbar's button classes but with
     `text-accent` in place of `text-secondary`.
  2. A `span` with `data-tauri-drag-region` and classes `min-w-0 flex-1
     truncate px-2 text-toolbar-title`: the subject, or "New Message" when
     the trimmed subject is empty.
  3. Attach Files (`Paperclip`, title "Attach Files (Ctrl+Shift+A)"),
     Show Format Bar (`Type`, title "Show Format Bar", `aria-pressed` =
     `formatBarShown`), Delete Draft (`Trash2`, title "Delete Draft
     (Ctrl+Backspace)").
  4. Minimize (`Minus`), Maximize (`Square`) or, while `maximized`, Restore
     (`Copy`), and Close (`X`); each titled with its name.
- Every control is a `button type="button"` with an `aria-label` (its
  name) and the main toolbar's classes. Clicks call `onCommand` with
  `send`, `attach`, `toggleFormatBar`, `delete`, `minimize`,
  `toggleMaximize` or `close`.

**`FormatBar`**

- A `div` with `role="toolbar"`, `aria-label="Format"`, classes `flex h-8
  shrink-0 items-center gap-0.5 border-b border-separator bg-window px-2`.
- Three groups of toggles, with a `span` (`aria-hidden="true"`, classes
  `mx-1 h-4 w-px bg-separator`) between groups:
  1. Bold (`Bold`, key `bold`, title "Bold (Ctrl+B)"), Italic (`Italic`,
     `italic`, "Italic (Ctrl+I)"), Underline (`Underline`, `underline`,
     "Underline (Ctrl+U)"), Strikethrough (`Strikethrough`, `strike`,
     "Strikethrough").
  2. Bulleted List (`List`, `bulletList`), Numbered List (`ListOrdered`,
     `orderedList`), Quote (`TextQuote`, `blockquote`), each titled with its
     name.
  3. Link (`Link`, `link`, "Link (Ctrl+K)").
- Then Clear Formatting (`RemoveFormatting`, title "Clear Formatting"),
  which is not a toggle: no `aria-pressed`.
- Each control is a `button type="button"` with its name as `aria-label`,
  16px icon, classes `flex h-6 w-7 items-center justify-center rounded`
  plus `bg-selection-inactive text-primary` when pressed, else
  `text-secondary hover:bg-selection-inactive`. A toggle's `aria-pressed`
  is `active[key] === true`.
- Every control calls `preventDefault` in `onMouseDown` (so the editor
  keeps focus and its selection) and calls `onCommand(key)` on click;
  Clear Formatting sends `clear`.

## Tests (given, do not edit)

`app/src/features/compose/ComposeToolbar.test.tsx`.

## Gotchas

- Keep `text-secondary` out of Send's classes: two text colors in one
  class list make the result depend on CSS order.
- Grouping wrappers must not add buttons or drop the separators; a
  `div className="contents"` per group (keyed by its first toggle) keeps
  the layout flat.
- `active` is partial: a missing key is not pressed.

## Out of scope

The editor and its commands, the compose window container, and every file
except `app/src/features/compose/ComposeToolbar.tsx`.

## Done when

`make accept T=0045`, `make check` and `make ui-check` pass, and only
`app/src/features/compose/ComposeToolbar.tsx` changed.
