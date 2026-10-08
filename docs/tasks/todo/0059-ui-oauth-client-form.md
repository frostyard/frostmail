---
id: "0059"
title: Enter the Google OAuth client
milestone: M4
size: S
touch:
  - app/src/features/settings/OAuthClientForm.tsx
given:
  - app/src/features/settings/OAuthClientForm.test.tsx
acceptance: make ui-vitest F=src/features/settings/OAuthClientForm.test.tsx
---
# T-0059: Enter the Google OAuth client

## Goal

Gmail sign-in uses the user's own Google OAuth client. The settings
window's Sign-In pane shows whether one is stored and lets the user enter
its ID and secret. Build the presentational `OAuthClientForm`.

## Read first

- `app/src/features/settings/OAuthClientForm.tsx`: the props and stub.
- `docs/specs/settings-ui.md`: "Shared pieces" and "OAuthClientForm".
- `app/src/features/settings/labels.ts`: `GRID`, `LABEL`, `FIELD`,
  `PRIMARY_BUTTON`, `ALERT`.
- `app/src/rpc/gen/api.ts`: `OAuthClient`.
- The given test `app/src/features/settings/OAuthClientForm.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names and props; replace the file's "Task T-0059 …"
sentence with what the component does. Build the spec's
"OAuthClientForm":

- a `form` with `aria-label="Google client"` holding, in order, the `h2`,
  the explanation `p`, a `div` with `GRID` and the two labeled inputs
  (`oauth-client-id`, `oauth-client-secret`), the status `p`, the alert
  when `error` is set, and the Save button (`PRIMARY_BUTTON` plus ` mt-5
  self-end`);
- the client ID field is state that starts as the stored ID (or "") and
  follows the stored ID when it changes; the secret field is state that
  starts empty and is emptied after a save;
- Save is disabled while `busy`, while the trimmed ID is empty, or while
  the trimmed ID equals the stored ID and the secret is empty; submitting
  prevents the default and calls `onSave(trimmed ID, secret)`.

## Tests (given, do not edit)

`app/src/features/settings/OAuthClientForm.test.tsx`.

## Gotchas

- `placeholder` is "Stored" only when the stored client has a secret;
  otherwise leave it unset (`undefined`).
- Follow a newly stored ID with `useEffect(() => setClientId(stored),
  [stored])`.
- The explanation text is one paragraph, word for word from the spec.

## Out of scope

Requests to maild, other providers, and every file except
`app/src/features/settings/OAuthClientForm.tsx`.

## Done when

`make accept T=0059`, `make check` and `make ui-check` pass, and only
`app/src/features/settings/OAuthClientForm.tsx` changed.
