---
id: "0062"
title: Join contacts into people and serve them
milestone: M4.5
size: M
touch:
  - internal/store/people.go
  - internal/engine/people.go
  - internal/engine/compose.go
  - internal/pimsync/dav.go
given:
  - internal/store/people_test.go
  - internal/engine/people_test.go
  - internal/pimsync/people_test.go
acceptance: go test ./internal/store ./internal/engine ./internal/pimsync -count=1
---
# T-0062: Join contacts into people and serve them

## Goal

The same person often has a card in several address books and accounts.
Join contacts that share an email address into people, keep them in step
after every DAV pass, and serve them: the People list, a person with all
their contacts, the contact card mail shows for an address, the person's
photo, and contacts ahead of merely seen addresses in recipient
autocomplete (`docs/design/pim.md`, "People in mail"; ADR-0020).

## Read first

- `docs/design/pim.md`: "Storage" (`people`, `contacts`) and "People in
  mail".
- `schema/rpc/people.yaml` and `api/zz_generated.go`: `PersonSummary`,
  `Person`, `Contact`, `LabeledValue`, `PostalAddress`, `Photo`,
  `ContactCard`, `PeopleService`.
- `internal/store/migrations/0007_pim.sql`: `people`, `contacts`,
  `contact_emails`, `collections`, `account_services`.
- `internal/store/people.go`: the type and stubs. `internal/store/contacts.go`,
  `collections.go`, `objects.go`, `addresses.go` (`SuggestAddresses`,
  `escapeLike`), `read.go` (`viewOrder`, `Summaries`).
- `internal/engine/people.go`: the stubs; `internal/engine/compose.go`:
  `addresses.Suggest`, `toAPIAddresses`; how other domains turn
  `store.Summary` into `api.MessageSummary` (search `engine` for it).
- `internal/vcardx/vcardx.go`: `Parse` and `Card`'s fields.
- `internal/pimsync/dav.go`: the transactions of a pass.
- The given tests.
- `docs/tasks/EXECUTOR.md`

## Contract

Replace each stub's doc comment and "Task T-0062" sentence with what it
does; remove `errNotYet` from `people.go` and the stub comment from
`engine/people.go`.

### Store (`people.go`)

**Eligible contacts** are `contacts` rows whose object's collection is an
enabled `addressbook` and whose account has the `contacts` service row
enabled. Others belong to no person.

- **`RelinkPeople(ctx)`** recomputes every person from the eligible
  contacts. Two contacts are the same person when they share an email in
  `contact_emails` (already lowercased), directly or through others. A
  person's ID is the smallest object ID among its contacts. Taking its
  contacts by ascending object ID: `display_name` is the first non-empty
  display name (else ""); `sort_key` is that contact's sort key, or the
  lowercased display name when it is empty; `organization` is the first
  non-empty organization. Afterwards `people` holds exactly these persons
  (rows inserted, updated or deleted as needed) and every contact's
  `person_id` is its person's ID, or NULL when it is not eligible. It
  emits nothing.
- **`People(ctx, query)`**: persons ordered by `sort_key`, then ID. The
  query is lowercased and split with `strings.Fields`; a person matches
  when every query word is a prefix of a word of their display name,
  organization, or any of their contacts' emails, where words are runs of
  letters and digits (`unicode.IsLetter`, `unicode.IsDigit`), lowercased.
  No query words match everyone.
- **`PersonRow`**: `Email` is the first email (by position) of the
  person's first contact (by object ID), or ""; `HasPhoto` is whether any
  of its contacts has a photo; `ContactIDs` ascend.
- **`Person(ctx, id)`**, **`PersonByEmail(ctx, email)`** (the email
  trimmed and lowercased; the person of an eligible contact with it):
  one `PersonRow`, or `ErrNotFound`.
- **`PersonPhoto(ctx, id)`**: the photo and its type of the person's
  first contact (by object ID) that has one; `ErrNotFound` when there is
  no such person or photo.
