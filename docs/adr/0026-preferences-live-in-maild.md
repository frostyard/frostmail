# 0026 — Preferences live in maild

- **Status:** Accepted
- **Date:** 2026-10-10

## Context

M5 brings preferences Frostmail has not had: VIPs, flag names, favorite
mailboxes, which new mail notifies (Inbox only, VIPs, Contacts, All
Mailboxes or a smart mailbox), blocked senders, muted conversations, the
Undo Send delay, smart mailboxes and rules. Some of them change what
maild does with the app closed: rules, notifications, blocking and Undo
Send. All of them should look the same from the app and from `mailctl`.

Today maild keeps per-account settings in `accounts` (`notify`,
`readOnly`, `syncDays`), changed with `account.update`. The app keeps its
own view state, such as the window, the filter bar's choice and
conversation mode. Mail.app syncs its VIPs, rules and smart mailboxes
through iCloud.

## Decision

- **maild stores the preferences** in its database, and the app and
  `mailctl` read and change them through the API. The app keeps only view
  state.
- **Scalars are one record.** `settings.get` returns a `Settings` record:
  the Undo Send delay, the notification scope, the flag names, and what
  happens to blocked mail. `settings.set` changes the fields it is given,
  like `account.update`, and maild checks each. They are stored as rows of
  a key and value table with typed accessors, and every change emits
  `settings.changed`.
- **Lists have their own tables and methods,** each with an event:
  `vip.*`, `smartMailbox.*`, `rule.*`, favorites, blocked senders and
  muted conversations.
- **VIPs are addresses,** lowercased. Adding a person from People or a
  contact card adds each of their addresses; the VIPs mailbox shows one
  row per person where People knows one, else per address.
- **The notification scope** is `inbox` (the default and today's
  behavior), `vips`, `contacts`, `all` or a smart mailbox. It applies
  after an account's own `notify` and read-only switches. Muted
  conversations and blocked senders never notify, and a rule's Send
  Notification always does.
- **Local to this machine.** Nothing syncs to Mail.app or iCloud.
  Exporting and importing preferences is later work.

## Consequences

- Rules, notifications, blocking and Undo Send behave the same with the
  app open or closed, and `mailctl` can script every preference.
- Every new preference is an IDL change, a migration and an event. The
  app re-reads on the event, as it does for accounts.
- Moving to a new machine loses them until there is an export, which the
  database backup covers meanwhile.

## Alternatives considered

- **App-side storage (localStorage):** maild could not use VIPs, the
  scope or the delay with the app closed, and `mailctl` would not see
  them.
- **One JSON document of all preferences:** simple to store, but lists
  such as rules want ordering and per-item events, and concurrent edits
  from the app and `mailctl` would overwrite each other.
- **IMAP METADATA or a mailbox of settings messages, to sync through the
  server:** iCloud and Gmail do not support METADATA, and a settings
  folder would show in every other client.

## References

- Shapes: [design/organize.md](../design/organize.md),
  [specs/settings-ui.md](../specs/settings-ui.md) (the General pane)
- Builds on: [ADR-0002](0002-daemon-and-thin-clients.md),
  [ADR-0003](0003-sqlite-store.md)
