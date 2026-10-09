---
id: "0066"
title: Turn an account's contacts, calendars and tasks on in Settings
milestone: M4.5
size: M
touch:
  - app/src/features/settings/ServicesSection.tsx
  - app/src/features/settings/AccountForm.tsx
  - app/src/app/SettingsWindow.tsx
given:
  - app/src/features/settings/ServicesSection.test.tsx
  - app/src/app/SettingsWindow.services.test.tsx
acceptance: make ui-vitest F="src/features/settings src/app/SettingsWindow"
---
# T-0066: Turn an account's contacts, calendars and tasks on in Settings

## Goal

Contacts, calendars and tasks sync once the user turns them on per account
(ADR-0017). Add the services section to an account's settings: a checkbox
per service the provider offers, its sync state, a prompt to sign in to
Google again when the grant lacks the service, and a server field when
maild could not find one.

## Read first

- `docs/specs/settings-ui.md`: "Shared pieces", "AccountForm",
  "ServicesSection" and "Behavior (container)".
- The stub `app/src/features/settings/ServicesSection.tsx` (props).
- `app/src/features/settings/AccountForm.tsx` and `labels.ts` (the shared
  classes); `app/src/app/SettingsWindow.tsx` (how the selected account
  loads, saves and signs in).
- `schema/rpc/account.yaml` / `app/src/rpc/gen/api.ts`: `ServiceKind`,
  `ServiceSettings`, `account.services`, `account.setService`.
- `app/src/rpc/mock/people.ts`: the mock's `account.services` and
  `account.setService` (an IMAP account without a URL, and
  `MOCK_UNREACHABLE`, fail discovery with `unavailable`).
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

- **`ServicesSection`** exactly as the spec's "ServicesSection" says; it
  keeps each server field's text in its own state.
- **`AccountForm`** gains an optional `services?: ReactNode`, rendered
  after the options in edit mode and ignored in add mode.
- **`SettingsWindow`** (edit mode only): fetches `account.services` for the
  selected account when it is selected and after each `account.changed`
  with its ID; passes a `ServicesSection` as `services`. Toggling calls
  `account.setService({ id, service, enabled })`, with `url` only when one
  was given, marking the service busy until it answers; success clears its
  error and refetches; failure keeps the message under the service. Sign
  In… does what the form's Google Sign In… does. A service's busy and error
  state belongs to the selected account and is dropped when another is
  selected.

## Tests (given, do not edit)

`app/src/features/settings/ServicesSection.test.tsx`,
`app/src/app/SettingsWindow.services.test.tsx`. The existing
`AccountForm.test.tsx` and `SettingsWindow.test.tsx` must keep passing.

## Gotchas

- Use the classes in `labels.ts` (`BUTTON`, `FIELD`, `ALERT`, …) rather
  than repeating them.
- `lastSyncAt` is an ISO time; compare days in local time.
- The services request for a just-created account may race the account
  list; fetch by the selected ID and ignore answers for an account no
  longer selected.

## Out of scope

Listing or hiding individual address books and calendars (the modules'
sidebars), maild, and every file not under `touch`.

## Done when

`make accept T=0066` and `make ui-check` pass (taskrun runs them), and only
the files under `touch` changed.
