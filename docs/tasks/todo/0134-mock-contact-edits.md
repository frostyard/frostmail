---
id: "0134"
title: Contact edits in MockTransport
milestone: M5.5
size: M
touch:
  - app/src/rpc/mock/people.ts
  - app/src/rpc/mock/peopleEdits.ts
given:
  - app/src/rpc/mock/people.edit.test.ts
acceptance: make ui-vitest F="src/rpc/mock/people"
---
# T-0134: Contact edits in MockTransport

## Goal

The app's tests run against `MockTransport`. Give its people domain the
writes M5.5 adds (`people.create`, `update`, `delete`, `setPhoto` and
`restore`; schema/rpc/people.yaml), behaving as maild's will (ADR-0030,
ADR-0032), so T-0135 to T-0137 can build the contact editor against it.

## Read first

- `schema/rpc/people.yaml`: the five methods, `ContactFields`,
  `ContactWrite`; `app/src/rpc/gen/api.ts` has their types.
- ADR-0030 and ADR-0032; `docs/design/pim.md`: "Editing".
- `app/src/rpc/mock/people.ts`: `MockPeople`, its `dispatch`,
  `writableBooks` and `add` (Add to Contacts), which these follow.
- `app/src/rpc/mock/fixture.ts`: the fixture's address books (`BOOKS`:
  Contacts 101, the read-only Shared 102) and people.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

Put the edits in a new `peopleEdits.ts`: a class `MockContactEdits`, made
by `MockPeople` with the mock's data, its `emit`, its accounts, a way to
take a new ID (`nextId`), and its writable books. `MockPeople.dispatch`
answers the five methods through it. Every successful write emits
`people.changed {accountId}`, keeps a restore under a new ID (`r1`, `r2`,
…), and returns `ContactWrite`: the contact's ID, its person as changed
(a copy; absent when no contact of theirs is left), and the restore's ID.

- **`people.create {collectionId?, fields}`**
  - The book is the one named, else the first writable one. A book that
    does not exist is `notFound`; one that is not writable (read-only,
    disabled, or in a read-only account) is `conflict`.
  - The new contact has a new ID, empty fields with `fields` over them,
    and `readOnly` false. It goes in a new person whose ID is the
    contact's.
  - The display name is the derived name: given and family names joined
    and trimmed, else the organization, else the first email. A contact
    with none of these is `invalidParams`.
- **`people.update {id, fields}`**: the contact (by contact ID; unknown
  is `notFound`) must be writable. A read-only contact, book or account
  is `conflict`. The fields replace the contact's, and lists replace
  whole lists. The display name follows only when it was the derived name
  (or empty) before the change; otherwise it stays, as a name the user
  chose.
- **`people.delete {id}`**: writable as above. The contact leaves its
  person; a person with no contacts left is removed, with their photo.
  The result's person is the one left, if any.
- **`people.setPhoto {id, photo?}`**: writable as above. A photo must be
  `image/jpeg` or `image/png`, at most 256 KB decoded (otherwise
  `invalidParams`); it is stored by person ID, where `people.photo`
  finds it. No photo removes it.
- **Validation** (create and update): every email needs text, an "@" and
  text, with no spaces; a birthday must be `YYYY-MM-DD` or `--MM-DD` and a
  day that exists; otherwise `invalidParams`.
- **A person's** `displayName` and `organization` are their first
  contact's, and `hasPhoto` says whether a photo is stored for them.
- **`people.restore {restoreId}`** writes back what the change replaced.
  A created contact is removed; a changed one gets its old fields back;
  a deleted one returns to its person at its old place, or with its
  person when they had gone. The person's photo goes back as it was.
  A restore runs once; an unknown or used ID is `notFound`. It returns
  null and emits `people.changed`.

## Tests (given, do not edit)

`app/src/rpc/mock/people.edit.test.ts`. The existing `people.test.ts`
must still pass.

## Gotchas

- Return copies (`structuredClone`), never the stored objects.
- The fixture's Elif Tanaka is one person with two contacts. Deleting the
  writable one leaves her read-only Shared card, and restoring it puts it
  back first.
- "Vertex News" has the organization "Vertex", but its name is one the
  user chose: it must not follow an edit.

## Out of scope

The UI (T-0135 to T-0137), maild, and every file not under `touch`.

## Done when

`make accept T=0134` and `make ui-check` pass, and only the files under
`touch` changed.
