---
id: "0092"
title: Scrub DAV and Tasks traces
milestone: M4.5
size: L
touch:
  - tools/davrec/main.go
  - tools/davrec/scrub.go
  - tools/davrec/collect.go
  - tools/davrec/vocab.go
given:
  - tools/davrec/davrec_test.go
  - tools/davrec/testdata/session.trace
  - tools/davrec/testdata/test.key
acceptance: go test ./tools/davrec -count=1
---
# T-0092: Scrub DAV and Tasks traces

## Goal

`tools/davrec`: turn a trace that maild recorded with `MAILD_DAV_TRACE`
(CardDAV, CalDAV and Google Tasks exchanges) into one that can be checked
in. Every personal value becomes a stable fake of the same shape, and a
replay of the scrubbed trace drives pimsync exactly as the original does.
Phase 5 records Google's and iCloud's sessions with it.

## Read first

- `docs/design/testing.md`: "DAV and Tasks recordings", and "IMAP,
  recorded" (`tools/imaprec` is the IMAP twin: its key handling, fakes and
  leftover check are the model).
- `internal/httprec` (`Exchange`, `Load`, `Parse`, `Write`, `Serve`).
- `tools/imaprec/main.go` and `scrub.go`.
- `internal/contentline` (iCalendar and vCard lines: parse, unfold,
  escapes).
- The stub `tools/davrec/main.go`, the given test and its
  `testdata/session.trace` (recorded by `TestRecordSession`: pimsync
  against `davtest` and `gtaskstest`).
- `docs/tasks/EXECUTOR.md`

## Contract

- **Command:** `davrec -account EMAIL [-name NAME] [-keyfile FILE]
  [-note TEXT] [-o FILE] TRACE`. `-account` is required. Output goes to
  `-o` (default standard output) through `httprec.Write` with `-note` as
  the comment. Nothing is written when davrec refuses: build the result in
  memory and write it only when it is complete.
- **Key:** with `-keyfile`, the key is the SHA-256 of the file (at least 16
  bytes, else an error); otherwise 32 random bytes per run. The same key
  makes the same fakes, so traces of one account scrubbed with one key
  agree.
- **What is personal** (collected first, from the whole trace):
  - the `-account` address and the `-name`;
  - every email address anywhere (URLs and hrefs, raw, `%40`- or
    `%2540`-encoded; headers; bodies);
  - in vCards: FN, N, NICKNAME, ORG, TITLE, ROLE, NOTE, EMAIL, TEL, ADR,
    LABEL, URL, BDAY, ANNIVERSARY, UID, IMPP, RELATED, CATEGORIES and X-
    property values;
  - in iCalendar: SUMMARY, DESCRIPTION, LOCATION, COMMENT, CONTACT, URL,
    UID, RELATED-TO, RESOURCES, CATEGORIES, ATTACH, X- property values, and
    ORGANIZER's and ATTENDEE's values and CN and EMAIL parameters;
  - in DAV XML: `displayname`, `calendar-description`,
    `addressbook-description`;
  - in Google Tasks JSON: a list's `title`; a task's `title`, `notes` and
    `links` (`description`, `link`).
  Read vCards and iCalendar unfolded (a fold can split a value); the
  scrubbed trace keeps them unfolded.
- **Fakes:** split each personal value into words (runs of letters and
  digits) of three or more characters, leaving out protocol vocabulary
  (`vocab.go`: the vCard, iCalendar, DAV and HTTP names and keywords, and
  parameter values such as WORK, HOME, CELL, ACCEPTED, NEEDS-ACTION;
  "mailto"; the last label of a domain). Replace every such word
  everywhere in the trace (URLs, headers, bodies; raw, URL-encoded and in
  XML or iCalendar escapes) at word boundaries with a fake of the same
  length and the same pattern of upper case, lower case and digits,
  derived from an HMAC of the lower-cased word under the key, so a word
  is the same fake everywhere and a phone number stays a phone number.
- **Photos:** a vCard PHOTO or LOGO value (base64 or a `data:` URI)
  becomes a one-pixel PNG in the same form; a binary body with an `image/`
  type becomes that PNG; any other binary body is refused.
- **Leftovers:** after replacing, davrec refuses (an error naming how many
  remain, not what) when any collected value or word of six or more
  characters is still in the trace, compared case-insensitively and after
  URL-decoding.
- The exchanges keep their count, order, methods, statuses and every
  non-personal byte: a replay of the scrubbed trace drives pimsync to the
  same store shape as the original (the given test checks collections,
  people, events and tasks).

## Tests (given, do not edit)

`tools/davrec/davrec_test.go` with `testdata/session.trace` and
`testdata/test.key` (a key for tests only). `TestRecordSession` records the
session again with `-update`; it is skipped otherwise.

## Gotchas

- Collect from every exchange before replacing anything: an href in an
  early response and the same href in a later request must get the same
  fake.
- Replace longer words before their substrings, and keep a fake from
  creating a protocol word.
- `httprec.Serve` compares PROPFIND and REPORT bodies byte for byte: the
  scrubbed request bodies must be what pimsync sends after reading the
  scrubbed answers.
- Keep functions under 60 lines; no `//nolint`.

## Out of scope

Recording (done), the Phase 5 replay tests, and every file not under
`touch`.

## Done when

`make accept T=0092` and `make check` pass (taskrun runs them), and only
the files under `touch` changed.
