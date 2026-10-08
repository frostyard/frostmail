---
id: "0040"
title: Write the rules for replies and forwards
milestone: M3
size: M
touch:
  - internal/compose/reply.go
given:
  - internal/compose/reply_test.go
acceptance: go test ./internal/compose -count=1
---
# T-0040: Write the rules for replies and forwards

## Goal

Reply, Reply All and Forward start a draft from an existing message: who it
goes to, its subject, its `References`, and a body quoting the original
(`docs/design/send.md`, Replies and forwards). Implement these rules as pure
functions; maild's draft domain calls them.

## Read first

- `internal/compose/reply.go`: `Address`, `Source` and the stubs.
- `docs/design/send.md`, "Replies and forwards".
- The given test `internal/compose/reply_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the types and signatures; replace each stub's doc comment (and the
"Task T-0040 …" sentence) with the rules below. Address comparisons ignore
case.

- **`ReplyRecipients(src, self, all)`:** To is `src.ReplyTo` if not empty,
  else `src.From`, without the account's own addresses (`self`). If that
  leaves To empty (a reply to one's own message), To is `src.To` without
  `self`. With `all`, Cc is `src.To` followed by `src.Cc`, without `self`,
  without addresses already in To, and without repeats (the first occurrence
  wins, name included); without `all`, Cc is `nil`. Empty results are `nil`.
- **`ReplySubject(s)`:** trimmed; unchanged if it starts (case-insensitively)
  with `re:`, `aw:` or `sv:`; otherwise `"Re: " + s`.
- **`ForwardSubject(s)`:** trimmed; unchanged if it starts with `fwd:` or
  `fw:`; otherwise `"Fwd: " + s`.
- **`ReplyReferences(src)`:** `src.References` then `src.MessageID`, skipping
  empty strings and repeats; when longer than 20, the first entry and the
  last 19. `nil` when empty.
- **`Attribution(date, name, loc)`:** `"On " + date.In(loc).Format("Mon, Jan 2, 2006 at 3:04 PM") + ", " + name + " wrote:"`,
  with `name` = `"someone"` when empty.
- **The quoted body** (shared by both): `src.HTML` as it is when not empty;
  otherwise `<p>` + the HTML-escaped `src.Text` with each `\n` replaced by
  `<br>` + `</p>`.
- **`QuoteHTML(src, loc)`:** `<p><br></p><p>` + the HTML-escaped
  attribution (name: `From.Name`, else `From.Addr`) + `</p><blockquote
  type="cite">` + the quoted body + `</blockquote>`.
- **`ForwardHTML(src, loc)`:** `<p><br></p><p>Begin forwarded message:</p><blockquote type="cite"><p>`
  then `<b>From:</b> ` + the sender, `<br><b>Subject:</b> ` + the subject,
  `<br><b>Date:</b> ` + `date.In(loc).Format("January 2, 2006 at 3:04 PM")`,
  `<br><b>To:</b> ` + the To list joined with `", "`, then `</p>` + the
  quoted body + `</blockquote>`. An address is shown as `Name <addr>`, or
  `addr` without a name. Every inserted value is HTML-escaped
  (`html.EscapeString`).

## Tests (given, do not edit)

`internal/compose/reply_test.go`: TestReplyRecipients, TestSubjects,
TestReplyReferences, TestAttribution, TestQuoteHTML, TestForwardHTML.

## Gotchas

- `"Reply to Plan"` and `"Release notes"` do not start with `re:`: compare
  the prefix with its colon.
- Escape the attribution and header values, never `src.HTML` (maild has
  already sanitized it).

## Out of scope

Building messages and the draft domain (planner), and every file except
`internal/compose/reply.go`.

## Done when

`make accept T=0040` and `make check` pass, and only
`internal/compose/reply.go` changed.
