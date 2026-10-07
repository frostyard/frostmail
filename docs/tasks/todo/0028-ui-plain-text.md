---
id: "0028"
title: Render plain-text bodies with quotes and links
milestone: M2
size: M
touch:
  - app/src/features/reader/PlainText.tsx
given:
  - app/src/features/reader/PlainText.test.tsx
acceptance: make ui-vitest F=src/features/reader/PlainText.test.tsx
---
# T-0028: Render plain-text bodies with quotes and links

## Goal

Text-only messages render as React elements in the reader, Mail.app style:
quoted lines indented with colored bars instead of `>` prefixes, long quotes
collapsed, links clickable, the signature dimmed (`docs/specs/ui.md`,
Reader: Body). Implement the parser and the component.

## Read first

- `app/src/features/reader/PlainText.tsx`: `TextBlock`, `PlainTextProps`,
  the stubs.
- `docs/specs/ui.md`, "Reader" (Body) and the `--quote-N` tokens.
- `app/src/styles/app.css`: the `quote-1…3` colors and the `reader-body`
  type role.
- The given test `app/src/features/reader/PlainText.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names and types; replace the stubs' doc comments and the
file's "Task T-0028 …" sentence with the rules below.

**`parsePlainText(text)`**

1. Normalize `\r\n` to `\n`, split into lines, and drop leading and
   trailing lines that are empty or only whitespace. Nothing left: `[]`.
2. A line's quote level is the number of leading `>` characters, where a
   single space may separate consecutive markers (`">> a"`, `"> > a"` and
   `">>a"` are level 2). Its content drops the markers and at most one space
   after the last one.
3. The first line at level 0 that is exactly `"-- "` starts the signature:
   that line is dropped and every later line, unchanged and whatever its
   prefix, goes into one `{ kind: "signature", lines }` block at the end.
4. The other lines form `{ kind: "text", quote, lines }` blocks of
   consecutive lines at the same level, in order.

**`PlainText({ text, onOpenLink })`**

- The root `div` has `select-text whitespace-pre-wrap break-words
  text-reader-body p-5`.
- Each text block is one element with `data-quote-level="<level>"`. Quote
  blocks (level ≥ 1) also get `border-l-2 pl-2`, the class
  `border-quote-<k>` with `k = ((level - 1) % 3) + 1`, and a left margin of
  `(level - 1) * 10` px (inline style). Lines are joined with `\n`.
- A quote block with more than 4 lines starts collapsed, showing its first
  2 lines and a `button` "See More" with `aria-expanded="false"`; clicking
  shows every line and the button reads "See Less" with
  `aria-expanded="true"`. Shorter blocks have no button. The button is
  12px `text-accent`.
- The signature block has `data-signature` and `text-secondary`.
- **Links.** Within each line, find, left to right with one regular
  expression: `https?://` URLs and `www.` hosts (`[^\s<>"]+` after the
  prefix), and addresses (`[\w.+-]+@[\w-]+(\.[\w-]+)+`). Strip trailing
  `.,;:!?)` characters from a match; they stay as text. Render each match
  as `<a href>` (`text-accent`): the URL itself, `https://` + the match for
  `www.`, `mailto:` + the address for addresses. On click,
  `preventDefault()` and call `onOpenLink(href)`.

## Tests (given, do not edit)

`app/src/features/reader/PlainText.test.tsx`.

## Gotchas

- `parsePlainText` must be pure; the component calls it inside `useMemo`.
- Keep each collapsed block's state by block index in one `useState` set.
- Do not render the `>` markers anywhere, and never use
  `dangerouslySetInnerHTML`.

## Out of scope

HTML bodies (the planner's `MessageFrame`), and every file except
`app/src/features/reader/PlainText.tsx`.

## Done when

`make accept T=0028`, `make check` and `make ui-check` pass, and only
`app/src/features/reader/PlainText.tsx` changed.
