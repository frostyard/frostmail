---
id: "0136"
title: Edit and make contacts from the person pane
milestone: M5.5
size: L
touch:
  - app/src/data/stores.ts
  - app/src/app/usePeople.ts
  - app/src/features/people/PersonPane.tsx
  - app/src/app/PeopleModule.tsx
  - app/src/features/toolbar/Toolbar.tsx
  - app/src/app/ToolbarContainer.tsx
  - app/src/app/MainWindow.tsx
given:
  - app/src/app/PeopleModule.edit.test.tsx
acceptance: make ui-vitest F="src/app/PeopleModule src/features/people src/features/toolbar src/app/MainWindow"
---
# T-0136: Edit and make contacts from the person pane

## Goal

Put T-0135's `ContactEditor` in the People module
(docs/specs/pim-ui.md, "Editing a contact", "People toolbar",
"Behavior"): Edit on each writable contact, New Contact in the toolbar
and on Ctrl+N, saving through `people.update` and `people.create`, and
People held still while an edit is open.

## Read first

- `docs/specs/pim-ui.md`: "Person pane", "Editing a contact", "People
  toolbar", "Behavior".
- `app/src/features/people/ContactEditor.tsx` and
  `app/src/lib/contactDraft.ts` (T-0135).
- `app/src/rpc/mock/people.ts` and `peopleEdits.ts` (T-0134), which the
  test runs against.
- The files under `touch`, especially `PeopleModule.tsx`'s
  `PeopleDetail`, `usePeople.ts`'s `useAddressBooks`, `Toolbar.tsx`'s
  `ListSegment`, and `MainWindow.tsx`'s `moduleCommand`.
- The given test, and `PeopleModule.test.tsx`, which must still pass.
- `docs/tasks/EXECUTOR.md`

## Contract

- **State** (`useUI`): `peopleEditing: number | "new" | null` (the
  contact ID being edited, a new contact, or none; first `null`) and
  `editContact(value)`. Not persisted.
- **`usePeople`** returns `writableBooks: BookChoice[]`: the address
  books that are not read-only (nor in a read-only account), each
  labeled `"<book> — <account email>"`, in section order.
- **`PersonPane`** gains `editing`, `editor` (a node) and
  `onEdit(contactId)`:
  - with `onEdit`, each contact that is not `readOnly` has an Edit text
    button named "Edit Contact" at the end of its heading row. It is
    disabled while another contact is being edited, and absent on the
    edited one;
  - the edited contact's block shows `editor` instead of its fields;
  - with `editing === "new"`, the pane shows only a section with the
    level-2 heading "New Contact" and `editor`.
- **`PeopleModule`**: while `peopleEditing` is set, the pane gets a
  `ContactEditor` (keyed by what is being edited) for that contact of
  the selected person, or for a new one with `books` set to
  `writableBooks`.
  - **Saving an edit:** empty fields close the editor without a call.
    Otherwise it sends `people.update {id, fields}`, closes when that
    succeeds, and shows maild's error (`error`) when it fails, with
    `busy` while the call runs.
  - **Saving a new contact:** `people.create` with `collectionId` set to
    the editor's book (when it gives one) and `fields`, then selects the
    new person and closes.
  - **Cancel** closes the editor.
  - While editing, the People sidebar (its fieldset) and the list's
    container carry `inert`.
- **Toolbar** (People mode): a New Contact button (`UserPlus`, named "New
  Contact", tooltip "New Contact (Ctrl+N)") before New Message, whose own
  tooltip then has no shortcut. A `people: {canCreate}` prop disables it,
  and `ToolbarContainer` passes `canCreate` as "some book is writable and
  no edit is open". The new command is `{kind: "newContact"}`, which sets
  `peopleEditing` to `"new"`. The search field's wrapper is `inert` while
  a People edit is open.
- **Keys** (`MainWindow`): in People, Ctrl+N starts a new contact
  (`"new"`) when a book is writable, instead of a new message. While an
  edit is open, People's commands do nothing, Ctrl+1 to Ctrl+4 included.

## Tests (given, do not edit)

`app/src/app/PeopleModule.edit.test.tsx`. The existing People, toolbar
and main window tests must still pass.

## Gotchas

- `inert` is a boolean prop in React 19.
- The editor's Escape stops propagation, so the window's key handler
  does not see it.
- After a save, `people.changed` refetches the list and the person
  (100 ms); do not patch local state by hand.

## Out of scope

Photos, Delete Contact and refused-change notices (T-0137); every file
not under `touch`.

## Done when

`make accept T=0136` and `make ui-check` pass, and only the files under
`touch` changed.
