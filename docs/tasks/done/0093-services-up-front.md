---
id: "0093"
title: Choose contacts, calendars and tasks when adding an account
milestone: M4.5
size: M
touch:
  - app/src/features/settings/AccountForm.tsx
  - app/src/features/settings/ServicesSection.tsx
  - app/src/app/SettingsWindow.tsx
  - app/src/rpc/mock/mock.ts
  - app/src/rpc/mock/people.ts
given:
  - app/src/features/settings/AccountForm.test.tsx
  - app/src/features/settings/AccountForm.services.test.tsx
  - app/src/features/settings/ServicesSection.test.tsx
  - app/src/features/settings/ServicesSection.signin.test.tsx
  - app/src/app/SettingsWindow.add.test.tsx
acceptance: make ui-vitest F="src/features/settings src/app/SettingsWindow"
---
# T-0093: Choose contacts, calendars and tasks when adding an account

## Goal

Adding a Google account and then turning on contacts, calendars and tasks
one at a time sends the user to Google four times: each new service needs
a wider grant. Ask which services to turn on in the add form, turn them
on before the one sign-in, and let maild ask Google for mail and all of
them at once. An existing account whose enabled services wait for a
sign-in gets one prompt and one Sign In… button for all of them.

## Read first

- `docs/specs/settings-ui.md`: "AccountForm" (Changing the kind, Services
  by kind, the Google row, Add mode after the options), "ServicesSection"
  (the waiting status, One sign-in for all) and "Behavior (container)"
  (Add).
- `app/src/features/settings/AccountForm.tsx`: the stubs
  `offeredServices` and `defaultServices`, and `AccountFormValue.services`.
- `app/src/features/settings/ServicesSection.tsx` and `labels.ts`.
- `app/src/app/SettingsWindow.tsx`: `emptyAccount`, `discover`, `submit`,
  `signIn`, `noClient`, `useRequest`.
- `app/src/rpc/mock/mock.ts` (`dispatch`, `createAccount`) and
  `app/src/rpc/mock/people.ts` (`services`, `setService`): the mock maild.
- `app/src/rpc/gen/api.ts`: `Discovery`, `AuthorizeResult`,
  `ServiceKind`, `ServiceSettings`.
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

- **`offeredServices(kind)`**: `imap` and `gmail` → contacts, calendar,
  tasks; `icloud` → contacts, calendar; `microsoft` → none.
  **`defaultServices(kind)`**: all offered for `gmail` and `icloud`, none
  for `imap`. Both return new arrays in the order contacts, calendar,
  tasks.
- **`AccountForm`**, add mode only: changing Account Type also sets
  `services` to `defaultServices` of the new kind (auth still drops to
  `password` away from Gmail); the Google row's new text; after the
  options, the services `section` exactly as the spec says. Changing Sign
  In does not change `services`. Edit mode is unchanged.
- **`ServicesSection`**: an enabled service without `signedIn` shows
  `Waiting for Google sign-in` instead of its own sign-in prompt and
  button; after the rows, one prompt naming every such service and one
  `Sign In…` button.
- **`SettingsWindow`**: `emptyAccount()` has `services: []`; Find
  Settings sets `services` to `defaultServices(d.kind)`; Add Account turns
  the form's `services` on in order (password: after `setPassword`;
  oauth2: before the one `authorize`), never stopping at a failed one, and
  shows "Could not turn on <Name>: <message>" for the first failure after
  the rest of the flow. The names are Contacts, Calendars, Tasks.
- **Mock maild:**
  - `account.authorize({ id })`: not found for an unknown account,
    `invalidParams` for a password account; otherwise marks the account
    and each of its services `signedIn`, emits `account.changed` for it,
    and returns `{ url: "https://accounts.google.test/…" }` (any https
    URL).
  - `account.discover({ email })`: `gmail.com` and `googlemail.com` →
    `{ kind: "gmail", source: "profile", auth: ["oauth2", "password"] }`;
    `icloud.com` and `me.com` → `{ kind: "icloud", source: "profile",
    auth: ["password"] }`; anything else → `{ kind: "imap", source:
    "none", auth: ["password"] }`. Match the domain case-insensitively.

## Tests (given, do not edit)

`app/src/features/settings/AccountForm.test.tsx` (updated for the kind
change and the Google text), `AccountForm.services.test.tsx`,
`ServicesSection.test.tsx` (updated for the waiting status),
`ServicesSection.signin.test.tsx`, and
`app/src/app/SettingsWindow.add.test.tsx`. The existing
`SettingsWindow.test.tsx` and `SettingsWindow.services.test.tsx` must keep
passing.

## Gotchas

- The add test stubs `window.open`; `openInBrowser` already uses it
  outside Tauri. Do not call it more than once per add.
- `useRequest.run` shows a thrown error's message; throw the "Could not
  turn on …" error only after the sign-in or password step, so a service
  failure never skips them.
- The services section's checkboxes and hint are disabled while `busy`,
  like the server fields.
- The mock's `services()` creates an account's services on first use with
  `signedIn: !google`; `authorize` has to change that stored list, not a
  copy.
- Use the classes in `labels.ts` where one fits.

## Out of scope

maild (it already records a Google service without its scope as enabled
and waiting, and asks for every enabled service's scope at sign-in),
`mailctl`, edit mode's per-service checkboxes, and every file not under
`touch`.

## Done when

`make accept T=0093` and `make ui-check` pass (taskrun runs them), and only
the files under `touch` changed.
