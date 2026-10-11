# Spec: the condition editor and the smart mailbox sheet (M5)

The look and behavior of the editors built on maild's conditions
([organize.md](../design/organize.md#conditions)): the condition editor,
which the smart mailbox sheet and (in Phase 4) the rule editor share, and
the smart mailbox sheet. Tokens, type and button styles are those of the
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

- Above the rows, "Contains messages that match" a `select` named "Match",
  all or any, "of the following conditions:".
- Each row: the field `select` (named "Condition N field"), the operator
  `select` ("Condition N operator", hidden for yes-or-no fields, whose
  operator is always `is`), the value controls (named "Condition N value",
  and "Condition N unit" for a duration's unit), then two 22px icon
  buttons, Remove (`circle-minus`, "Remove condition N") and Add
  (`circle-plus`, "Add condition after N"). N counts from 1.
- Choosing a field gives the row that field's first operator and its
  starting value. Choosing an operator keeps the value when it is the same
  kind (text to text, one choice to one choice), else gives the operator's
  starting value.
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

## References

- Rationale: [ADR-0023](../adr/0023-one-condition-language-for-smart-mailboxes-and-rules.md)
- Context: [design/organize.md](../design/organize.md),
  [specs/ui.md](ui.md) (the sidebar's Smart Mailboxes, the search Save)