- **`SuggestContacts(ctx, prefix, limit)`**: the distinct emails of
  eligible contacts that start with the lowercased prefix, or whose
  person's display name has a word (as above) starting with it, as
  `Address{Addr: email, Name: the person's display name}`. Ordered by the
  `addresses` row's `count` descending (0 without one), its `last_seen`
  descending (without one last), the display name, then the email. An
  empty prefix returns nil; a limit at or below 0 means 10.
- **`MessagesWithAddress(ctx, email, limit)`**: the IDs of messages, not
  deleted, whose From, To or Cc address equals the email
  (case-insensitive), in `viewOrder`, at most `limit`.
- **`SeenName(ctx, email)`**: the `addresses` row's name for the
  lowercased email, or "" without a row.
- **`WritableAddressBooks(ctx)`**: enabled address books that are not
  read-only, of accounts that are not read-only and have the `contacts`
  service enabled; ordered by account ID, then `is_default` first, then
  position, then ID.

### pimsync (`dav.go`)

For address books, every transaction of the pass that stores or deletes
objects, and the `ReplaceCollections` transaction, ends with
`tx.RelinkPeople(ctx)`. Calendars do not relink.

### Engine (`people.go`, `compose.go`)

- **`people.list`**: `People(query)` as `PersonSummary` rows (an empty
  query when absent).
- **`people.get`**: the person and a `Contact` per contact, read from its
  source with `vcardx.Parse`: `DisplayName()` and every `Contact` field
  from the card's fields of the same name (`Birthday`, `Note`, labeled
  values with their label and value, addresses field by field);
  `collectionId`, `accountId` from the object's collection; `readOnly`
  when the collection or its account is read-only. Contacts are ordered
  by account ID, then collection position, then object ID. A contact
  whose source no longer parses is left out. Unknown ID: `notFound`.
- **`people.card`**: the email trimmed and lowercased; without an `@` it
  is `invalidParams`. `person` is `PersonByEmail`'s person in full (as
  `people.get`), absent when there is none. `name` is the person's
  display name, else `SeenName`. `recent` is `MessagesWithAddress(email,
  5)` as `MessageSummary` rows in that order. `upcoming` is empty until
  the calendar exists. `canAdd` is true when there is no person and
  `WritableAddressBooks` is not empty. Lists are never nil.
- **`people.photo`**: `PersonPhoto` with the data in standard base64;
  `notFound` when there is none.
- **`people.add`** stays the planner's (`notBuilt`).
- **`address.suggest`**: `SuggestContacts` first, then `SuggestAddresses`
  for the same prefix, skipping addresses already listed, together at most
  the limit (default 10).

## Tests (given, do not edit)

- `internal/store/people_test.go`: TestRelinkPeople, TestPeopleQuery,
  TestPersonPhoto, TestSuggestContacts, TestWritableAddressBooks,
  TestMessagesWithAddress.
- `internal/engine/people_test.go`: TestPeopleListAndGet, TestPeoplePhoto,
  TestContactCard, TestSuggestContactsFirst.
- `internal/pimsync/people_test.go`: TestPassLinksPeople.

## Gotchas

- Rebuilding people is a union-find over a few thousand contacts: load
  the eligible contacts and their emails in two queries and join them in
  Go. Do not query per contact.
- Keep person rows that did not change: updating only differing rows
  keeps `RelinkPeople` cheap enough to run after every batch.
- `MessagesWithAddress` must stay fast on 100,000 messages: find
  candidates in `messages_fts`'s `from_text` and `to_text` columns with a
  phrase of the address's words, then check `from_addr`, `to_json` and
  `cc_json` exactly. A scan with `json_each` over every message is too
  slow. Quote the phrase for FTS5 (double quotes, inner quotes doubled).
- `contact_emails` emails are already lowercase; `addresses.address` too.

## Out of scope

`people.add` and writing contacts (planner), upcoming events (Phase 3),
photos by URL, the app, and every file not under `touch`.

## Done when

`make accept T=0062` and `make check` pass, and only the files under
`touch` changed.
