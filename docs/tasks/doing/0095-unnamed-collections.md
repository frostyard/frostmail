---
id: "0095"
title: Name the collections a server leaves unnamed
milestone: M4.5
size: S
touch:
  - internal/pimsync/dav.go
  - internal/pimsync/gtasks.go
given:
  - internal/pimsync/names_test.go
acceptance: go test ./internal/pimsync -count=1
---
# T-0095: Name the collections a server leaves unnamed

## Goal

iCloud's address book has no display name, so People lists it as a blank
row. A collection whose name from the server is empty or only spaces is
stored under its kind's name: "Contacts" for an address book, "Calendar"
for a calendar, "Tasks" for a task list (CalDAV or Google). A name the
server gives later replaces it on the next pass.

## Read first

- `internal/pimsync/dav.go`: `syncDAV` builds each
  `store.RemoteCollection` from a `davx.Collection`.
- `internal/pimsync/gtasks.go`: `syncGoogleTasks` builds them from Google
  task lists.
- `internal/store/collections.go`: `ReplaceCollections` (stores the name
  it is given).
- The given test; `docs/tasks/EXECUTOR.md`.

## Contract

- One unexported helper in `dav.go`, `collectionName(name string, kind
  api.CollectionKind) string`: `name` unchanged when it has any
  non-space character, otherwise the kind's name above.
- `syncDAV` and `syncGoogleTasks` pass every collection's name through
  it. Nothing else changes.

## Tests (given, do not edit)

`internal/pimsync/names_test.go`. Every other `internal/pimsync` test must
keep passing.

## Out of scope

The app (it shows whatever name is stored), the store, and every file not
under `touch`.

## Done when

`make accept T=0095` and `make check` pass (taskrun runs them), and only
the files under `touch` changed.
