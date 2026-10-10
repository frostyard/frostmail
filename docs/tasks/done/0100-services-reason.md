---
id: "0100"
title: Say why a service is out of reach in Settings
milestone: M4.5
size: S
touch:
  - app/src/features/settings/ServicesSection.tsx
  - app/src/rpc/mock/people.ts
given:
  - app/src/features/settings/ServicesSection.reason.test.tsx
  - app/src/app/SettingsWindow.reason.test.tsx
acceptance: make ui-vitest F="src/features/settings src/app/SettingsWindow"
---
# T-0100: Say why a service is out of reach in Settings

## Goal

An iCloud account's services show Contacts and Calendars and silently no
Tasks. Apple Reminders has been out of third-party reach since iOS 13
(probed on the user's account: iCloud's CalDAV lists are frozen copies).
maild now sends a service's `reason` when it is not available;
Settings shows it under the account's services.

## Read first

- `docs/specs/settings-ui.md`: "ServicesSection", the "Reasons" bullet.
- `app/src/features/settings/ServicesSection.tsx`.
- `app/src/rpc/gen/api.ts`: `ServiceSettings.reason`.
- `app/src/rpc/mock/people.ts`: the mock's `services()`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`ServicesSection`:** for each service with `available: false` and a
  `reason`, in service order, a `p` (`mt-2 text-[12px] leading-4
  text-secondary`) with the reason, inside the section after the rows and
  the sign-in prompt; with no available service, after the existing
  paragraph (which then sits in a fragment with them). No other change.
- **Mock maild:** an iCloud account's tasks service carries the reason
  "Apple Reminders can't be reached by apps outside Apple's (since iOS
  13)." (maild's text, from `internal/providers`).

## Tests (given, do not edit)

`ServicesSection.reason.test.tsx` and `SettingsWindow.reason.test.tsx`.
The other settings tests must keep passing.

## Out of scope

maild, the add form, and every file not under `touch`.

## Done when

`make accept T=0100` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
