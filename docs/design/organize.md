# Design: organizing mail

Living document. Rationale: [ADR-0023](../adr/0023-one-condition-language-for-smart-mailboxes-and-rules.md),
[ADR-0024](../adr/0024-rules-run-in-maild-on-new-mail.md),
[ADR-0025](../adr/0025-maild-keeps-send-later-remind-me-and-undo-send.md),
[ADR-0026](../adr/0026-preferences-live-in-maild.md).
Contracts: [specs/rpc-api.md](../specs/rpc-api.md) (`settings.*`, `vip.*`,
`smart.*`, `rule.*`, `message.remind`),
[specs/organize-ui.md](../specs/organize-ui.md),
[specs/parity.md](../specs/parity.md).

## Overview

Mail.app's ways of organizing mail, kept by maild: conditions, and what is
built on them (smart mailboxes, rules, filters, the notification scope);
VIPs and flag names; Remind Me. Everything here is maild's, on this
machine, and works with the app closed.

```
Conditions ──compile (store)──► SQL predicate ──┬─► view (smart mailbox, Flagged color, VIPs, filter)
                                                ├─► rules over waiting mail (mailsync, before announce)
                                                └─► notification scope (announce)
```

## Conditions

A `Conditions` is `match` (`all` or `any`) and a list of `Condition`s
`{field, op, value}`. `value` is a string; what it holds depends on the
field. maild checks every condition when a smart mailbox or rule is saved
(`invalidParams` names the first bad one). The store compiles conditions
into one predicate over `messages m` each time a view or rule needs it
(`store.CompileConditions`), so relative dates use the current day in
`time.Local`.

### Fields and operators

Field and operator names are single lowercase words, as the IDL's enums
require.

| Field | Ops | Value | SQL |
| --- | --- | --- | --- |
| `from` | `contains`, `notcontains` | words | FTS `from_text` |
| `from` | `is`, `begins`, `ends` | text | `from_addr` or `from_name`, case-insensitive |
| `to`, `cc` | `contains`, `is`, `begins`, `ends` | text | any address or name in `to_json` or `cc_json` |
| `recipient` | `contains`, `notcontains` | words | FTS `to_text` (To and Cc) |
| `tome`, `ccme` | `is` | `true`, `false` | an address of the account's identities in `to_json` or `cc_json` |
| `subject` | `contains`, `notcontains` | words | FTS `subject` |
| `subject` | `is`, `begins`, `ends` | text | `subject`, case-insensitive |
| `content` | `contains`, `notcontains` | words | FTS, every column |
| `filename` | `contains`, `notcontains` | words | FTS `attachment_names` |
| `listid` | `contains`, `is` | text | `list_id` |
| `account` | `is`, `isnot`, `anyof` | account IDs | `m.account_id` |
| `mailbox` | `is`, `isnot`, `anyof` | mailbox IDs | a membership |
| `role` | `is`, `isnot`, `anyof` | mailbox roles | a membership in a mailbox of the role |
| `received`, `sent` | `today`, `yesterday`, `thisweek`, `thismonth`, `thisyear` | none | the arrival date (`internal_date`) or the `Date` header |
| `received`, `sent` | `within`, `notwithin` | `N` and `d`, `w`, `m` or `y` (`7d`) | since the start of the day N days (weeks, 30-day months, 365-day years) ago |
| `received`, `sent` | `on`, `since`, `before` | `YYYY-MM-DD` | that day; from that day; before that day |
| `unread`, `flagged`, `attachments` | `is` | `true`, `false` | `seen`, `flagged`, `has_attachments` |
| `color` | `is`, `isnot`, `anyof` | 1–7 | flagged with that color |
| `vip` | `is` | `true`, `false` | `from_addr` in `vips` |
| `contact` | `is` | `true`, `false` | `from_addr` in `contact_emails` |
| `reminder` | `is` | `true`, `false` | a row in `message_reminders` |

- "Words" are as in the search language: each word a prefix match, all of
  them required, and a value in double quotes a phrase.
