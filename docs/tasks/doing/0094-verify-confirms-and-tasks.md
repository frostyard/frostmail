---
id: "0094"
title: Verify confirms what is missing, and checks task lists
milestone: M4.5
size: M
touch:
  - internal/pimsync/verify.go
given:
  - internal/pimsync/verify_test.go
  - internal/pimsync/verify_more_test.go
acceptance: go test ./internal/pimsync -count=1
---
# T-0094: Verify confirms what is missing, and checks task lists

## Goal

`account.verify` compares each synced collection with the server. On the
user's Gmail account it reported two events missing locally, though
Google's own sync had reported both as deleted: Google's PROPFIND listing
keeps some objects that its sync-collection calls deleted and a multiget
cannot fetch. Verify also skips the tasks service entirely, so "verify is
clean for every service" (plan 0007, Phase 5) cannot be met. Fix both.

## Read first

- `internal/pimsync/verify.go` (all of it) and `verify_test.go`.
- `internal/pimsync/pimsync.go`: `serviceOnce` (how a pass picks Google
  Tasks or CalDAV for the tasks service), `pass.client`, `Config.Batch`.
- `internal/pimsync/dav.go`: `fetchDAV` (how a pass multigets in batches).
- `internal/pimsync/gtasks.go`: `syncGoogleTaskList`, `indexGoogleTask`
  (a Google task is stored with its ID as href and the `etag` from its
  JSON); `gtasks_ops.go`: `googleTasksClient`.
- `internal/gtasks/gtasks.go`: `Client.Tasks` (a zero `updatedMin` lists
  every task, deleted ones included).
- `internal/davx/dav.go`: `List`, `Multiget` (hrefs it cannot fetch come
  back in `missing`).
- `internal/davtest`: `Phantom` (an href listed by PROPFIND, unknown to
  everything else).
- The given tests; `docs/tasks/EXECUTOR.md`.

## Contract

`Manager.Verify(ctx, accountID)` keeps its signature and `Clean` is
unchanged. It still changes nothing on either side.

- **Confirmed absence:** for a DAV collection, every listed href with no
  local object (all of them, not only the first `maxListed`) is asked for
  with `Multiget`, in batches of `cfg.Batch`, with the collection's davx
  kind. Hrefs the multiget returns as missing are phantoms: they count in
  neither `Server` nor `MissingLocally`. An href the server does return
  stays missing locally (a pass has not fetched it yet).
- **CalDAV task lists:** an enabled tasks service with a home set that is
  not Google Tasks checks its enabled `tasklist` collections exactly as
  calendars are checked (`davx.Calendars`).
- **Google Tasks:** for an account whose provider's tasks are Google's
  (`providers.ForKind(kind).DAV.Tasks == "google"`) and whose tasks service
  is on, each enabled `tasklist` collection is checked against
  `Client.Tasks(ctx, col.Href, time.Time{})`: the server side is every task
  not deleted (completed and hidden ones included), keyed by ID, with the
  `etag` of its JSON (`encoding/json/v2`); the local side is the
  collection's `ObjectETags`. The comparison is the DAV one: missing
  locally (at most `maxListed`, in the server's order), missing on the
  server (sorted, at most `maxListed`, leaving out pending changes), and
  ETag differences when both ETags are known and no change is pending.
  Before the first pass there are no task lists and so no checks.
- One comparison helper serves both kinds; no change outside
  `verify.go`.

## Tests (given, do not edit)

`internal/pimsync/verify_test.go` (now allows the multiget REPORT) and
`internal/pimsync/verify_more_test.go`.

## Gotchas

- `missing` from `Multiget` holds the hrefs exactly as you passed them.
- Keep the existing order of checks: services in their stored order,
  collections in theirs.
- A Google Tasks call can fail with `gtasks.ErrUnauthorized`; return
  errors as the DAV path does.

## Out of scope

Repairing what verify finds, the RPC schema, the engine, `davtest`, and
every file not under `touch`.

## Done when

`make accept T=0094` and `make check` pass (taskrun runs them), and only
the files under `touch` changed.
