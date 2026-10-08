---
id: "0048"
title: Parse the search language
milestone: M4
size: M
touch:
  - internal/search/search.go
given:
  - internal/search/search_test.go
acceptance: go test ./internal/search -count=1
---
# T-0048: Parse the search language

## Goal

The search field accepts a small, forgiving subset of Gmail's search
syntax: words, quoted phrases, `-` to exclude, and operators such as
`from:`, `is:unread`, `has:attachment`, `after:` and `in:sent`
(`docs/specs/search.md`). Implement the parser and the FTS5 expressions the
store will run.

## Read first

- `docs/specs/search.md`: the whole spec. Its Rules and Examples are the
  contract.
- `internal/search/search.go`: the types and the stubs.
- The given test `internal/search/search_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the types and signatures; replace the stubs' doc comments and the
package comment's "Task T-0048 …" sentence with what the code does.

- **Tokenize** the text into tokens split on whitespace outside quotes. A
  token may start with `-` (negation). A `"` opens a phrase only at the
  start of a token (after the optional `-`) or right after `name:`; the
  phrase runs to the next `"`, or to the end when unclosed. Elsewhere a
  `"` is an ordinary character of the word (`5"x`).
- `name:value` is an operator when `name` is one or more ASCII letters or
  `_` (case-insensitive); the value is the rest of the token, or a quoted
  phrase.
- **Text terms:** bare words and phrases become `Term{Text, Phrase, Not}`;
  `from:`, `to:`, `cc:`, `subject:`, `filename:` become terms with the
  columns in the spec (`from_text`, `to_text`, `to_text`, `subject`,
  `attachment_names`), keeping `Phrase` and `Not`.
- **Flags:** `is:unread`/`is:read` set `Unread` (negation inverts),
  `is:flagged`/`is:starred` set `Flagged` (true, false when negated),
  `has:attachment` sets `HasAttachment` (true, false when negated). Values
  are case-insensitive. A later one replaces an earlier one.
- **Dates** in `loc`: `after:D` sets `After` = D at 00:00; `before:D` sets
  `Before`; `on:D` sets both (D and the next day, `AddDate(0, 0, 1)`). D is
  `YYYY-MM-DD` or `YYYY/MM/DD` and must be a real date
  (`time.ParseInLocation`). `newer_than:N<u>` / `older_than:N<u>` set
  `After` / `Before` to midnight (in `loc`) of the day `N` days before
  `now`'s day in `loc`, with `u` = `d` (days), `w` (×7), `m` (×30) or `y`
  (×365) and `N` ≥ 1.
- **Roles:** `in:inbox|drafts|sent|junk|spam|trash|archive` (`spam` is
  `junk`) append to `Roles` in first-seen order without repeats.
- **Anything else is text:** an unknown operator name, an empty or
  malformed value, and a negated `in:`/date operator become a text term
  whose `Text` is the token without its `-` (`size:3`, `in:nowhere`,
  `from:`), with `Not` set when it was negated.
- **Dropped:** a text term whose text has no letter or digit
  (`unicode.IsLetter`/`IsDigit`): `-`, `!!!`, `"..."`, `""`.
- **Match / Exclude / Empty** exactly as the spec says: each term as a
  quoted phrase with `"` doubled, `*` after non-phrases, `column : ` before
  column terms; Match joins positive terms with spaces; Exclude joins
  negated terms with ` OR `, in parentheses when more than one; Empty is
  true when there are no terms, no flags, no dates and no roles.

## Tests (given, do not edit)

`internal/search/search_test.go`: TestParseTerms, TestParseFlags,
TestParseDates, TestParseRoles, TestMatchAndExclude, TestEmpty.

## Gotchas

- Work on runes (`[]rune`), not bytes, so multi-byte text stays whole.
- Two small helpers keep `Parse` short: one that tokenizes, one that
  applies an operator and reports whether it was one.
- golangci-lint's staticcheck QF1001 wants De Morgan's form: write
  `r < 'a' || r > 'z'` style conditions rather than `!(…)`.
- Compute dates from `now.In(loc)`; never use `time.Now()`.

## Out of scope

Running searches in the store (planner), and every file except
`internal/search/search.go`.

## Done when

`make accept T=0048` and `make check` pass, and only
`internal/search/search.go` changed.
