---
id: "0057"
title: Add and edit an account in the settings window
milestone: M4
size: M
touch:
  - app/src/features/settings/AccountForm.tsx
given:
  - app/src/features/settings/AccountForm.test.tsx
acceptance: make ui-vitest F=src/features/settings/AccountForm.test.tsx
---
# T-0057: Add and edit an account in the settings window

## Goal

The Accounts pane's form adds a new account (its address, Find Settings,
the sign-in, both servers and the options) and edits an existing one (its
name, servers, options, a new password, or signing in to Google again).
Build the presentational `AccountForm`; a container does the requests.

## Read first

- `app/src/features/settings/AccountForm.tsx`: the types, props and stub.
- `docs/specs/settings-ui.md`: "Shared pieces" and "AccountForm".
- `app/src/features/settings/ServerFields.tsx` (done in T-0056): use it
  for both servers.
- `app/src/features/settings/labels.ts`: `KIND_LABEL`, `GRID`, `LABEL`,
  `FIELD`, `BUTTON`, `PRIMARY_BUTTON`, `ALERT`.
- `app/src/rpc/gen/api.ts`: `AccountKind`, `AuthKind`, `DiscoverySource`,
  `ServerConfig`.
- The given test `app/src/features/settings/AccountForm.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names, types and props; replace the file's "Task T-0057
…" sentence with what the component does. Build the spec's "AccountForm":

- the `form` (submit prevents the default and calls `onSubmit`) and its
  `h2`;
- a `div` with class `GRID` holding the rows of the spec's table in order:
  Email Address, then (add mode) the Find Settings row, Full Name, Account
  Type, Sign In, then the Password row (auth `password`) or the Google row
  (auth `oauth2`);
- the two `ServerFields`, each in a `div` with `mt-5`, disabled while
  `busy`;
- the two option checkboxes, the alert, and the buttons.

Rules to keep exactly:

- The Account Type options are `imap`, `gmail`, `icloud` in that order,
  labeled with `KIND_LABEL`; changing it sets `auth` to `"password"`
  unless the new kind is `gmail`, where `auth` stays.
- The Sign In select lists `password` "Password", and `oauth2` "Google
  sign-in" only when the kind is `gmail`.
- Find Settings is disabled while `busy`, while `discovery.kind` is
  `"finding"`, or when the trimmed email does not match
  `/^[^@\s]+@[^@\s]+$/`.
- The status texts and hints are the spec's, character for character
  (`…` is one character).
- In edit mode: the email input is `readOnly`, both selects are
  `disabled`, there is no Find Settings row and no Cancel button, and the
  submit button says "Save".

## Tests (given, do not edit)

`app/src/features/settings/AccountForm.test.tsx`.

## Gotchas

- Split the form into small functions in the same file (for example the
  Find Settings row, the password row and the Google row) to keep each
  under 60 lines; React fragments (`<>…</>`) keep a row's label and control
  as two cells of the grid.
- Read select values without type casts: find the kind in a constant list
  of the three kinds, and map the auth select with
  `e.target.value === "oauth2" ? "oauth2" : "password"`.
- The Google row's label is a `span` with `LABEL`, since it labels no
  control.
- A checkbox's change reads `e.target.checked`.

## Out of scope

Requests to maild, discovery results filling the form, opening the
browser, the settings window, and every file except
`app/src/features/settings/AccountForm.tsx`.

## Done when

`make accept T=0057`, `make check` and `make ui-check` pass, and only
`app/src/features/settings/AccountForm.tsx` changed.
