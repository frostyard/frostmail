---
id: "0050"
title: Read RFC 6186 SRV records
milestone: M4
size: S
touch:
  - internal/discover/srv.go
given:
  - internal/discover/srv_test.go
acceptance: go test ./internal/discover -run SRV -count=1
---
# T-0050: Read RFC 6186 SRV records

## Goal

Domains without an autoconfig document may still publish SRV records
naming their mail servers (RFC 6186, with implicit-TLS submission from RFC
8314). Implement the lookup that turns them into IMAP and SMTP settings
(`docs/design/accounts.md`, Discovery).

## Read first

- `internal/discover/discover.go`: `Server`, `Settings`, `ErrNothing`,
  `Resolver`.
- `internal/discover/srv.go`: the stub.
- The given test `internal/discover/srv_test.go`.
- `docs/tasks/EXECUTOR.md`

## Contract

Replace the stub's doc comment and its "Task T-0050 …" sentence with what
the function does.

`FromSRV(ctx, r, email)`:

- An address without an `@`, or with nothing before or after it, is an
  error (not `ErrNothing`). The domain (after the last `@`) is lowercased.
- **IMAP:** look up `r.LookupSRV(ctx, "imaps", "tcp", domain)`; when it
  gives a usable record, IMAP is that server with `api.TLSModeTLS`;
  otherwise look up `"imap"` and use `api.TLSModeStartTLS`.
- **SMTP:** the same with `"submissions"` (TLS), then `"submission"`
  (STARTTLS).
- **Usable record:** target not empty and not `"."` (RFC 6186: the service
  is not offered) and port not 0. Among usable records take the lowest
  `Priority`, then the highest `Weight`, then the first. The host is the
  target without its trailing `.`; the port is the record's; the user name
  is the address as given.
- **Errors:** a lookup failing with a `*net.DNSError` whose `IsNotFound` is
  true just means no records; any other error is returned at once.
- Return the `Settings` with nil for a server not found, or `ErrNothing`
  when both are missing.

## Tests (given, do not edit)

`internal/discover/srv_test.go`: TestFromSRVPrefersImplicitTLS,
TestFromSRVFallsBackAndPicksByPriorityAndWeight, TestFromSRVNothing,
TestFromSRVReportsDNSFailures.

## Gotchas

- Use `errors.As` to find the `*net.DNSError`.
- Query `imaps` before `imap`: the test checks the first query.

## Out of scope

Autoconfig (T-0049), calling DNS for real (planner), and every file except
`internal/discover/srv.go`.

## Done when

`make accept T=0050` and `make check` pass, and only
`internal/discover/srv.go` changed.
