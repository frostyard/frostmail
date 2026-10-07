---
id: "0021"
title: Generate a reproducible Maildir of test mail
milestone: M1
size: M
touch:
  - tools/mailgen/main.go
given:
  - tools/mailgen/mailgen_test.go
acceptance: go test ./tools/mailgen -count=1
---
# T-0021: Generate a reproducible Maildir of test mail

## Goal

M1's exit test syncs 50,000 messages from the test server. `mailgen` writes
them as a Maildir that `doveadm import` loads in one step: realistic enough
(threads, MIME variety, flags) to exercise sync, and byte-for-byte
reproducible from a seed.

## Read first

- `tools/mailgen/main.go`: `Options` and the stubs `Generate` and `run`.
- The given test `tools/mailgen/mailgen_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Replace the stubs; keep `Options`, the signatures and the package doc.

**`run(args)`**: a `flag.FlagSet` with `-n` (int), `-seed` (uint64, default 1)
and `-out` (string). `-n` must be at least 1 and `-out` must be set; return
an error otherwise. Then `Generate`.

**`Generate(o)`**: create `o.Out/cur`, `o.Out/new` and `o.Out/tmp` (0755).
Use one `math/rand/v2` source, `rand.New(rand.NewPCG(o.Seed, o.Seed^0x9e3779b97f4a7c15))`,
and nothing else that varies (no `time.Now`, no map iteration order). For
message i = 1…N, write `cur/<i as 7 digits>.mailgen:2,<flags>`:

- **Headers**, in order: `Date`, `From`, `To`, optional `Cc`, `Subject`,
  `Message-ID`, and for replies `In-Reply-To` and `References`, then
  `MIME-Version: 1.0` and the content headers.
- **Date**: `2025-10-07T00:00:00Z` plus `i` × (365 days / N), formatted
  `time.RFC1123Z`, so dates increase with i.
- **From**: one of at least 50 senders built from first- and last-name
  lists (`First Last <first.last@<company>.test>`). **To**:
  `Test <test@mailtest.test>`; in about 20% of messages also a `Cc` sender.
- **Message-ID**: `<gen-<seed>-<i>@mailgen.test>`.
- **Replies**: for i > 1, with probability 0.25, reply to a random message
  among the previous 200: `In-Reply-To` is its Message-ID, `References` is
  its References followed by its Message-ID, and the subject is `Re: ` plus
  its subject without any `Re: `. Otherwise the subject is 3–7 random words
  from a word list, capitalized.
- **Body**: 1–4 paragraphs of random words, wrapped at 72 columns.
- **Structure**: about 70% `text/plain; charset=utf-8`; 20%
  `multipart/alternative` (the text, plus an HTML part with the paragraphs in
  `<p>`); 10% `multipart/mixed` (the text, plus an attachment with
  `Content-Disposition: attachment; filename="report-<i>.csv"`, type
  `text/csv`, base64). Boundaries are `b<i>`.
- **Flags** in the file name, in ASCII order: `F` with probability 0.05,
  `R` for 30% of replies, `S` with probability 0.6.
- Every line ends in CRLF.

## Tests (given, do not edit)

`tools/mailgen/mailgen_test.go`: TestMaildirLayout, TestDeterministic,
TestMessagesAreValidAndVaried (parses every message with go-message/mail),
TestRunFlags.

## Gotchas

- Build each message in a `bytes.Buffer` with `\r\n` line endings; write
  files with `os.WriteFile` (0644).
- Keep the per-message records (Message-ID, references, base subject) in a
  slice, not a map, so replies pick parents deterministically.
- Base64 attachment lines must be at most 76 characters.

## Out of scope

Loading into Dovecot (the planner's `make mailtest-seed`), and every file
except `tools/mailgen/main.go`.

## Done when

`make accept T=0021` and `make check` pass, and only
`tools/mailgen/main.go` changed.
