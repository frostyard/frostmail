---
id: "0052"
title: Write new-mail notification texts
milestone: M4
size: S
touch:
  - internal/notify/notes.go
given:
  - internal/notify/notes_test.go
acceptance: go test ./internal/notify -count=1
---
# T-0052: Write new-mail notification texts

## Goal

maild shows a desktop notification when new mail arrives: one per message,
or one summary when many arrive at once (`docs/design/desktop.md`,
Notifications). Implement the function that turns one sync pass's new
mail into notification texts; the planner sends them over D-Bus.

## Read first

- `internal/notify/notes.go`: `Mail`, `Note` and the stub.
- `docs/design/desktop.md`: "Notifications".
- The given test `internal/notify/notes_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the types and the signature; replace the stub's doc comment and the
package comment's "Task T-0052 …" sentence with what it does.

`Notes(mail)`:

- No mail: `nil`.
- **One to three messages:** one `Note` per message, in order:
  - `Summary`: the sender: `FromName` trimmed, else `FromAddr` trimmed,
    else `Unknown Sender`;
  - `Body`: the subject trimmed (`(no subject)` when empty), then, when the
    preview is not empty, a newline and the preview with every run of
    whitespace collapsed to one space and the ends trimmed
    (`strings.Fields`), cut to its first 120 runes plus `…` when longer;
  - `MessageID`: the message's `ID`.
- **Four or more:** one `Note` with `Summary` `"<n> new messages"`,
  `MessageID` 0, and `Body` `"From "` + the distinct senders (as above, in
  order of first appearance): one name; `A and B`; `A, B and C`; for more
  than three, `A, B, C and <k> others` with `k` the remaining distinct
  senders.

## Tests (given, do not edit)

`internal/notify/notes_test.go`: TestNotesOnePerMessage,
TestNotesTruncatesPreviews, TestNotesGroupFourOrMore, TestNotesNone.

## Gotchas

- Count runes, not bytes, when cutting the preview.
- Two helpers (the sender, the body) keep `Notes` short.

## Out of scope

D-Bus, when to notify, and every file except `internal/notify/notes.go`.

## Done when

`make accept T=0052` and `make check` pass, and only
`internal/notify/notes.go` changed.
