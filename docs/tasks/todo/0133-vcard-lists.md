---
id: "0133"
title: Patch a vCard's emails, phones, addresses and URLs
milestone: M5.5
size: L
touch:
  - internal/vcardx/patchlist.go
given:
  - internal/vcardx/patchlist_test.go
acceptance: go test ./internal/vcardx -count=1
---
# T-0133: Patch a vCard's emails, phones, addresses and URLs

## Goal

Finish the contact patch of T-0132 (ADR-0030): map the editor's lists of
emails, phones, addresses and URLs onto a card's `EMAIL`, `TEL`, `ADR`
and `URL` lines. Lines that did not change keep their bytes, and Apple's
grouped labels (`itemN.X-ABLabel`) survive. Also write the new cards
`people.create` stores (`NewCard`).

## Read first

- ADR-0018 and ADR-0030; `docs/design/pim.md`: "Editing".
- `internal/vcardx/patch.go` (T-0132): `Patch` calls `listEdits` and adds
  its edits; reuse its helpers for `BDAY` and `PHOTO` in `NewCard`.
- `internal/vcardx/vcardx.go`: `Parse`, and how `groupLabels`,
  `propertyLabel`, `readLabeled` and `readAddress` read labels and values.
  A patched card must read back as the lists given.
- `internal/contentline`: `Prop.Encode`, `Prop.Fields`, `ParamValues`,
  `Apply`, `EscapeText`, `JoinFields`.
- The stub `internal/vcardx/patchlist.go`: keep `listEdits`'s and
  `NewCard`'s signatures; replace the bodies and `errListsNotYet`.
- The given test, and `patch_test.go`, whose helpers it uses.
- `docs/tasks/EXECUTOR.md`

## Contract

### `listEdits(src, card, f, v4, eol)`

`f.Emails`, `f.Phones`, `f.Addresses` and `f.URLs` map onto the card's
`EMAIL`, `TEL`, `ADR` and `URL` lines, handled in that order. A nil list
makes no edit. For each list that is set, take the property's lines in
source order (`L1…Lk`, any group) and the new values (`V1…Vn`):

- **`i ≤ min(k, n)`:** when `Vi` reads the same as `Li` (its label and
  its value as `Parse` reads them; for addresses, every field), `Li` is
  left alone. Otherwise `Li` is rewritten with `Prop.Encode`, but only
  when its group, value or parameters actually change. A label change
  on a grouped line can leave the line itself untouched.
  - A changed **value** keeps the line's group and parameters. A version
    4 value keeps its `mailto:` (emails) or `tel:` (phones) prefix when
    the line had one; CR and LF are dropped. An address's value is
    `JoinFields` of seven fields. When the street reads the same as
    before, the first three fields stay as they were. When it changed,
    they become `"", "", street`.
  - A changed **label** first drops the label `TYPE` values (`home`,
    `work`, `other`, `cell`, `mobile`, `iphone`, `fax`, `pager`, `main`,
    any case) from every `TYPE` parameter. It keeps the parameter's other
    values, joined by commas, and drops a `TYPE` left empty. It then
    removes the `X-ABLabel` lines of the line's group, and labels it:
    - **known labels** add a `TYPE` parameter at the end: for every list,
      home `HOME`, work `WORK` and other `OTHER`; for phones also mobile
      `CELL`, main `MAIN`, fax `FAX` and pager `PAGER`. Version 4 writes
      them lowercase;
    - **any other label** is an Apple group label. The line keeps its
      group, or takes a new one, `itemN`, where N is one more than the
      highest `itemN` in the card, counting groups this patch has made.
      Then the line `<group>.X-ABLabel:<label>` is written right after
      it: the label escaped, and for `homepage` Apple's
      `_$!<HomePage>!$_`;
    - an empty label adds nothing.
- **`i > k` (added values):** new lines are inserted right after the
  property's last line, or after the last `X-AB…` line of its group when
  it has one. When the card has no line of that kind, they go before
  `END:VCARD`. A new line is the property with the label as above: a
  version 3 email starts with `TYPE=INTERNET`, and a value has no URI
  prefix. For example, `EMAIL;TYPE=INTERNET;TYPE=HOME:a@b.example`,
  `TEL;TYPE=CELL:+1 555 0100`, and in version 4 `EMAIL;TYPE=home:a@b.example`.
- **`i > n` (removed values):** `Li` is removed, with the other lines of
  its group whose names start with `X-AB` (its `X-ABLabel`, an
  `X-ABADR`).

### `NewCard(uid, f, now)`

A vCard 3.0 with CRLF lines, written with `Prop.Encode`, in this order:
- `BEGIN:VCARD`, `VERSION:3.0`, `PRODID:-//Frostyard//Frostmail//EN`,
  `UID`;
- `FN`: the derived name of T-0132, or the first email when that is
  empty;
- `N` (family and given, three empty fields);
- `NICKNAME`, `ORG` and `TITLE` when not empty;
- every email, phone, address and URL as an added line above, in that
  order, with group numbers from `item1`;
- `BDAY` (version 3 form) and `NOTE` when set and not empty, and `PHOTO`
  (version 3 form) when it has data;
- `REV` (`now`, version 3 form), and `END:VCARD`.

It returns the errors `Patch` would for the birthday and photo.

## Tests (given, do not edit)

`internal/vcardx/patchlist_test.go`: `TestPatchListValueKeepsItsLine`,
`TestPatchRelabels`, `TestPatchKeepsAppleGroups`,
`TestPatchAddsAndRemoves`, `TestPatchVersion4`, `TestPatchRemovesAList`,
`TestNewCard`. T-0132's `patch_test.go` must still pass.

## Gotchas

- `Parse` uppercases parameter names, so re-encoding a line changes
  `type=` to `TYPE=`. Compare the parameters before rewriting, and leave
  the bytes of a line whose group, value and parameters are unchanged.
- A relabel can insert an `X-ABLabel` at the very place where the old
  one is removed. Put the insertion before the removal, or `Apply`
  reports an overlap.
- `Parse` reads every fax number as `fax`, and Apple's `_$!<Other>!$_`
  as `other`. Compare labels as `Parse` reads them.
- Keep functions under 60 lines.

## Out of scope

The people API's use of these (the planner's), and every file not under
`touch`.

## Done when

`make accept T=0133` and `make check` pass, and only the files under
`touch` changed.
