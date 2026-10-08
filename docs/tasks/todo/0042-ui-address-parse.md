---
id: "0042"
title: Parse typed recipients into addresses
milestone: M3
size: S
touch:
  - app/src/lib/addressParse.ts
given:
  - app/src/lib/addressParse.test.ts
acceptance: make ui-vitest F=src/lib/addressParse.test.ts
---
# T-0042: Parse typed recipients into addresses

## Goal

The compose window's recipient fields turn typed or pasted text into address
tokens, and mark what does not parse (`docs/design/send.md`, Compose
windows). Implement the two pure functions the recipient field calls.

## Read first

- `app/src/lib/addressParse.ts`: `ParsedAddresses` and the stubs.
- `app/src/rpc/gen/api.ts`: the `Address` type (`name`, `address`).
- `docs/design/send.md`, "Compose windows".
- The given test `app/src/lib/addressParse.test.ts`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the types and signatures; replace each stub's doc comment and the
"Task T-0042 …" sentence in the file comment with the rules below.

- **`isValidAddress(text)`:** `text` as given (no trimming) is one or more
  characters other than whitespace and `@`, then `@`, then a domain of at
  least two dot-separated labels, each one or more characters other than
  whitespace, `@` and `.`. One regular expression is enough.
- **`parseAddresses(text)`:**
  1. Split `text` on `,`, `;` and newlines, except inside double quotes
     (`"Smith, Bob" <bob@x.test>` is one piece). Walk the characters and
     toggle an "in quotes" flag on each `"`.
  2. Trim each piece; skip empty ones.
  3. A piece of the form `name <addr>` (the name may be empty or in double
     quotes) gives `{ name, address: addr }`: the name trimmed with its
     surrounding quotes removed, the address trimmed. It parses only if
     `isValidAddress(addr)`.
  4. Any other piece parses if `isValidAddress(piece)`, with name `""`.
  5. A piece that does not parse goes to `invalid` as trimmed.
  6. An address whose lowercased form was already added is skipped (the
     first one wins, name included). Addresses keep the case they were typed
     in.

Blank text gives `{ addresses: [], invalid: [] }`.

## Tests (given, do not edit)

`app/src/lib/addressParse.test.ts`: `isValidAddress` cases and the
`parseAddresses` cases (separators, names, invalid pieces, repeats, blank
text).

## Gotchas

- `"Bob <bob@>"` is invalid as a whole: report the trimmed piece, not just
  the address inside the angle brackets.
- Match the angle form with an anchored expression such as
  `/^(.*)<([^<>]*)>$/` on the trimmed piece, then strip quotes from the
  name.
- No `any`; `noUncheckedIndexedAccess` is on, so regular expression groups
  are `string | undefined`.

## Out of scope

The recipient field component (T-0043), suggestions, and every file except
`app/src/lib/addressParse.ts`.

## Done when

`make accept T=0042` and `make ui-check` pass, and only
`app/src/lib/addressParse.ts` changed.
