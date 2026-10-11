---
id: "0111"
title: The rule sheet and its action editor
milestone: M5
size: M
touch:
  - app/src/lib/rules.ts
  - app/src/features/organize/ActionEditor.tsx
  - app/src/features/organize/RuleSheet.tsx
  - app/src/features/organize/ConditionEditor.tsx
given:
  - app/src/lib/rules.test.ts
  - app/src/features/organize/ActionEditor.test.tsx
  - app/src/features/organize/RuleSheet.test.tsx
acceptance: make ui-vitest F="src/lib/rules src/features/organize"
---
# T-0111: The rule sheet and its action editor

## Goal

The sheet that makes and edits a rule, as Mail.app's: a description, the
condition editor reading "If any of the following conditions are met:", and
an editor of the rule's actions in order. The Rules pane (T-0112) opens it.

## Read first

- `docs/specs/organize-ui.md`: "The condition editor" (its Behavior, for
  `lead` and `trail`), "The action editor" and "The rule sheet".
- `app/src/features/organize/SmartSheet.tsx`: the sheet frame to follow
  (dialog, focus, Tab, Escape, OK and Cancel).
- `app/src/features/organize/ConditionEditor.tsx`: the rows, the 22px icon
  buttons, and the mailbox `optgroup`s to follow.
- `app/src/lib/conditions.ts` and `app/src/lib/flags.ts` (`flagLabel`).
- `app/src/rpc/gen/api.ts`: `RuleAction`, `RuleActionKind`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`app/src/lib/rules.ts`** exports:
  - `ACTIONS: { kind: RuleActionKind; label: string }[]`, the seven kinds
    in the spec's order with its labels;
  - `takesMailbox(kind)`: true for `move` and `copy`;
  - `newAction()`: `{ kind: "move" }`;
  - `withKind(action, kind)`: `move`/`copy` keep `mailboxId` when the old
    action was `move` or `copy` and had one, else none; `flag` keeps the
    old color when the old action was a flag, else 1; every other kind is
    `{ kind }` alone;
  - `actionsComplete(actions, mailboxes)`: at least one action, and every
    `move` and `copy` has a `mailboxId` that is the ID of one of
    `mailboxes`.
- **`ActionEditor`** (`ActionEditor.tsx`), props `value: RuleAction[]`,
  `onChange(value)`, `accounts`, `mailboxes`, `flagNames?`: the spec's
  rows and names ("Action N", "Action N mailbox", "Action N color",
  "Remove action N", "Add action after N"), reporting every change as a
  whole new array. Objects it reports have no keys set to `undefined`.
- **`ConditionEditor`**: optional props `lead` and `trail`, defaulting to
  "Contains messages that match" and "of the following conditions:".
- **`RuleSheet.tsx`** exports `RuleDraft` (`{name, conditions, actions}`),
  `newRuleDraft(n)`, `RuleSheetProps` (`title`, `initial`, `accounts`,
  `mailboxes`, `flagNames?`, `today`, `busy?`, `error?`, `onSave`,
  `onCancel`) and `RuleSheet`, the spec's sheet: 640 wide, "Description:"
  labeling its input, OK disabled unless the trimmed description is not
  empty and `actionsComplete(draft.actions, mailboxes)`, and not busy.

## Tests (given, do not edit)

`rules.test.ts`, `ActionEditor.test.tsx` and `RuleSheet.test.tsx`. Every
other app test must keep passing, `ConditionEditor.test.tsx` and
`SmartSheet.test.tsx` included.

## Gotchas

- The tests compare reported actions with `toEqual`/`toHaveBeenCalledWith`:
  `{ kind: "read" }`, not `{ kind: "read", mailboxId: undefined }`.
- A `select` whose value is not among its options shows the first option;
  the "No Mailbox Selected" and "Missing Mailbox" options exist so the
  select shows the truth.
- Do not refactor `SmartSheet.tsx` to share the frame; it is not in
  `touch`.

## Out of scope

The Rules pane, maild, the mock, and every file not under `touch`.

## Done when

`make accept T=0111` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
