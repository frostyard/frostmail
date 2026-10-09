---
id: "0060"
title: Read contacts from vCards
milestone: M4.5
size: M
touch:
  - internal/vcardx/vcardx.go
given:
  - internal/vcardx/vcardx_test.go
  - internal/vcardx/testdata/google.vcf
  - internal/vcardx/testdata/icloud.vcf
  - internal/vcardx/testdata/nextcloud.vcf
  - internal/vcardx/testdata/group.vcf
acceptance: go test ./internal/vcardx -count=1
---
# T-0060: Read contacts from vCards

## Goal

maild syncs address books over CardDAV and keeps each vCard as the server
sent it (ADR-0018); the People module, contact cards and autocomplete read
an index built from it. Write `internal/vcardx`: `Parse` reads the fields
Frostmail shows from vCard 3.0 and 4.0 as Google, iCloud and Nextcloud
write them, and `New` builds the vCard that Add to Contacts stores.

## Read first

- `internal/vcardx/vcardx.go`: the types and the stubs.
- `internal/contentline/contentline.go` and `encode.go`: `Parse`,
  `Component.Props`, `Prop.Group`, `Prop.Param`, `Prop.ParamValues`,
  `Prop.HasParam`, `Prop.Text`, `Prop.Fields`, `EscapeText`, `JoinFields`,
  `Fold`. Use them; do not parse content lines yourself.
- `docs/design/pim.md`: "Storage" and "People in mail".
- The given test `internal/vcardx/vcardx_test.go` and the vCards under
  `internal/vcardx/testdata/`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the types and signatures; replace each stub's doc comment and its
"Task T-0060 writes it" sentence with what the function does, and remove
`errNotYet`.

**`Parse(raw)`** parses `raw` with `contentline.Parse` (wrap its error with
`%w`, so `errors.Is(err, contentline.ErrSyntax)` holds) and reads the first
`VCARD` component; no `VCARD` is an error. Property names are uppercase
after parsing; parameter names and values compare case-insensitively.

- `UID`, `FN`, `NICKNAME`, `TITLE`: `Prop.Text()`, trimmed. `NOTE`:
  `Prop.Text()`, not trimmed.
- `KIND`, or Apple's `X-ADDRESSBOOKSERVER-KIND`: lowercased into `Kind`.
- `N`: `Prop.Fields()` in order family, given, middle, prefix, suffix, each
  trimmed; missing fields are "".
- `ORG`: the first field is `Organization`; the other non-empty fields,
  trimmed, joined by ", " are `Department`.
- `EMAIL`, `TEL`, `URL`: a `Labeled` each, in source order. The value is
  `Prop.Text()` trimmed, without a leading `mailto:` (EMAIL) or `tel:`
  (TEL), matched case-insensitively and followed by trimming again.
  `Pref` is true when `TYPE` has `pref` (vCard 3) or the `PREF` parameter
  is present with any value (vCard 4).
- `ADR`: an `Address` each. Fields: PO box, extended, street, locality,
  region, postcode, country (RFC 6350 §6.3.1), each trimmed. `Street` is
  the non-empty PO box, extended and street fields joined by "\n".
- **Labels** (`EMAIL`, `TEL`, `URL`, `ADR`), first match wins:
  1. A property in a group (`item1.EMAIL`) takes the `X-ABLABEL` of the
     same group, if there is one. Apple's `_$!<Name>!$_` becomes `Name`
     lowercased, except `HomeFAX` and `WorkFAX`, which become `fax`, and
     `iPhone`, which becomes `mobile`; any other text is the label as
     written.
  2. `TYPE` values, all occurrences and comma-separated lists together
     (`Prop.HasParam`). For `TEL`, in this order: `fax`; `cell`, `mobile`
     or `iphone` → `mobile`; `pager`; `main`; `home`; `work`; `other`.
     For the others: `home`, `work`, `other`.
  3. Otherwise "".
- `BDAY`: cut at a `T` (a date-time), then `YYYY-MM-DD` or `YYYYMMDD`
  becomes `YYYY-MM-DD`, and `--MM-DD` or `--MMDD` becomes `--MM-DD`. When
  the `X-APPLE-OMIT-YEAR` parameter equals the year, the result is
  `--MM-DD`. Anything else is "".
- `PHOTO`, the first usable one:
  - a value starting with `http://` or `https://` (case-insensitive) is
    `Photo{URI: value}`, whatever its parameters;
  - a `data:` URI with `;base64,` is decoded into `Data`, its media type
    lowercased into `Type`;
  - `ENCODING=b` or `ENCODING=BASE64` decodes the value (whitespace
    removed) with `base64.StdEncoding`. `Type` is the `TYPE` parameter
    lowercased, with `image/` prefixed when it has no `/` (`JPEG` →
    `image/jpeg`);
  - when neither says, `Type` is `http.DetectContentType(Data)`;
  - a value that does not decode, or decodes to nothing, gives no photo
    (`Photo{}`), not an error.
- Every other property is ignored.

**`DisplayName()`**: `FormattedName` trimmed when not empty; else
`GivenName + " " + FamilyName` trimmed when not empty; else `Nickname`,
`Organization`, the first email's value; else "".

**`SortKey()`**: when `FamilyName` is set, `strings.ToLower` of
`FamilyName + " " + GivenName`, trimmed; else `strings.ToLower` of
`DisplayName()`.

**`New(uid, name, email)`** returns these lines, each passed through
`contentline.Fold(line, "\r\n")`:

```
BEGIN:VCARD
VERSION:3.0
PRODID:-//Frostyard//Frostmail//EN
UID:<uid>
FN:<EscapeText(name trimmed), or email when the name is empty>
N:<JoinFields(family, given, "", "", "")>
EMAIL;TYPE=INTERNET:<email>
END:VCARD
```

The name is split with `strings.Fields`: one word is the given name; more
make the last word the family name and the rest, joined by single spaces,
the given name; no words leave both empty.

## Tests (given, do not edit)

`internal/vcardx/vcardx_test.go`: TestGoogle, TestICloud, TestNextcloud,
TestGroup, TestNames, TestDetails, TestLFAndLowercase, TestParseErrors,
TestNew; the vCards in `internal/vcardx/testdata/`.

## Gotchas

- A folded line's continuation is already joined by `contentline`; base64
  photos span several physical lines.
- `X-ABLABEL` comes after the property it labels; collect the labels by
  group before reading the properties.
- Groups compare case-insensitively (`item1`, `ITEM1`).
- Keep functions under 60 lines: one helper per property kind reads well.
- Precompile regular expressions at package level, or avoid them.

## Out of scope

Writing or patching vCards beyond `New`, contact groups' members, vCard
2.1's quoted-printable values, and every file except
`internal/vcardx/vcardx.go`.

## Done when

`make accept T=0060` and `make check` pass, and only
`internal/vcardx/vcardx.go` changed.
