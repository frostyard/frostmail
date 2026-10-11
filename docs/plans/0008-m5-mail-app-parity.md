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
  Account is, Mailbox is). The store compiles it to SQL beside the search
  language's, and `internal/search` turns a search into conditions, so a
  search can be saved as a smart mailbox. A smart mailbox opens as a view like any mailbox, so its
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
  the chosen time, built with that time as its `Date` (rebuilt when
  rescheduled), listed in a Send Later mailbox and editable until then. Remind Me stores a time on
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
- ADRs 0023–0026; [design/organize.md](../design/organize.md)
  (conditions, smart mailboxes, rules, VIPs, flags, settings, the
  notification scope, Remind Me); additions to
  [design/send.md](../design/send.md) (Send Later, the Undo Send setting),
  [design/desktop.md](../design/desktop.md) and
  [design/storage.md](../design/storage.md). Phase 6's mechanisms (mute,
  block, favorites, sort, mailbox operations, redirect, unsubscribe) are
  designed when it starts.
- UI specs for Phases 2–5: [specs/ui.md](../specs/ui.md) (the sidebar's
  Flagged colors, VIPs, Smart Mailboxes and Remind Me; the VIP star; the
  Remind Me banner and menus), [specs/compose-ui.md](../specs/compose-ui.md)
  (Send Later), [specs/settings-ui.md](../specs/settings-ui.md) (a General
  pane and a Rules pane), and [specs/organize-ui.md](../specs/organize-ui.md)
  (the condition editor the smart mailbox sheet and the rule editor
  share).
- IDL and `make gen` for Phases 2–5: `settings.get` and `set`, `vip.*`,
  `smart.*`, `rule.*` with `rule.apply`, `message.remind`,
  `draft.send {sendAt}`, `outbox.reschedule`, `view.count`; `ViewQuery`
  gains `conditions` and `smartMailboxId`; their events. Migration 0011,
  engine stubs. Phase 6's contracts come with Phase 6.
- The condition compiler (`store.CompileConditions`: every field and
  operator, any and all, Trash and Sent left out unless asked) and
  `search.ToConditions`, with tests, since Phases 2 to 4 all build on
  them.
- New accounts writable by default (P-952): the add-account form's
  Read-only box starts clear.
- **Done when:** `make check` and `make ui-check` are green, the ADRs and
  specs are merged, and Phase 2's cards are ready.

## Phase 2 — Flags, VIPs and the notification scope — done

- Planner (done with Phase 1, so the cards meet a working maild): the
  settings and VIP stores and services, `view.count`, the scope in
  `announce.go` (what notifies), and the Undo Send delay as a setting.
- Cards: Flagged with a row per color and real counts; VIPs in the app
  (the star in the list and the reader, the VIPs section, adding from the
  header and the contact card); the General pane (notification scope,
  Undo Send, flag names).
- **Done when:** Flagged shows a row per color in use with its count, and
  a color set by another client on the Dovecot account lists under its
  row; renamed flags label the sidebar and menus; mail from a VIP shows
  the star, lists under VIPs and is the only mail that notifies under the
  VIPs scope; Undo Send's Off sends at once and 30 seconds waits 30.
- **Evidence (2026-10-11):** T-0104, T-0105 and T-0106 each verified on
  Codex's first attempt; the checklist's P-103, P-104, P-208, P-312,
  P-408, P-505 and P-802 name their tests. A color set by another client
  lists under its row through the existing flag sync
  (`TestFlagsFromIMAP`, `TestFlagChangesReplayToServer`) and the `color`
  condition (`TestCompileEveryCondition`), not yet by one run against
  Dovecot.

## Phase 3 — Smart mailboxes — done

