---
id: "0053"
title: Sign in from mailctl
milestone: M4
size: M
touch:
  - cmd/mailctl/signin.go
given:
  - cmd/mailctl/signin_test.go
acceptance: go test ./cmd/mailctl -run 'OAuth|Connect|Authorize' -count=1
---
# T-0053: Sign in from mailctl

## Goal

Until the settings window exists, accounts are added from the command
line: store the user's Google OAuth client, add an account from just its
address (maild finds the servers), and sign an OAuth account in through
the browser (`docs/design/accounts.md`). Implement the three commands.

## Read first

- `cmd/mailctl/signin.go`: the three stub constructors, already registered
  (`oauth` in `root.go`; `account connect` and `account authorize` in
  `account.go`).
- `cmd/mailctl/account.go`: `readPasswordLine`, how `account add` dials and
  creates; `cmd/mailctl/send.go`: waiting for events (`waitForSend`).
- `api/zz_generated.go`: `OauthService` (`SetClient`, `GetClient`),
  `AccountService` (`Discover`, `Create`, `SetPassword`, `Authorize`, `Get`),
  `AccountChanged`.
- The given test `cmd/mailctl/signin_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Keep the constructor names; replace the file comment's "Task T-0053 …"
sentence and the stub doc comments with what the commands do. Remove
`errSignInNotYet`.

**`mailctl oauth`** (a parent command) with two subcommands:

- `oauth set-client PROVIDER --client-id ID [--secret-stdin]`: calls
  `oauth.setClient` with the provider as given (maild validates it); with
  `--secret-stdin`, the secret is the first line of stdin
  (`readPasswordLine`). Prints `<provider> client set with its secret` or
  `<provider> client set`.
- `oauth show PROVIDER`: prints `<provider> client <id> (secret stored)` or
  `(no secret)`; maild's notFound error is returned as is.

**`mailctl account connect EMAIL`** with `--name`, `--oauth`,
`--password-stdin`, `--read-only`, `--no-notify`, `--no-wait`, `--timeout`
(default 5m):

1. Exactly one of `--oauth` and `--password-stdin`, else the error
   `sign in with either --password-stdin or --oauth` (before dialing).
2. Dial; `account.discover`. When either server is missing, return
   `no servers found for <domain>; use mailctl account add with --imap and
   --smtp` (domain = after the last `@`) without creating anything.
3. Unless this is `--oauth --no-wait`, subscribe to events
   (`c.Events().Subscribe(ctx, nil)`) now, before anything changes.
4. With `--password-stdin`, read the password line.
5. `account.create` with the discovered kind and servers, the name, auth
   `oauth2` or `password`, `readOnly` = `--read-only`, `notify` = not
   `--no-notify`. Print `added account <id> (<email>, <kind>)`.
6. Password: `account.setPassword`, then print `password stored`.
   OAuth: sign in as below.

**`mailctl account authorize ID`** with `--no-wait` and `--timeout`
(default 5m): a non-numeric ID is the error `invalid account id "<arg>"`
(before dialing); dial; subscribe unless `--no-wait`; sign in.

**Signing in** (shared helper): `account.authorize`; print `open this
address in your browser to sign in:` and the URL on its own line. With
`--no-wait`, return. Otherwise read `c.Notifications()` until an
`AccountChanged` for the account, after which `account.get` reports
`signedIn`; print `signed in`. When `--timeout` passes first: `not signed
in after <timeout>`.

## Tests (given, do not edit)

`cmd/mailctl/signin_test.go`: TestOAuthClientCommands,
TestConnectGmailWithOAuth, TestConnectICloudWithPassword,
TestConnectChecksItsArguments, TestAuthorizeWithoutWaiting.

## Gotchas

- Subscribe before `account.create`/`account.authorize`, or the sign-in's
  `account.changed` can arrive before you listen.
- Creating an account also emits `account.changed`; check `signedIn` with
  `account.get` instead of trusting the first event.
- The test's "browser" follows the URL as soon as it is printed with its
  newline: print it with `fmt.Fprintln`.

## Out of scope

The settings window, `mailctl verify`, and every file except
`cmd/mailctl/signin.go`.

## Done when

`make accept T=0053` and `make check` pass, and only
`cmd/mailctl/signin.go` changed.
