---
id: "0029"
title: Draw message list rows
milestone: M2
size: M
touch:
  - app/src/features/list/MessageRow.tsx
given:
  - app/src/features/list/MessageRow.test.tsx
acceptance: make ui-vitest F=src/features/list/MessageRow.test.tsx
---
# T-0029: Draw message list rows

## Goal

The message list shows each message as a Mail.app row: sender and date,
subject and thread count, a two-line preview, and the unread dot and flag in
the left gutter (`docs/specs/ui.md`, Message list). Implement `MessageRow`;
the planner's list container virtualizes it and handles selection.

## Read first

- `app/src/features/list/MessageRow.tsx`: `ROW_HEIGHT`, `SelectMode`,
  `MessageRowProps` and the stub.
- `docs/specs/ui.md`, "Message list".
- `app/src/lib/format.ts` (`formatListDate`, `displayName`) and
  `app/src/lib/flags.ts` (`flagName`).
- `app/src/styles/app.css`: the colors and the `list-*` type roles.
- The given test `app/src/features/list/MessageRow.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names; replace the stub's doc comment and the file's
"Task T-0029 …" sentence with a description of the row.

**Root:** a `div` with `role="option"`, `tabIndex={-1}`,
`aria-selected={selected}`, `data-message-id={message.id}`, classes
`relative h-[84px] pt-2 pb-2 pl-6 pr-3`, plus:
- selected and focused: `bg-accent text-accent-contrast`, and every inner
  element that is otherwise `text-secondary`, `text-tertiary` or
  `bg-accent` uses `text-accent-contrast` / `bg-accent-contrast` instead;
- selected, not focused: `bg-selection-inactive`;
- not selected: neither.

A separator: an absolutely positioned 1px `bg-separator` line at the bottom
from 24px to the right edge (`absolute bottom-0 left-6 right-0 h-px`),
hidden while selected.

**Line 1** (`flex items-center gap-1`): the sender, `displayName(from)`, in
a `span` with `text-list-sender truncate flex-1`; then, when
`hasAttachments`, a Lucide `Paperclip` of size 12 with
`aria-label="Has attachments"`; then the date,
`formatListDate(new Date(message.date), now)`, in a `span` with
`text-list-date text-secondary tabular-nums shrink-0`.

**Line 2** (`flex items-center gap-1`): the subject in a `span` with
`text-list-subject truncate flex-1`; an empty or blank subject shows
`(No Subject)` with `text-tertiary` added. When `showThreadCount` and
`threadCount > 1`: a `span` with `aria-label="<n> messages"`, text `n`,
classes `h-4 px-[5px] rounded-lg bg-badge text-badge-text text-[10px]
font-semibold leading-4 shrink-0`.

**Preview:** a `div` with `text-list-preview text-secondary line-clamp-2`.

**Gutter:**
- unseen: a `span` with `aria-label="Unread"`, classes `absolute
  left-[7.5px] top-[12.5px] size-[9px] rounded-full bg-accent`;
- flagged: a Lucide `Flag` of size 12 with
  `aria-label={"Flagged " + flagName(flagColor)}`, classes `absolute
  left-[6px] top-[29px] fill-current text-flag-<flagColor>` (use a lookup
  of the seven full class names, not string building, so Tailwind sees
  them).

**Events:**
- click: `onSelect(id, mode)` with `"toggle"` when Ctrl or Meta is held,
  else `"range"` when Shift is held, else `"replace"`;
- context menu: `preventDefault()`, then `onContextMenu(id, clientX,
  clientY)`.

## Tests (given, do not edit)

`app/src/features/list/MessageRow.test.tsx`.

## Gotchas

- Tailwind only generates classes that appear whole in the source:
  `text-flag-${n}` does not work; list the seven classes.
- Lucide icons pass `aria-label` and `className` through to the `svg`.
- Do not wrap the row in a `button`; the list container owns focus and
  keyboard handling.

## Out of scope

Virtualization, selection state and keyboard handling (the planner's
`ListContainer`), and every file except
`app/src/features/list/MessageRow.tsx`.

## Done when

`make accept T=0029`, `make check` and `make ui-check` pass, and only
`app/src/features/list/MessageRow.tsx` changed.
