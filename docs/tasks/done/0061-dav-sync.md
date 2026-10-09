---
id: "0061"
title: Sync address books and calendars over DAV
milestone: M4.5
size: L
touch:
  - internal/store/collections.go
  - internal/store/objects.go
  - internal/store/contacts.go
  - internal/pimsync/dav.go
  - internal/pimsync/index.go
given:
  - internal/store/collections_test.go
  - internal/pimsync/dav_test.go
acceptance: go test ./internal/store ./internal/pimsync -count=1
---
# T-0061: Sync address books and calendars over DAV

## Goal

maild keeps each account's address books and calendars as the server
holds them: the collections, every object's source byte for byte with its
ETag, and for contacts an index People and autocomplete read
(`docs/design/pim.md`, ADR-0017, ADR-0018). Write the store's collections,
objects and contacts, and the DAV pass in `internal/pimsync` that fills
them from `internal/davx`. Events and calendars' own index come later;
this card stores calendar objects with their kind and UID only.

## Read first

- `docs/design/pim.md`: "Services and sync" and "Storage".
- `internal/store/migrations/0007_pim.sql`: `collections`, `objects`,
  `contacts`, `contact_emails`, `pim_ops`.
- `internal/store/collections.go`, `objects.go`, `contacts.go`: the types
  and stubs. `internal/store/services.go` and `pimops.go`: finished
  neighbours in the same style. `internal/store/store.go`: `Tx`, `Emit`,
  `FormatTime`, `ParseTime`, `ErrNotFound`.
- `internal/pimsync/pimsync.go`: `Manager.Pass`, `pass`, `pass.client`,
  `Config.Batch`; `internal/pimsync/dav.go`: the stub.
- `internal/davx/davx.go` and `dav.go`: `Client.Collections`, `Sync`,
  `List`, `Multiget`, `Collection`, `Delta`, `Change`, `ErrInvalidToken`,
  `ErrUnauthorized`. `internal/davtest/davtest.go`: the server the tests
  run against.