- `anyof` takes IDs, roles or colors separated by commas.
- Weeks start on Monday. "This month" and "this year" are calendar ones.
- `isnot` and `notcontains` include messages with no value at all (no
  Cc, no attachment names).
- A condition naming an account or mailbox that no longer exists matches
  nothing.

### From a search

`search.ToConditions(q)` turns a parsed search into conditions with
`match: all`, exactly: a bare word or phrase is `content contains`;
`from:`, `subject:` and `filename:` are their fields' `contains`; `to:`
and `cc:` are `recipient contains`; a negated term is `notcontains`;
`is:` and `has:` are `unread`, `flagged` and `attachments`; `in:` roles
are one `role anyof`; `after:`, `before:` and `on:` are `received
since`, `before` and `on`; `newer_than:` and `older_than:` are
`received within` and `notwithin`. A search scoped to one mailbox adds
`mailbox is`. The smart mailbox it saves includes Trash and Sent, as the
search does.

## Views

`ViewQuery` gains `conditions`, `filter` (more conditions that must also
hold: the filter bar's, beside a source's own) and `smartMailboxId`;
`store.ViewFilter` gains all three. `ViewIDs` adds the compiled predicate to its other conditions
and, for a smart mailbox, reads its row each time, so an edit shows at the
next recompute. Besides the commits that touch an account's messages,
the view manager recomputes every view with conditions when VIPs, people,
smart mailboxes or settings change, and at local midnight.

`view.count {queries}` counts what each query lists, messages and unread
ones, without opening views (`store.CountView`); the sidebar counts its
built-in sources with it. `store.MatchingIDs` runs conditions over given
messages, and `store.FilterIDs` a whole view filter (a smart mailbox's
included), for rules and the notification scope.

