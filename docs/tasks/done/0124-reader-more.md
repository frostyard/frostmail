---
id: "0124"
title: The reader's More Actions, All Headers, Raw Source, Save As, Print
milestone: M5
size: M
touch:
  - app/src/features/reader/MessageHeader.tsx
  - app/src/features/reader/RawSourceSheet.tsx
  - app/src/app/ReaderContainer.tsx
  - app/src/styles/app.css
given:
  - app/src/features/reader/RawSourceSheet.test.tsx
  - app/src/app/ReaderMore.test.tsx
acceptance: make ui-vitest F="src/features/reader src/app/ReaderMore src/app/RemindMe"
---
# T-0124: The reader's More Actions, All Headers, Raw Source, Save As, Print

## Goal

A More Actions menu on each message in the reader, as in Mail.app: show all
headers, see the raw source, save the message as an .eml file, and print
it (and with the print dialog's Print to File, export a PDF).

## Read first

- `docs/specs/ui.md`: "Reader", the More Actions bullet;
  `docs/design/app.md`: "Raw Source, All Headers, Save As, Print".
- `app/src/features/reader/MessageHeader.tsx` (the header, `RemoteBanner`),
  `app/src/app/ReaderContainer.tsx` (`ConversationMessage`; the reader
  pane's `section`), `app/src/features/menu/ContextMenu.tsx`,
  `app/src/features/organize/SmartSheet.tsx` (the sheet frame), and
  `app/src/styles/app.css`.
- `app/src/rpc/gen/api.ts`: `message.source`, `message.save`,
  `MessageSource`. Tauri's `save` dialog from `@tauri-apps/plugin-dialog`
  (the capability `dialog:allow-save` is granted).
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`MessageHeader`**: an optional prop `onMore(at: {x, y})`. When it is
  given, a 28px button named "More Actions" (title too,
  `aria-haspopup="menu"`, Lucide `Ellipsis` 16px, `text-secondary`,
  `print:hidden`) follows the date and calls it with the button's
  bottom-left.
- **`RawSourceSheet`** (`RawSourceSheet.tsx`), props `text`, `truncated`,
  `onClose`: a dialog named "Raw Source" (the sheet frame, 720 wide) with
  `text` in a `pre`, "The message is longer than what is shown." when
  `truncated`, and a Close button that has focus when it opens; Escape
  closes too.
- **`ReaderContainer`**, per message:
  - The header's `onMore` opens a `ContextMenu`: "Show All Headers" (or
    "Hide All Headers" while shown), "Raw Source…", a separator, "Save
    As…", "Print…".
  - Show All Headers calls `message.source {id}` and shows `headers` in a
    `section` named "All Headers" holding a `pre` (11px monospace,
    `text-secondary`, wrapped); Hide removes it.
  - Raw Source… calls `message.source` and opens the sheet with `text`
    and `truncated`.
  - Save As…, in Tauri only, opens the `save` dialog as the spec says and
    calls `message.save {id, path}` with the chosen path; nothing on
    cancel, and nothing outside Tauri.
  - Print… calls `window.print()`.
  - The reader pane's `section` has the attribute `data-print-area`.
- **`app.css`**: an `@media print` block that shows only `[data-print-area]`
  and what is inside it, placed at the page's top-left, full width,
  unscrolled (`height: auto; overflow: visible`).

## Tests (given, do not edit)

`RawSourceSheet.test.tsx`, `ReaderMore.test.tsx`. Every other app test must
keep passing, `RemindMe.test.tsx` and the MessageHeader tests included.

## Gotchas

- `window.print` may not exist in the test DOM; the test stubs it, so call
  it as `window.print()`.
- Biome refuses `aria-label` on a `pre`: name the `section`.
- Escape inside the sheet must not reach the window's key handler (stop
  propagation, as the other sheets do).

## Out of scope

Printing more than the reader pane, maild, the mock, and every file not
under `touch`.

## Done when

`make accept T=0124` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
