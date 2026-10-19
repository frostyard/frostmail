# Plan 0011: M8 Mail in lodge

M8 makes maild lodge's `mail` app: one maild on lodge's host serves the
user's phone, any browser on their tailnet, the desktop app and agents
([ADR-0041](../adr/0041-maild-runs-as-a-lodge-app.md)). It is frostmail's
side of [lodge's roadmap Phase 4](https://github.com/frostyard/lodge/blob/main/docs/plans/0001-roadmap.md#phase-4--mail-in-lodge),
and depends on lodge's Phases 1 (the kit) and 2 (lodge v1). It runs after
[M7](0010-m7-microsoft-and-windows.md) in the [roadmap](0002-roadmap.md),
and leaves the friend's Windows install on its local maild.
The contract is [specs/lodge-app.md](../specs/lodge-app.md).

## Decisions for M8

The user chose these on 2026-10-18:

- **lodge, not a frostmail server of its own.** lodge is the only process
  on the network; maild is an app behind it
  ([core ADR-0057](https://github.com/frostyard/core/blob/main/docs/adr/0057-run-first-party-apps-behind-lodge.md)).
- **One maild for every client,** the desktop app included. A local maild
  stays for installs outside the owner's tailnet.
- **Tailnet only,** with no public endpoint.
- **Generated mail until the user moves their own.** Until Phase 7, every
  test, smoke run and deployment uses generated mail (`tools/mailgen`,
  `tools/uifixture`, the mailtest container) and no real account.

lodge's decisions that shape mail's side (the lodge program, 2026-10-18):

- **D4:** the frame learns the content origin from lodge-client's `init`,
  and its policy admits `lodge-content` in images, media and frames.
- **D6:** a manifest that sets `contentLinks` gets
  `sandbox allow-popups allow-popups-to-escape-sandbox` on its content,
  and never scripts, same-origin, forms or top navigation. Mail sets it.
- **D7:** a native client on an allowed node, such as the desktop app's
  bridge, may open an app's `/api` with `Origin: https://LODGE` and no
  fetch metadata; its identity comes from WhoIs alone. It reads the
  content origin from `GET /lodge/apps`.

## How the phases map onto lodge's Phase 4

| lodge's Phase 4 | Here |
| --- | --- |
| maild under user `frostmail`, on the kit, with its manifest | Phase 2; `mcp` in Phase 5 |
| a local maild for installs outside the tailnet; both local and remote | Phase 6 |
| mail's bundle for `/app/mail/`, the reader from `lodge-content` | Phase 3 |
| `/mcp/mail`, with a scope grammar of frostmail's | Phase 5 |
| links between apps | Phase 4 |
| "mail and notes switch in the shell on the phone and a desktop" | Phases 3 and 4 (the narrow layout) |

## Phase 1 — The decision and the contract (planner, small)

- [ADR-0041](../adr/0041-maild-runs-as-a-lodge-app.md), [specs/lodge-app.md](../specs/lodge-app.md)
  and this plan, cross-linked with the design docs and lodge's.
- **Done when:** the three are indexed in [docs/README.md](../README.md),
  and the roadmap names M8.

## Phase 2 — maild's lodge socket (planner, medium)

Core work: the socket, the callers and the content route are security
boundaries, so the planner builds them.

- **The kit.** `go get github.com/frostyard/lodge/kit@<sha>` at a pushed
  lodge commit, bumped when lodge publishes a release.
- **Callers in the IDL.** `schema/rpc` gains `callers` per domain and per
  method, and `tools/rpcgen` generates the table. The router answers
  `methodNotFound` to a method the connection's caller may not use.
  `draft.attach` and `message.save` are `local` only.
- **`maild --lodge`:** the app socket through the kit (a passed socket, or
  one maild makes; the peer check; `LODGE_ROOT` in development), with
  `/health`, `/api` and `/content/` as the spec has them.
  - `/api` serves the existing JSON-RPC connection over WebSocket text
    frames, one message per frame, as caller `session`.
  - `rpc.ping`, and `Hello`'s caller fields.
  - `message.render {content}` keeps reader pages in a bounded in-memory
    store, and `PartFile.contentPath` names parts under a per-start key.
- **Packaging.** `packaging/lodge/`: `app.json`, `frostmail.socket`,
  `frostmail.service` and `sysusers.d/frostmail.conf`. `make
  lodge-package` stages them, with the binary and the web build, under a
  root for lodge's installer.
- **Tests** play lodge with the kit's `kittest`: the peer check, each
  path against every caller kind, the required headers, `/health`, an
  `/api` session that says hello, pings and renders, the reader page and
  a part from `/content/` with their headers, a range and a download, and
  404 for a stale token or a bad `mac`.
- **Done when:** `kittest` passes every rule of the spec's socket
  sections against `maild --lodge`, each with a test that fails without
  it, and `make check` is green.

## Phase 3 — The web build (planner and executor, medium)

- **`make web-build`** in the nsl machine: Vite's `lodge` mode, the base
  `/app/mail/`, `build/web/` with `icon.svg`.
- **`@frostyard/lodge-client`** as a pnpm git dependency
  (`github:frostyard/lodge#<sha>&path:client`), at a pushed lodge commit.
- **The lodge transport** (`rpc/transport/lodge.ts`, from the dev
  gateway's `ws.ts`): `connectLodge()`, the WebSocket to `apiURL()`,
  `rpc.ping` every 30 s while visible, and a reconnect that resumes events
  when the page is visible again.
- **The reader** shows the reader page from `lodge-content` in a frame that
  fills the pane. Links open new tabs (D6), and attachments open or
  download from their content URLs.
- **Routes, context, theme and storage** as the spec has them. Nothing
  binds the switcher's chord.
- **Done when,** in lodge's development build (`make e2e-build`) with a
  maild on a `tools/uifixture` data directory:
  - Ctrl+\` then `m` opens mail, and a mailbox lists;
  - an HTML message renders from `lodge-content`, with its images, and
    the hostile corpus runs no script and loads nothing;
  - a link in a message opens a new tab, and an attachment opens;
  - reloading `/mail/mailbox/<id>/<message id>` opens that message;
  - `make ui-check` is green.

## Phase 4 — Mail's windows and files in a browser (executor, large)

- **Windows become routes** (built). Compose (`/compose/<draft id>`),
  Settings (`/settings`), the event editor and the reminder window open in
  the frame, each with its way back. The desktop app keeps its windows.
  A reply composed in lodge's frame goes out through `tools/smtpsink`
  (`dev/lodge/run.sh ROOT fixture`).
- **Files.** Attaching sends the file's bytes over the RPC
  (`draft.attachData`, in chunks within the 16 MiB message limit; built),
  and an attachment's Save downloads it from the content route with
  `?download=1` (built). Save As and Raw Source of a whole message are to
  download from a content route for the message's source.
- **Links between apps.** A permanent route by Message-ID
  (`/message/<Message-ID>`, resolved by maild), which notes can link to.
  A canonical lodge URL in a message opens in the shell through
  `lodge.open`, when the link is in mail's own UI.
- **The narrow layout:** one pane at a time below a width
  [ui.md](../specs/ui.md) sets, for the phone.
- **Notifications** while lodge is open: the web app shows new mail and
  reminders with the Notification API. Web Push waits on lodge.
- **Done when,** in lodge's development build: a reply with an attachment
  composed in the frame goes out through `tools/smtpsink`; Save As
  downloads the `.eml`; a note's link to `/mail/message/<id>` opens the
  thread; and the phone layout reads and replies at 390 px wide.

## Phase 5 — Agents through `/mcp/mail` (planner, medium)

- **Read-only first (built):** `list_mailboxes`, `search_messages` and
  `read_thread`, under `account=*` or `account=<id>[,<id>…]`, with the
  engine enforcing the scope and the kit's `mcpsession` binding sessions.
- **Tools (built):** list mailboxes, search, read a thread; with
  `;write`, set flags, move messages and draft a reply; with `;send`,
  send a draft, a tool of its own.
- **The scope grammar** (built), in `Lodge-Scope`: `account=<id>` or
  `account=*`, then optionally `;write` (flags, moves, drafts) and
  `;send` (queue a draft to send). Anything else is 403.
- **The engine enforces it** (built). Each MCP session reaches maild over
  an in-process connection of caller `agent` carrying its scope. Another
  account is `notFound`, and a method the scope does not cover is
  `methodNotFound`: the IDL's per-method `scope` (`write`, `send`), which
  the router checks.
- **Sessions** through the kit's helper: bound to the token's name and
  scope, 8 per token name with the least recently used evicted, an hour
  idle. The manifest gains `mcp: true`.
- **Done when:** Claude Code reads and drafts mail through `/mcp/mail`
  with an `account=<id>;write` token on generated mail, a send under that
  scope is `methodNotFound`, another account is `notFound`, and revoking
  the token cuts it off at once.
  - In the tests, through the kit's `kittest` and the MCP SDK's client
    (`internal/lodge/mcp_test.go`): under `account=<id>;write`,
    `draft_reply` drafts Ann's reply above the quote, `send_draft` is not
    offered, and the other account's message is an error; on the agent's
    connection `draft.send` is `methodNotFound` and the other account's
    message, mailbox and draft are `notFound`; under `;write;send`,
    `send_draft` queues the draft.
  - Live, 2026-10-19: Claude Code 2.1.300 (`claude -p` with an HTTP MCP
    server and a bearer token) on an allowed node read showcase mail
    through LODGE's `/mcp/mail` with an `account=*` token: `list_mailboxes`
    returned both accounts. It sends no `Origin` and no `Sec-Fetch-*`,
    and every request was 200 or 202. Evicted by 8 newer sessions on its
    token, its stream and its next POST were 404, and it initialized a
    new session and retried by itself. It probes MCP 2026-07-28's
    `server/discover` first, which the kit answers 400, and falls back to
    `initialize` at 2025-11-25. Revoking the token made the next request
    401 (frostyard/lodge#9). Drafting live under `;write` is still to
    run.

## Phase 6 — The desktop app as a lodge client (planner, medium)

The design is [design/desktop.md](../design/desktop.md#on-a-maild-behind-lodge).
It depends on Phase 4's attaching by bytes and Save As by download.
Built: `app.json` and the Server pane, the bridge's remote mode with the
keepalive, `open_part`'s downloads, and the Flatpak's network, with
`app/e2e/lodge.e2e.ts` replying through lodge's development build.
Notifications are not built yet.

- **Which maild:** the app's own `app.json` names lodge's URL, else the
  local socket. Settings shows and changes it.
- **The bridge's remote mode:** a WebSocket to `wss://LODGE/app/mail/api`
  with `Origin: https://LODGE`, from the desktop's own tailnet node, one
  line per text frame (Rust: `tokio-tungstenite` with rustls). lodge-http
  admits native clients this way (D7).
- **Keepalive:** the webview's transport sends `rpc.ping` every 30 s in
  remote mode.
- **Parts:** `open_part` downloads from lodge's content origin, read
  from `GET https://LODGE/lodge/apps`, into the app's cache and opens the
  file, so the webview still fetches nothing.
- **Notifications:** an IDL event for announcements, sent to connected
  clients when maild has no session bus, and shown by the app's own
  notifier.
- **The Flatpak** gains the network for the Rust layer.
- **Done when:** with `app.json` naming lodge's development build, the
  desktop app reads, composes with an attachment and sends through a maild
  behind lodge, a new message notifies on the desktop, and the friend's
  Windows install still runs on its local maild.

## Phase 7 — The user's mail moves to lodge's host

- Answer the sign-in question below.
- Move the user's data directory and secrets from the desktop's maild to
  `/var/lib/frostmail`, and stop the desktop's maild.
- **Done when:** the user's accounts sync only on lodge's host for a week,
  read from the phone, a browser and the desktop app, with clean daily
  verifies.

## Later / ideas

- Web Push for new mail on the phone, once lodge decides it
  ([lodge ADR-0008](https://github.com/frostyard/lodge/blob/main/docs/adr/0008-lodge-installs-as-one-home-screen-app.md)).
- `mailto:` links in a message composing in mail rather than the
  browser's handler.
- Extracting maild's JSON-RPC server into the kit, with frostnotes'
  (lodge's Phase 1), once the endpoint works.

## Open questions

- **Google sign-in from a browser on another machine.** ADR-0011's
  redirect goes to a loopback port on maild's machine, which a browser
  elsewhere cannot reach. Candidates: a redirect to a lodge route, pasting
  the redirect URL back into mail, or signing in from the desktop app and
  handing the grant to the server's maild. Decide by Phase 7.
- **Secrets on the server:** the file store, or something sealed (systemd
  credentials, a TPM). Decide by Phase 7.
- **Reminders with no client open:** with no session bus on the server, a
  reminder that fires while no app is attached reaches nobody until one
  connects. Decide with Phase 4's notifications.

## References

- Implements: [specs/lodge-app.md](../specs/lodge-app.md), [ADR-0041](../adr/0041-maild-runs-as-a-lodge-app.md)
- Context: [design/overview.md](../design/overview.md#behind-lodge), [design/rendering.md](../design/rendering.md), [design/desktop.md](../design/desktop.md), [design/app.md](../design/app.md)
- lodge: [roadmap](https://github.com/frostyard/lodge/blob/main/docs/plans/0001-roadmap.md), [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md), [lodge-http](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md), [lodge-client](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md)
- Model: frostnotes' [plan 0006](https://github.com/frostyard/frostnotes/blob/main/docs/plans/0006-m4-notes-inside-lodge.md)