The sidebar's built-in sources are conditions too: Flagged's color rows
are `color is N`, VIPs is `vip is true`, one VIP is `from is
<address>` (each of a person's addresses, `any`), and Remind Me is
`reminder is true`.

## Smart mailboxes

`smart_mailboxes (id, name, position, conditions_json, include_trash,
include_sent)`, the conditions as JSON with a version. `smart.list`,
`create`, `update`, `delete` and `move` (position, the others moving
along); `smart.changed`. Unless a smart mailbox includes them, messages in
a Trash or a Sent mailbox (by role) are left out; on Gmail, where sent
mail is also in All Mail, that is still the Sent label. A view of a smart
mailbox (`ViewQuery.smartMailboxId`) reads its row at every recompute, so
an edit shows at once, and `smart.list` counts its unread messages.
`smart.fromSearch {text, mailboxId}` is `search.ToConditions`, with
`mailbox is` for a search scoped to one mailbox, for Save as Smart
Mailbox. Deleting the smart mailbox that is the notification scope makes
the scope Inbox only.

## VIPs

`vips (address, name)`, the address lowercased. `vip.list`, `add`
(addresses, or a person's ID for each of their addresses), `remove`;
`vip.changed`. The list and the reader star a VIP sender. The VIPs
section shows one row per person People knows, else per address.

## Flags

Seven colors as `$MailFlagBit0–2` ([storage.md](storage.md)). Their names
are a setting (`Settings.flagNames`, seven strings, empty for the default
name). Flagged lists every flagged message; under it, one row per color
in use, with its count.

## Settings

`settings (key, value_json)`, read and written as one `Settings` record
by `settings.get` and `settings.set`; `settings.changed` after a change.

| Setting | Values | Default |
| --- | --- | --- |
| `undoDelay` | 0, 10, 20, 30 (seconds) | 10 |
| `notifyScope` | `inbox`, `vips`, `contacts`, `all`, `smart` | `inbox` |
| `notifySmartId` | a smart mailbox, with `notifyScope: smart` | none |
| `flagNames` | seven strings | empty (Red … Gray) |

## Notifications

The actor collects the new mail of every folder that has synced before,
not only the inbox. Then `announceNew` keeps:

1. Nothing, when the account has notifications off or is read-only.
2. Unread messages only.
3. By the scope:
   - `inbox`: messages in the inbox (today's behavior).
   - `vips`: in the inbox, from a VIP.
   - `contacts`: in the inbox, from an address in People.
   - `all`: in any mailbox but Junk, Trash and Sent.
   - `smart`: matching the smart mailbox, in any mailbox.
4. Plus the messages a rule's Send Notification named, whatever the scope.

A smart mailbox deleted while it is the scope makes the scope `inbox`.

## Rules

`rules (id, position, name, enabled, conditions_json, actions_json)`.
`rule.list`, `create`, `update`, `delete`, `move`, `apply`;
`rule.changed`. Actions are
`{kind: move | copy | read | flag | delete | notify | stop, mailboxId,
color}`.

- **Waiting mail.** `messages.rules_waiting` is set, in the inserting
  transaction, on a message a pass stores in an inbox (`\Inbox` on Gmail)
  of a writable account, in a folder that has synced before. A partial
  index finds them.
- **The run.** In `settled`, before `announceNew`, the actor reads its
  account's waiting messages and the enabled rules. For each rule in
  order, it compiles the conditions, restricted to the messages still in
  the run, and records each match's actions. Every rule sees the messages
  as they were before the run: a rule that marks read does not stop a
  later "Unread is Yes" from matching. Stop takes the matched messages
  out of the rest of the run. Then one transaction applies every action
  through the transaction-level forms of `SetFlags`, `Move`, `Copy` and
  `Delete` (`setFlagsTx` and the rest in `internal/mailsync/ops.go`),
  queues their ops, and clears `rules_waiting` on every message it read.
  When that transaction fails, the marks are cleared on their own and the
  failure is logged: the messages stay as they came, and one bad message
  cannot hold the rules for the rest.
- **Order of actions** for one message: flags and read marks first, then
  copies, then the move or delete. The last move wins; a delete overrides
  any move. A move or copy acts only on messages of its mailbox's account.
  The ops replay in that order, and a flags op names the UIDs the messages
  had when it was queued, so the flags reach the server before the move
  and travel with the message.
- **Send Notification** puts the message in the run's announcement
  whatever the scope, and even when a move took it out of the inbox.
- **Replay first.** The actor replays queued ops before every pass, so a
  rule's move reaches the server before the inbox is reconciled again;
  otherwise the moved message's old UID would come back as new mail.
- `rule.apply {ids}` runs the same on chosen messages of writable
  accounts in one transaction, and returns how many matched a rule. A
  message of a read-only account makes it a conflict; a missing one,
  `notFound`.
- **Problems.** `rule.update` checks only the fields it changes, so a rule
  whose mailbox is gone can still be renamed or turned off. `Rule.problem`
  names the first action whose mailbox is gone; the run skips that move
  or copy.

## Remind Me

`message_reminders (message_id, remind_at)`, and `messages.list_date`,
which views order by. `list_date` is the arrival date, set by every
insert, until a reminder fires. `message.remind {ids, at}` sets or, with
no `at`, clears. The reminder scheduler (`internal/reminders`) also asks
for message reminders due by now. For each, one transaction:

1. deletes the reminder;
2. sets `list_date` to now;
3. moves the message back to its account's inbox if it is not there (an
   op; a label edit on Gmail);
4. emits `message.changed`.

maild then notifies, with "Reminder" before the subject.
`MessageSummary.remindAt` and `Message.remindAt` show a pending
reminder.

## Later in M5

Phase 6's mute, block, favorites, sort, role overrides and mailbox
operations are designed here when that phase starts.

## References

- Rationale: ADRs 0023–0026
- Contracts: [specs/organize-ui.md](../specs/organize-ui.md),
  [specs/rpc-api.md](../specs/rpc-api.md)
- Built in: [plan 0008](../plans/0008-m5-mail-app-parity.md), Phases 2–5
