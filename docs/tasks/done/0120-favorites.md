---
id: "0120"
title: Favorites in the sidebar
milestone: M5
size: M
touch:
  - app/src/lib/mailboxTree.ts
  - app/src/data/stores.ts
  - app/src/app/SidebarContainer.tsx
given:
  - app/src/lib/mailboxTree.favorites.test.ts
  - app/src/app/Favorites.test.tsx
acceptance: make ui-vitest F="src/lib/mailboxTree src/app/Favorites src/app/Mailboxes src/data"
---
# T-0120: Favorites in the sidebar

## Goal

Mailboxes added to Favorites (T-0119's menu item) show in the sidebar's
Favorites section in their order. Each can be opened, moved up or down,
and removed, as in Mail.app.

## Read first

- `docs/specs/ui.md`: "Sidebar", the Favorites bullet.
- `app/src/lib/mailboxTree.ts` (`buildSidebar`, `SidebarExtras`,
  `ROLE_ICON`), `app/src/data/stores.ts` (`sourceFromKey`),
  `app/src/app/SidebarContainer.tsx` (the mailbox menu T-0119 made).
- `app/src/rpc/gen/api.ts`: `Settings.favorites`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`SidebarExtras`** gains `favorites?: Mailbox[]`. `buildSidebar` puts
  them, in order, at the end of the Favorites section: key
  `favorite:<id>`, the mailbox's `name`, its role's icon, depth 0, its
  unread count, selectable.
- **`sourceFromKey("favorite:<id>")`** is `{ kind: "mailbox", mailboxId }`.
- **The container** passes the `favorites` setting's mailboxes (in the
  setting's order, unknown IDs skipped). A favorite row's context menu:
  "Remove from Favorites", "Move Up", "Move Down" (disabled at the ends),
  each a `settings.set {favorites}` with the new list.

## Tests (given, do not edit)

`mailboxTree.favorites.test.ts` and `Favorites.test.tsx`. Every other app
test must keep passing, T-0119's `Mailboxes.test.tsx` included.

## Gotchas

- Keys must stay unique in the tree: a favorite is `favorite:<id>`, not
  `mailbox:<id>`.
- `settings` can be null before it loads; favorites are then none.

## Out of scope

Drag and drop (T-0121), maild, the mock, and every file not under `touch`.

## Done when

`make accept T=0120` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
