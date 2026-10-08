---
id: "0056"
title: Edit one server's settings
milestone: M4
size: S
touch:
  - app/src/features/settings/ServerFields.tsx
given:
  - app/src/features/settings/ServerFields.test.tsx
acceptance: make ui-vitest F=src/features/settings/ServerFields.test.tsx
---
# T-0056: Edit one server's settings

## Goal

The account form shows the incoming (IMAP) and outgoing (SMTP) server each
as a group of four labeled fields: server, port, security and user name.
Changing the security mode moves a default port along with it. Build the
presentational `ServerFields` and `defaultPort`.

## Read first

- `app/src/features/settings/ServerFields.tsx`: the props and the stubs.
- `docs/specs/settings-ui.md`: "Shared pieces" and "ServerFields".
- `app/src/features/settings/labels.ts`: `GRID`, `LABEL`, `FIELD`,
  `TLS_MODES`, `TLS_LABEL`.
- `app/src/rpc/gen/api.ts`: `ServerConfig`, `TLSMode`.
- The given test `app/src/features/settings/ServerFields.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported names and props; replace the file's "Task T-0056 …"
sentence with what the file does.

- `defaultPort(id, tls)`: the table in the spec's "Default ports".
- `ServerFields`: a `fieldset` with class `GRID`, `disabled` when
  `disabled`, its `legend`, then the four `label` + control pairs of the
  spec's table, in order, with the IDs `<id>-host`, `<id>-port`,
  `<id>-tls`, `<id>-username`. Labels use `LABEL` and `htmlFor`; controls
  use `FIELD` (the port adds ` w-24`).
- Every change calls `onChange` with `{ ...value, <field>: … }`; never
  mutate `value`.
- Security: the new port is the new mode's default when the current port
  is 0 or the current mode's default; otherwise the port stays.

## Tests (given, do not edit)

`app/src/features/settings/ServerFields.test.tsx`.

## Gotchas

- Read the select's new value without a type cast: find it in
  `TLS_MODES` (`TLS_MODES.find((t) => t === e.target.value)`) and ignore
  a value that is not there.
- The port input's value is a string: `""` for port 0, else
  `String(port)`.
- `Number.parseInt("", 10)` is `NaN`; `NaN || 0` is 0.

## Out of scope

The account form, validation messages, and every file except
`app/src/features/settings/ServerFields.tsx`.

## Done when

`make accept T=0056`, `make check` and `make ui-check` pass, and only
`app/src/features/settings/ServerFields.tsx` changed.
