# 0003 — SQLite store with a single writer and a durable event log

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

maild caches mail locally for instant lists and offline use, indexes it for
full-text search, queues offline actions, and tells clients what changed,
including clients that reconnect after a gap. The engine is pure Go.

## Decision

- SQLite through `modernc.org/sqlite` (pure Go; FTS5 verified, including
  `remove_diacritics` and `contentless_delete`), in WAL mode, `STRICT` tables,
  `foreign_keys` on. Raw messages live in a content-addressed blob store
  beside the database.
- Every write runs in `store.DB.Tx`, which serializes writers. A change and
  the durable events describing it are written in the same transaction:
  `Tx.Emit` appends to the `changes` table (AUTOINCREMENT `seq`) and the
  events reach subscribers only after commit, in commit order.
- Migrations are embedded `NNNN_name.sql` files applied in order. A database
  newer than the binary is refused. Pre-release, `0001_init.sql` is edited in
  place; from the first daily-driver release (M4) migrations are
  append-only.

## Consequences

- Clients resume with `events.subscribe(sinceSeq)` and never miss or
  double-apply a durable event; a pruned gap forces a full refetch
  (`resync`).
- Optimistic UI updates and offline actions are atomic with their events.
- modernc is roughly 1.3–2× slower than C SQLite; access stays behind
  `database/sql`, so a driver swap remains possible.
- The database must be on a local filesystem (no virtiofs, NFS or `/mnt/host`).

## Alternatives considered

- **mattn/go-sqlite3 (cgo):** faster, but breaks `CGO_ENABLED=0` builds.
- **An embedded KV store plus a separate search index (bbolt + Tantivy/Bleve):**
  more moving parts, no SQL for smart mailboxes and rules.
- **Events without a log:** clients would refetch everything after every
  reconnect.

## References

- Shapes: [design/storage.md](../design/storage.md), [specs/rpc-protocol.md](../specs/rpc-protocol.md)
- Tests: `internal/store/store_test.go`, `internal/events/broker_test.go`
