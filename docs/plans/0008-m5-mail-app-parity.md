# Plan 0008: M5 Mail.app parity

M5 brings Frostmail's mail to Mail.app's feature set: flag colors in the
sidebar and named flags, VIPs, smart mailboxes, rules, Send Later, Remind
Me, a chosen Undo Send delay, and the smaller features of the parity
checklist ([specs/parity.md](../specs/parity.md)), each with a test. It
runs during [M4](0006-m4-daily-driver.md)'s seven-day trial, after
[M4.5](0007-m4.5-people-calendar-tasks.md) in the
[roadmap](0002-roadmap.md). Editing events, which plan 0007 named the next
milestone, follows M5. Apple Intelligence and iCloud's services are out of
scope; Gmail's categories get a spike.

## Decisions for M5

Phase 1 records these as ADRs 0023–0026 before the designs they shape
change; the user's choices of 2026-10-10 are recorded as they were made.

- **One condition language for smart mailboxes and rules** (ADR-0023).
  Both store the same structure: conditions joined by any or all, each a
  field, an operator and a value, as in Mail.app's editors (From contains,
  Date Received in the last N days, Sender is a VIP, Flag color is,
  Account is, Mailbox is). `internal/search` compiles it to the SQL it
  already builds for the search language, so a search can be saved as a
  smart mailbox. A smart mailbox opens as a view like any mailbox, so its
  list and count stay live; a rule runs the same SQL over the messages it
  is given.
- **Rules run in maild** (ADR-0024): on new mail in an inbox, after it is
  stored and before it is announced, in the user's order until Stop
  Evaluating Rules; and on a selection with Apply Rules. Actions are moves,
  copies, flags, read marks, deletes and notifications, queued as offline
  ops in the transaction that records the message as evaluated, so a
  restart neither skips nor repeats a rule. Rules act while the app is
  closed, since maild is a user service. They are maild's, on this machine:
  no Sieve, no Gmail filters, no iCloud sync. A read-only account's mail
  runs no rule that writes. Rules do not send: reply, forward and redirect
  actions wait until after M5 (the user, 2026-10-10), since they would send
  mail with nobody looking.
- **maild keeps the clock** (ADR-0025). Send Later is an outbox row due at
  the chosen time, built when it goes so its `Date` is right, listed in a
  Send Later mailbox and editable until then. Remind Me stores a time on
  the message; when it comes, maild brings the message back to the top of
  its account's inbox (moving it back if it left) and notifies. Undo Send's
  delay becomes a setting: off, 10, 20 or 30 seconds. What falls due while
  maild is stopped happens at its next start.
- **Preferences live in maild** (ADR-0026): VIPs, flag names, favorites,
  the notification scope, blocked senders, muted conversations and the
  Undo Send delay are stored by maild, so the app and `mailctl` agree and
  rules and notifications can use them with the app closed. VIPs are
  addresses; adding a person adds each of their addresses. Notifications
  take Mail.app's scopes: Inbox only (today's behavior and the default),
  VIPs, Contacts, All Mailboxes, or a smart mailbox.
