---
id: "0019"
title: Add mailctl mailboxes, ls and search
milestone: M1
size: M
touch:
  - cmd/mailctl/list.go
  - cmd/mailctl/root.go
given:
  - cmd/mailctl/list_test.go
acceptance: go test ./cmd/mailctl -count=1
---
# T-0019: Add mailctl mailboxes, ls and search

## Goal

Read mail from the command line: list mailboxes with their counts, list a
mailbox's messages newest first, and search. `ls` and `search` read through
views (`view.open`, `view.range`, `view.close`), like the app will.

## Read first

- `cmd/mailctl/account.go` and `cmd/mailctl/hello.go`: the command pattern,
  `clix.OutputJSON`, and tables with `text/tabwriter`.
- `docs/specs/rpc-api.md`: `mailbox.list`, the view domain, `MessageSummary`.
- The given test `cmd/mailctl/list_test.go`, especially the exact tables.
- `docs/tasks/EXECUTOR.md`

## Contract

Create `cmd/mailctl/list.go` with `newMailboxesCmd(opts)`, `newLsCmd(opts)`
and `newSearchCmd(opts)`, and register all three in `root.go` after
`newAccountCmd(opts)`. Tables use
`tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)`; `--json` outputs the API result
instead (`clix.OutputJSON`).

**`mailboxes`** (flag `--account ID`, default all): `mailbox.list`. Columns
`ID ACCOUNT ROLE TOTAL UNREAD PATH`.

**`ls MAILBOX_ID`** (`cobra.ExactArgs(1)`, ID parsed with `strconv.ParseInt`;
flags `--limit N` default 50, `--unread`, `--flagged`): open a view with
`MailboxID` set, plus `Unread: &true` with `--unread` and `Flagged: &true`
with `--flagged`; range `[0, min(limit, count))`; close the view; print the
message table.

**`search TERMS...`** (`cobra.MinimumNArgs(1)`; flags `--account ID`,
`--limit N` default 50): open a view with `Text` set to the terms joined by
spaces (and `AccountID` when given); then as `ls`.

**Message table** (shared helper `printMessages(w, rows)`): columns
`ID FLAGS DATE FROM SUBJECT`.
- FLAGS: `N` when unseen, then `!` when flagged (so `N!`, `N`, `!` or empty).
- DATE: `row.Date.Local().Format("2006-01-02 15:04")`.
- FROM: the sender's name, or the address when the name is empty.
- SUBJECT: when longer than 46 runes, its first 45 runes plus `…`.
- With no rows, print exactly `no messages` instead of the table.

## Tests (given, do not edit)

`cmd/mailctl/list_test.go`: TestMailboxesCommand, TestLsCommand,
TestSearchCommand (exact tables, filters, `--limit`, JSON, bad input).

## Gotchas

- `--json` with no rows outputs `[]`, not `no messages`.
- Close the view with `defer` right after opening it.
- Count runes with `[]rune(subject)`, not `len(subject)`.

## Out of scope

`show` and `sync` (T-0020), and every file except the two listed.

## Done when

`make accept T=0019` and `make check` pass, and only the two listed files
changed.
