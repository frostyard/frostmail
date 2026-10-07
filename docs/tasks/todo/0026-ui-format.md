---
id: "0026"
title: Format dates, sizes, names and avatars for the UI
milestone: M2
size: S
touch:
  - app/src/lib/format.ts
given:
  - app/src/lib/format.test.ts
acceptance: make ui-vitest F=src/lib/format.test.ts
---
# T-0026: Format dates, sizes, names and avatars for the UI

## Goal

The message list, reader header and attachment chips show dates, sizes,
names and avatars the way Mail.app does (`docs/specs/ui.md`). Implement the
pure formatting helpers they share.

## Read first

- `app/src/lib/format.ts`: the stubs and their signatures.
- `docs/specs/ui.md`: "Message list" (Dates), "Reader" (Header, Attachments).
- The given test `app/src/lib/format.test.ts`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep every signature. Replace each doc comment with one that states the
rules below for that function, and remove the file's "Task T-0026 …" line.
`locale` parameters are passed straight to `Intl`/`toLocale…` (undefined
means the runtime's locale).

- **`formatListDate(date, now, locale)`**, by local calendar days between
  `date` and `now` (compare `getFullYear/getMonth/getDate`, not
  milliseconds):
  - same day: `date.toLocaleTimeString(locale, { hour: "numeric", minute: "2-digit" })`;
  - one day before: `"Yesterday"`;
  - 2 to 6 days before: `date.toLocaleDateString(locale, { weekday: "long" })`;
  - anything else, including later days: `date.toLocaleDateString(locale,
    { year: "2-digit", month: "numeric", day: "numeric" })`.
- **`formatHeaderDate(date, locale)`**: `new Intl.DateTimeFormat(locale,
  { dateStyle: "long", timeStyle: "short" }).format(date)`.
- **`formatSize(bytes)`**, decimal units: below 1000, `"N bytes"` (`"1 byte"`
  for 1); then `KB = Math.round(bytes / 1000)`, shown as `"N KB"` while
  below 1000; otherwise MB (`bytes / 1e6`) and, from 1000 MB, GB
  (`bytes / 1e9`), rounded to one decimal with a trailing `.0` dropped
  (`"1.4 MB"`, `"2 MB"`).
- **`formatCount(n, locale)`**: `n.toLocaleString(locale)`.
- **`displayName(a)`**: the trimmed name if not empty, else the trimmed
  address if not empty, else `"Unknown Sender"`.
- **`formatAddressList(list, max = 3)`**: the `displayName`s joined with
  `", "`; when the list is longer than `max`, the first `max` followed by
  `" & N more"` (N = the rest).
- **`initials(a)`**: split the name on whitespace, strip every character
  that is not a Unicode letter (`/\p{L}/u`) from each word, drop empty
  words; take the first letter of the first word and, if there are two or
  more words, of the last word. With no letters in the name, the first
  letter of the address (before `@`). Uppercased; `"?"` when there is none.
- **`avatarTone(address)`**: 32-bit FNV-1a over the UTF-8 bytes of
  `address.toLowerCase()` (offset basis `0x811c9dc5`, prime `0x01000193`,
  multiply with `Math.imul` and keep unsigned with `>>> 0`), modulo 8.
  Encode with `new TextEncoder()`.

## Tests (given, do not edit)

`app/src/lib/format.test.ts`.

## Gotchas

- Calendar-day difference: build dates at local midnight
  (`new Date(y, m, d)`) for both values and divide the difference by
  86,400,000 with `Math.round` (daylight-saving days are 23 or 25 hours).
- Time strings may contain a narrow no-break space before AM/PM; return
  the `toLocale…` result unchanged.

## Out of scope

Components that use these helpers, and every file except
`app/src/lib/format.ts`.

## Done when

`make accept T=0026`, `make check` and `make ui-check` pass, and only
`app/src/lib/format.ts` changed.
