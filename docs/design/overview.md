# Design: overview

## Purpose

Frostmail is a Linux mail client with the feature set and feel of Apple
Mail.app and no GNOME or KDE look. This document is the entry point to the
architecture; the reasons are in the [ADRs](../README.md#decisions-adrs).

## Architecture

```
 Tauri app (WebKitGTK)            mailctl (Go CLI)
 React UI ── Client (gen/api.ts)  Client (api)
     │ lines over Tauri IPC            │
 Rust bridge (bridge.rs) ──────┐       │
 mailpart:// (protocol.rs)     │       │
     ▲ files                   ▼       ▼
     │               $XDG_RUNTIME_DIR/frostmail/maild.sock
     │                 JSON-RPC 2.0, one message per line
     │                               │
     │   maild ─ rpcserver ─ api.Router ─ engine ─ store (SQLite, WAL)
     │             │ api.Conn           │  │       ├─ changes log ─▶ events.Broker
     │             └── view.delta ◀─ view.Manager ◀─┘ (OnCommit)
     └── parts cache                    └─ mailsync ─ imapx ─ go-imap fork ─ IMAP
                                           │  └─ blob store (raw messages)
                                           └─ smtpx (M3) ─ SMTP
```

- **maild** (`cmd/maild`) wires the store, the broker, the engine and the
  socket server, and stops cleanly on SIGINT or SIGTERM.
- **api** is the contract ([ADR-0007](../adr/0007-rpc-contract.md)): generated
  types, services, router and client over a hand-written runtime.
- **rpcserver** frames messages, checks the peer UID, rejects calls before
  `rpc.hello`, runs requests concurrently (up to 32 per connection) and
  delivers events after the `events.subscribe` reply.
- **engine** implements the domains over the store and validates input.
- **store** is SQLite with a single writer; durable events are written in
  the same transaction as the change ([storage.md](storage.md)).
- **events.Broker** fans committed events out, replays the log for resuming
  subscribers, and drops subscribers more than 1024 events behind (they
  reconnect and resume).
- **mailsync** runs one actor per account: the reconcile pass, an IDLE
  connection on INBOX, STATUS polling, body fetches into the blob store, and
  replay of offline actions ([sync.md](sync.md)). The engine reaches it
  through the `engine.Syncer` interface.
- **view.Manager** keeps each open view's ordered ID snapshot, recomputes it
  (debounced 50 ms) after commits that touch its account, and sends the owning
  connection a `view.delta` computed by `view.Diff`. Request handlers reach
  their connection through `api.ConnFrom(ctx)`.
- **The app** keeps all mail logic out of Rust: `bridge.rs` moves lines and
  `transport.ts` matches responses to calls. The `mailpart://` scheme serves
  decoded parts from maild's cache so large data never travels as JSON
  ([ADR-0005](../adr/0005-html-mail-rendering.md)).

## Key patterns

- **Contract first.** Change `schema/rpc/*.yaml`, run `make gen`, implement
  the new interface method, and the Go and TS clients follow.
- **Write, emit, commit.** Every write is a `store.Tx`; a durable event is
  emitted in the same transaction and delivered after commit, in order.
- **Optimistic local state:** `message.setFlags`, `move` and `delete` change
  the store and queue a `pending_ops` row in one transaction; the account's
  actor replays them before its next pass ([sync.md](sync.md)).
- **Views, not pages** (M2): the UI opens a view and asks for row ranges;
  maild keeps the ordered ID snapshot and sends deltas, so the virtualized
  list never pages through SQL.
- **One IMAP door.** Only `internal/imapx` imports go-imap.

## Configuration

| Variable | Default | Used by |
| --- | --- | --- |
| `FROSTMAIL_DATA_DIR` | `$XDG_DATA_HOME/frostmail` (`~/.local/share/frostmail`) | maild: `frostmail.db`, `blobs/` |
| `FROSTMAIL_CACHE_DIR` | `$XDG_CACHE_HOME/frostmail` | maild writes `parts/`; the app serves it as `mailpart://` |
| `FROSTMAIL_SOCKET` | `$XDG_RUNTIME_DIR/frostmail/maild.sock` | maild, mailctl, the app |
| `FROSTMAIL_WEBKIT_SAFE=1` | unset | the app: WebKitGTK CPU-rendering workarounds |
| `FROSTMAIL_INSECURE_TLS=1` | unset | maild: accept any IMAP certificate (test servers only) |
| `FROSTMAIL_IT_HOST` | set by `make engine-it` | integration tests |
| `FROSTMAIL_EXECUTOR_MODEL` | `EXECUTOR_MODEL` in the Makefile | `taskrun` |

`internal/config/paths.go` and `app/src-tauri/src/paths.rs` resolve the same
paths and must agree. The socket directory must be private (0700, owned by
the user); the socket is 0600.

## Status

M0 (foundations) is done; see
[plans/0001](../plans/0001-m0-foundations.md). The roadmap is
[plans/0002](../plans/0002-roadmap.md).
