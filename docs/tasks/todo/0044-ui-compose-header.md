---
id: "0044"
title: Build the compose header fields
milestone: M3
size: S
touch:
  - app/src/features/compose/ComposeHeader.tsx
given:
  - app/src/features/compose/ComposeHeader.test.tsx
acceptance: make ui-vitest F=src/features/compose/ComposeHeader.test.tsx
---
# T-0044: Build the compose header fields

## Goal

The compose window shows its header fields as Mail.app does: To, Cc, Bcc
when asked for, Subject, and From when the account has several identities
(`docs/specs/compose-ui.md`). Build the presentational stack of rows from
`RecipientField` (T-0043) and plain inputs.

## Read first

- `app/src/features/compose/ComposeHeader.tsx`: the props and the stub.
- `app/src/features/compose/RecipientField.tsx`: the recipient field and
  its props.
- `docs/specs/compose-ui.md`: "Header" under Layout.
- `app/src/rpc/gen/api.ts`: `DraftContent`, `Identity`.
- The given test `app/src/features/compose/ComposeHeader.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names and props; replace the stub's doc comment and the
"Task T-0044 …" sentence in the file comment with what the component does.

- A wrapper `div` holding rows in this order: To, Cc, Bcc (only when
  `showBcc`), Subject, From (only when `identities.length > 1`).
- Each row is a `div` with classes `flex min-h-[30px] items-center
  border-b border-separator px-4`, holding the label then the field. The
  label is a `span` with a `data-header-label` attribute,
  `aria-hidden="true"` (the fields have their own accessible names), the
  text "To:", "Cc:", "Bcc:", "Subject:" or "From:", and classes `w-16
  shrink-0 pr-2 text-right text-[13px] leading-[18px] text-secondary`.
- To, Cc, Bcc: a `RecipientField` with `label` "To", "Cc" or "Bcc", the
  matching list from `content`, `suggest` passed through, and `onChange`
  calling the header's `onChange` with only that field (`{ to }`, `{ cc }`
  or `{ bcc }`). The To field gets `autoFocus={autoFocusTo}`.
- Subject: an `input` with `type="text"`, `aria-label="Subject"`, `value`
  = `content.subject`, classes `min-w-0 flex-1 border-none bg-transparent
  text-[13px] leading-[18px] text-primary outline-none`; each change calls
  `onChange({ subject })`.
- From: a `select` with `aria-label="From"`, `value` =
  `String(content.identityId)`, the same text classes without `flex-1`,
  and one `option` per identity with `value` = its id as a string and the
  text "Name <email>", or the email when the name is empty; a change calls
  `onChange({ identityId: Number(value) })`.

## Tests (given, do not edit)

`app/src/features/compose/ComposeHeader.test.tsx`.

## Gotchas

- A small `Row` component (label and children) keeps the five rows
  alike.
- The header is controlled: render from `content`, never copy it into
  state.

## Out of scope

`RecipientField` itself, the compose window container, and every file
except `app/src/features/compose/ComposeHeader.tsx`.

## Done when

`make accept T=0044`, `make check` and `make ui-check` pass, and only
`app/src/features/compose/ComposeHeader.tsx` changed.
