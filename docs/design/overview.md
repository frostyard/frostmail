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
  socket server, and stops cleanly on SIGINT or SIGTERM. Platform code
  sits in per-OS files behind the same seams (ADR-0038): the socket
  (`rpcserver`, `api.Dial`), paths (`config`), secrets (`secrets.Open`),
  notifications (`notify`) and the local zone (`calendar.LocalZone`).
- **api** is the contract ([ADR-0007](../adr/0007-rpc-contract.md)): generated
  types, services, router and client over the JSON-RPC runtime of lodge's
  kit (`github.com/frostyard/lodge/kit/jsonrpc`), which frostnotes shares.
- **rpcserver** serves `rpc.hello`, events and each connection's caller on
  the kit's server, which frames messages, rejects calls before
  `rpc.hello`, runs requests concurrently (up to 32 per connection), answers
  `internal` to a panic or an oversized reply, and delivers events after the
  `events.subscribe` reply. The private socket checks the peer: the kit's
  `jsonrpc.Listen` on Linux, and on Windows maild's own named pipe.
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
  connection a `view.delta` computed by `view.Diff`. A view filtered by
  unread keeps rows that are read while it is open (`ViewFilter.Keep`), so
  a message read in an Unread list stays until the view is reopened.
  Request handlers reach their connection through `api.ConnFrom(ctx)`.
- **The app** keeps all mail logic out of Rust: `bridge.rs` moves lines and
  `transport.ts` matches responses to calls. The `mailpart://` scheme serves
  decoded parts from maild's cache so large data never travels as JSON
  ([ADR-0005](../adr/0005-html-mail-rendering.md)). The UI lists messages
  only through views and renders mail that maild has sanitized
  ([design/app.md](app.md), [design/rendering.md](rendering.md)).

## Behind lodge

On a server, maild is lodge's `mail` app
([ADR-0041](../adr/0041-maild-runs-as-a-lodge-app.md),
[specs/lodge-app.md](../specs/lodge-app.md)). lodge is the only process
on the network; maild listens on its app socket, which only lodge's user
may open, and runs as the system user `frostmail`.

```
 browser · phone · desktop app ──https──▶ lodge ──Unix socket──▶ maild --lodge (user frostmail)
   /app/mail/        the web build, served by lodge from disk     ├─ /health
   /app/mail/api     WebSocket, caller session ─────────────────▶ ├─ /api   JSON-RPC, as above
   lodge-content     /mail/r/…, /mail/p/…, sandboxed ────────────▶ ├─ /content/  reader pages, parts
   /mcp/mail         agents, with lodge's tokens (plan 0011) ────▶ └─ /mcp
                                         mailctl ── private socket ─┘ (caller local)
```

- **Callers.** The private socket is caller `local`, as on the desktop.
  lodge's `/api` is caller `session`, and the IDL says which methods each
  admits: a session never names a file on maild's machine.
- **The reader's page** comes from lodge's content node, not `srcdoc`
  ([design/rendering.md](rendering.md#behind-lodge)).
- **One maild for every client.** The desktop app reaches the server's
  maild through lodge too, and a local maild stays for installs outside the
  owner's tailnet ([plan 0011](../plans/0011-m8-mail-in-lodge.md)).

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
| `FROSTMAIL_DATA_DIR` | `$XDG_DATA_HOME/frostmail` (`~/.local/share/frostmail`); Windows: `%LOCALAPPDATA%\Frostmail\Data` | maild: `frostmail.db`, `blobs/`, on Windows `logs\` and `secrets.dpapi` |
| `FROSTMAIL_CACHE_DIR` | `$XDG_CACHE_HOME/frostmail`; Windows: `%LOCALAPPDATA%\Frostmail\Cache` | maild writes `parts/`; the app serves it as `mailpart://` (`http://mailpart.localhost` on WebView2) |
| `FROSTMAIL_SOCKET` | `$XDG_RUNTIME_DIR/frostmail/maild.sock`; Windows: `\\.\pipe\frostmail-<user SID>` | maild, mailctl, the app |
| `FROSTMAIL_SECRETS=file` | unset | maild: keep secrets in a plain file, not the Secret Service or DPAPI (development, CI), with a warning |
| `FROSTMAIL_SECRETS=sealed` | unset | maild on a server: seal the secrets in `secrets.sealed` with the key of the systemd credential `frostmail-secrets-key`, never a fallback (Linux only; [ADR-0044](../adr/0044-seal-the-servers-secrets-with-a-systemd-credential.md)) |
| `FROSTMAIL_NOTIFIER` | `frostmail-notify.exe` beside maild | maild on Windows: the toast notifier ([desktop.md](desktop.md#windows)) |
| `FROSTMAIL_APP` | `frostmail` on `PATH`; Windows: `frostmail.exe` beside the notifier | maild on Linux, the notifier on Windows: the app a notification's click runs |
| `FROSTMAIL_WEBKIT_SAFE=1` | unset | the app: WebKitGTK CPU-rendering workarounds |
| `FROSTMAIL_DEVTOOLS_PORT` | unset | the app on Windows: open WebView2's DevTools on this loopback port (the end-to-end tests) |
| `FROSTMAIL_OPEN_LOG` | unset | the app: append what it would open to this file instead (the end-to-end tests on Windows) |
| `FROSTMAIL_INSECURE_TLS=1` | unset | maild: accept any IMAP certificate (test servers only) |
| `FROSTMAIL_IT_HOST` | set by `make engine-it` | integration tests |
| `FROSTMAIL_EXECUTOR_MODEL` | `EXECUTOR_MODEL` in the Makefile | `taskrun` |

`internal/config` and `app/src-tauri/src/paths.rs` resolve the same paths
and must agree (the app's Windows paths come in plan 0010's Phase 5). The
socket directory must be private (0700, owned by the user); the socket is
0600. On Windows the named pipe's DACL grants only the user
([rpc-protocol.md](../specs/rpc-protocol.md#transport)).

## Status

M0 (foundations) is done; see
[plans/0001](../plans/0001-m0-foundations.md). The roadmap is
[plans/0002](../plans/0002-roadmap.md).
