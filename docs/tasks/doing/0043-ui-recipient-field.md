---
id: "0043"
title: Build the recipient token field
milestone: M3
size: M
touch:
  - app/src/features/compose/RecipientField.tsx
given:
  - app/src/features/compose/RecipientField.test.tsx
acceptance: make ui-vitest F=src/features/compose/RecipientField.test.tsx
---
# T-0043: Build the recipient token field

## Goal

The compose window's To, Cc and Bcc fields turn typed and pasted text into
address tokens and suggest addresses as the user types, like Mail.app
(`docs/specs/compose-ui.md`). Build `RecipientField`, the presentational
token field; the container gives it the addresses and a `suggest` callback.

## Read first

- `app/src/features/compose/RecipientField.tsx`: the props and the stub.
- `docs/specs/compose-ui.md`: "Recipient field" under Layout and
  "Recipients" under Behavior.
- `app/src/lib/addressParse.ts`: `parseAddresses` and `isValidAddress`
  (T-0042).
- The given test `app/src/features/compose/RecipientField.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names and props; replace the stub's doc comment and the
"Task T-0043 …" sentence in the file comment with what the component does.

**Structure** (one wrapper `div`: `relative flex min-w-0 flex-1 flex-wrap
items-center gap-1 py-1`):

1. One token per address in `value`: a `button` with `type="button"`,
   `tabIndex={-1}`, its text the name or, when the name is empty, the
   address; `title` "Name <address>" (or the address); `aria-pressed` true
   only for the selected token. A token whose address fails
   `isValidAddress` has `data-invalid="true"` and the title
   "<address> is not a valid address". Classes: `h-[22px] rounded px-1.5
   text-[13px] leading-[18px]`, plus `bg-accent text-accent-contrast` when
   selected, else `text-flag-1` when invalid, else `text-accent`.
2. The `input`: `type="text"`, `role="combobox"`, `aria-label={label}`,
   `aria-expanded` (true while the list shows), `aria-controls` (the list's
   id, from `useId`), `aria-autocomplete="list"`, `aria-activedescendant`
   (the highlighted option's id, else undefined), `autoFocus={autoFocus}`;
   classes `min-w-[80px] flex-1 border-none bg-transparent text-[13px]
   leading-[18px] text-primary outline-none`. The typed text lives in
   state; it is not part of `value`.
3. While there are suggestions to show: a `div` with `role="listbox"`, the
   id from `aria-controls`, `aria-label={`${label} suggestions`}`, classes
   `absolute top-full left-0 z-10 w-full overflow-y-auto rounded-md border
   border-separator bg-window shadow-md`, holding at most 8 options: each a
   `div` with `role="option"`, `tabIndex={-1}`, the id `${listId}-${index}`,
   `aria-selected` (true only when highlighted), classes `flex h-9 flex-col
   justify-center px-2` plus `bg-accent text-accent-contrast` when
   highlighted; inside, the name in a `span` (13/18) only when it is not
   empty, then the address in a `span` (12/16, `text-secondary` unless
   highlighted).

**Behavior** (spec, Behavior: Recipients):

- *Committing* text parses it with `parseAddresses`; it adds the parsed
  addresses, then one `{ name: "", address: piece }` per invalid piece,
  skipping any address the list already has (case-insensitively, including
  ones added in the same commit). It calls `onChange` only when something
  was added, clears the text and closes the list.
- Keys in the input (`onKeyDown`):
  - ArrowDown / ArrowUp while the list shows: move the highlight (none at
    first; ArrowDown from none is the first, ArrowUp from none the last;
    clamp at the ends) and `preventDefault`.
  - Escape while the list shows: close it (keep the text).
  - Enter or Tab with a highlighted option: add that address (as a pick,
    below) and `preventDefault`.
  - Enter, `,` or `;`: `preventDefault` and commit the text if it is not
    blank.
  - Tab: commit the text if it is not blank, without `preventDefault`, so
    focus moves on.
  - Backspace or Delete with empty text: when a token is selected, remove
    it (`onChange` without it) and clear the selection; otherwise Backspace
    selects the last token. `preventDefault` in both cases.
- Typing (`onChange` of the input) sets the text, clears the selection and
  the highlight, and, when the trimmed text is not empty, calls
  `suggest(trimmed)`; when the promise resolves, the results are kept only
  if the trimmed text is still the same (keep the latest text in a `useRef`
  for this check). Blank text clears the suggestions without calling
  `suggest`.
- The list shows the suggestions whose address is not already in `value`,
  at most 8; it is open when that list is not empty.
- Picking an option (Enter/Tab on the highlight, or a mouse press on it):
  add the address, clear the text and the list, and keep focus in the
  input. Use `onMouseDown` with `preventDefault` on options so the input
  never loses focus.
- A mouse press on a token selects it and focuses the input (again
  `onMouseDown` with `preventDefault`, then select in `onClick`).
- Paste (`onPaste`): when the pasted text contains `,`, `;` or a line
  break, `preventDefault` and commit the current text plus the pasted text;
  otherwise let it paste.
- When the input loses focus: clear the selection, then commit the text if
  it is not blank, else close the list.

## Tests (given, do not edit)

`app/src/features/compose/RecipientField.test.tsx` (13 tests: tokens,
commits on separators, Enter, Tab and blur, invalid tokens, paste,
Backspace and Delete, suggestions with the keyboard and the mouse, Escape,
stale suggestions).

## Gotchas

- Clicks on options and tokens must not blur the input: a blur would
  commit the half-typed text as an invalid token.
- Biome wants `role="option"` elements focusable: that is what
  `tabIndex={-1}` is for. The `autoFocus` prop needs a
  `// biome-ignore lint/a11y/noAutofocus: <reason>` comment on the line
  above it.
- Key tokens and options by the lowercased address.
- `noUncheckedIndexedAccess` is on: `visible[highlight]` is
  `Address | undefined`.

## Out of scope

`ComposeHeader` (T-0044), the container that calls `address.suggest`, and
every file except `app/src/features/compose/RecipientField.tsx`.

## Done when

`make accept T=0043`, `make check` and `make ui-check` pass, and only
`app/src/features/compose/RecipientField.tsx` changed.
