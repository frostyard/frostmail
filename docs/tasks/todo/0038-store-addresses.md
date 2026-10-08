---
id: "0038"
title: Record seen addresses and suggest recipients
milestone: M3
size: S
touch:
  - internal/store/addresses.go
given:
  - internal/store/addresses_test.go
acceptance: go test ./internal/store -run 'Address' -count=1
---
# T-0038: Record seen addresses and suggest recipients

## Goal

The compose window completes recipients as the user types, from addresses
already seen in mail (`docs/design/send.md`, Addresses). Implement the two
store functions over the `addresses` table: counting addresses as they are
seen, and suggesting matches for a typed prefix.

## Read first

- `internal/store/addresses.go`: the stubs.
- `internal/store/migrations/0003_drafts_outbox.sql`: the `addresses` table.
- `internal/store/message.go`: `Address`.
- The given test `internal/store/addresses_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the signatures; replace each stub's doc comment (and its "Task T-0038 …"
sentence) with the rules below.

**`RecordAddresses(ctx, as, seen)`**, for each address in `as` whose
trimmed `Addr` is not empty, once per call per lowercased address (the first
occurrence in `as`):

- the key is the trimmed, lowercased address;
- a new row gets the trimmed name, `count` 1 and `last_seen` =
  `FormatTime(seen)`;
- an existing row gets `count + 1`, the given name if it is not empty
  (otherwise it keeps its name), and `last_seen` = the later of the two.

Use one `INSERT … ON CONFLICT (address) DO UPDATE` statement per address.

**`SuggestAddresses(ctx, prefix, limit)`**: an empty prefix returns `nil`.
`limit` at or below 0 means 10. A row matches when, case-insensitively, the
address starts with `prefix`, or the name starts with it, or any later word
of the name (after a space) starts with it. `%`, `_` and `\` in the prefix
are literal: escape them for `LIKE … ESCAPE '\'`. Order by `count`
descending, then `last_seen` descending, then address. Return the addresses
(`Addr`) with their names.

## Tests (given, do not edit)

`internal/store/addresses_test.go`: TestRecordAddresses,
TestSuggestAddresses, TestSuggestAddressesDefaultLimit.

## Gotchas

- SQLite's `LIKE` is already case-insensitive for ASCII; lowercase the
  prefix anyway, since addresses are stored lowercased.
- `last_seen` comparisons work on the stored text because `FormatTime` has
  a fixed width (`max(last_seen, excluded.last_seen)`).

## Out of scope

Calling `RecordAddresses` from message storage and sending (planner), and
every file except `internal/store/addresses.go`.

## Done when

`make accept T=0038` and `make check` pass, and only
`internal/store/addresses.go` changed.
