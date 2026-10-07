---
id: "0022"
title: List attachments and pass the MIME corpus
milestone: M1
size: M
touch:
  - internal/mimex/attach.go
  - internal/mimex/body.go
given:
  - internal/mimex/corpus_test.go
acceptance: go test ./internal/mimex -count=1
---
# T-0022: List attachments and pass the MIME corpus

## Goal

M1 must handle at least 150 kinds of real-world MIME: six charsets, three
transfer encodings, eight structures, and ten broken or unusual messages.
The given corpus generates them from a known truth. Implement `Attachments`
and fix the one `BodyText` gap the corpus finds.

## Read first

- `internal/mimex/attach.go`: `Attachment` and the `Attachments` stub.
- `internal/mimex/body.go`: `BodyText`, its `bodyParts` visitor and
  `isDecodingError`; reuse them.
- The given test `internal/mimex/corpus_test.go`: the generator and the ten
  broken fixtures.
- `docs/tasks/EXECUTOR.md`

## Contract

**`BodyText`** (one change): a multipart message whose final boundary is
missing makes `Walk` return an error after the parts it read. If at least
one part was visited, keep what was found and return no error; add a
`visited bool` to `bodyParts`, set in `visit`.

**`Attachments(raw []byte) ([]Attachment, error)`**, keeping the type and
the doc comment (extend it):

1. `message.Read`, accepting `isDecodingError` errors as `BodyText` does.
2. `Walk` the tree, applying the same truncation rule (an error after at
   least one visited part is ignored). Skip `multipart/*` containers and
   parts whose walk error is not a decoding error.
3. **Filename:** the disposition's `filename` parameter (go-message decodes
   RFC 2231 `filename*=`), else the content type's `name` parameter; then
   decode RFC 2047 encoded words with
   `(&mime.WordDecoder{CharsetReader: charset.Reader}).DecodeHeader`,
   keeping the undecoded name if that fails.
4. **Body text is not an attachment:** skip `text/*` parts that have no
   filename and whose disposition is not `attachment`.
5. Every other part is an attachment, in tree order: `ContentType` is the
   lowercase media type; `Size` is the number of bytes read from
   `part.Body` (`io.Copy(io.Discard, …)`, decoded; `message/rfc822` parts are
   not decoded); `ContentID` is `Content-Id` without angle brackets;
   `Inline` is true when the disposition is `inline`, or when there is no
   disposition and there is a Content-ID.
6. No attachments: return nil.

## Tests (given, do not edit)

`internal/mimex/corpus_test.go`: TestCorpus, 154 fixtures, each checking
`BodyText` and `Attachments`. The existing mimex tests must still pass.

## Gotchas

- `message.Read` returns a usable entity together with an unknown-charset
  error; do not stop on it.
- The `charset` package is imported for its side effect in `body.go`;
  `attach.go` imports it by name for `charset.Reader`.

## Out of scope

Downloading parts (M2), and every file except the two listed.

## Done when

`make accept T=0022` and `make check` pass, and only the two listed files
changed.
