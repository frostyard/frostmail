---
id: "0003"
title: Normalize message subjects for threading
milestone: M0
size: S
touch:
  - internal/mimex/subject.go
given:
  - internal/mimex/subject_test.go
acceptance: go test ./internal/mimex -count=1
---
# T-0003: Normalize message subjects for threading

## Goal

Threading groups replies whose subjects match once reply and forward
prefixes and mailing-list tags are removed ("Re: [dev] Fwd: Plan" and "Plan"
belong together). Create `NormalizeSubject` in a new package
`internal/mimex`, frostmail's message-parsing helpers.

## Read first

- The given test `internal/mimex/subject_test.go`: every case it checks.
- `docs/tasks/EXECUTOR.md`

## Contract

Create `internal/mimex/subject.go` beginning with exactly:

```go
// Package mimex parses and normalizes mail messages for frostmail; the x
// marks it as frostmail's layer over the standard mime packages.
package mimex
```

and define `func NormalizeSubject(s string) string` with a doc comment.

1. Replace every run of whitespace (spaces, tabs, `\r`, `\n`) with one space
   and trim both ends.
2. Then repeatedly remove from the start of the string, whichever matches:
   - a reply or forward prefix: one of `re`, `fwd`, `fw`, `aw`, `wg`, `sv`,
     `vs`, `antw`, `rif`, `tr` in any letter case, or `回复`, `答复`, `转发`;
     then optionally a counter `[n]` or `(n)`; then optional spaces; then
     `:` or the full-width colon `：`; then optional spaces;
   - a list tag: `[`, any characters except `]`, `]`, then optional spaces.
3. Stop when neither matches and return what remains (trimmed). Brackets that
   are not at the start stay. Case is preserved. `Re:` alone becomes `""`.

The function must be idempotent: normalizing twice gives the same result.

## Tests (given, do not edit)

`internal/mimex/subject_test.go`: TestNormalizeSubject (32 cases),
TestNormalizeSubjectIsIdempotent.

## Gotchas

- A prefix needs its colon: `Rebate offer`, `Regarding: the lease` and
  `Fwdx: not a prefix` are unchanged. Anchor the prefix pattern so the word
  must be followed by the optional counter, spaces and a colon.
- Compile regular expressions once, at package level, with
  `regexp.MustCompile`.
- `(?i)` makes a Go regexp case-insensitive.

## Out of scope

Other header parsing, RFC 2047 decoding (go-imap already decodes subjects),
and any file outside `internal/mimex/subject.go`.

## Done when

`make accept T=0003` and `make check` pass, and only
`internal/mimex/subject.go` changed.
