---
id: "0027"
title: Build the sidebar's sections from mailboxes
milestone: M2
size: S
touch:
  - app/src/lib/mailboxTree.ts
given:
  - app/src/lib/mailboxTree.test.ts
acceptance: make ui-vitest F=src/lib/mailboxTree.test.ts
---
# T-0027: Build the sidebar's sections from mailboxes

## Goal

The sidebar shows Favorites and then each account's mailboxes in Mail.app's
order, nested by hierarchy (`docs/specs/ui.md`, Sidebar). Implement
`buildSidebar`, the pure function that turns `mailbox.list` output into
display rows.

## Read first

- `app/src/lib/mailboxTree.ts`: the types (`SidebarItem`, `SidebarSection`,
  `SidebarIcon`) and the stub.
- `app/src/rpc/gen/api.ts`: `Account`, `Mailbox`, `MailboxRole`.
- `docs/specs/ui.md`, "Sidebar": order and icons.
- The given test `app/src/lib/mailboxTree.test.ts`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the types and the signature; replace the stub's doc comment and the
file's "Task T-0027 …" sentence with the rules below.

**Sections.** First `{ key: "favorites", title: "Favorites", items }` with
two rows: `all-inboxes` ("All Inboxes", icon `inbox`, unread = the sum of
`unread` over every mailbox with role `inbox`) and `flagged` ("Flagged",
icon `flag`, unread 0); both depth 0 and selectable. Then one section per
account, in the order of `accounts`: key `account:<id>`, title the
account's `email`, `accountId` set, items from that account's mailboxes.

**Rows of an account.**

- Each mailbox becomes a row: key `mailbox:<id>`, `mailboxId` set,
  `unread` from the mailbox, selectable.
- **Role mailboxes** (role other than `none`) are always at depth 0, in this
  order: `inbox, drafts, sent, junk, trash, archive, all, flagged`. Their
  labels are fixed: Inbox, Drafts, Sent, Junk, Trash, Archive, All Mail,
  Flagged. Icons: inbox `inbox`, drafts `file`, sent `send`, junk
  `shield-alert`, trash `trash-2`, archive `archive`, all `archive`, flagged
  `flag`.
- **Other mailboxes** (role `none`) are labeled with their `name`, use icon
  `folder`, and nest by path: split `path` on `delimiter` (no nesting when
  the delimiter is empty). A mailbox's parent is the mailbox whose path is
  its path without the last component. If that parent is not in the list,
  a row is synthesized for it: key `path:<accountId>:<parent path>`, label
  the last component of the parent path, icon `folder`, unread 0, not
  selectable, no `mailboxId`; synthesized parents nest the same way.
- **Order:** the role mailboxes in role order, then the top-level folders
  sorted by label (`a.localeCompare(b, undefined, { sensitivity: "base" })`).
  Every row is followed directly by its children (depth + 1), sorted the
  same way, recursively. A folder whose parent is a role mailbox (for
  example `INBOX.Receipts` under `INBOX`) is that mailbox's child.

## Tests (given, do not edit)

`app/src/lib/mailboxTree.test.ts`.

## Gotchas

- Build a tree first (a map from path to node per account), then flatten it
  depth-first; do not sort the flat list.
- Role mailboxes never nest under anything, even when their path has a
  parent (`INBOX.Sent` stays at depth 0).
- Only mailboxes of the section's own account belong to it.

## Out of scope

The `Sidebar` component (T-0030), and every file except
`app/src/lib/mailboxTree.ts`.

## Done when

`make accept T=0027`, `make check` and `make ui-check` pass, and only
`app/src/lib/mailboxTree.ts` changed.
