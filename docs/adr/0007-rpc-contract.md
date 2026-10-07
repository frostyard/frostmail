# 0007 — A YAML IDL generates the RPC contract

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

The API between maild and its clients is used from Go (maild, mailctl) and
TypeScript (the app). Most of its implementation will be written by a local
model, so a mismatch must fail at compile time, and adding a method must be a
small, obvious edit.

## Decision

- The contract is YAML in `schema/rpc/*.yaml`: one file per domain, with
  enums, types, methods (params, result, errors) and events (durable or
  transient). Application errors live in `errors.yaml`.
- `tools/rpcgen` validates the schema strictly (unknown keys, names,
  references, collisions) and generates:
  - `api/zz_generated.go`: types, one `XxxService` interface per domain,
    router registration, a typed client implementing the same interfaces, and
    event decoding;
  - `app/src/rpc/gen/api.ts`: types and a typed `Client`;
  - `docs/specs/rpc-api.md`.
  `make gen-check` (part of `make verify`) fails when they are stale.
- The wire format is JSON-RPC 2.0, one JSON message per line, on a 0600 Unix
  socket with a peer-UID check ([specs/rpc-protocol.md](../specs/rpc-protocol.md)).
- In the app, JSON-RPC multiplexing runs in TypeScript; the Rust bridge only
  moves lines, so it never changes with the API.

## Consequences

- A domain without an implementation fails `api.NewRouter`; a method with the
  wrong signature fails to compile; a TS call with wrong params fails
  `pnpm typecheck`.
- `encoding/json/v2` encodes nil slices as `[]`, so required arrays never
  arrive as `null`.
- The generator is about 900 lines owned by the planner, not by task cards.

## Alternatives considered

- **Protobuf/Connect:** the buf toolchain, and binary-first framing that is
  awkward to debug by hand.
- **JSON Schema plus quicktype:** verbose, and does not generate method stubs.
- **TypeSpec:** another Node toolchain on the host.

## References

- Shapes: [specs/rpc-protocol.md](../specs/rpc-protocol.md), [specs/rpc-api.md](../specs/rpc-api.md)
- Tests: `tools/rpcgen/rpcgen_test.go`, `internal/rpcserver/server_test.go`, `app/src/rpc/transport.test.ts`
