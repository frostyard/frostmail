---
id: "0102"
title: Flag, archive and delete from a message row
milestone: M4.5
size: S
touch:
  - app/src/features/list/MessageRow.tsx
  - app/src/app/ListContainer.tsx
given:
  - app/src/features/list/MessageRow.actions.test.tsx
  - app/src/app/RowActions.test.tsx
acceptance: make ui-vitest F="src/features/list src/app/RowActions"
---
# T-0102: Flag, archive and delete from a message row

## Goal

The user wants to flag, archive or delete a message without going up to
the toolbar. While the pointer is over a row, its date gives way to three
small buttons that act on that row's message alone.

## Read first

- `docs/specs/ui.md`: "Message list", the "Row actions" bullet.
- `app/src/features/list/MessageRow.tsx` and its test (contract for
  T-0029; it must keep passing).
- `app/src/app/ListContainer.tsx`: how rows are rendered, `onDelete`, and
  the menu's Archive (`archiveMailbox`, `moveMessages`).
- `app/src/app/commands.ts`: `toggleFlag`, `archiveOf`, `archiveMailbox`,
  `moveMessages`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`MessageRow`:**
  - Exports `type RowAction = "flag" | "archive" | "delete"`.
  - New optional props:
    - `onAction?: (id: number, action: RowAction) => void`: without it
      the row shows no buttons and nothing else changes.
    - `canArchive?: boolean`.
  - The root `div` also gets the class `group`.
  - **Line 1, with `onAction`:**
    - The paperclip and the date move into a
      `span.flex.shrink-0.items-center.gap-1` that also has
      `group-hover:hidden`. Without `onAction` that span has no
      `group-hover:hidden`.
    - After it comes a `span` with `role="group"`,
      `aria-label="Message actions"` and classes including
      `hidden shrink-0 items-center group-hover:flex`.
  - **The group holds `button type="button" tabIndex={-1}`s, in order:**
    - Flag (`aria-label` "Flag", or "Unflag" with the `Flag` icon given
      `className="fill-current"` when the message is flagged).
    - Archive (`Archive` icon, only when `canArchive`).
    - Delete (`Trash2` icon).
  - **Each button:**
    - Icons are Lucide at `size={14}`.
    - `title` is the label with the shortcut: "Flag (Ctrl+Shift+L)",
      "Unflag (Ctrl+Shift+L)", "Archive (Ctrl+Alt+A)", "Delete (Delete)".
    - Classes include `flex h-[18px] w-[22px] items-center justify-center
      rounded border-0 bg-transparent p-0`.
    - Plus `text-secondary hover:bg-selection-inactive`, or on a selected
      row in a focused list `text-accent-contrast
      hover:bg-accent-contrast/20`.
  - **Events:**
    - `onMouseDown` prevents the default, so focus stays put.
    - `onDoubleClick` stops propagation, so the list does not open a
      draft.
    - `onClick` stops propagation (no selection) and calls
      `onAction(message.id, action)`.
- **`ListContainer`:**
  - Each row gets `onAction` and
    `canArchive={archiveOf(row.accountId, mailboxes) !== undefined}`.
  - `onAction` is undefined for rows of a read-only account (`useMail`'s
    `accounts`).
  - The handler acts on `[id]` alone, never the selection:
    - `"flag"`: `toggleFlag(client, model, [id])`.
    - `"archive"`: `moveMessages(client, [id], archiveMailbox(model, [id],
      mailboxes), source, mailboxes)`.
    - `"delete"`: `onDelete([id])`.

## Tests (given, do not edit)

`MessageRow.actions.test.tsx` and `RowActions.test.tsx` (the window against
the mock maild). `MessageRow.test.tsx` and every other app test must keep
passing.

## Gotchas

- Biome's `useSemanticElements` asks for a `fieldset` instead of
  `role="group"`; keep the `span` and add
  `// biome-ignore lint/a11y/useSemanticElements: a fieldset's border and legend do not belong in a list row.`
  on the line before it, inside the JSX.
- Tailwind only generates classes it finds whole in the source: write
  every class literally.
- Cards T-0101 and T-0103 change other parts of `ListContainer.tsx` at the
  same time; touch only the row rendering and the lines this needs.

## Out of scope

maild, the context menu, the toolbar, keyboard shortcuts, and every file
not under `touch`.

## Done when

`make accept T=0102` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
