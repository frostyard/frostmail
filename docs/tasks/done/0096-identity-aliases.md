---
id: "0096"
title: Add and remove an account's addresses
milestone: M4.5
size: S
touch:
  - internal/engine/compose.go
given:
  - internal/engine/identity_alias_test.go
acceptance: go test ./internal/engine -count=1
---
# T-0096: Add and remove an account's addresses

## Goal

The user's iCloud account receives mail for a custom domain
(mail@bjk.fyi), but Frostmail knows only brianketelsen@icloud.com, so an
invitation to the custom address cannot be answered: Frostmail does not
find the user among its attendees. `identity.create` adds such an address
to an account (an identity that is not its default), and `identity.delete`
removes one again. Invitations already use an account's identities as the
user's addresses, so nothing else changes for them.

## Read first

- `schema/rpc/identity.yaml`: `create` and `delete`.
- `internal/engine/compose.go`: `identities.List`, `identities.Update`
  (validation and error style), the stubs `Create` and `Delete`.
- `internal/store/identities.go`: `Tx.AddIdentity`, `Tx.DeleteIdentity`
  (they emit `account.changed`, check duplicates and the default, and
  move drafts), `DB.DefaultIdentity`.
- `internal/engine/engine.go`: `apiError`.
- The given test; `docs/tasks/EXECUTOR.md`.

## Contract

- **`Create`:** the email trimmed must be a valid address
  (`compose.ValidAddress`), else `invalidParams`; a name with a line
  break is `invalidParams`. Without `name`, the name is the account's
  default identity's. In one transaction, `tx.AddIdentity`. An unknown
  account is `notFound` ("account N does not exist"); an address the
  account already has is `conflict` ("ADDRESS is already an address of
  account N"). Returns the identity (`toAPIIdentity`).
- **`Delete`:** `tx.DeleteIdentity` in one transaction; an unknown
  identity is `notFound`, the account's default identity is `conflict`
  ("identity N is its account's own address").
- Replace the stubs' doc comments with ones describing the methods.

## Tests (given, do not edit)

`internal/engine/identity_alias_test.go`.

## Out of scope

The store, the schema, the app, and every file not under `touch`.

## Done when

`make accept T=0096` and `make check` pass (taskrun runs them), and only
the files under `touch` changed.
