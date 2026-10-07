---
id: "0020"
title: Add mailctl show and sync
milestone: M1
size: M
touch:
  - cmd/mailctl/show.go
  - cmd/mailctl/root.go
given:
  - cmd/mailctl/show_sync_test.go
acceptance: go test ./cmd/mailctl -count=1
---
# T-0020: Add mailctl show and sync

## Goal

Finish M1's command line: `show` prints one message (headers and readable
text, fetching the body from the server if needed), and `sync` asks maild to
sync now and, with `--wait`, reports when every account is up to date.

## Read first

- `cmd/mailctl/list.go` and `cmd/mailctl/account.go`: the command pattern and
  helpers already in the package.
- `docs/specs/rpc-api.md`: `message.get`, `message.body`, `sync.now`,
  `sync.status`, `events.subscribe`, the `sync.progress` event and
  `SyncPhase`.
- `api/client.go`: `Client.Notifications()` delivers events after
  `events.subscribe`; decode them with `api.DecodeEvent`.
- The given test `cmd/mailctl/show_sync_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Create `cmd/mailctl/show.go` with `newShowCmd(opts)` and `newSyncCmd(opts)`,
registered in `root.go` after `newSearchCmd(opts)`.

**`show MESSAGE_ID`** (`cobra.ExactArgs(1)`, ID via `strconv.ParseInt`): call
`message.get`, then `message.body`. With `--json`, output one object
`{"message": <Message>, "body": <Body>}`. Otherwise print, each line ending
in `\n`:

```
From: <address>
To: <addresses>
Cc: <addresses>            (only when there are Cc recipients)
Date: <date>
Subject: <subject>
Attachments: <names>       (only when there are attachments)

<body text>
```

An address is `Name <addr>`, or just `addr` when the name is empty; lists
are joined with `, `. Date is `Summary.Date.Local().Format("2006-01-02
15:04")`. Attachments are the filenames of parts whose disposition is
`attachment`, or that have a filename and a disposition other than
`inline`, joined with `, `. A maild error is returned unchanged.

**`sync [ACCOUNT_ID]`** (`cobra.MaximumNArgs(1)`; flags `--wait` and
`--timeout DURATION`, default 10 minutes): the target is that account, or
every account from `account.list`, in ID order.
- Without `--wait`: call `sync.now` for each and print
  `account <id>: sync requested`.
- With `--wait`: first `events.subscribe` (live events), then `sync.now` for
  each account. Then read `Notifications()` until every account is done or
  the timeout passes (an error naming the accounts still waiting). For each
  `sync.progress` event of a target account: a phase other than `idle`
  marks the account as having started; `idle` after it started marks it
  done; `unauthorized` or `failed` end the command with the error
  `account <id>: <phase>: <status error>`. `offline` keeps waiting, because
  maild retries. When all are done, print `account <id>: up to date` for
  each, in ID order.

## Tests (given, do not edit)

`cmd/mailctl/show_sync_test.go`: TestSyncWait, TestSyncWaitReportsBadPassword,
TestShow. They run a real sync engine against an in-memory IMAP server.

## Gotchas

- Subscribe before calling `sync.now`, or the events you wait for can pass
  before you listen.
- An account may already be `idle` when `sync` starts; only an `idle` after
  a non-idle phase counts as done.
- `Notifications()` is closed if the connection drops; treat that as an
  error.

## Out of scope

Flag and move commands (later cards), and every file except the two listed.

## Done when

`make accept T=0020` and `make check` pass, and only the two listed files
changed.