- Planner (done before the cards): the smart mailbox store and `smart.*`
  with `smart.fromSearch`, `ViewQuery.smartMailboxId` and `filter` (so a
  filter combines with a source's own conditions), views recomputed on
  `smart.changed`, counts, and a smart mailbox as the notification scope.
- Cards: the smart mailbox store and `smart.*`; the condition
  editor; smart mailboxes in the sidebar with their sheet and Save from the
  search field; the filter bar's To: Me, Cc: Me and VIPs (P-205).
- **Done when:** smart mailboxes over several accounts, with any and all
  conditions, list exactly the messages their given tests name and stay
  live as mail arrives and changes; one saved from a search lists what the
  search does; a smart mailbox can be the notification scope.
- **Evidence (2026-10-11):** T-0107 to T-0110 each verified on Codex's
  first attempt. `TestSmartMailboxViews` and
  `TestSmartMailboxAcrossAccounts` list exactly the named messages, any
  and all, over two accounts, Trash and Sent left out unless included;
  `TestSmartMailboxes` shows a view following an edit (views of smart
  mailboxes recompute on every account's commits, like All Inboxes);
  `TestSearchAsConditions` shows a saved search listing what the search
  does; `TestNotifySmartScope` notifies for a smart mailbox. The
  checklist's P-105, P-205, P-602 and P-803 name their tests.

## Phase 4 — Rules — done

- Planner (done before the cards): evaluation in `mailsync` (after
  insert, before announce, once per message, through
  `messages.rules_waiting`), actions as ops through the transaction-level
  forms of the message actions, read-only accounts, `rule.*` with
  `rule.apply`. Two sync fixes came with it: queued ops replay before
  every pass, and a flags op keeps the UIDs it was queued for, so a read
  mark followed by a move (a rule's, or a user's offline) reaches the
  server.
- Cards: the Rules pane (the list in order, the editor over the condition
  editor); Apply Rules in the menus; `mailctl rules`.
- **Done when:** on the Dovecot account and the throwaway Gmail account,
  new mail that matches a rule is moved, flagged or marked as the rule
  says exactly once, a restart in the middle included, and before it would
  notify; Apply Rules does the same to stored mail; a read-only account's
  mail is untouched.
- **Evidence (2026-10-10):** T-0111 to T-0114 each verified on Codex's
  first attempt. `TestRulesActOnNewInboxMail` files, marks and flags new
  inbox mail before the announcement, with Stop and Send Notification,
  and the read mark reaches the server with the move;
  `TestRulesRunOnce` shows a rule acting once, a waiting message honored
  after a restart, and a read-only account's mail never waiting;
  `TestApplyRules` and `TestRulesApplyCommand` apply rules to stored mail.
  `TestDovecotRules` does both against Dovecot, the server's flags
  checked; `TestGmailRulesMoveToALabel` files to a label on the Gmail
  fake. Not yet run against the throwaway Gmail account itself. The
  checklist's P-414 and P-701 to P-703 name their tests.

## Phase 5 — Send Later and Remind Me — done

- Planner (done before the cards): `draft.send {sendAt}` building with
  the chosen `Date`, `outbox.reschedule`, the Remind Me store and its
  timer (`internal/reminders` has mailsync fire them each minute), the
  return to the inbox, and views ordered by `list_date`. It also closed
  the race in which a pass over a mailbox took a message that a queued
  move was taking out of it for new mail.
- Cards: Send Later (the Send menu's times, the Send Later mailbox, edit
  and cancel; scheduled rows are not undo toasts); Remind Me (the menu, the
  reader's banner, the clock in the row).
- **Done when:** a message scheduled for a time goes out then with the app
  closed, with that time as its `Date`, and at maild's next start when it
  was stopped; a reminded message is back at the top of the inbox at its
  time, from the archive too, and notifies.
- **Evidence (2026-10-11):** T-0115 to T-0118 each verified on Codex's
  first attempt (T-0117's first finish stopped on a flaky mock test of
  the planner's, fixed in a start commit). `TestSendLater` sends at the
  chosen time with it as the `Date`, after a reschedule, and a time past
  at once; `TestSendLaterCanBeEdited` returns the draft. `TestRemindMe`
  brings a filed message back to the top of the inbox and to INBOX on the
  server, announces it as a reminder, and fires a reminder that fell due
  while maild was stopped (`FireReminders` at a later time); deleting
  drops a reminder. maild sends with the app closed by design: the
  sender and the reminder scheduler are maild's. The checklist's P-306,
  P-412 and P-506 name their tests.

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
- **6a, mailboxes (2026-10-11):** maild's mailbox operations, role
  choices, erase and favorites (planner), then T-0119 to T-0121, each
  verified on Codex's first attempt. `TestMailboxOperations` and
  `TestDovecotMailboxes` reach the server, a nested rename and delete on
  Dovecot included; `TestMigration12KeepsQueuedOps` keeps a schema-11
  database's queued ops. P-102 and P-107 to P-110 are Have.
- **6b, list and reader (2026-10-11):** sorted views, `message.source`
  and `message.save`, and `people.senders` (planner), then T-0122 to
  T-0124, each verified on Codex's first attempt. P-202, P-203, P-209,
  P-304, P-307 and P-308 are Have; Print to File's PDF is not tested by
  the suite (the GTK print dialog).

## Phase 7 — Categories spike

- Gmail's categories over IMAP (`X-GM-RAW "category:promotions"` searches
  on All Mail) against classifying locally, measured on the user's Gmail.
- **Done when:** an ADR decides whether Frostmail shows categories and
  how, and P-901 is Have, Later or Out accordingly.

Phases 2 to 7 overlap once Phase 1's compiler is in.

## Exit

- **Done when:** [specs/parity.md](../specs/parity.md) holds no M5 row,
  every Have row names its test, and every Later and Out row gives its
  reason.

## Cards (Codex, [ADR-0021](../adr/0021-run-task-cards-with-codex.md))

Each phase's cards are written as it starts, from T-0104. Each has a
fixed file list, a contract and given tests checked against a reference
solution, and runs with `make task T=NNNN`.

| Card | Size | Phase | What |
| --- | --- | --- | --- |
| T-0104 | L | 2 | VIPs and Flagged's colors in the sidebar, with their counts; the new sources' titles |
| T-0105 | M | 2 | the VIP star in the list and the reader; Add to VIPs on the contact card |
| T-0106 | M | 2 | the settings window's General pane; the flags' names in the menus |
| T-0107 | L | 3 | the condition editor and what it knows of fields and operators |
| T-0108 | L | 3 | smart mailboxes in the sidebar, their sheet, Save from search |
| T-0109 | M | 3 | the filter bar's More menu: To Me, Cc Me, From VIPs |
| T-0110 | S | 3 | a smart mailbox as the notification scope |

Phase 2's cards share `ListContainer.tsx` and `ToolbarContainer.tsx`, and
Phase 3's `stores.ts` and `MainWindow.tsx`, with T-0108 building on
T-0107: run each phase's cards in order, each merged before the next
starts. The planner keeps
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
