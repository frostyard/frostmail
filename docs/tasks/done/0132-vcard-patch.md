---
id: "0132"
title: Patch a vCard's names, single fields and photo
milestone: M5.5
size: M
touch:
  - internal/vcardx/patch.go
given:
  - internal/vcardx/patch_test.go
acceptance: go test ./internal/vcardx -run 'TestPatch' -count=1
---
# T-0132: Patch a vCard's names, single fields and photo

## Goal

`people.update` changes a contact by patching its vCard, which pimsync
then puts back on the server (ADR-0030; docs/design/pim.md, Editing). Patch
a card's names, nickname, organization, title, birthday, note and photo
while leaving every other byte as the server sent it (ADR-0018). The
lists (emails, phones, addresses, URLs) are T-0133's, in `patchlist.go`.

## Read first

- ADR-0018 and ADR-0030; `docs/design/pim.md`: "Editing".
- `internal/contentline`: `Parse` (each property's `Start` and `End` bytes,
  a component's `End`), `Prop.Encode`, `Prop.Fields`, `Prop.Text`,
  `Prop.Param`, `Apply` and `Edit`, `Fold`, `EscapeText`, `JoinFields`,
  `LineEnding`.
- `internal/vcardx/vcardx.go`: `Parse`, which the tests read results back
  with, and how it reads `BDAY` and `PHOTO`.
- The stubs `internal/vcardx/patch.go` (keep `Fields` and `Patch`'s
  signature; replace the body and `errPatchNotYet`) and `patchlist.go`
  (do not edit: `Patch` must call `listEdits` and add its edits).
- `internal/calendar/todopatch.go`, the same kind of patch for VTODOs.
- The given test.
- `docs/tasks/EXECUTOR.md`

## Contract

`Patch(src, f, now)` works on the first `VCARD` of `src`. It returns an
error when the source does not parse or has no `VCARD`. When `f` sets
nothing (every field nil), it returns `src` unchanged. A card whose
`VERSION` is `4.0` is version 4; any other card is version 3.

**Lines.** Each property `f` sets replaces the first line of that name in
the card, keeping its group and parameters unless this contract says
otherwise, and removes any later lines of that name. When the card has no
line of that name, the new line is inserted at the start of the
`END:VCARD` line, without parameters. A property set to `""` has every
line of its name removed. Rewritten and inserted lines are written with
`Prop.Encode` in the source's own line ending (`contentline.LineEnding`),
so they are folded at 75 octets. Every other byte stays as it was. Collect
the edits and `contentline.Apply` them once. Insertions come in this
order: `N`, `FN`, `NICKNAME`, `ORG`, `TITLE`, `BDAY`, `NOTE`, `PHOTO`, and
then `listEdits`'s.

- **`N`**, when `GivenName` or `FamilyName` is set: five fields, the
  family name and the given name from `f` where set, and the old line's
  values otherwise (middle name, prefix and suffix are always the old
  ones). Each is escaped (`JoinFields`).
- **`FN`**, when `GivenName`, `FamilyName` or `Organization` is set. The
  derived name is the given and family names joined by a space and
  trimmed, or, when both are empty, the organization trimmed.
  - `FN` follows when the card has no `FN`, or when its text (trimmed)
    equals the name derived from the card's old values. It then becomes
    the name derived from the new ones, escaped.
  - `FN` keeps its line when the new derived name is empty, when it
    differed from the old derived name (the user chose it), or when it
    already equals the new one.
- **`NICKNAME`**, **`TITLE`**, **`NOTE`**: the escaped text (`EscapeText`;
  a newline becomes `\n`).
- **`ORG`**: the organization replaces the first field, and the old
  line's other fields (departments) stay, all escaped. When the result is
  all empty, the line is removed.
- **`BDAY`** (`YYYY-MM-DD`, `--MM-DD`, or `""`). Anything else, or a day
  that does not exist (February 30, month 13; a `--MM-DD` is checked in a
  leap year), is an error. The `VALUE` and `X-APPLE-OMIT-YEAR`
  parameters are dropped, then:
  - without a year, on a line that had `X-APPLE-OMIT-YEAR` (Apple's form):
    `BDAY;X-APPLE-OMIT-YEAR=1604:1604-MM-DD`;
  - version 4: `YYYYMMDD`, or `--MMDD` without a year;
  - version 3: `YYYY-MM-DD`, or `--MM-DD` without a year.
- **`PHOTO`**: a `Photo` with `Data` must be `image/jpeg` or `image/png`
  (anything else is an error). Its base64 (standard encoding) is written
  as `PHOTO;ENCODING=b;TYPE=JPEG:<base64>` (or `TYPE=PNG`) in version 3,
  or as `PHOTO:data:image/jpeg;base64,<base64>` in version 4. Either way,
  the old line's parameters are dropped and its group kept. A `Photo`
  without `Data` removes every `PHOTO` line.
- **`REV`**: when `f` sets anything and the card has a `REV`, it becomes
  `now` in UTC: `2026-10-14T15:30:00Z` in version 3, `20261014T153000Z`
  in version 4. A card without `REV` gets none.

## Tests (given, do not edit)

`internal/vcardx/patch_test.go`: `TestPatchNamesKeepTheRest`,
`TestPatchKeepsAFormattedNameTheUserChose`,
`TestPatchWritesAMissingFormattedName`, `TestPatchCompanyCard`,
`TestPatchTextFields`, `TestPatchInsertsInOrder`,
`TestPatchKeepsGroupsAndParametersAndDropsRepeats`, `TestPatchBirthday`,
`TestPatchPhoto`, `TestPatchRevision`, `TestPatchKeepsLineEndingsAndFolds`,
`TestPatchRefusesWhatIsNotACard`. The test cards are the existing
`testdata/google.vcf`, `icloud.vcf`, `nextcloud.vcf` and `group.vcf` (LF
line endings).

## Gotchas

- Edits must not overlap. A line that is rewritten is one edit; do not
  also remove it as a repeat.
- `Prop.Encode` folds. Do not fold again.
- Property names from `Parse` are uppercased; compare with them.
- The insertion point is the start of the card's last line (its `END`),
  found from the component's `End`. Inserting at `End` itself would land
  after `END:VCARD`.
- Keep functions under 60 lines.

## Out of scope

The lists (`listEdits`, `NewCard`: T-0133), the people API's use of
`Patch` (the planner's), and every file not under `touch`.

## Done when

`make accept T=0132` and `make check` pass, and only the files under
`touch` changed.
