---
id: "0049"
title: Read Mozilla autoconfig documents
milestone: M4
size: S
touch:
  - internal/discover/autoconfig.go
given:
  - internal/discover/autoconfig_test.go
acceptance: go test ./internal/discover -run Autoconfig -count=1
---
# T-0049: Read Mozilla autoconfig documents

## Goal

Adding an account finds its servers without asking the user: many domains
publish a Mozilla autoconfig document, and Thunderbird's ISPDB serves the
same format for the big providers (`docs/design/accounts.md`, Discovery).
Implement the reader that turns such a document into IMAP and SMTP
settings; maild fetches the documents.

## Read first

- `internal/discover/discover.go`: `Server`, `Settings`, `ErrNothing`.
- `internal/discover/autoconfig.go`: the stub.
- `docs/design/accounts.md`: "Discovery".
- The given test `internal/discover/autoconfig_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Replace the stub's doc comment and its "Task T-0049 …" sentence with what
the function does.

`ParseAutoconfig(doc, email)`:

- An address without an `@`, or with nothing before or after it, is an
  error (not `ErrNothing`).
- Decode the XML with `encoding/xml` into structs for `clientConfig` →
  `emailProvider` (any number) → `incomingServer` / `outgoingServer`
  elements with a `type` attribute and `hostname`, `port`, `socketType`,
  `username` and repeated `authentication` children. A decoding error is
  returned as is.
- **IMAP** is the first `incomingServer` with `type="imap"` that is usable;
  **SMTP** the first usable `outgoingServer` with `type="smtp"`; both in
  document order across providers.
- **Usable** means: `socketType` (trimmed, case-insensitive) is `SSL`
  (→ `api.TLSModeTLS`) or `STARTTLS` (→ `api.TLSModeStartTLS`) — `plain`
  and anything else are skipped; at least one `authentication` value starts
  with `password-` (OAuth2-only servers are skipped); after expansion the
  trimmed host name is not empty; the trimmed port is an integer from 1 to
  65535.
- **Placeholders** in `hostname` and `username`: `%EMAILADDRESS%` (the
  address as given), `%EMAILLOCALPART%` (the part before the last `@`),
  `%EMAILDOMAIN%` (the part after it, lowercased). Host names are trimmed
  and lowercased; user names are trimmed.
- Return the `Settings` with nil for a server not found, or `ErrNothing`
  when both are missing.

## Tests (given, do not edit)

`internal/discover/autoconfig_test.go`: TestParseAutoconfig,
TestParseAutoconfigPlaceholdersAndChoices,
TestParseAutoconfigPartialAndEmpty.

## Gotchas

- `strings.NewReplacer` expands all three placeholders in one pass.
- XML element and attribute names are case-sensitive; match the test's
  documents.

## Out of scope

Fetching documents and the ISPDB (planner), SRV records (T-0050), and every
file except `internal/discover/autoconfig.go`.

## Done when

`make accept T=0049` and `make check` pass, and only
`internal/discover/autoconfig.go` changed.