- **Unsubscribe with one click** ([ADR-0027](../adr/0027-unsubscribe-with-one-click.md),
  the user's choice): after a confirmation, maild sends the RFC 8058
  `POST` to the sender's server when the provider's DKIM verdict passes,
  with nothing of the user's in the request and never to an address on the
  user's network; otherwise it mails the `mailto:` address or the app opens
  the page.
- **The trial stays safe.** M5's migrations are additive. Nothing new acts
  on mail until the user makes it (a rule, a reminder, a scheduled send, a
  block), the Undo Send delay stays 10 seconds, and notifications stay
  Inbox only.
- **API, protocol 1, additive.**

## Phase 1 — Contracts and foundations (planner)

- This plan and [specs/parity.md](../specs/parity.md), with a test named
  for every Have row.
- ADRs 0023–0026; `design/organize.md` (conditions, smart mailboxes,
  rules, VIPs, flags, favorites, mute and block); additions to
  [design/send.md](../design/send.md) (Send Later, the Undo Send setting,
  redirect, unsubscribe), [design/sync.md](../design/sync.md) (mailbox
  operations, Remind Me's return) and
  [design/desktop.md](../design/desktop.md) (the notification scope).
- UI specs: [specs/ui.md](../specs/ui.md) (the sidebar's Flagged colors,
  VIPs and Smart Mailboxes, sorting, the reader's banners, menus and keys),
  [specs/settings-ui.md](../specs/settings-ui.md) (a General pane and a
  Rules pane), and `specs/organize-ui.md` (the condition editor the smart
  mailbox sheet and the rule editor share).
- IDL and `make gen`: `settings.get` and `set`, `vip.*`, `smartMailbox.*`,
  `rule.*` with `rule.apply`, `mailbox.create`, `rename`, `delete`,
  `message.remind`, `draft.send {sendAt}`, `outbox.reschedule`;
  `ViewQuery` gains `smartMailboxId`, `flagColor`, `vip` and `sort`; their
  events. Migration 0011.
- New accounts writable by default (P-952): the add-account form's
  Read-only box starts clear.
- **Done when:** `make check` and `make ui-check` are green, the ADRs and
  specs are merged, and Phase 2's cards are ready.

## Phase 2 — Flags, VIPs and the notification scope

- Planner: the settings and VIP stores, `ViewQuery.flagColor` and `vip`,
  and the scope in `announce.go` (what notifies).
- Cards: Flagged with a row per color and real counts; VIPs in the app
  (the star in the list and the reader, the VIPs section, adding from the
  header and the contact card); the General pane (notification scope,
  flag names).
- **Done when:** Flagged shows a row per color in use with its count, and
  a color set by another client on the Dovecot account lists under its
  row; renamed flags label the sidebar and menus; mail from a VIP shows
  the star, lists under VIPs and is the only mail that notifies under the
  VIPs scope.

## Phase 3 — Smart mailboxes

- Planner: the condition compiler in `internal/search` (every condition,
  any and all, Trash and Sent left out unless asked) with given tests,
  `ViewQuery.smartMailboxId` and counts.
- Cards: the smart mailbox store and `smartMailbox.*`; the condition
  editor; smart mailboxes in the sidebar with their sheet and Save from the
  search field; the filter bar's To: Me, Cc: Me and VIPs (P-205).
- **Done when:** smart mailboxes over several accounts, with any and all
  conditions, list exactly the messages their given tests name and stay
  live as mail arrives and changes; one saved from a search lists what the
  search does; a smart mailbox can be the notification scope.

## Phase 4 — Rules

- Planner: evaluation in `mailsync` (after insert, before announce, once
  per message), actions as ops, read-only accounts, `rule.apply`.
- Cards: the Rules pane (the list in order, the editor over the condition
  editor); Apply Rules in the menus; `mailctl rules`.
- **Done when:** on the Dovecot account and the throwaway Gmail account,
  new mail that matches a rule is moved, flagged or marked as the rule
  says exactly once, a restart in the middle included, and before it would
  notify; Apply Rules does the same to stored mail; a read-only account's
  mail is untouched.

## Phase 5 — Send Later, Remind Me and Undo Send

- Planner: `draft.send {sendAt}` building at send time,
  `outbox.reschedule`, the Remind Me store and its timer (as
  `internal/reminders` keeps alarms), the return to the inbox, the delay
  setting.
- Cards: Send Later (the Send menu's times, the Send Later mailbox, edit
  and cancel; scheduled rows are not undo toasts); Remind Me (the menu, the
  reader's banner, the clock in the row); the Undo Send setting in the
  General pane.
- **Done when:** a message scheduled for a time goes out then with the app
  closed, with that time as its `Date`, and at maild's next start when it
  was stopped; a reminded message is back at the top of the inbox at its
  time, from the archive too, and notifies; Off sends at once and 30
  seconds waits 30.

## Phase 6 — The rest of the checklist

- Planner: mailbox create, rename and delete as ops (labels on Gmail);
  Erase Deleted Items and Erase Junk Mail (permanent, so confirmed);
  redirect and forward as attachment (what is sent); unsubscribe (ADR-0027:
  the one-click client with its address rule, mailto through the outbox);
  undo as the inverse op; mute, block, role overrides, favorites and sort.
- Cards, grouped: the sidebar's context menu and mailbox operations; drag
  and drop; sorting and the conversations toggle; the reader's raw source,
  all headers, Print and Save As; the unsubscribe banner; mute and block;
  undo; contact photos in the list; favorites; the remaining shortcuts.
- **Done when:** every M5·6 row of the checklist is Have with its test.

## Phase 7 — Categories spike

- Gmail's categories over IMAP (`X-GM-RAW "category:promotions"` searches
  on All Mail) against classifying locally, measured on the user's Gmail.
- **Done when:** an ADR decides whether Frostmail shows categories and
  how, and P-901 is Have, Later or Out accordingly.

Phases 2 to 7 overlap; Phase 3's compiler comes before Phase 4's rules and
the condition-based rows of Phases 2 and 3.

## Exit

- **Done when:** [specs/parity.md](../specs/parity.md) holds no M5 row,
  every Have row names its test, and every Later and Out row gives its
  reason.

## Cards (Codex, [ADR-0021](../adr/0021-run-task-cards-with-codex.md))

Numbered from T-0104 as each phase's cards are written. The planner keeps
what ADR-0021 reserves, and in M5 also the condition compiler, rule
evaluation, the Remind Me timer and everything that sends or deletes for
good: redirect, unsubscribe, rule actions and erasing.

## Later / ideas

- Rule actions that send (P-704).
- Smart mailbox folders, On My Mac, Follow Up, classic layout: the
  checklist's Later rows, each with its reason.

## Open questions

- **Categories:** Phase 7.

## References

- Implements: [specs/parity.md](../specs/parity.md), and the designs and
  specs Phase 1 writes
- Roadmap: [plans/0002](0002-roadmap.md)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
