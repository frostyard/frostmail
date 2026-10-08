---
id: "0041"
title: Send mail and manage the outbox from mailctl
milestone: M3
size: M
touch:
  - cmd/mailctl/send.go
given:
  - cmd/mailctl/send_test.go
acceptance: go test ./cmd/mailctl -run 'Send|Outbox' -count=1
---
# T-0041: Send mail and manage the outbox from mailctl

## Goal

Tests and scripts send mail without the app: `mailctl send` composes a
message from flags and stdin and queues it, optionally waiting for the
outcome, and `mailctl outbox` lists, cancels and retries messages on their
way out (`docs/design/send.md`, Operational notes). Implement both commands
over the draft and outbox API.

## Read first

- `cmd/mailctl/send.go`: the two stub constructors (already registered in
  `root.go`).
- `cmd/mailctl/ops.go` and `cmd/mailctl/list.go`: how commands dial maild,
  print, use `clix.OutputJSON`, and parse IDs; `truncateSubject`.
- `api/zz_generated.go`: `DraftService`, `OutboxService`, `OutboxItem`,
  `OutboxChanged`, `EventsService.Subscribe`, `Client.Notifications`.
- `docs/design/send.md`: "Drafts" and "Outbox".
- The given test `cmd/mailctl/send_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the two constructor names; replace the file comment's "Task T-0041 …"
sentence and the stub doc comments with what the commands do. Remove
`errNotYet`.

**`mailctl send`** (no arguments):

- Flags: `--to`, `--cc`, `--bcc` (repeatable, `StringArrayVar`; each value
  `addr` or `Name <addr>`), `--subject`, `--attach` (repeatable paths),
  `--account` (int64; 0 means maild's choice), `--html` (stdin is HTML),
  `--wait` (wait for the outcome), `--timeout` (duration for `--wait`,
  default 2m).
- Before dialing: no `--to`, `--cc` or `--bcc` is the error
  `no recipients: use --to, --cc or --bcc`; a value `net/mail.ParseAddress`
  rejects is the error `invalid address "<value>"`; `--attach` paths are
  made absolute with `filepath.Abs` (maild needs absolute paths).
- The body is all of stdin (`cmd.InOrStdin()`). Without `--html`, it becomes
  HTML: trailing newlines trimmed, `\r\n` turned into `\n`, HTML-escaped
  (`html.EscapeString`), each `\n` replaced by `<br>`, wrapped in
  `<p>…</p>`.
- Then: dial; with `--wait`, subscribe to events first
  (`c.Events().Subscribe(ctx, nil)`), so no event is missed; create a draft
  (`draft.create`, kind `new`, the account when given); update it with the
  created content's identity, the recipients and subject, and the body
  followed by the created draft's HTML (the signature block); attach each
  file (an error names the file and says the draft is kept); send
  (`draft.send`; an error says the draft is kept).
- Output: `queued message <outbox ID> from draft <draft ID>`, or with
  `--json` the queued `OutboxItem` (`clix.OutputJSON`).
- With `--wait`, read `c.Notifications()` until an `OutboxChanged` event
  for that outbox ID arrives with: `sent` → print `sent` (not in JSON
  mode, `clix.JSONOutput`) and succeed; `failed` → the error
  `message <ID> not sent: <error>` with the row's error from
  `outbox.list`; `Deleted` → `message <ID> was cancelled`. When
  `--timeout` passes first: `message <ID> not sent after <timeout>`.

**`mailctl outbox`** (no arguments; `--account` filters):

- Lists `outbox.list` as a tab-separated table (`text/tabwriter`, as in
  `list.go`) with the header `ID  STATE  ATTEMPTS  SUBJECT  ERROR`, one row
  per message (subject through `truncateSubject`, `-` when there is no
  error), or `outbox is empty`; with `--json`, the list as JSON.
- `mailctl outbox cancel OUTBOX_ID`: `outbox.cancel`, then
  `cancelled message <ID>; draft <draft ID> kept`.
- `mailctl outbox retry OUTBOX_ID`: `outbox.retry`, then
  `queued message <ID> again`.
- A non-numeric ID is the error `invalid outbox id "<arg>"`, before dialing.

## Tests (given, do not edit)

`cmd/mailctl/send_test.go`: TestSendComposesFromFlagsAndStdin,
TestSendTakesHTMLAndBcc, TestSendChecksArgumentsBeforeDialing,
TestSendReportsARefusal, TestOutboxListsAndCancels.

## Gotchas

- `clix.JSONOutput` is a `bool` variable, not a function.
- Use `StringArrayVar`, not `StringSliceVar`: a name such as
  `"Smith, Bob" <bob@x.test>` contains a comma.
- Subcommands of `outbox` are added with `cmd.AddCommand`; each dials on
  its own.
- Close the client (`defer c.Close()`) in every command.

## Out of scope

The engine and the outbox worker (done), and every file except
`cmd/mailctl/send.go`.

## Done when

`make accept T=0041` and `make check` pass, and only `cmd/mailctl/send.go`
changed.
