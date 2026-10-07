---
id: "0015"
title: Map IMAP flags and Mail.app flag colors
milestone: M1
size: S
touch:
  - internal/store/flags.go
given:
  - internal/store/flags_test.go
acceptance: go test ./internal/store -run 'TestFlagsFromIMAP|TestIMAPFlags|TestFlagsRoundTrip' -count=1
---
# T-0015: Map IMAP flags and Mail.app flag colors

## Goal

The server reports flags as IMAP flags and keywords; maild stores and shows
them as `store.Flags`. Mail.app stores a flag's color in the keywords
`$MailFlagBit0`, `$MailFlagBit1` and `$MailFlagBit2`. Implement the two stub
functions in `internal/store/flags.go` so frostmail reads and writes the same
colors as Mail.app.

## Read first

- `internal/store/flags.go`: the `Flags` type and the two stubs.
- The given test `internal/store/flags_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the `Flags` type unchanged. Improve the two doc comments to state the
rules below.

**`FlagsFromIMAP(flags []string) Flags`**, comparing names without case:

- `\Seen`, `\Flagged`, `\Answered`, `\Draft`, `\Deleted` set their fields.
  Every other name starting with `\` (such as `\Recent` and `\*`) is ignored.
- `$Forwarded` sets `Forwarded`.
- `$MailFlagBit0`, `$MailFlagBit1` and `$MailFlagBit2` set bits 1, 2 and 4 of a
  number `bits`. `Color` is `bits + 1` when `Flagged`, else 0.
- Every other non-empty name is a keyword. Keep the first spelling of each,
  drop later ones that differ only in case, and sort by lowercase. `Keywords`
  is nil when there are none.

**`(f Flags) IMAPFlags() []string`**, in this order: `\Seen`, `\Answered`,
`\Flagged`, `\Deleted`, `\Draft` (each if set), `$Forwarded` (if set), then,
when `Flagged` and `Color > 1`, `$MailFlagBit0`, `$MailFlagBit1` and
`$MailFlagBit2` for the set bits of `Color - 1`, then `Keywords` as they are.
Return nil when the list is empty.

## Tests (given, do not edit)

`internal/store/flags_test.go`: TestFlagsFromIMAP (12 cases), TestIMAPFlags
(7 cases), TestFlagsRoundTrip (every color).

## Gotchas

- Red is `Color` 1 with no bits; a color without `Flagged` means nothing in
  either direction.
- Use `strings.EqualFold` or compare `strings.ToLower` forms; keep the
  original spelling in `Keywords`.

## Out of scope

Storing flags in the database (T-0013), and every file except
`internal/store/flags.go`.

## Done when

`make accept T=0015` and `make check` pass, and only
`internal/store/flags.go` changed.
