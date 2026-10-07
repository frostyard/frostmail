---
id: "0018"
title: Extract a message's readable text
milestone: M1
size: S
touch:
  - internal/mimex/body.go
given:
  - internal/mimex/body_test.go
acceptance: go test ./internal/mimex -run TestBodyText -count=1
---
# T-0018: Extract a message's readable text

## Goal

`message.body` returns a message's text: the text/plain part, or text made
from the HTML part. Add `BodyText`, which takes the raw RFC 5322 message
from the blob store and finds that text through any MIME structure.

## Read first

- `go doc github.com/emersion/go-message`: `Read`, `Entity`, `Entity.Walk`,
  `Header.ContentType`, `Header.ContentDisposition`, `IsUnknownCharset`,
  `IsUnknownEncoding`.
- `internal/mimex/html.go`: `HTMLToText`.
- The given test `internal/mimex/body_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Replace the stub in `internal/mimex/body.go`, keeping the signature,
`func BodyText(raw []byte) (text string, hasHTML bool, err error)`, with a doc
comment. Import `_ "github.com/emersion/go-message/charset"` so go-message
converts charsets.

1. `message.Read(bytes.NewReader(raw))`. An error that
   `message.IsUnknownCharset` or `message.IsUnknownEncoding` accepts is not
   fatal (go-message still returns the entity); any other error is returned,
   wrapped with `mimex:`.
2. `Entity.Walk` over the tree. For each part whose media type does not
   start with `multipart/`:
   - Skip it if its disposition is `attachment`, or if it has a filename
     (the disposition's `filename` or the content type's `name` parameter)
     and its disposition is not `inline`.
   - Skip `message/rfc822` (a forwarded message is not the body).
   - Keep the body of the first `text/plain` part and of the first
     `text/html` part (`io.ReadAll` of `part.Body`; an unknown charset error
     from the read leaves the bytes as read).
3. `hasHTML` is whether an HTML part was kept. The text is the plain part if
   it is not blank after `strings.TrimSpace`, else `HTMLToText(html)`.
4. Replace `\r\n` with `\n`, then trim trailing whitespace from every line
   and leading and trailing blank lines from the whole (so the result
   begins and ends with text).

## Tests (given, do not edit)

`internal/mimex/body_test.go`: TestBodyText (11 cases), TestBodyTextSeedMessages
(three files from `dev/incus/seed`), TestBodyTextMalformed.

## Gotchas

- A part with no Content-Type is `text/plain` (`ContentType` returns that
  default with an error; ignore the error and use the type).
- `Walk`'s callback receives the walk error for parts go-message could not
  decode; skip those parts unless the error is an unknown charset.

## Out of scope

Sanitizing HTML for display (M2), attachments, and every file except
`internal/mimex/body.go`.

## Done when

`make accept T=0018` and `make check` pass, and only
`internal/mimex/body.go` changed.
