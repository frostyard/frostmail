---
id: "0104"
title: VIPs and Flagged's colors in the sidebar
milestone: M5
size: L
touch:
  - app/src/lib/mailboxTree.ts
  - app/src/data/stores.ts
  - app/src/features/sidebar/Sidebar.tsx
  - app/src/app/useSidebarCounts.ts
  - app/src/app/SidebarContainer.tsx
  - app/src/app/ToolbarContainer.tsx
  - app/src/app/MainWindow.tsx
given:
  - app/src/lib/mailboxTree.sources.test.ts
  - app/src/data/stores.sources.test.ts
  - app/src/app/useSidebarCounts.test.ts
  - app/src/app/SidebarSources.test.tsx
acceptance: make ui-vitest F="src/lib/mailboxTree.sources src/data/stores.sources src/app/useSidebarCounts src/app/SidebarSources"
---
# T-0104: VIPs and Flagged's colors in the sidebar

## Goal

Favorites gains VIPs, with a row per VIP person, and Flagged gains a row
per flag color in use, named as the user named the flags. Each row lists
its messages through view conditions, and their counts come from one
`view.count`. The toolbar's title and the search scope bar name the new
sources.

## Read first

- `docs/specs/ui.md`: "Sidebar" (the VIPs, Flagged and counts bullets).
- `docs/design/organize.md`: "Views" and "VIPs".
- `app/src/lib/mailboxTree.ts`: `buildSidebar`, `SidebarItem`.
- `app/src/data/stores.ts`: `Source`, `sourceKey`, `sourceFromKey`,
  `sourceQuery`; `useMail`'s `vips` and `settings`.
- `app/src/data/session.tsx`: `useClient`, `client.transport.onEvent`.
- `app/src/lib/flags.ts`: `flagLabel`.
- `app/src/features/sidebar/Sidebar.tsx`: `ICONS`, `Row`.
- `app/src/app/SidebarContainer.tsx`, `ToolbarContainer.tsx`
  (`toolbarTitle`), `MainWindow.tsx` (`MailScope`).
- `app/src/rpc/gen/api.ts`: `Conditions`, `ViewCount`, `Vip`,
  `client.view.count`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`lib/mailboxTree.ts`**:
  - `SidebarIcon` gains `"star"` and `"user"`; `SidebarItem` gains
    `flagColor?: number`, set only on Flagged's color rows.
  - `interface VipGroup { key: string; label: string; addresses: string[] }`
    and `vipGroups(vips: Vip[]): VipGroup[]`: in `vip.list` order, one
    group per `personId` (its addresses in list order), else one per
    address; `key` is `vip:` plus the group's first address; `label` is
    the first VIP's `name`, else its address.
  - `interface SidebarCounts { vips: ViewCount; vip: Record<string,
    ViewCount>; flagged: ViewCount; colors: ViewCount[] }`, `vip` by group
    key, `colors[n - 1]` for color n.
  - `interface SidebarExtras { vips?: Vip[]; flagNames?: readonly string[];
    counts?: SidebarCounts }` and `buildSidebar(accounts, mailboxes,
    extras = {})`. Favorites is All Inboxes, the unified rows (as now),
    then, only when `vips` is not empty, `vips` ("VIPs", icon `star`,
    depth 0, `unread` = `counts.vips.unread`) and a row per group (key,
    label, icon `user`, depth 1, `unread` = that group's unread), then
    `flagged` ("Flagged", icon `flag`, `unread` = `counts.flagged.total`,
    the number of flagged messages), then per color whose
    `counts.colors[n - 1].total` is above 0: `flag:<n>`, labeled
    `flagLabel(n, flagNames)`, icon `flag`, depth 1, `unread` = that total,
    `flagColor: n`. Every new row is selectable. Without counts, VIP rows
    count 0, Flagged 0, and there are no color rows.
- **`data/stores.ts`**:
  - `Source` gains `{ kind: "flagColor"; color: number }`,
    `{ kind: "vips" }` and `{ kind: "vip"; key: string; addresses: string[] }`.
  - `sourceKey`: `flag:<color>`, `vips`, the vip's `key`.
  - `sourceFromKey(key, vips: Vip[] = [])`: `vips`; `flag:1`…`flag:7`;
    `vip:<address>` when `vipGroups(vips)` has that key (its addresses),
    else `null`; every other key as now.
  - `sourceQuery`: a color is `{ conditions: { match: "all", conditions:
    [{ field: "color", op: "is", value: "<n>" }] } }`, VIPs is
    `vip is "true"` the same way, and a VIP is `match: "any"` of `from is`
    each address, in order; each with `threads: true` when conversations
    are on, as the other sources are.
- **`app/useSidebarCounts.ts`** (new):
  - `countQueries(groups: VipGroup[]): ViewQuery[]`: VIPs, then each
    group, then `{ flagged: true }`, then colors 1 to 7, each as
    `sourceQuery(..., false)` gives it.
  - `toCounts(groups, results: ViewCount[]): SidebarCounts`, the inverse
    (a missing answer is `{ total: 0, unread: 0 }`).
  - `useSidebarCounts(groups): SidebarCounts | undefined`: one
    `client.view.count({ queries: countQueries(groups) })` when mounted,
    when the groups change, and 300 ms after the last `mailbox.changed`,
    `vip.changed` or `settings.changed` event; failures leave the last
    counts.
- **`Sidebar.tsx`**: `star` and `user` icons (Lucide `Star`, `User`); a row
  with `flagColor` draws its icon in `text-flag-<n>` (listed whole, as
  `MessageRow`'s `FLAG_CLASSES` are) instead of `text-accent`, except on a
  selected row in a focused sidebar.
- **`SidebarContainer`**: `buildSidebar` with `useMail`'s `vips`,
  `settings?.flagNames` and `useSidebarCounts(vipGroups(vips))`;
  `sourceFromKey(key, vips)`.
- **Titles:** the toolbar's title, and `MailScope`'s source label, are the
  row's label for the new sources: "VIPs", the VIP group's label, the
  flag's name. Their subtitle is "N messages", as Flagged's.

## Tests (given, do not edit)

`mailboxTree.sources.test.ts`, `stores.sources.test.ts`,
`useSidebarCounts.test.ts` and `SidebarSources.test.tsx` (the window
against the mock maild). Every other app test must keep passing.

## Gotchas

- `stores.ts` may import `vipGroups` from `mailboxTree.ts`; `mailboxTree.ts`
  must not import from `stores.ts` (a cycle).
- Switches over `Source.kind` (`sourceKey`, `sourceQuery`, `toolbarTitle`)
  must handle the new kinds or TypeScript reports a missing return.
- The mock answers `view.count` and the conditions above; it refuses
  fields it does not know, so use exactly those.
- Keep `useSidebarCounts`'s effect from re-running on every render: key it
  on the groups' content, not the array's identity.

## Out of scope

maild, the mock, the VIP star in rows and the reader (T-0105), the General
pane and the menus' flag names (T-0106), and every file not under `touch`.

## Done when

`make accept T=0104` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
