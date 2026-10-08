---
id: "0047"
title: Show the undo toast and the outbox
milestone: M3
size: S
touch:
  - app/src/features/outbox/Outbox.tsx
given:
  - app/src/features/outbox/Outbox.test.tsx
acceptance: make ui-vitest F=src/features/outbox/Outbox.test.tsx
---
# T-0047: Show the undo toast and the outbox

## Goal

After Send, the main window shows a toast with Undo while the message
waits out its undo delay, and an Outbox section lists messages that are not
yet sent, with what went wrong and what can be done
(`docs/specs/compose-ui.md`). Build both presentational components.

## Read first

- `app/src/features/outbox/Outbox.tsx`: the props and the stubs.
- `docs/specs/compose-ui.md`: "Undo toast" and "Outbox" under Layout,
  "Saving and sending" under Behavior.
- `app/src/lib/format.ts`: `formatAddressList`.
- `app/src/rpc/gen/api.ts`: `OutboxItem`, `OutboxState`.
- The given test `app/src/features/outbox/Outbox.test.tsx`.
- `docs/tasks/EXECUTOR.md`, especially the TypeScript conventions.

## Contract

Keep the exported types and names; replace the stubs' doc comments and the
file's "Task T-0047 …" sentence with what each component does.

The subject shown is the subject, or "(no subject)" when its trimmed value
is empty (both components).

**`UndoToast`**

- An `output` element (its role is `status`) with classes `flex h-9
  items-center gap-3 rounded-lg border border-separator bg-toolbar px-3
  text-[13px] leading-[18px] shadow-md`, holding:
  1. a `span` (`min-w-0 max-w-[320px] truncate`) with the text
     `Sending “<subject>”…` (curly quotes, an ellipsis character);
  2. while `secondsLeft > 0`: a `button type="button"` "Undo"
     (`font-semibold text-accent`) calling `onUndo`, then a `span`
     (`text-[12px] leading-4 tabular-nums text-secondary`) with
     `<secondsLeft>s`.

**`OutboxStatus`**

- Shows the items whose state is not `sent`, in the given order; with none,
  it renders `null`.
- A `section` with `aria-label="Outbox"`: an `h2` "Outbox" (`px-4 pt-3 pb-1
  text-sidebar-section text-secondary`), then a `ul` with one `li` per item
  (`flex flex-col px-4 py-2`) holding, each in its own `span`:
  1. the subject (`truncate text-[13px] leading-[18px] font-semibold`);
  2. `To: ` + `formatAddressList(item.to, 2)` (`truncate text-[12px]
     leading-4 text-secondary`);
  3. the state line (`text-[12px] leading-4`, plus `text-flag-1` for
     `failed`, else `text-secondary`):
     - `queued` with `attempts` 0: `Waiting to send`;
     - `queued` after attempts: `Retrying in N min`, plus `: <error>` when
       `error` is not empty, where N is the minutes from `now` to `sendAt`
       rounded up, at least 1 (`sendAt` missing counts as now);
     - `sending`: `Sending…`; `accepted`: `Saving to Sent…`;
     - `failed`: `Not sent: <error>`, or `Not sent` when there is no error;
  4. for `queued` and `failed` items, a `span` (`flex gap-3`) of buttons
     (`button type="button"`, `text-[12px] leading-4 text-accent`): `Retry`
     (failed only) calling `onRetry(item.id)`, then `Edit` calling
     `onEdit(item)`.

## Tests (given, do not edit)

`app/src/features/outbox/Outbox.test.tsx`.

## Gotchas

- Use the `output` element rather than `role="status"` on a `div`, and a
  `section` with `aria-label` rather than `role="region"`: Biome prefers
  the elements.
- Parse `sendAt` with `Date.parse`; compare in milliseconds.
- Write the toast text as one template string so it is one text node.

## Out of scope

Placing and stacking the toasts, timers, and the sidebar container; every
file except `app/src/features/outbox/Outbox.tsx`.

## Done when

`make accept T=0047`, `make check` and `make ui-check` pass, and only
`app/src/features/outbox/Outbox.tsx` changed.
