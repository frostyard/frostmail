---
id: "0008"
title: Build message list previews from text
milestone: M1
size: S
touch:
  - internal/mimex/preview.go
given:
  - internal/mimex/preview_test.go
acceptance: go test ./internal/mimex -run TestPreview -count=1
---
# T-0008: Build message list previews from text

## Goal

Message lists show a preview: the first words the sender actually wrote,
without quoted replies, "On … wrote:" lines or signatures. Add `Preview` to
package `mimex`.

## Read first

- `internal/mimex/subject.go`: the package's style.
- The given test `internal/mimex/preview_test.go`: every case it checks.
- `docs/tasks/EXECUTOR.md`

## Contract

Create `internal/mimex/preview.go` with
`func Preview(text string, max int) string` and a doc comment.

1. `max <= 0` returns `""`.
2. Replace `\r\n` with `\n` and split into lines.
3. Walk the lines in order:
   - **Stop** (drop this line and every later line) at a signature or a
     forwarded-message separator: a line whose right-trimmed form is `--`
     (so `-- ` too); a line whose trimmed form starts with
     `-----Original Message-----`; a trimmed line of at least 10 characters
     that are all `_`.
   - **Drop** a quoted line: its left-trimmed form starts with `>`.
   - **Drop** an attribution line: its trimmed form ends with `:`, its
     lowercased form contains `wrote`, `schrieb`, `écrit` or `escribió`, and
     the next non-blank line is a quoted line.
   - Keep every other line.
4. Join the kept lines with spaces, then collapse every run of whitespace to
   one space (`strings.Fields` and `strings.Join`).
5. If the result has at most `max` runes, return it. Otherwise take its first
   `max-1` runes. If that prefix contains a space at a rune index greater than
   `(max-1)/2`, cut at the last such space and trim trailing spaces. Append
   `…` (U+2026). The result never exceeds `max` runes.

## Tests (given, do not edit)

`internal/mimex/preview_test.go`: TestPreview (26 cases).

## Gotchas

- Count and cut in runes (`[]rune`), never bytes: the Japanese case breaks a
  byte cut.
- "Next non-blank line" skips blank lines: see the German case.

## Out of scope

HTML input (T-0007 converts HTML to text first), and every file except
`internal/mimex/preview.go`.

## Done when

`make accept T=0008` and `make check` pass, and only
`internal/mimex/preview.go` changed.
