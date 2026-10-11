---
id: "0129"
title: Blocked mail and blocked senders in the General pane
milestone: M5
size: S
touch:
  - app/src/features/settings/GeneralPane.tsx
  - app/src/app/SettingsWindow.tsx
given:
  - app/src/features/settings/GeneralPane.block.test.tsx
  - app/src/app/SettingsWindow.block.test.tsx
acceptance: make ui-vitest F="src/features/settings/GeneralPane src/app/SettingsWindow"
---
# T-0129: Blocked mail and blocked senders in the General pane

## Goal

The settings for blocking, as Mail.app's Junk Mail → Blocked: what
happens to a blocked sender's mail, and the list of blocked senders with
a way to unblock each.

## Read first

- `docs/specs/settings-ui.md`: "General pane" (Blocked mail, Blocked
  senders); `docs/design/organize.md`: "Mute and block".
- `app/src/rpc/gen/api.ts`: `BlockAction`, `Settings.blockAction`,
  `SettingsSetParams.blockAction`, `BlockedSender`, `block.remove`.
- `app/src/features/settings/GeneralPane.tsx`,
  `app/src/features/settings/labels.ts`, and `GeneralPaneContainer` in
  `app/src/app/SettingsWindow.tsx`. `useMail().blocked` holds the blocked
  list, kept current by the session.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`GeneralPane`**: optional props `blocked?: BlockedSender[]` (default
  none) and `onUnblock?(address)`. After the flag names, two `GRID`
  rows:
  - "Blocked mail:" labelling a `FIELD` `select` (disabled without
    settings) with options `mark` "Mark it, and leave it where it
    arrives" and `trash` "Move it to the Trash", its value
    `settings.blockAction`, else `mark`; a change calls
    `onChange({blockAction})`.
  - "Blocked senders:" (a `LABEL` span, as Flag names') and either a
    `ul` named "Blocked senders" with an `li` per sender, in the order
    given: the name in a span when not empty, the address in a span
    (12/16, `text-secondary`), and a `BUTTON` "Unblock" with
    `aria-label` "Unblock <address>" calling `onUnblock(address)`; or,
    with no senders, "None" in `text-secondary`.
- **`GeneralPaneContainer`**: passes the store's `blocked`, and
  `onUnblock` calls `block.remove {addresses: [address]}`, showing an
  error as `onChange`'s are shown and clearing it on success.

## Tests (given, do not edit)

`GeneralPane.block.test.tsx`, `SettingsWindow.block.test.tsx`. Every
other app test must keep passing, the General pane's included.

## Out of scope

Adding blocked senders here (the contact card does), maild, the mock,
and every file not under `touch`.

## Done when

`make accept T=0129` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
