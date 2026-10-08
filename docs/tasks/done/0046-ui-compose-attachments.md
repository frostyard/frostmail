---
id: "0046"
title: Show a draft's attachments
milestone: M3
size: S
touch:
  - app/src/features/compose/ComposeAttachments.tsx
given:
  - app/src/features/compose/ComposeAttachments.test.tsx
acceptance: make ui-vitest F=src/features/compose/ComposeAttachments.test.tsx
---
# T-0046: Show a draft's attachments

## Goal

The compose window lists the files attached to a draft under the editor,
with their sizes, a way to remove each, the files still being attached, and
the total against the size limit (`docs/specs/compose-ui.md`). Build the
presentational strip.

## Read first

- `app/src/features/compose/ComposeAttachments.tsx`: the props and the stub.
- `docs/specs/compose-ui.md`: "Attachments" under Layout and Behavior.
- `app/src/lib/format.ts`: `formatSize`.
- `app/src/rpc/gen/api.ts`: `DraftAttachment`.
- The given test `app/src/features/compose/ComposeAttachments.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported types and names; replace the stub's doc comment and the
file's "Task T-0046 …" sentence with what the component does.

- With no attachments and nothing pending, render `null`.
- Otherwise a `section` with `aria-label="Attachments"` and classes `flex
  flex-wrap items-center gap-2 border-t border-separator bg-banner px-3
  py-2`, holding:
  1. One chip per attachment: a `div` with a `data-attachment` attribute
     and classes `flex h-8 items-center gap-1.5 rounded-md border
     border-separator bg-window px-2`, holding the icon (16px, `shrink-0
     text-secondary`), the file name in a `span` with `title` = the file
     name and classes `max-w-[200px] truncate text-[13px] leading-[18px]`,
     the size (`formatSize`) in a `span` (`text-[12px] leading-4
     text-secondary`), and a `button type="button"` with `aria-label` and
     `title` "Remove <file name>" showing Lucide `X` at 14px, which calls
     `onRemove(id)`.
  2. One chip per pending file, after the attachments: the same `div` with
     `aria-busy="true"`, a spinning Lucide `LoaderCircle` (16px, `shrink-0
     animate-spin text-secondary`) and the name `span`; no size, no button.
  3. When there is at least one attachment, the total in a `span` with
     classes `ml-auto text-[12px] leading-4 tabular-nums` plus
     `text-flag-1` when the total is over `limit`, else `text-secondary`:
     "1 file, <size>" or "N files, <size>" (attachments only, sizes
     summed, `formatSize`), followed over the limit by
     " — over the <formatSize(limit)> limit" (an em dash with spaces).
- Icons by `contentType`: `FileImage` for `image/*`; `FileText` for
  `text/*` and `application/pdf`; `FileArchive` for `application/zip`,
  `application/gzip`, `application/x-gzip`, `application/x-tar` and
  `application/x-7z-compressed`; `File` otherwise.

## Tests (given, do not edit)

`app/src/features/compose/ComposeAttachments.test.tsx`.

## Gotchas

- Key attachment chips by `id` and pending chips by `key`.
- Pending files do not count in the total.
- Use a `section` with `aria-label`, not a `div` with `role="region"`
  (Biome prefers the element).

## Out of scope

The file dialog, drag and drop, and the container; every file except
`app/src/features/compose/ComposeAttachments.tsx`.

## Done when

`make accept T=0046`, `make check` and `make ui-check` pass, and only
`app/src/features/compose/ComposeAttachments.tsx` changed.
