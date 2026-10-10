---
id: "0098"
title: Add and remove addresses in Settings
milestone: M4.5
size: M
touch:
  - app/src/features/settings/IdentityEditor.tsx
  - app/src/app/SettingsWindow.tsx
  - app/src/rpc/mock/compose.ts
given:
  - app/src/features/settings/IdentityEditor.test.tsx
  - app/src/features/settings/IdentityEditor.addresses.test.tsx
  - app/src/app/SettingsWindow.identities.test.tsx
acceptance: make ui-vitest F="src/features/settings src/app/SettingsWindow"
---
# T-0098: Add and remove addresses in Settings

## Goal

An account can receive mail for addresses Frostmail does not know, such
as an alias or the user's own domain, and an invitation to one cannot be
answered until Frostmail knows it is the user's. In the Signatures pane
the user adds such an address to an account, as Mail.app lists several
addresses per account, and removes one again.

## Read first

- `docs/specs/settings-ui.md`: "Signatures pane", `IdentityEditor` (the
  bar, the add form, selecting the address added) and its container.
- `app/src/features/settings/IdentityEditor.tsx` (the props `onAdd` and
  `onRemove` are declared, not used) and `labels.ts`.
- `app/src/features/settings/AccountList.tsx` (the + − bar to copy) and
  `AccountForm.tsx` (the grid, the alert and the button row).
- `app/src/app/SettingsWindow.tsx`: `SignaturesPane`, `useRequest`,
  `confirmRemove` (the app's dialog or `window.confirm`).
- `schema/rpc/identity.yaml` / `app/src/rpc/gen/api.ts`:
  `identity.create`, `identity.delete`.
- `app/src/rpc/mock/compose.ts`: the mock's `identity.list` and
  `identity.update`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`IdentityEditor`** exactly as the spec says: the column with the bar
  (Add Address, Remove Address), the add form in place of the identity
  form, and the selection of a new identity with the email added.
- **`SignaturesPane`**: `onAdd` calls `identity.create({ accountId,
  email, name })`, leaving out `name` when it is ""; `onRemove` asks
  "Remove EMAIL? Drafts from it will be sent from ACCOUNT_EMAIL." and,
  when confirmed, calls `identity.delete({ id })`. Both run through
  `run` and reload the list after.
- **Mock maild** (`compose.ts`): `identity.create` adds a non-default
  identity (the next ID; the name given, or the account's default
  identity's), `notFound` for an unknown account and `conflict` for an
  address the account has, ignoring case ("ADDRESS is already an address
  of account N"); `identity.delete` removes one, `notFound` for an
  unknown ID and `conflict` for a default. Both emit `account.changed`
  for the account.

## Tests (given, do not edit)

`IdentityEditor.test.tsx` (now passes the new props),
`IdentityEditor.addresses.test.tsx` and `SettingsWindow.identities.test.tsx`.
The other settings tests must keep passing.

## Gotchas

- "Add" and "Add Address" are different buttons; names are matched
  exactly.
- The address typed may differ in case from an existing one; only an
  identity new since Add was pressed is selected.
- Use the classes in `labels.ts` (`BUTTON`, `PRIMARY_BUTTON`, `FIELD`,
  `LABEL`, `GRID`, `ALERT`).

## Out of scope

maild, the compose window's From menu, and every file not under `touch`.

## Done when

`make accept T=0098` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
