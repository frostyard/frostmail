---
id: "0135"
title: The contact editor and its draft
milestone: M5.5
size: L
touch:
  - app/src/lib/contactDraft.ts
  - app/src/features/people/ContactEditor.tsx
given:
  - app/src/lib/contactDraft.test.ts
  - app/src/features/people/ContactEditor.test.tsx
acceptance: make ui-vitest F="src/lib/contactDraft src/features/people/ContactEditor"
---
# T-0135: The contact editor and its draft

## Goal

The editor that turns a contact's block in the person pane into fields
(docs/specs/pim-ui.md, "Editing a contact"). It is a presentational
component and a pure draft model: rows for every field, labels, added and
removed rows, validation, and the fields that changed, for `people.update`
or `people.create`. T-0136 puts it in the pane.

## Read first

- `docs/specs/pim-ui.md`: "Editing a contact (`ContactEditor`,
  `PhotoWell`)" (the photo well is T-0137's; here it is a slot).
- `schema/rpc/people.yaml`: `Contact`, `ContactFields`, `LabeledValue`,
  `PostalAddress`.
- `app/src/features/people/PersonPane.tsx`: how a contact's fields show.
- `app/src/features/settings/labels.ts`: `FIELD`, `BUTTON`,
  `PRIMARY_BUTTON`.
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

### `app/src/lib/contactDraft.ts`

- `ListKind = "email" | "phone" | "address" | "url"`, and `LABELS`, the
  labels each list's select offers, in order:
  - email: home, work, other;
  - phone: mobile, home, work, main, fax, pager, other;
  - address: home, work, other;
  - url: homepage, home, work, other.
- `Birthday {month, day, year}`: month and day `"01"`… (two digits),
  year four digits or `""`; all `""` means no birthday.
  `ContactDraft` holds the editable fields (`givenName`, `familyName`,
  `nickname`, `organization`, `title`, `emails`, `phones`, `addresses`,
  `urls`, `birthday`, `note`). `DraftErrors` is a record of messages by
  key.
- `draftOf(contact | null)`: the contact's fields, with the lists
  copied (not shared), and `"--04-12"` read as `{"04", "12", ""}`; or an
  empty draft.
- `birthdayValue(b)`: `YYYY-MM-DD`, `--MM-DD` without a year, `""` when
  all are empty.
- `nextLabel(kind, used)`: the first of the kind's labels not in `used`,
  else its last.
- `validate(draft, isNew)` returns these messages, and `{}` when all is
  well:
  - `emails.<index>`: "Enter an address like name@example.com." for a
    non-blank email that is not text, "@", text without spaces;
  - `birthday`: "This day doesn't exist." for a month without a day (or
    the reverse), a year that is not four digits, or a day that does not
    exist (Feb 29 only in a leap year, or with no year);
  - `name`: "Add a name, a company or an email address." for a new
    contact with no given or family name, no organization and no
    non-blank email.
- `changedFields(contact | null, draft)`: blank list rows are dropped,
  and values (and labels) trimmed, but the note is not.
  - For a new contact (`null`): every field that is not empty.
  - Otherwise: each text field whose trimmed value differs; each list,
    whole, when it differs from the contact's; the birthday when its
    value differs (`""` to clear).

### `app/src/features/people/ContactEditor.tsx`

`ContactEditor` takes these props, and keeps its draft and errors in
state:
- `contact` (`Contact | null`); `books?` (`BookChoice[]`, each
  `{id, label}`); `photo?` (a node rendered first); `busy?`; `error?`;
- `onSave(fields, bookId?)`, `onCancel()`, and `onDelete?()`.

Every name below is a control's accessible name, which the tests use.
- **Address book**: for a new contact with more than one book, a select
  "Address Book" of the books' labels, starting on the first. A new
  contact's `onSave` gets the chosen book's ID (the only one when there
  is one); an edit gets `undefined`.
- **Text fields** "First", "Last", "Nickname", "Company", "Job Title",
  and a textarea "Note".
- **Lists:**
  - per email, phone and URL row: a select "Label for email N" (the
    row's own label first when it is not one of `LABELS`; Custom… last,
    which swaps the select for a text field "Custom label for email N"),
    a text field "Email N" ("Phone N", "URL N"), and a button "Remove
    email N";
  - per address row: the label select, a textarea "Street N", and text
    fields "City N", "State N", "ZIP N", "Country N";
  - after each list, a button "add email" ("add phone", "add address",
    "add URL") that adds an empty row labeled `nextLabel`. N counts
    from 1.
- **Birthday**: selects "Birthday month" (values "01"…"12") and
  "Birthday day" ("01"…"31"), each with an empty choice, and a text field
  "Birthday year".
- **Buttons**: "Cancel" and "Done", and "Delete Contact" only with
  `onDelete`. Escape in the editor calls `onCancel` and stops there.
- **Done** validates. With errors it shows each message, marks its field
  `aria-invalid="true"`, and disables Done until a change makes the
  draft valid again (validation reruns on every change once it has
  failed). Without errors it calls
  `onSave(changedFields(contact, draft), bookId)`. Done is also disabled
  while `busy`.
- **`error`** shows in an element with `role="alert"`.

Measurements and colors are the spec's.

## Tests (given, do not edit)

`app/src/lib/contactDraft.test.ts`, `app/src/features/people/ContactEditor.test.tsx`.

## Gotchas

- The Custom… option needs a value no label can have; the test selects
  it with `"\u0000custom"`.
- Rows have no IDs: key them by position, and say why in a
  `biome-ignore` comment.
- Keep the component presentational: no imports from `app/src/data/` or
  `app/src/rpc/` beyond types.

## Out of scope

Putting the editor in the person pane, the toolbar and the keys
(T-0136); the photo well and deleting (T-0137); every file not under
`touch`.

## Done when

`make accept T=0135` and `make ui-check` pass, and only the files under
`touch` changed.
