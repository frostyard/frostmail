---
id: "0007"
title: Convert HTML message parts to plain text
milestone: M1
size: S
touch:
  - internal/mimex/html.go
given:
  - internal/mimex/html_test.go
acceptance: go test ./internal/mimex -run TestHTMLToText -count=1
---
# T-0007: Convert HTML message parts to plain text

## Goal

Many messages have only an HTML part. Previews, the search index and
`message.body`'s `text` need readable plain text from it. Implement
`HTMLToText` in `internal/mimex/html.go`, which is a stub today.

## Read first

- `internal/mimex/html.go`: the stub and its doc comment.
- `go doc golang.org/x/net/html Tokenizer`: `NewTokenizer`, `Next`,
  `TagName`, `Text`, and the token types.
- The given test `internal/mimex/html_test.go`: every case it checks.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the signature and doc comment, drop the `_ = html.NewTokenizer` line,
and implement with `html.NewTokenizer(strings.NewReader(s))`:

1. **Skipped elements:** ignore all text inside `script`, `style`, `head`,
   `title`, `template` and `noscript`. Track a skip depth: increase on their
   start tags, decrease on their end tags. Tags inside skipped elements add
   nothing either.
2. **Text tokens** (`z.Text()` is already entity-decoded): replace each run
   of whitespace (`unicode.IsSpace`, which includes the no-break space) with
   one space. If the output is empty or ends with `"\n"`, drop leading
   spaces. Append the rest. Raw newlines in HTML text are only whitespace.
3. **Paragraph breaks:** the start and end tags of `p`, `h1`–`h6`,
   `blockquote`, `table`, `ul`, `ol`, `pre` and `hr`: if the output is not
   empty, append `"\n"` until it ends with `"\n\n"`.
4. **Line breaks:** the start and end tags of `div`, `li`, `tr`, `section`,
   `article`, `header`, `footer`, `dt` and `dd`: if the output is not empty
   and does not end with `"\n"`, append `"\n"`. A `li` start tag then
   appends `"- "`. A `br` tag always appends `"\n"`, so repeated `<br>`
   makes blank lines.
5. **Cells:** the end tags of `td` and `th` append `" "`.
6. Everything else (links, images, comments, unknown tags) adds nothing.
7. **Normalize:** split the output on `"\n"`; in each line collapse every run
   of whitespace to one space and trim the line; join with `"\n"`; replace
   every run of three or more `"\n"` with `"\n\n"`; trim the result.

## Tests (given, do not edit)

`internal/mimex/html_test.go`: TestHTMLToText (18 cases) and
TestHTMLToTextNewsletter.

## Gotchas

- `TagName` returns the lowercase name and whether attributes follow; use
  `name, _ := z.TagName()`.
- `html.SelfClosingTagToken` is a separate token type: treat it like a
  start tag for `br` and `hr`.
- Stop at `html.ErrorToken` (end of input).
- Call `z.Text()` once per token and keep the result: a second call returns
  nothing.
- Collapse whitespace with a loop over runes and `unicode.IsSpace`, keeping a
  single space where a run was, including at the edges (`"Hello "` stays
  `"Hello "`, so the next token does not glue on). `strings.Fields` drops the
  edges and `\s` in a regexp misses the no-break space.

## Out of scope

Sanitizing HTML for display (that is M2's sanitizer), and every file except
`internal/mimex/html.go`.

## Done when

`make accept T=0007` and `make check` pass, and only
`internal/mimex/html.go` changed.
