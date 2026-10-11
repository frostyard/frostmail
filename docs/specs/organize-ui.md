# Spec: the condition editor, the smart mailbox sheet and the rule sheet (M5)

The look and behavior of the editors built on maild's conditions
([organize.md](../design/organize.md#conditions)): the condition editor,
which the smart mailbox sheet and the rule sheet share, the smart mailbox
sheet, the action editor and the rule sheet. Tokens, type and button styles are those of the
reader UI ([ui.md](ui.md)); fields use the settings window's classes
(`app/src/features/settings/labels.ts`).

## The condition editor

`app/src/lib/conditions.ts` holds what the editor knows about fields and
operators; `app/src/features/organize/ConditionEditor.tsx` draws it. The
editor is controlled: it shows a `Conditions` and reports every change as
a whole new `Conditions`.

```
 Contains messages that match [all ▾] of the following conditions:
 [From          ▾] [contains        ▾] [ann                 ]  ⊖ ⊕
 [Date Received ▾] [is in the last  ▾] [7] [Days ▾]             ⊖ ⊕
 [Unread        ▾] [is              ▾] [Yes ▾]                  ⊖ ⊕
```

### Fields, in menu order

| Field | Label | Kind |
| --- | --- | --- |
| `from` | From | text |
| `to` | To | text |
| `cc` | Cc | text |
| `recipient` | Any Recipient | words |
| `tome` | Sent to Me | yes or no |
| `ccme` | Cc'd to Me | yes or no |
| `subject` | Subject | text |
| `content` | Entire Message | words |
| `filename` | Attachment Name | words |
| `listid` | Mailing List | text |
| `account` | Account | choice |
| `mailbox` | Mailbox | choice |
| `role` | Mailbox Type | choice |
| `received` | Date Received | date |
| `sent` | Date Sent | date |
| `unread` | Unread | yes or no |
| `flagged` | Flagged | yes or no |
| `attachments` | Has Attachments | yes or no |
| `color` | Flag Color | choice |
| `vip` | Sender Is a VIP | yes or no |
| `contact` | Sender Is in Contacts | yes or no |
| `reminder` | Has a Reminder | yes or no |

### Operators by field, with their labels

- `from`, `subject`: contains, does not contain (`notcontains`), is, begins
  with, ends with.
- `to`, `cc`: contains, is, begins with, ends with.
- `recipient`, `content`, `filename`: contains, does not contain.
- `listid`: contains, is.
- `account`, `mailbox`, `role`, `color`: is, is not (`isnot`), is any of
  (`anyof`).
- `received`, `sent`: is today, is yesterday, is this week, is this month,
  is this year, is in the last (`within`), is not in the last
  (`notwithin`), is (`on`), is on or after (`since`), is before.
- Yes-or-no fields: is.

### Values

- **Text and words:** a text input, empty to start.
- **Choice, `is` and `isnot`:** a `select`; **`anyof`:** a `select multiple`
  (4 rows high) whose selection, in option order, joins with commas.
  - `account`: each account, labeled by its email, valued by its ID.
  - `mailbox`: each account's mailboxes in an `optgroup` labeled with the
    account's email, each labeled by its path, valued by its ID.
  - `role`: Inbox, Drafts, Sent, Junk, Trash, Archive, All Mail (`inbox` …
    `all`).
  - `color`: the seven colors, labeled with the flags' names
    (`flagLabel`), valued 1–7.
- **`within`, `notwithin`:** a number input (1 to 999) and a `select` of
  Days, Weeks, Months, Years (`d`, `w`, `m`, `y`); the value is the number
  and the letter, `7d` to start.
- **`on`, `since`, `before`:** a date input; the value is `YYYY-MM-DD`,
  today to start.
- **`today` … `thisyear`:** nothing; the value is empty.
- **Yes or no:** a `select`, Yes (`true`) or No (`false`), Yes to start.

### Behavior

- Above the rows, the lead text, a `select` named "Match", all or any, and
  the trailing text. The lead and trailing text are props, `lead` and
  `trail`, by default "Contains messages that match" and "of the following
  conditions:"; the rule sheet passes "If" and "of the following
  conditions are met:".
- Each row: the field `select` (named "Condition N field"), the operator
  `select` ("Condition N operator", hidden for yes-or-no fields, whose
  operator is always `is`), the value controls (named "Condition N value",
  and "Condition N unit" for a duration's unit), then two 22px icon
  buttons, Remove (`circle-minus`, "Remove condition N") and Add
  (`circle-plus`, "Add condition after N"). N counts from 1.
- Choosing a field gives the row that field's first operator and its
  starting value. Choosing an operator keeps the value when it is the same
  kind (text to text, a duration to a duration, a day to a day, and
  choices to choices, a list going to a single choice keeping its first
  item), else gives the operator's starting value.
- Add inserts, after its row, a new `from contains ""` row. Remove deletes
  its row; it is disabled when there is one row.
- A value that does not fit its control (a condition made elsewhere) shows
  as it can and changes only when edited.
- The editor checks nothing: maild refuses what it cannot compile, and the
  sheet shows why.

## The smart mailbox sheet

`app/src/features/organize/SmartSheet.tsx`: a modal sheet over the main
window, to make or edit a smart mailbox.

```
┌ New Smart Mailbox ───────────────────────────────────────┐
│ Smart Mailbox Name: [Smart Mailbox                     ] │
│ Contains messages that match [all ▾] of the following…   │
│ [From ▾] [contains ▾] [                    ]  ⊖ ⊕        │
│ ☐ Include messages from Trash                           │
│ ☐ Include messages from Sent                            │
│ maild's error, if any                                   │
│                                      [Cancel] [  OK  ]  │
└──────────────────────────────────────────────────────────┘
```

- A `div` with `role="dialog"`, `aria-modal="true"`, named by its title
  ("New Smart Mailbox" or "Edit Smart Mailbox", an `h2`), 560 wide,
  centered over a backdrop of `--bg-window` at 60%, `--bg-window` with a
  1px `--separator` border, rounded 10, 20px padding.
- "Smart Mailbox Name:", a text input (at most 100 characters), then the
  condition editor, then the two checkboxes, then an `ALERT` with maild's
  error, then Cancel (`BUTTON`) and OK (`PRIMARY_BUTTON`). OK is disabled
  while the name is blank or a save is under way.
- A new smart mailbox starts named "Smart Mailbox" with one `from contains
  ""` condition and both boxes clear; one saved from a search starts with
  the search's text as its name, `smart.fromSearch`'s conditions, and both
  boxes checked.
- OK calls `smart.create` or `smart.update`; when it succeeds, the sheet
  closes and a new smart mailbox becomes the list's source. Cancel and
  Escape close it unchanged.

## The action editor

`app/src/lib/rules.ts` holds what the editor knows about actions;
`app/src/features/organize/ActionEditor.tsx` draws it. Like the condition
editor it is controlled: it shows a `RuleAction[]` and reports every change
as a whole new array.

```
 Perform the following actions:
 [Move Message      ▾] to mailbox: [Receipts           ▾]  ⊖ ⊕
 [Mark as Read      ▾]                                      ⊖ ⊕
 [Mark as Flagged   ▾] [Orange ▾]                           ⊖ ⊕
```

### Actions, in menu order

| Kind | Label | Takes |
| --- | --- | --- |
| `move` | Move Message | a mailbox |
| `copy` | Copy Message | a mailbox |
| `read` | Mark as Read | |
| `flag` | Mark as Flagged | a color |
| `delete` | Delete Message | |
| `notify` | Send Notification | |
| `stop` | Stop Evaluating Rules | |

### Behavior

- "Perform the following actions:" above the rows.
- Each row: the kind `select` (named "Action N"), then what the kind takes,
  then Remove (`circle-minus`, "Remove action N") and Add (`circle-plus`,
  "Add action after N"), 22px icon buttons as in the condition editor.
  N counts from 1.
  - **A mailbox:** "to mailbox:" and a `select` ("Action N mailbox") of
    each account's mailboxes in an `optgroup` labeled with the account's
    email, each labeled by its path, valued by its ID. Without a
    `mailboxId`, it shows a first, disabled option "No Mailbox Selected"
    (value `""`). With a `mailboxId` not among the mailboxes (deleted
    since), it shows a first, disabled option "Missing Mailbox" valued
    with that ID.
  - **A color:** a `select` ("Action N color") of the seven colors labeled
    with the flags' names (`flagLabel`), valued 1–7.
- Choosing a kind: Move and Copy keep the row's mailbox when it was a Move
  or Copy, else have none; Mark as Flagged starts red (1); the rest take
  nothing.
- Add inserts, after its row, a Move Message with no mailbox. Remove
  deletes its row; it is disabled when there is one row.
- `actionsComplete(actions, mailboxes)` in `lib/rules.ts` is true when
  there is at least one action and every Move and Copy names a mailbox
  among `mailboxes`; the rule sheet's OK waits for it.

## The rule sheet

`app/src/features/organize/RuleSheet.tsx`: a modal sheet over the settings
window, to make or edit a rule.

```
┌ Edit Rule ─────────────────────────────────────────────────────┐
│ Description: [Receipts                                       ] │
│ If [any ▾] of the following conditions are met:                │
│ [Subject ▾] [contains ▾] [receipt                ]  ⊖ ⊕        │
│ Perform the following actions:                                 │
│ [Move Message ▾] to mailbox: [Receipts ▾]           ⊖ ⊕        │
│ [Mark as Read ▾]                                    ⊖ ⊕        │
│ maild's error, if any                                          │
│                                            [Cancel] [  OK  ]   │
└────────────────────────────────────────────────────────────────┘
```

- The smart mailbox sheet's frame (a `role="dialog"` `div`,
  `aria-modal`, named by its `h2` title "New Rule" or "Edit Rule", the
  backdrop, focus on the first input when it opens and back where it was
  when it closes, Tab kept inside, Escape cancels), 640 wide.
- "Description:", a text input (at most 100 characters); the condition
  editor with "If" and "of the following conditions are met:"; the action
  editor; an `ALERT` with maild's error; Cancel (`BUTTON`) and OK
  (`PRIMARY_BUTTON`). OK is disabled while the description is blank, the
  actions are not complete, or a save is under way, and saves the
  description trimmed.
- `RuleDraft` is `{name, conditions, actions}`. `newRuleDraft(n)` starts a
  new rule named "Rule n", matching any of one `from contains ""`
  condition, with one Move Message and no mailbox, as Mail.app does.

## References

- Rationale: [ADR-0023](../adr/0023-one-condition-language-for-smart-mailboxes-and-rules.md),
  [ADR-0024](../adr/0024-rules-run-in-maild-on-new-mail.md)
- Context: [design/organize.md](../design/organize.md),
  [specs/ui.md](ui.md) (the sidebar's Smart Mailboxes, the search Save),
  [specs/settings-ui.md](settings-ui.md) (the Rules pane)
