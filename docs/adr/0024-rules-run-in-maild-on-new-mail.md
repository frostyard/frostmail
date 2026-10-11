# 0024 — Rules run in maild on new mail

- **Status:** Accepted
- **Date:** 2026-10-10

## Context

Mail.app's rules act on incoming mail while Mail is running: each rule's
conditions are checked in the list's order, a match performs the rule's
actions, and Stop Evaluating Rules ends the run for that message. Apply
Rules runs them on chosen messages (parity rows P-414, P-701 to P-707).

maild runs as a user service whether or not the app is open
([design/desktop.md](../design/desktop.md)). Its account actor stores new
mail in a reconcile or Gmail pass. It collects the new inbox mail of a
folder that has synced before, in memory (`actor.fresh`), and announces it
when the pass settles. Moves, copies, flag changes and deletes change the
store and queue an offline op in one transaction, which the actor replays
against the server ([design/sync.md](../design/sync.md)). Gmail's
operations are label edits ([ADR-0012](0012-gmail-labels-as-memberships.md)).
A crash loses `actor.fresh`, which costs a notification but would cost a
rule.

The user chose (2026-10-10) that rules do not send mail in M5.

## Decision

- **maild runs rules**, on this machine, whether or not the app is open.
  They live in maild's store, in order; no Sieve script or Gmail filter is
  written, and nothing syncs them elsewhere.
- **New inbox mail waits for the rules, durably.** A message a pass stores
  in an account's inbox (with `\Inbox` on Gmail), in a folder that has
  synced before, is marked as waiting for rules, in the transaction that
  inserts it. The first sync of a folder marks nothing, as it announces
  nothing.
- **Before announcing,** the actor runs the enabled rules over the waiting
  messages. Each rule's conditions are an [ADR-0023](0023-one-condition-language-for-smart-mailboxes-and-rules.md)
  predicate restricted to those messages. Rules go in order, and a message
  that met Stop Evaluating Rules leaves the run. One transaction makes
  every action's local change, queues its ops, and clears the waiting
  mark. A crash before it leaves the messages waiting for the next start;
  a crash after it has nothing left to do. Each message is evaluated
  exactly once.
- **Actions** are Move to Mailbox, Copy to Mailbox, Mark as Read, Mark as
  Flagged (with a color), Delete (to Trash, as `message.delete`), Send
  Notification (announce the message whatever the notification scope),
  and Stop Evaluating Rules. A move or copy names a mailbox, and acts only
  on messages of that mailbox's account. They are the same
  transaction-level operations `message.*` uses, so Gmail gets label edits.
- **Rules never send.** Reply, forward and redirect actions wait until
  after M5.
- **Apply Rules** (`rule.apply`) runs the enabled rules, in order, on
  chosen messages in any mailbox, waiting or not, in one transaction.
- **Read-only accounts run no rules.** Their mail is never marked as
  waiting, and `rule.apply` on their messages is a `conflict`.
- A rule whose mailbox is gone skips that action, and the Rules pane marks
  it. A rule's failure on one message is logged and does not stop the run
  for the others.

## Consequences

- Rules act while the app is closed, sooner than Mail.app's would, and
  before the notification, so mail a rule moves out of the inbox does not
  notify unless the rule says to.
- Mail that arrives while the computer is off is filtered when maild next
  syncs. A phone sees it unfiltered until then.
- The insert path, Gmail's included, gains the waiting mark, and the
  actions need transaction-level forms of `SetFlags`, `Move`, `Copy` and
  `Delete`, which today each open their own transaction.
- A move to another account's mailbox is not a rule action; that would be
  a download and an upload.

## Alternatives considered

- **Server-side rules (Sieve, Gmail filters):** they would act while the
  computer is off, but each provider has its own language and limits,
  iCloud offers no API for its rules, and Mail.app does not do it either.
- **Rules in the app:** they would act only while the app is open.
- **Run rules on every new message, any mailbox:** Mail.app's rules see
  incoming mail, which is the inbox. Mail that a server filter already
  filed is left where it was filed.

## References

- Shapes: [design/organize.md](../design/organize.md),
  [specs/settings-ui.md](../specs/settings-ui.md) (the Rules pane),
  [specs/parity.md](../specs/parity.md) (P-414, P-701 to P-707)
- Builds on: [ADR-0023](0023-one-condition-language-for-smart-mailboxes-and-rules.md),
  [ADR-0012](0012-gmail-labels-as-memberships.md),
  [design/sync.md](../design/sync.md) (offline actions)
