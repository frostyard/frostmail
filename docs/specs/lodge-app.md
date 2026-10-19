# Spec: maild as a lodge app

What maild installs for lodge and serves on its lodge app socket, and what
the web build does inside lodge's frame. Its consumers:

- lodge, which proxies to maild;
- the web app, through `/app/mail/`;
- agents, through `/mcp/mail`;
- the desktop app, when it uses a maild behind lodge (Phase 6).

This spec is mail's side of lodge's
[app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md).
Sign-in, sessions, tokens, origins and the browser's headers are lodge's
([lodge-http](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md)),
and so are the frame's messages
([lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md)).
The RPC's wire rules are [rpc-protocol.md](rpc-protocol.md), and its
methods [rpc-api.md](rpc-api.md).

Rationale: [ADR-0041](../adr/0041-maild-runs-as-a-lodge-app.md),
[ADR-0005](../adr/0005-html-mail-rendering.md),
[ADR-0014](../adr/0014-images-embedded-in-the-reader-frame.md),
[ADR-0036](../adr/0036-reader-links-open-as-new-window-requests.md).
Where it fits: [design/overview.md](../design/overview.md#behind-lodge),
[design/rendering.md](../design/rendering.md).

**Status:** M8's contract, written in
[plan 0011](../plans/0011-m8-mail-in-lodge.md)'s Phase 1 and built in
Phases 2, 3 and 5 (merged 2026-10-19). `/mcp` is read-only for now: write
and send scopes come later in Phase 5.

## Interface

### What maild installs

| What | Where (lodge's contract) | Contents |
| --- | --- | --- |
| manifest | `/usr/share/lodge/apps/mail/app.json` | the JSON below; `version` is maild's |
| web build | `/usr/share/lodge/apps/mail/web/` | `make web-build`'s output, built for the base `/app/mail/`, with `icon.svg` |
| system user | `sysusers.d/frostmail.conf` | `frostmail`, in group `lodge-apps` and not in `lodge` |
| the app socket | `frostmail.socket` | `ListenStream=/run/lodge/apps/mail.sock`, `SocketUser=frostmail`, `SocketGroup=lodge`, `SocketMode=0660` |
| the daemon | `frostmail.service` | `maild --lodge` as `frostmail`, with the environment below |

```json
{ "contract": 1, "id": "mail", "name": "Mail", "icon": "icon.svg",
  "version": "0.1.0", "user": "frostmail", "key": "m",
  "content": true, "contentLinks": true, "mcp": true }
```

`frostmail.service` runs maild with:

| Variable | Value |
| --- | --- |
| `FROSTMAIL_DATA_DIR` | `/var/lib/frostmail` (`StateDirectory=frostmail`, 0700) |
| `FROSTMAIL_CACHE_DIR` | `/var/cache/frostmail` (`CacheDirectory=frostmail`, 0700) |
| `FROSTMAIL_SOCKET` | `/run/frostmail/maild.sock` (`RuntimeDirectory=frostmail`, 0700) |
| `FROSTMAIL_SECRETS` | `file`: `/var/lib/frostmail/secrets.json`, 0600 |

### Listeners

| Listener | Where | Who may connect | Caller |
| --- | --- | --- | --- |
| the app socket (`maild --lodge`) | `/run/lodge/apps/mail.sock`, passed by `frostmail.socket` or made by maild; under `LODGE_ROOT` in development | lodge's user only, by the socket peer's uid (the kit) | from `Lodge-Caller` |
| the private socket | `$FROSTMAIL_SOCKET` ([rpc-protocol](rpc-protocol.md#transport)) | maild's own user | `local` |
| `maild -devgw ADDR` | a loopback address only | the development UI, with its token | `local` |

- maild creates the app socket with the contract's owners and mode when
  none is passed, and closes a connection from any other uid before it
  reads from it.
- maild reads `Lodge-*` headers on the app socket only. Elsewhere they are
  ignored like any unknown header.
- Without `--lodge`, maild serves no app socket, and everything else in
  this spec but the IDL's callers is absent.
- lodge and its app sockets run on Linux. Elsewhere, such as a Windows
  install, `maild --lodge` refuses at start ("lodge runs on Linux"), and
  maild serves only its local socket.

### Requests on the app socket

HTTP/1.1. Every request but `/health` must carry `Lodge-Caller`. Each
path admits one caller kind, and any other is 403:

| Request | `Lodge-Caller` | Purpose |
| --- | --- | --- |
| `GET /health` | any, or none | maild's state for lodge's rail |
| `GET /api`, WebSocket | `session` | JSON-RPC as caller `session` |
| `GET`, `HEAD` `/content/r/<token>.html` | `content` | a reader page |
| `GET`, `HEAD` `/content/p/<mac>/<path>` | `content` | a decoded part |
| `POST`, `GET`, `DELETE` `/mcp` | `agent` | MCP Streamable HTTP |

- A path outside this table is 403, since it admits no caller, as is a
  caller kind its row does not admit
  ([lodge's app-contract.md](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md#what-an-app-serves-on-its-socket)).
  `/x/…` and `/device/…`, which maild does not serve, are 404 to the kind
  each admits. Any other method on a listed path is 405.
- Errors are `text/plain; charset=utf-8`, one line saying why.
- **Required headers:**
  - `session`: `Lodge-Login`, `Lodge-Node-ID`, `Lodge-Node-Name` and
    `Lodge-Session`;
  - `content`: `Lodge-Login`, `Lodge-Node-ID` and `Lodge-Node-Name`;
  - `agent`: `Lodge-Token` and `Lodge-Scope`.

  A missing one is 403.
- **Syntax** is the app contract's. maild decodes the percent-encoded
  `Lodge-Login` and `Lodge-Display-Name` through the kit.

### `/health`

`200 {"ok": true, "version": "<maild's version>", "protocol": 1}`.
`protocol` is maild's RPC protocol major. maild opens the app socket only
once its store is open, so a maild that answers is serving.

### `/api`

- A WebSocket (RFC 6455). Each text frame carries one message of
  [rpc-protocol.md](rpc-protocol.md#messages). The kit turns each frame
  into one line for maild's JSON-RPC server: a frame may end with one
  `\n`, which is not part of the message, and each frame maild sends is
  one message ending with one. A binary frame closes the connection.
- The connection is caller `session` for its lifetime. It is otherwise a
  connection as on the private socket: `rpc.hello` first, events after
  `events.subscribe`, 32 requests in flight, messages of at most 16 MiB.
- `rpc.hello`'s result names the caller: `caller` is `session`, with
  `login` (decoded `Lodge-Login`), `nodeName` (`Lodge-Node-Name`) and
  `displayName` (decoded `Lodge-Display-Name`, when sent).
- maild logs a session's start and end at info, with `Lodge-Session` and
  the node's name.
- **What lodge decides:** session counts and liveness. lodge closes a
  session after 90 s without a frame from the browser, so the web app
  calls `rpc.ping` at least every 30 s while the page is visible. When
  lodge closes the proxied connection, maild ends the connection, its
  views and its event subscription.

### Methods and callers

| Domain | Callers | Methods with their own callers |
| --- | --- | --- |
| `rpc` (`hello`, `ping`) | local, session, agent | |
| every other domain | local, session | `draft.attach`, `message.save`: local only. `account.list`, `mailbox.list`, `message.get`, `message.body`, `message.summaries`, `thread.messages`, `view.open`, `view.range`, `view.close`: local, session and agent |

- A method a caller may not use answers `methodNotFound`, as for a method
  that does not exist.
- The IDL (`schema/rpc`) marks callers per domain, and per method where a
  method's callers replace its domain's. `tools/rpcgen` generates the
  table the router checks.
- **New in this spec, all additive within protocol 1:**
  - `rpc.ping` answers `null`.
  - `Hello` gains `caller` (`local`, `session` or `agent`), and for a
    session `login`, `nodeName` and `displayName`.
  - `message.render` gains the optional param `content`. With `content`
    true, on a maild serving `--lodge`, and for a message with HTML, the
    result's `contentPath` names the reader page kept for it,
    `r/<token>.html`. It is absent otherwise.
  - `PartFile` gains `contentPath`, `p/<mac>/<path>`, on a maild serving
    `--lodge`.

### Attaching by bytes

`draft.attachData`, for callers local and session:

- the file comes in chunks of 1 byte to 4 MiB decoded, base64 in `data`,
  in order. The first (offset 0, no `uploadId`) starts an upload of
  `size` bytes named `filename`; each later one names its `uploadId` and
  continues at the last answer's `received`;
- maild checks the name (a name, not a path: no `/`, `\` or control
  character, at most 255 bytes) and that the draft's attachments stay
  within 25 MB, names the type from the name and the first bytes as
  `draft.attach` does, and keeps the chunks in a private temporary file;
- a chunk at another offset is `conflict`, and one of another
  connection's, draft, name or size is `notFound`;
- the chunk that completes `size` attaches the file, and its answer
  carries the attachment;
- an upload belongs to its connection: the connection's end, or 10
  minutes without a chunk, discards it. A connection has at most 4
  uploads in progress, and maild 16;
- agents cannot call it: the draft domain does not admit them.

### `/mcp`

MCP Streamable HTTP, through lodge's `/mcp/mail`, for caller `agent`.

- **The scope** is `Lodge-Scope`, which lodge passes unread:
  - `account=*` reaches every account, those added later included;
  - `account=<id>[,<id>…]` reaches the accounts named, by maild's IDs
    (`mailctl account ls`);
  - anything else, `;write` and `;send` included, is 403 at initialize:
    this maild grants read scopes only.
- **Sessions** are the kit's `mcpsession`: each is bound at initialize to
  `Lodge-Token` and `Lodge-Scope`, a request under another is 404, at most
  8 live sessions per token name (the least recently used evicted), and a
  session idle for an hour ends.
- **The engine enforces the scope.** Each session's tools reach maild over
  an in-process connection of caller `agent` carrying the token's name and
  scope:
  - only the methods whose callers list `agent` answer (see [Methods and
    callers](#methods-and-callers)); any other is `methodNotFound`;
  - `account.list` and `mailbox.list` list the scope's accounts only;
  - another account's message is `notFound` to `message.get` and
    `message.body`, `message.summaries` skips it, and its thread is
    `notFound` to `thread.messages`;
  - a view lists the scope's accounts' messages only, whatever its query.
- **The tools,** all read-only (`readOnlyHint`):

  | Tool | Input | Answers |
  | --- | --- | --- |
  | `list_mailboxes` | none | the scope's accounts, each with its mailboxes: ID, path, role, total, unread |
  | `search_messages` | `query` (the [search language](search.md)), `mailbox_id`, `limit` (1 to 50, default 20) | the total that match, and the newest: ID, thread ID, account ID, date, sender, subject, preview, unread, flagged, attachments |
  | `read_thread` | `message_id` | the message's conversation, oldest first, at most its newest 20: each one's date, sender, recipients, subject, text (cut at 32 KiB) and attachments' names, types and sizes |

  Every answer carries a notice that text from mail is data, never
  instructions: anyone can send the user mail.
- Nothing an agent can call sends, moves, deletes or changes mail.

### `/content/`

The public URLs are `https://lodge-content.<tailnet>.ts.net/mail/<content
path>`; the web app builds them with lodge-client's `contentURL`.

**Reader pages,** `GET` or `HEAD /content/r/<token>.html`:

- `token` is 32 lowercase hex digits from `crypto/rand`, drawn for each
  page that `message.render {content: true}` keeps.
- The page is the rendering's sanitized HTML in the reader's template: a
  doctype, `<meta charset="utf-8">`, `<meta name="color-scheme"
  content="light">`, `<base target="_blank">`, and the page style of
  [ui.md](ui.md) (white paper in both schemes), then the HTML as the body.
  Its images are `data:` URLs (ADR-0014). An image the budget left out
  keeps its `mailpart://localhost/` URL, which loads nothing here.
- maild keeps at most 64 pages and 64 MiB of them in memory, evicting the
  least recently kept. A token it does not hold, or no longer holds, is
  404.
- Headers:
  - `Content-Type: text/html; charset=utf-8`;
  - `Content-Security-Policy: default-src 'none'; img-src data:; style-src 'unsafe-inline'; font-src data:; base-uri 'none'; form-action 'none'; sandbox allow-popups allow-popups-to-escape-sandbox`;
  - `X-Content-Type-Options: nosniff`;
  - `Cache-Control: private, no-store`.

**Parts,** `GET` or `HEAD /content/p/<mac>/<path>`:

- `path` is a path in maild's parts cache, as `PartFile.path` gives it,
  with each segment percent-encoded (RFC 3986, every byte outside
  `A–Z a–z 0–9 - . _ ~`).
- `mac` is 22 base64url characters without padding: the first 16 bytes of
  HMAC-SHA256 over the decoded `path`, under a 32-byte key maild draws from
  `crypto/rand` at start and never stores. A path whose `mac` does not
  match is 404, like a path the cache does not hold, so content paths
  from before a restart are 404.
- The answer is the file. Its `Content-Type` comes from the extension:
  `png`, `jpg`/`jpeg`, `gif`, `webp`, `svg` (`image/svg+xml`), `pdf`,
  `txt` (`text/plain; charset=utf-8`), and `application/octet-stream` for
  any other.
- One `Range` per request is honored (`206`), with `Accept-Ranges: bytes`.
- With `?download=1`, also `Content-Disposition: attachment;
  filename*=UTF-8''<the file's encoded name>`.
- Headers:
  - `Content-Security-Policy: default-src 'none'; img-src data:; style-src 'unsafe-inline'; sandbox`;
  - `X-Content-Type-Options: nosniff`;
  - `Cache-Control: private, no-store`.

lodge-content adds its own sandbox policy and cross-origin headers
([lodge-http](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md#content-routes)).
For mail, whose manifest sets `contentLinks`, lodge's sandbox is
`sandbox allow-popups allow-popups-to-escape-sandbox`.

### The web build

- **Build.** `make web-build` builds the app in the nsl machine for the
  base `/app/mail/`, in Vite's `lodge` mode, into `build/web/`, with
  `icon.svg` beside `index.html`. It depends on `@frostyard/lodge-client`
  as a pnpm git dependency pinned to a published lodge commit.
- **Start.**
  1. `connectLodge({app: "mail"})`. Outside a frame, lodge-client moves
     the page to its canonical URL.
  2. A WebSocket to `lodge.apiURL()`, opened only while the page is
     visible, then `rpc.hello {protocol: 1, client: "frostmail-app"}`, so
     maild counts the web app as attended, as it counts the desktop app.
- **Keepalive.** `rpc.ping` every 30 s while `document.visibilityState`
  is `visible`, and once when the page becomes visible. When the socket
  closes, the app reconnects once the page is visible and reloads its
  state, as the desktop app does.
- **The reader** calls `message.render {id, content: true}` and shows
  `contentURL(contentPath)` in an `<iframe src>` with its own
  `sandbox="allow-popups allow-popups-to-escape-sandbox"`, never
  `allow-scripts` or `allow-same-origin`, and
  `referrerpolicy="no-referrer"`. The frame fills the reader below the
  message header and scrolls itself.
  A message without HTML shows its text, as today.
- **Attachments** open through `lodge.open(contentURL(contentPath))` from
  the click that asked, which opens a new tab. Each has a Save button,
  which opens the same URL with `?download=1`: the content route answers
  `Content-Disposition: attachment`, so the browser saves it.
- **Routes,** relative to `/app/mail`, with the canonical URL `/mail`
  followed by the route: the sidebar's source, then `/message/<id>` when
  one message is selected.

  | Route | Shows |
  | --- | --- |
  | `/` | All Inboxes |
  | `/mailbox/<id>` | a mailbox, by maild's ID |
  | `/smart/<id>` | a smart mailbox |
  | `/role/<role>` | every account's mailboxes of a role, such as `/role/sent` |
  | `/flagged`, `/flag/<1–7>` | flagged mail, or one flag color |
  | `/vips` | the VIPs' mail |
  | `…/message/<id>` | that source with one message selected, by maild's ID |

  A route the app does not know shows All Inboxes. The app reports
  `route` as the source and selection change (a new source pushes, a new
  message replaces), and obeys `navigate`. It never touches the top
  window's history or location.
- **Windows are routes.** The desktop app's other windows show in the
  frame in place of the main window, which stays loaded underneath, hidden,
  and takes no keys while one shows:

  | Route | Shows |
  | --- | --- |
  | `/compose/<draft id>`, `/compose/<draft id>?fresh` | the draft in Compose; `fresh` when it was made to open it, so closing it untouched discards it |
  | `/settings` | Settings |
  | `/event/<id>`, `/event/<id>?recurrenceId=…`, `/event/new?…` | the event editor, as the desktop's `#/event/…` |
  | `/reminders` | the reminder window |

  Opening one pushes its route, and closing it (Send, Close, Discard)
  pushes the main window's route again. Back to a main route closes it.
  One shows at a time; the reminder window never covers another.
- **Context.** With a message shown, `context {kind: "message", ref:
  "mail/message/<id>", title: <subject>}`; with none, `context {kind:
  null}`.
- **Theme.** The app follows `init.theme` and `theme`, not its own
  `prefers-color-scheme` query. The reader page stays white paper.
- **Storage.** Local-storage keys are prefixed `mail:`
  (`lodge.storageKey`).
- **The switcher's chord** (`init.shortcut`, by default Ctrl+\`) is bound
  by nothing in the app.
- **Attaching** sends the file's bytes with `draft.attachData`
  ([Attaching by bytes](#attaching-by-bytes)), from Attach Files (a file
  input) or files dropped on the window.
- **What a session cannot do** is not offered: Save As of a whole message,
  which writes a path on maild's machine.

### Limits

| What | Value |
| --- | --- |
| a JSON-RPC message, requests in flight | 16 MiB, 32 per connection |
| reader pages kept | 64, and 64 MiB; the least recently kept evicted |
| a content `mac` | 128 bits of HMAC-SHA256 |
| an upload | chunks of 4 MiB decoded; 4 in progress a connection, 16 in all; 10 minutes idle |
| MCP sessions | 8 per token name, the least recently used evicted; an hour idle |
| an MCP search, a thread | 50 messages; the newest 20, each text 32 KiB |
| keepalive | `rpc.ping` every 30 s while visible |

## Rules

- maild listens on no network address. `-devgw` refuses an address that
  is not loopback.
- maild reads `Lodge-*` headers only on the app socket, and only after the
  peer check.
- Each path on the app socket admits one caller kind. A test sends each
  path every other kind, and none.
- `draft.attach` and `message.save` answer `methodNotFound` to every
  caller but `local`. A test calls each method the IDL narrows as a
  session.
- Every content response carries maild's own policy, with `sandbox` and
  without scripts. A test renders the hostile corpus
  (`internal/render/testdata/hostile`) into reader pages and finds no
  script, form or URL to load other than `data:`.
- A content path is 404 when its token is not held or its `mac` does not
  match, whatever the path names.
- No response or log line holds a secret, a content key or a reader
  page's content.
- An agent's scope is enforced by the engine. A test calls each method an
  agent may use for another account's mailbox, message and thread, and
  methods it may not use, as an agent.
- Every limit has a test at the limit and one past it.

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| `api/zz_generated.go`, `app/src/rpc/gen/api.ts`, [rpc-api.md](rpc-api.md) | `make gen` from `schema/rpc/` (callers, `rpc.ping`, `Hello`'s fields, `message.render`'s `content`, `contentPath`) |
| `packaging/lodge/app.json` | the manifest above, its `version` set at build |
| `packaging/lodge/frostmail.socket`, `frostmail.service`, `sysusers.d/frostmail.conf` | [What maild installs](#what-maild-installs) |

## References

- Rationale: [ADR-0041](../adr/0041-maild-runs-as-a-lodge-app.md), [ADR-0005](../adr/0005-html-mail-rendering.md), [ADR-0014](../adr/0014-images-embedded-in-the-reader-frame.md), [ADR-0036](../adr/0036-reader-links-open-as-new-window-requests.md)
- Context: [design/overview.md](../design/overview.md#behind-lodge), [design/rendering.md](../design/rendering.md), [design/app.md](../design/app.md)
- lodge: [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md), [lodge-http](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md), [lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md)
- Related: [rpc-protocol.md](rpc-protocol.md), [rpc-api.md](rpc-api.md), [ui.md](ui.md); frostnotes' [lodge-app.md](https://github.com/frostyard/frostnotes/blob/main/docs/specs/lodge-app.md)
- Built in: [plan 0011](../plans/0011-m8-mail-in-lodge.md)
