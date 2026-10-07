---
id: "0002"
title: Store accounts in SQLite
milestone: M0
size: S
touch:
  - internal/store/account.go
given:
  - internal/store/account_test.go
  - internal/engine/accounts_test.go
acceptance: go test ./internal/store ./internal/engine -count=1
---
# T-0002: Store accounts in SQLite

## Goal

Implement the five account functions in `internal/store/account.go`, which
are stubs returning `errNotImplemented`. The engine (`internal/engine`)
already maps them to the `account.*` RPC methods, so this task makes
accounts work end to end: socket, engine, store.

## Read first

- `internal/store/account.go`: the types and the five stubs, with doc
  comments that state each function's contract.
- `internal/store/migrations/0001_init.sql`: the `accounts` table.
- `internal/store/store.go`: `Tx`, `Tx.Emit`, `Tx.Now`, `FormatTime`,
  `ParseTime`, `IsUniqueViolation`, `ErrNotFound`, `ErrConflict`.
- `internal/store/mailbox.go`: an example query that scans rows.
- `docs/tasks/EXECUTOR.md`

## Contract

Replace the stub bodies and delete `errNotImplemented`. Keep every signature
and doc comment.

- `InsertAccount`: ignore `a.ID` and `a.CreatedAt`. Store
  `CreatedAt = t.Now().UTC().Truncate(time.Millisecond)` as
  `FormatTime(...)`. Return the account with its new ID (`LastInsertId`) and
  that CreatedAt. A UNIQUE violation (duplicate email, compared without case
  by the schema) returns `ErrConflict`. On success emit
  `api.AccountChanged{ID: id}` with `t.Emit`.
- `GetAccount`: one row by ID; no row returns `ErrNotFound`. CreatedAt comes
  back through `ParseTime`, so it is UTC.
- `ListAccounts`: every account ordered by `id`. No accounts returns
  `[]Account{}`, not nil.
- `UpdateAccount`: read the row inside the transaction (`t.QueryRowContext`),
  apply each non-nil field of `AccountUpdate`, write it back, emit
  `api.AccountChanged{ID: id}` and return the updated account. A missing ID
  returns `ErrNotFound`.
- `DeleteAccount`: delete by ID. Zero rows affected returns `ErrNotFound`.
  Otherwise emit `api.AccountChanged{ID: id, Deleted: true}`. Mailboxes and
  messages are removed by `ON DELETE CASCADE`; do not delete them yourself.

## Tests (given, do not edit)

- `internal/store/account_test.go`: TestAccountInsertAndGet,
  TestAccountCreatedAtIsUTCMillis, TestAccountDuplicateEmailConflicts,
  TestAccountGetMissing, TestAccountList, TestAccountUpdate, TestAccountDelete.
- `internal/engine/accounts_test.go`: TestAccountAPI, which drives the same
  code over the RPC socket.

## Gotchas

- A shared scan helper avoids writing the 14-column scan three times:
  `func scanAccount(row interface{ Scan(...any) error }) (Account, error)`
  works for both `*sql.Row` and `*sql.Rows`. Map `sql.ErrNoRows` to
  `ErrNotFound` there.
- `database/sql` scans TEXT straight into `api.AccountKind`, `api.AuthKind`
  and `api.TLSMode` because they are string types.
- Inside a transaction, query with the `Tx` (`t.QueryRowContext`,
  `t.ExecContext`), never with `d.db`.
- Return `Emit`'s error: if it fails, the transaction must roll back.

## Out of scope

Secrets and passwords (M1), identities, mailboxes, and any file but
`internal/store/account.go`.

## Done when

`make accept T=0002` and `make check` pass, and only
`internal/store/account.go` changed.
