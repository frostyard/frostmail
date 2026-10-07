---
id: "0010"
title: Diff two view snapshots into insert and remove ops
milestone: M1
size: S
touch:
  - internal/view/diff.go
given:
  - internal/view/diff_test.go
acceptance: go test ./internal/view -count=1
---
# T-0010: Diff two view snapshots into insert and remove ops

## Goal

A view is an ordered list of message IDs. When it changes, maild sends the
client a `view.delta` of insert and remove ops rather than the whole list
(see the `view` domain in `docs/specs/rpc-api.md`). Write `Diff`, which
turns an old list into ops that produce the new one, staying fast for
50,000 rows.

## Read first

- `docs/specs/rpc-api.md`, section "view": `ViewOp` and `ViewOpKind`.
- The `ViewOp` type and `ViewOpKindInsert` / `ViewOpKindRemove` constants in
  `api/zz_generated.go`.
- The given test `internal/view/diff_test.go`, especially `apply`, which
  defines what a correct delta is.
- `docs/tasks/EXECUTOR.md`

## Contract

Create `internal/view/diff.go` beginning with:

```go
// Package view keeps live, ordered message lists for clients
// (the view domain in docs/specs/rpc-api.md).
package view
```

and define `func Diff(old, new []int64) []api.ViewOp` with a doc comment.
IDs are unique within each list. Return nil when nothing changed.

Algorithm (O(n log n)):

1. Map each ID of `new` to its index.
2. Walk `old`; for IDs also in `new`, collect their indexes in `new`. Find a
   longest strictly increasing subsequence of those indexes (patience
   sorting with binary search, keeping predecessor links). Those IDs are
   **kept**; every other old ID is **removed**; every new ID that is not
   kept is **inserted**.
3. **Removes first**, from the end of `old` toward the start: each run of
   consecutive removed rows becomes one op `{remove, at: run start in old,
   count: run length}`. Going backwards keeps earlier indexes valid.
4. **Then inserts**, from the start of `new` toward the end: each run of
   consecutive inserted rows becomes one op `{insert, at: run start in new,
   count: run length}`.

## Tests (given, do not edit)

`internal/view/diff_test.go`: exact ops for ten simple cases, op-count limits
for moves, 200 random rounds, and 50,000 rows in under 200 ms.

## Gotchas

- `api.ViewOp` fields are `int64`; convert indexes.
- Do not use an O(n²) longest-common-subsequence table: 50,000 rows would
  need 2.5 billion cells.

## Out of scope

Snapshots and delivery (the planner's view manager), and every file except
`internal/view/diff.go`.

## Done when

`make accept T=0010` and `make check` pass, and only `internal/view/diff.go`
changed.
