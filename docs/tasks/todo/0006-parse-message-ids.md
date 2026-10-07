---
id: "0006"
title: Parse Message-ID lists for threading
milestone: M1
size: S
touch:
  - internal/mimex/msgid.go
given:
  - internal/mimex/msgid_test.go
acceptance: go test ./internal/mimex -run TestParseMessageIDs -count=1
---
# T-0006: Parse Message-ID lists for threading

## Goal

Threading links a message to its parents through the Message-IDs in its
`References` and `In-Reply-To` headers. Real headers are messy: folded,
commented, comma-separated, or free text around the ID. Add
`ParseMessageIDs` to package `mimex`.

## Read first

- `internal/mimex/subject.go`: the package's style (`mimex` exists already).
- The given test `internal/mimex/msgid_test.go`: every case it checks.
- `docs/tasks/EXECUTOR.md`

## Contract

Create `internal/mimex/msgid.go` with
`func ParseMessageIDs(s string) []string` and a doc comment. Scan `s` once,
left to right:

1. Outside angle brackets, `(` starts a comment that ends at the matching
   `)`. Comments nest, an unterminated comment runs to the end, and
   everything inside a comment is ignored.
2. Outside comments, `<` starts an ID that ends at the next `>` or at the end
   of the string. The ID is the text between, with every whitespace
   character removed. Empty IDs are skipped.
3. If the string held at least one `<` outside comments, the result is the
   bracketed IDs only; text outside brackets is ignored.
4. Otherwise, split the comment-free text on whitespace and commas, and keep
   the tokens that contain `@`.
5. Remove duplicates, keeping the first occurrence; keep the order.

Return nil when there are no IDs.

## Tests (given, do not edit)

`internal/mimex/msgid_test.go`: TestParseMessageIDs (20 cases).

## Gotchas

- Use `unicode.IsSpace` to drop whitespace inside an ID; folded headers put
  `\r\n` and tabs there.
- Parentheses inside `<...>` are part of the ID, not a comment.

## Out of scope

Validating ID syntax, and every file except `internal/mimex/msgid.go`.

## Done when

`make accept T=0006` and `make check` pass, and only
`internal/mimex/msgid.go` changed.
