---
id: "0055"
title: List accounts in the settings window
milestone: M4
size: S
touch:
  - app/src/features/settings/AccountList.tsx
given:
  - app/src/features/settings/AccountList.test.tsx
acceptance: make ui-vitest F=src/features/settings/AccountList.test.tsx
---
# T-0055: List accounts in the settings window

## Goal

The settings window's Accounts pane lists every account with its status
(its kind, read-only, or that it needs signing in again) and has buttons to
add and remove accounts. Build the presentational `AccountList`.

## Read first

- `app/src/features/settings/AccountList.tsx`: the props and the stub.
- `docs/specs/settings-ui.md`: "Shared pieces" and "AccountList".
- `app/src/features/settings/labels.ts`: `KIND_LABEL`.
- `app/src/rpc/gen/api.ts`: `Account`.
- The given test `app/src/features/settings/AccountList.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names and props; replace the file's "Task T-0055 …"
sentence with what the component does. Build exactly the elements and
classes of the spec's "AccountList":

- the `nav` with `aria-label="Accounts"`, the `ul`, one `li` and `button`
  per account with its two `span`s (name or email; status);
- the "New Account" row while `selected` is `"new"`;
- the bar with the Add Account (`Plus`) and Remove Account (`Minus`) icon
  buttons from `lucide-react`, size 14, and their disabled rules.

## Tests (given, do not edit)

`app/src/features/settings/AccountList.test.tsx`.

## Gotchas

- The status is one text node: build it as a single template string, such
  as `` `${KIND_LABEL[a.kind]} · Read-only` ``.
- `aria-current` is the string `"true"` when selected and absent otherwise
  (pass `undefined`, not `false`).
- Append ` bg-selection-sidebar` to the row's classes only when selected,
  so the unselected rows do not contain it.

## Out of scope

The settings window, selection state, confirmation before removing, and
every file except `app/src/features/settings/AccountList.tsx`.

## Done when

`make accept T=0055`, `make check` and `make ui-check` pass, and only
`app/src/features/settings/AccountList.tsx` changed.
