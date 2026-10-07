---
id: "0009"
title: Decode message parts to UTF-8 text
milestone: M1
size: S
touch:
  - internal/mimex/decode.go
given:
  - internal/mimex/decode_test.go
acceptance: go test ./internal/mimex -run TestDecodePart -count=1
---
# T-0009: Decode message parts to UTF-8 text

## Goal

Sync fetches the first 2 KiB of a message's text part for its preview. Those
bytes are still transfer-encoded (base64 or quoted-printable), in the part's
charset, and usually cut off mid-sequence. Add `DecodePart`, which turns them
into valid UTF-8 text.

## Read first

- The given test `internal/mimex/decode_test.go`: every case it checks.
- `go doc github.com/emersion/go-message/charset Reader`: converts a named
  charset to UTF-8 (already a dependency).
- `docs/tasks/EXECUTOR.md`

## Contract

Create `internal/mimex/decode.go` with
`func DecodePart(data []byte, encoding, charset string, truncated bool) (string, error)`
and a doc comment. `truncated` means `data` may end in the middle of an
encoded unit.

1. **Transfer encoding** (compare lowercased, trimmed):
   - `base64`: remove all whitespace. If `truncated`, drop trailing
     characters until the length is a multiple of 4. Decode with
     `base64.StdEncoding`; a decode error is returned (wrapped, starting with
     `mimex:`).
   - `quoted-printable`: decode by hand, leniently. `=` followed by `\r\n` or
     `\n` is a soft line break (removed). `=` followed by two hex digits (any
     case) is that byte. If `truncated` and the data ends in `=` or `=X`
     (fewer than two characters after `=`), drop that tail. Any other `=` is
     kept as a literal `=`. Every other byte is copied.
   - Anything else (`7bit`, `8bit`, `binary`, empty, unknown): the bytes as
     they are.
2. **Charset** (compare lowercased, trimmed): empty, `utf-8`, `utf8` and
   `us-ascii` need no conversion. Otherwise convert with
   `charset.Reader(name, bytes.NewReader(decoded))` and `io.ReadAll`. If the
   charset is unknown (`charset.Reader` returns an error), use the bytes
   unconverted.
3. **UTF-8:** if `truncated` and the text ends with an incomplete UTF-8
   sequence, drop it (`utf8.FullRune` on the tail). Then replace remaining
   invalid sequences with U+FFFD (`strings.ToValidUTF8`). The result is
   always valid UTF-8.

## Tests (given, do not edit)

`internal/mimex/decode_test.go`: TestDecodePart (23 cases) and
TestDecodePartErrors (3 malformed base64 inputs).

## Gotchas

- Check for an incomplete trailing rune by looking at the last up to 3
  bytes: walk back to the start of the last rune
  (`utf8.RuneStart`), and drop it if `!utf8.FullRune(tail)`.
- A charset conversion error after a successful `charset.Reader` (from
  `io.ReadAll`) is not fatal either: fall back to the unconverted bytes.

## Out of scope

RFC 2047 encoded words (go-imap decodes headers), uuencode, and every file
except `internal/mimex/decode.go`.

## Done when

`make accept T=0009` and `make check` pass, and only
`internal/mimex/decode.go` changed.
