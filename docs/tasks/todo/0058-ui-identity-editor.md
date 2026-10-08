---
id: "0058"
title: Edit identities and signatures
milestone: M4
size: S
touch:
  - app/src/features/settings/IdentityEditor.tsx
given:
  - app/src/features/settings/IdentityEditor.test.tsx
acceptance: make ui-vitest F=src/features/settings/IdentityEditor.test.tsx
---
# T-0058: Edit identities and signatures

## Goal

The settings window's Signatures pane lists every account's identities and
edits the selected one's name, Reply-To and signature (as plain text).
Build the presentational `IdentityEditor`.

## Read first

- `app/src/features/settings/IdentityEditor.tsx`: the types, props and
  stub.
- `docs/specs/settings-ui.md`: "Shared pieces", "IdentityEditor" and
  "Signature text".
- `app/src/lib/signature.ts`: `signatureText` and `signatureHtml`.
- `app/src/features/settings/labels.ts`: `GRID`, `LABEL`, `FIELD`,
  `PRIMARY_BUTTON`, `ALERT`.
- `app/src/rpc/gen/api.ts`: `Account`, `Identity`.
- The given test `app/src/features/settings/IdentityEditor.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names and props; replace the file's "Task T-0058 …"
sentence with what the component does. Build the spec's "IdentityEditor":

- With no identities, only the `p` "Add an account to edit its
  signature.".
- Otherwise a `div` (`flex h-full`) with the `ul` (`aria-label=
  "Identities"`): per account that has identities, its header `li` and one
  `li` + `button` per identity; then the form for the selected identity.
- Selection is the component's own state (`useState`), starting at the
  first identity in list order (accounts in the given order, then the
  identities of each in the given order).
- The form's fields are its own state too. They take the identity's stored
  values (the signature through `signatureText`) when it is selected and
  again whenever its stored `name`, `replyTo` or `signatureHtml` change.
- Save is disabled while `busy` or while every field equals the stored
  value; it calls `onSave(id, { name, replyTo, signatureHtml:
  signatureHtml(text) })`.

## Tests (given, do not edit)

`app/src/features/settings/IdentityEditor.test.tsx`.

## Gotchas

- Put the form in its own function component that receives the selected
  identity, and give it `key={identity.id}` or a `useEffect` on the stored
  values, so switching identities resets the fields.
- The "Email:" label is a `span` with `LABEL`; the email itself is a
  `span`, not an input.
- The signature `label` adds ` self-start pt-1` to `LABEL`.
- A list cannot hold bare fragments with keys in some setups: return an
  array of `li` elements, each with a `key`, or use `Fragment` with a
  `key`.

## Out of scope

Requests to maild, creating or deleting identities, rich-text signatures,
and every file except `app/src/features/settings/IdentityEditor.tsx`.

## Done when

`make accept T=0058`, `make check` and `make ui-check` pass, and only
`app/src/features/settings/IdentityEditor.tsx` changed.