- `internal/vcardx/vcardx.go`: `Parse`, `Card.DisplayName`, `SortKey`.
- `internal/contentline/contentline.go`: `Parse`, `Component`.
- The given tests `internal/store/collections_test.go` and
  `internal/pimsync/dav_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the types and signatures; replace each stub's doc comment and its
"Task T-0061 writes" sentence with what the function does, and remove
`errNotYet` from `store` and `pimsync` once nothing uses it. Every write
runs inside the `*Tx` it is given.

### Store: collections (`collections.go`)

- **`Collections(ctx, f)`**: rows matching every set field of `f`
  (`AccountID` non-zero, `Kind` non-empty, `EnabledOnly`), ordered by
  `account_id`, `position`, `id`. `Components` is the column split at
  commas (nil for ""); `SyncedAt` is nil for NULL.
- **`GetCollection(ctx, id)`**: one row, or `ErrNotFound`.
- **`ReplaceCollections(ctx, accountID, kind, list)`** makes the account's
  collections of `kind` match `list`, compared by exact href:
  - a listed href with no row is inserted, enabled, not default, with
    `position` its index in `list`;
  - a listed href with a row gets the list's name, description, color,
    components (joined by commas), read-only flag and position, and keeps
    `enabled`, `is_default`, `sync_token`, `ctag` and `synced_at`;
  - rows whose href is not listed are deleted (their objects and indexes
    cascade);
  - then, when no row of the account and kind has `is_default`, the first
    by position that is not read-only gets it;
  - it emits one `api.AccountChanged{ID: accountID}` when any row was
    inserted, deleted or changed (any column above, or the default), and
    nothing otherwise;
  - it returns `Collections(ctx, {AccountID, Kind})` as of the end.
- **`SetCollectionSync(ctx, id, token, ctag)`**: sets `sync_token`, `ctag`
  and `synced_at = FormatTime(t.Now())`; `ErrNotFound` for a missing row;
  no event.
- **`UpdateCollection(ctx, id, enabled, makeDefault)`**: sets `enabled`
  when non-nil; when `makeDefault`, sets `is_default` and clears it on the
  account's other collections of the same kind. Emits
  `api.AccountChanged{ID: accountID}` and returns the row;
  `ErrNotFound` for a missing row.

### Store: objects (`objects.go`)

- **`ObjectETags(ctx, collectionID)`**: href → ETag of every object.
- **`GetObject(ctx, id)`**, **`ObjectByHref(ctx, collectionID, href)`**:
  one row, or `ErrNotFound`.
- **`PendingHrefs(ctx, collectionID)`**: the hrefs of the collection's
  `pim_ops` rows in state `queued` or `running`, as a set.
- **`PutObject(ctx, o)`**: inserts the object, or replaces the one at the
  same collection and href (which keeps its ID), and returns the ID.
  `updated_at` is `t.Now()`; `o.ID` and `o.UpdatedAt` are ignored.
- **`DeleteObjects(ctx, collectionID, hrefs)`**: deletes those objects and
  returns how many existed. Unknown hrefs are not an error.
- None of the object or contact functions emits events: the pass does.

### Store: contacts (`contacts.go`)

- **`IndexContact(ctx, objectID, c)`**: replaces the object's `contacts`
  row (keeping its `person_id` if it had one) and its `contact_emails`
  rows. Emails are trimmed and lowercased; empty ones are skipped;
  positions count from 0 over the emails kept. A nil `Photo` stores NULL.
- **`RemoveContact(ctx, objectID)`**: deletes the object's contact rows;
  none is not an error.
- **`Contact(ctx, objectID)`**: the index as stored (emails by position;
  `Photo` nil for NULL), or `ErrNotFound` when the object is not a
  contact.

### pimsync: the DAV pass (`dav.go`, `index.go`)

`(p *pass) syncDAV(ctx, c, kind, want)` brings the account's collections of
`kind` (`davx.AddressBooks` → `api.CollectionKindAddressbook`,
`davx.Calendars` → `api.CollectionKindCalendar`) in step with the server:

1. `c.Collections(ctx, kind)`, keeping those `want` accepts, then one
   transaction with `ReplaceCollections` (each `davx.Collection` becomes a
   `RemoteCollection` with its href, name, description, color,
   components and read-only flag).
2. For each stored collection that is enabled, in order: skip it when the
   server's `CTag` is not empty, equals the stored `CTag`, and the
   collection has been synced (`SyncedAt` set). Otherwise find what
   changed:
   - when the server's collection has `Sync`: `c.Sync` from the stored
     token, repeated from each answer's `Token` while `More` is set. When
     the server answers `davx.ErrInvalidToken` to a non-empty token, start
     again from "". A sync that started from "" is a full listing over all
     its requests;
   - otherwise `c.List`, which is a full listing.
   - For a full listing, the deleted hrefs are the stored hrefs it does
     not name.
3. Leave alone every href in `PendingHrefs`: neither fetched nor deleted.
   Fetch only changed hrefs whose ETag is empty or differs from the stored
   one, with `c.Multiget` in batches of `p.m.cfg.Batch`. Each batch is one
   transaction: `PutObject` each object returned, then index it (below);
   the hrefs the server reports missing are deleted in that transaction.
4. The deleted hrefs go in one transaction with `DeleteObjects`.
5. Last, one transaction with `SetCollectionSync(id, token, server CTag)`
   (token "" for a server without `Sync`), so a pass cut short starts again
   from the old token.
6. Every transaction above that stores or deletes objects emits one event:
   `api.PeopleChanged{AccountID}` for address books,
   `api.CalendarChanged{AccountID}` for calendars.
7. A failure in one collection does not stop the others: syncDAV goes on
   and returns the first error at the end. `davx.ErrUnauthorized` returns
   at once. Wrap errors with `%w`.

Indexing, in `index.go`, in the transaction that stores the object:

- **Address books**: kind `store.ObjectVCard`. `vcardx.Parse`; on an error
  store the object with `ParseError` set to the error's text and
  `RemoveContact`. A card whose `Kind` is `group` is not a contact:
  `RemoveContact`. Otherwise `IndexContact` with `DisplayName()`,
  `SortKey()`, `GivenName`, `FamilyName`, `Organization`, the emails (value
  and label) and the inline photo (`Photo.Data` and `Photo.Type`, only when
  `Data` is not empty). `UID` is the card's UID.
- **Calendars**: `contentline.Parse`; on an error, kind `store.ObjectOther`
  and `ParseError` set. Otherwise the kind is `store.ObjectVEvent` when the
  `VCALENDAR` has a `VEVENT`, else `store.ObjectVTodo` when it has a
  `VTODO`, else `store.ObjectOther`; `UID` is the first such component's
  `UID` (trimmed). No index rows yet.

## Tests (given, do not edit)

- `internal/store/collections_test.go`: TestReplaceCollections,
  TestUpdateCollection, TestObjects, TestContacts.
- `internal/pimsync/dav_test.go`: TestFirstSync, TestIncrementalSync,
  TestInvalidToken, TestWithoutSyncCollection, TestTruncatedSync,
  TestCollectionChanges, TestPendingLeftAlone, TestCalendars,
  TestRefusedPassword.

## Gotchas

- SQLite's `INSERT … ON CONFLICT … DO UPDATE … RETURNING id` returns the
  existing row's ID on an update; `LastInsertId` does not.
- `ReplaceCollections`' "changed" compares stored and listed values; a
  refresh that changes nothing must emit nothing (a quiet pass every five
  minutes would otherwise wake every client).
- The given tests compare hrefs exactly as the server wrote them; do not
  unescape or rewrite them.
- `davtest` answers sync-collection after `InvalidateTokens` with 403 and
  `valid-sync-token`, which `davx` turns into `ErrInvalidToken`.
- Keep functions under 60 lines: split the pass into listing, fetching and
  deleting helpers.

## Out of scope

People (`person_id`, the `people` table: T-0062), writing to the server and
`pim_ops` replay (planner), events, attendees, instances and tasks
(Phases 3 and 4), Google Tasks, and every file not under `touch`.

## Done when

`make accept T=0061` and `make check` pass, and only the files under
`touch` changed.
