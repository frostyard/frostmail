---
id: "0036"
title: Build a maild data directory for app tests
milestone: M2
size: M
touch:
  - tools/uifixture/main.go
given:
  - tools/uifixture/uifixture_test.go
acceptance: go test ./tools/uifixture -count=1
---
# T-0036: Build a maild data directory for app tests

## Goal

The app tests (`docs/plans/0004-m2-ui-read-path.md`, Phase 4) run the real
maild without a mail server: on a data directory that already holds an
account, mailboxes, 100,000 messages with their bodies, and the hostile-HTML
corpus. Implement `tools/uifixture`, which builds that directory with the
store's own write functions.

## Read first

- `tools/uifixture/main.go`: `Options`, the `Build` and `run` stubs and the
  package doc.
- `internal/mailgen/mailgen.go`: `Each` and `Message`.
- `internal/store`: `Open`, `DB.Tx`, `Tx.InsertAccount`,
  `Tx.ReplaceMailboxes` (`ServerMailbox`), `Tx.InsertHeaders`
  (`MessageHeader`, `Flags`, `Part`, `Address`), `Tx.SetBody`,
  `Tx.IndexMessage` with `SearchDocFor`, `Tx.AssignThreads`.
- `internal/blob/blob.go`: `New`, `Store.Put`.
- `internal/mimex`: `BodyText`, `Preview`, `ParseMessageIDs`, `WalkParts`.
- The given test `tools/uifixture/uifixture_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep `Options`, the signatures and the package doc; replace each stub's doc
comment, and remove the doc's "Task T-0036 …" line.

**`run(ctx, args)`**: a `flag.FlagSet` with `-out` (string, required),
`-n` (int, default 1000, at least 0), `-seed` (uint64, default 1) and
`-hostile` (string, default ""); then `Build`. Errors for a missing
`-out` or a negative `-n`.

**`Build(ctx, o)`**:

1. Fail if `o.Out` already exists. Create `o.Out` and `o.Out/blobs` (0700).
   Open the store at `o.Out/frostmail.db` and blobs at `o.Out/blobs`; close
   the store before returning.
2. In one `Tx`: `InsertAccount` with kind `imap`, email and IMAP/SMTP
   username `test1@mailtest.test`, display name `Test One`, auth
   `password`, IMAP `127.0.0.1` port 1 TLS `tls`, SMTP `127.0.0.1` port 1
   TLS `starttls`. No password is stored, so maild's sync never connects.
   Then `ReplaceMailboxes` with delimiter `/`, all selectable and
   subscribed: `INBOX` (inbox), `Drafts`, `Sent`, `Junk`, `Trash`,
   `Archive` (their roles), `Hostile` (none).
3. **INBOX:** `mailgen.Each(o.N, o.Seed, …)`, in transactions of at most
   1,000 messages. For each message: `blobs.Put` the raw bytes; build a
   `MessageHeader` (below) with `UID` = the message index; then per
   transaction `InsertHeaders` into INBOX, and for each returned ID
   `SetBody` with its blob ID and `IndexMessage(id, store.SearchDocFor(h))`,
   and finally `AssignThreads` for the transaction's IDs.
4. **Hostile:** when `o.Hostile` is set, every `*.html` file in it, in
   name order, becomes a message with UIDs from 1: headers `From: Hostile
   <hostile@mailtest.test>`, `To: test1@mailtest.test`,
   `Subject: Hostile: <file name without .html>`,
   `Date: Mon, 05 Oct 2026 12:00:00 +0000`,
   `Message-ID: <hostile-<file name without .html>@uifixture.test>`,
   `MIME-Version: 1.0`, `Content-Type: text/html; charset=utf-8`, a blank
   line, then the file's bytes; stored exactly like INBOX messages, in the
   Hostile mailbox.

**MessageHeader from a raw message** (write one helper): parse with
`message.Read` and `mail.Header{Header: entity.Header}` (go-message):
`Subject()`, `AddressList("From")` (first address), `AddressList("To")`,
`AddressList("Cc")`, `Date()`; `MessageID` from the `Message-ID` header
and `InReplyTo` / `References` with `mimex.ParseMessageIDs` (In-Reply-To
is the first ID). `InternalDate` and `Date` are the Date header. `Size` is
`len(raw)`. `Preview` is `mimex.Preview(text, 200)` of
`mimex.BodyText(raw)`. `Parts` come from `mimex.WalkParts`: path, content
type, disposition, filename, content ID, and the size of the decoded body.
`HasAttachments` is true when a part's disposition is `attachment`, or it
has a filename and its disposition is not `inline`. **Flags** from the
mailgen flags string: `S` → Seen, `R` → Answered, `F` → Flagged with
`Color` 1.

## Tests (given, do not edit)

`tools/uifixture/uifixture_test.go`: TestBuildAccountAndMailboxes,
TestBuildMessages, TestBuildHostileMailbox, TestRunFlags.

## Gotchas

- A whole 100,000-message build in one transaction is slow and memory
  hungry; commit every 1,000.
- `mail.Header.AddressList` fails on an absent header: treat an error as no
  addresses.
- go-message decodes text parts to UTF-8 for you; count the bytes you read
  as the part's size.

## Out of scope

Running maild or the app on the result (the planner's app tests), and every
file except `tools/uifixture/main.go`.

## Done when

`make accept T=0036` and `make check` pass, and only
`tools/uifixture/main.go` changed.
