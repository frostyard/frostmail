---
id: "0138"
title: mailctl people add, edit and rm
milestone: M5.5
size: S
touch:
  - cmd/mailctl/people.go
  - cmd/mailctl/people_edit.go
given:
  - cmd/mailctl/people_edit_test.go
  - cmd/mailctl/pim_test.go # T-0090's, amended: people show names each contact
acceptance: go test ./cmd/mailctl -run 'TestPeople' -count=1
---
# T-0138: mailctl people add, edit and rm

## Goal

Make contacts editable from the command line, over the people writes
maild now has (`people.create`, `people.update`, `people.delete`;
schema/rpc/people.yaml, ADR-0030). `people show` names each contact by
its ID, so `edit` and `rm` can take it.

## Read first

- `cmd/mailctl/people.go`: `newPeopleCmd`, `people ls` and `people show`,
  `pimID` and `pimIDFlag`, `printPerson`.
- `schema/rpc/people.yaml`: `ContactFields` (a nil list keeps the
  contact's, a list replaces it), `ContactWrite`, and the three methods.
- The given tests, and `pim_test.go`'s `pimFixture`, `mustRun`, `cells`.
- `docs/tasks/EXECUTOR.md`

## Contract

- **`people show`** prints, before each contact's emails, phones and
  note, a line `Contact <id>` (the contact's ID, not the person's), with
  ` (read-only)` after it for a read-only contact. Nothing else in its
  output changes.
- **Flags** shared by `add` and `edit` (in a new `people_edit.go`):
  `--first`, `--last`, `--nickname`, `--company`, `--title`, `--note`,
  `--birthday`, and the repeatable `--email` and `--phone`.
  - A repeatable flag's value is `VALUE` or `LABEL=VALUE`, trimmed.
  - Only flags the user gave become fields. Giving `--email` or
    `--phone` at least once replaces that whole list.
- **`people add [--book ID] [flags]`** calls `people.create` (with
  `collectionId` when `--book` is given) and prints
  `added <person's display name> (contact <id>)`.
- **`people edit CONTACT_ID [flags]`** calls `people.update` and prints
  `updated <person's display name>`. It is an error, before dialing,
  when no field flag is given.
- **`people rm CONTACT_ID`** calls `people.delete` and prints
  `deleted contact <id>`.
- With `--json`, each of the three prints the `ContactWrite` instead
  (`clix.OutputJSON`), as the other commands print their results.
- maild's errors (an unknown contact, an invalid email) end the command
  with an error, and so does an ID that is not a number.

## Tests (given, do not edit)

`cmd/mailctl/people_edit_test.go`, and `cmd/mailctl/pim_test.go` (T-0090's
contract test, amended here: its `people show` expectation has the new
`Contact` line).

## Gotchas

- `api.ContactFields` holds slices, so it cannot be compared with `==`.
  Check its fields one by one.
- Register the new commands in `newPeopleCmd`.
- Keep functions under 60 lines.

## Out of scope

Addresses, URLs and photos on the command line; every file not under
`touch`.

## Done when

`make accept T=0138` and `make check` pass, and only the files under
`touch` changed.
