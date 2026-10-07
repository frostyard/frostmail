# Spec: maild RPC protocol

The wire rules for every maild client. Methods, types and events are in the
generated [rpc-api.md](rpc-api.md). Rationale: [ADR-0007](../adr/0007-rpc-contract.md).

## Transport

- A Unix stream socket at `$FROSTMAIL_SOCKET`, default
  `$XDG_RUNTIME_DIR/frostmail/maild.sock`. maild creates the parent
  directory 0700 and refuses one that is not 0700 and owned by the user. The
  socket is 0600 and removed on shutdown. A stale socket is replaced; a live
  one makes a second maild exit.
- maild closes connections whose peer UID (`SO_PEERCRED`) is not its own.
- Each message is one line: a JSON object followed by `\n`, at most 16 MiB
  (`api.MaxMessageSize`). JSON escapes newlines in strings, so a message is
  never split.

## Messages

JSON-RPC 2.0, without batches:

- Request: `{"jsonrpc":"2.0","id":<number|string>,"method":"<domain>.<name>","params":{…}}`.
  `params` is an object or omitted; omitted and `null` mean `{}`.
- Response: `{"jsonrpc":"2.0","id":…,"result":…}` or
  `{"jsonrpc":"2.0","id":…,"error":{"code":…,"message":"…","data":…}}`. A
  method without a result returns `"result":null`. Responses may arrive in
  any order; match them by `id`.
- Server notification: `{"jsonrpc":"2.0","method":"event","params":<EventEnvelope>}`.
- Client notifications (requests without `id`) are ignored.

Params are decoded strictly: an unknown member or a missing required member
is `invalidParams` (-32602). Malformed JSON is `parseError` (-32700) with a
`null` id; a non-2.0 or method-less object is `invalidRequest` (-32600); an
unknown method is `methodNotFound` (-32601).

## Connection setup

1. The client sends `rpc.hello {protocol, client}`. Until hello succeeds,
   every other method returns `notReady` (1004). A different protocol major
   version returns `protocolMismatch` (1003). Requests are handled in order
   until hello succeeds, so a client may pipeline hello and its next call.
2. After hello, requests run concurrently, up to 32 per connection; reading
   pauses while 32 are in flight.

Within protocol 1, changes are additive: new methods, new optional params,
new result fields, new events, new enum values. Clients ignore unknown fields
and unknown events.

## Events

- `events.subscribe {sinceSeq?}` returns `{seq, resync}`. Events are sent
  only after that response.
- `EventEnvelope` is `{"seq":n,"event":"<domain>.<name>","data":{…}}`. Durable
  events have `seq` > 0 and are kept in maild's changes log; transient events
  omit `seq`.
- Without `sinceSeq` the client receives live events only. With `sinceSeq`,
  maild first replays the retained durable events with `seq > sinceSeq`, in
  order, then live events, delivering each `seq` once. If `sinceSeq` is older
  than the retained log, ahead of the latest `seq`, or negative, the result
  has `resync: true`, nothing is replayed, and the client must refetch its
  state.
- A connection may subscribe once (`conflict` (1002) after that).
- A subscriber more than 1024 events behind is disconnected; it reconnects
  and resumes with its last `seq`.

## Message parts (`mailpart://`)

In the app, `mailpart://localhost/<path>` serves
`$FROSTMAIL_CACHE_DIR/parts/<path>`. Paths contain only `[A-Za-z0-9._/-]`
and normal components; anything else is 400. A path that resolves outside
`parts/` (through a symlink) is 403. Responses carry `nosniff` and
`Content-Security-Policy: default-src 'none'; img-src data:; style-src
'unsafe-inline'; sandbox`. WebKitGTK loads the scheme only from
`tauri://localhost` pages, not from the `tauri dev` http origin.

## Clients

- Go: `api.Dial(ctx, socket, clientName)` connects and completes hello;
  `Client.<Domain>()` returns the typed methods; `Client.Notifications()`
  delivers events (the client fails with `ErrEventsOverflow` if 1024
  undelivered events pile up).
- TypeScript: `JsonRpcSession` over a line channel; `connectTauri()` uses the
  bridge's `maild_connect` (a `Channel` of `{kind:"line",line}` and a final
  `{kind:"closed",error}`) and `maild_send` (one line).
