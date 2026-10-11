---
id: "0107"
title: The condition editor
milestone: M5
size: L
touch:
  - app/src/lib/conditions.ts
  - app/src/features/organize/ConditionEditor.tsx
given:
  - app/src/lib/conditions.test.ts
  - app/src/features/organize/ConditionEditor.test.tsx
acceptance: make ui-vitest F="src/lib/conditions src/features/organize/ConditionEditor"
---
# T-0107: The condition editor

## Goal

The editor that smart mailboxes, and later rules, use to build maild's
conditions, as Mail.app's editors look: "Contains messages that match
all/any of the following conditions", then rows of field, operator and
value with remove and add buttons. It is a controlled component over
`Conditions`, with what it knows about fields and operators in a small
library.

## Read first

- `docs/specs/organize-ui.md`: "The condition editor" in full.
- `docs/design/organize.md`: "Fields and operators".
- `app/src/rpc/gen/api.ts`: `Condition`, `Conditions`, `ConditionField`,
  `ConditionOp`, `Account`, `Mailbox`.
- `app/src/lib/flags.ts`: `flagLabel`.
- `app/src/features/settings/labels.ts`: `FIELD`.
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

- **`lib/conditions.ts`** exports:
  - `interface FieldInfo { field: ConditionField; label: string }` and
    `FIELDS`, the spec's fields in menu order with their labels.
  - `OP_LABEL: Record<ConditionOp, string>`, the spec's labels (`on` is
    "is").
  - `opsFor(field): ConditionOp[]`, the spec's operators per field in
    order; yes-or-no fields are `["is"]`.
  - `type ValueKind = "text" | "yesno" | "choice" | "choices" |
    "duration" | "date" | "none"` and `valueKind(field, op)`: yes-or-no
    fields `yesno`; `account`, `mailbox`, `role`, `color` with `is` or
    `isnot` `choice`, with `anyof` `choices`; `within`, `notwithin`
    `duration`; `on`, `since`, `before` `date`; `today` … `thisyear`
    `none`; everything else `text`.
  - `interface ConditionContext { today: string; accounts: Account[];
    mailboxes: Mailbox[] }` and `startingValue(field, op, ctx)`: `yesno`
    `"true"`, `duration` `"7d"`, `date` `ctx.today`, a choice the first
    account's or mailbox's ID, `inbox`, or `1` for a color, else `""`.
  - `ROLES`, the Mailbox Type choices `[value, label]`: inbox Inbox,
    drafts Drafts, sent Sent, junk Junk, trash Trash, archive Archive,
    all All Mail.
  - `newCondition()`: `{ field: "from", op: "contains", value: "" }`.
  - `withField(c, field, ctx)`: the field's first operator and its
    starting value (`c` is unused; keep the parameter, named `_c`).
  - `withOp(c, op, ctx)`: the same value when the old and new operators'
    kinds are the same (`choice` and `choices` count as one kind; going to
    `choice`, a list keeps its first item), else the starting value.
- **`features/organize/ConditionEditor.tsx`**:
  `interface ConditionEditorProps { value: Conditions; onChange: (value:
  Conditions) => void; accounts: Account[]; mailboxes: Mailbox[];
  flagNames?: readonly string[]; today: string }` and
  `ConditionEditor(props)`, as the spec draws it:
  - the "Match" `select` (all, any) in its sentence;
  - per row, the field `select` "Condition N field" (options are
    `FIELDS`' labels), the operator `select` "Condition N operator"
    (absent for yes-or-no fields), the value controls by `valueKind`
    named "Condition N value" ("Condition N unit" for a duration's unit),
    and the buttons "Remove condition N" (disabled with one row) and "Add
    condition after N";
  - choices as the spec lists them: accounts by email; mailboxes in one
    `optgroup` per account (labeled with its email), each by path; roles
    by `ROLES`; colors by `flagLabel(n, flagNames)`;
  - `anyof` is a `select multiple` (size 4) whose selected values, in
    option order, join with commas;
  - every edit calls `onChange` with whole new `Conditions`, using
    `withField`, `withOp`, `newCondition`; the editor keeps no state of
    its own.

## Tests (given, do not edit)

`conditions.test.ts` and `ConditionEditor.test.tsx`. Every other app test
must keep passing.

## Gotchas

- A `select multiple` has the ARIA role `listbox`, not `combobox`.
- Rows have no identity but their place; an index key needs a Biome
  suppression with a reason, as `MessageRow` lists do elsewhere.
- Lucide's `CircleMinus` and `CirclePlus` are the icons.
- An unused parameter is a Biome warning; name it `_c`.

## Out of scope

The smart mailbox sheet (T-0108), the rule editor (Phase 4), maild, the
mock, and every file not under `touch`.

## Done when

`make accept T=0107` and `make ui-check` pass (taskrun runs them), and
only the files under `touch` changed.
