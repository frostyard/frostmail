# The app

Living document. Rationale: [ADR-0009](../adr/0009-ui-architecture.md),
[ADR-0005](../adr/0005-html-mail-rendering.md).
Contracts: [specs/ui.md](../specs/ui.md), [specs/rpc-api.md](../specs/rpc-api.md).

## Overview

The app is a Tauri v2 window running a React UI. It holds no mail of its
own: lists are views kept by maild, messages are fetched when shown, and
every change goes back to maild as an API call.

```
components ◄── containers ◄── stores, ViewModel ◄── Client (gen/api.ts)
                                                       │
                                     Transport: tauri (bridge.rs ⇄ maild.sock)
                                                or mock (in-memory maild)
```

## Source layout (`app/src`)

| Path | What | Owner |
| --- | --- | --- |
| `main.tsx` | picks the transport (`VITE_MOCK=1` selects the mock), renders `App` | planner |
| `rpc/` | `transport.ts` (JSON-RPC session), `transport/tauri.ts`, `gen/api.ts` (generated) | planner |
| `rpc/mock/` | `MockTransport` and its fixture data | planner |
| `data/` | `ViewModel`, `useView`, the mailbox, sync and UI stores, `ClientContext` | planner |
| `app/` | `App` (pane layout, splitters, focus), containers that connect features to data | planner |
| `features/<area>/` | presentational components and their tests: `sidebar`, `list`, `reader`, `toolbar`, `menu`, `search` | cards |
| `lib/` | pure helpers: formatting, the mailbox tree, the keymap | cards |
| `styles/` | `tokens.css` (spec tokens), `app.css` (Tailwind entry, base styles) | planner |
| `testing/` | Testing Library setup and render helpers | planner |

Presentational components take data and callbacks as props, hold only
transient state (hover, an open menu), and import nothing from `data/` or
`rpc/` except API types. Containers in `app/` wire them to stores and the
client.

## Data

- **Connection.** `main.tsx` creates one `Client`, calls `rpc.hello`, then
  `events.subscribe` without `sinceSeq` (the UI reloads what it shows on
  reconnect instead of replaying). On close it shows a "Reconnecting…" state
  and retries every second; on success it reloads the mailbox and sync
  stores and reopens open views.
- **ViewModel** (`data/view.ts`) wraps one maild view:
  - `open(query)` calls `view.open` and stores `id` and `count`. Changing the
    query closes the old view and opens a new one.
  - Rows are a sparse `Map<index, MessageSummary>`. `ensure(start, end)`
    requests missing rows in pages of 100 aligned to multiples of 100, one
    request per page in flight.
  - `view.delta` (matched by view ID) applies its ops in order: an insert at
    `at` shifts cached rows at or after `at` up by `count`; a remove drops
    rows in `[at, at + count)` and shifts later rows down. Then the count is
    set from the delta and visible missing rows are requested again.
  - A generation counter increases with every delta; a range response from an
    older generation is dropped and its page requested again, since its
    indices may be stale.
  - `message.changed` IDs that match cached rows are refreshed with
    `message.summaries` and replaced by ID.
  - It notifies subscribers with a version number for
    `useSyncExternalStore`; `useView(query)` returns `{count, row(i), ensure}`.
- **Stores** (zustand): `mailboxes` (all mailboxes from `mailbox.list`,
  reloaded on `mailbox.changed` and `account.changed`, debounced 100 ms);
  `sync` (latest `SyncStatus` per account from `sync.status` and
  `sync.progress`); `ui` (the source, the search, selected IDs and the anchor
  for range selection, the focused pane, sidebar visibility, pane widths,
  conversation mode).
- **Selection** is a list of message IDs. After a delta the list container
  keeps the IDs still present; if all are gone it selects the row now at the
  first removed index, or the last row (specs/ui.md, Behavior).

## Reader

- With one row selected, the reader loads the conversation:
  `thread.messages(threadId)` (or the message alone when `threadId` is 0),
  newest first, at most 20 with "Show N earlier messages" for the rest.
  Each message gets `message.get` (headers, parts) and `message.render`.
  Selecting marks the message seen with `message.setFlags`.
- **The frame** (`features/reader/MessageFrame.tsx`, planner): an `iframe`
  with `sandbox="allow-same-origin"` and `srcdoc` set to a fixed template
  around `Rendering.html`: a charset meta, a light `color-scheme`, a white
  page with 20px padding, `overflow-wrap: anywhere`, and images limited to
  the frame's width. In dev builds the template rewrites
  `mailpart://localhost/` to `/mailpart/`.
  - After load, a `ResizeObserver` on the frame document's root (reachable
    because the frame is same-origin) sets the frame's height to its content,
    so the reader scrolls as one page.
  - A capturing click listener on the frame document cancels every
    navigation; for links it calls the `open_link` command, which opens
    `http`, `https` and `mailto` URLs in the system browser and ignores the
    rest.
  - Text-only messages render as React elements (`PlainText`), not in a
    frame.
- **Remote content:** the banner's button calls `message.render` again with
  `remote: true`; the frame is replaced with the new HTML.
- **Attachments:** a chip calls `message.part`, then the `open_part`
  command with the returned path; Rust resolves it under the parts cache
  with the same checks as the `mailpart` protocol and opens it with the
  system's default application.

## Window

- Undecorated (`decorations: false`). The toolbar carries
  `data-tauri-drag-region`; double-click toggles maximize. The window
  controls call `minimize`, `toggleMaximize` and `close` on the current
  window. The capability file grants exactly those window permissions plus
  the app's own commands.
- Keyboard: one `keydown` listener on the window maps events through
  `lib/keymap.ts` to commands (specs/ui.md, Keyboard map) and dispatches them
  to the focused pane's handlers.
- The webview's own context menu is disabled; list rows open the app's
  `ContextMenu`.

## Dev and test

- `make app-dev` runs maild and `tauri dev` in the nsl machine; Vite serves
  `/mailpart/` from `$FROSTMAIL_CACHE_DIR/parts` (`vite.config.ts`
  middleware, dev only), because WebKitGTK will not load `mailpart:` into the
  dev server's page.
- `VITE_MOCK=1 pnpm dev` runs the UI against `MockTransport` and its fixture
  data in any browser; `VITE_MOCK_ROWS=100000` sizes the fixture's Inbox for
  scrolling checks.
- **Against a real maild in a browser:** `maild -devgw 127.0.0.1:7878` with
  `FROSTMAIL_DEVGW_TOKEN` serves the API over a WebSocket
  (`internal/devgw`); `VITE_MAILD_WS='ws://127.0.0.1:7878/?token=…' pnpm dev`
  connects the UI to it (`rpc/transport/ws.ts`), and `FROSTMAIL_CACHE_DIR`
  points Vite's `/mailpart/` at that maild's cache. The gateway listens only
  on loopback, needs the token and the dev server's Origin (browsers do not
  apply CORS to WebSockets), and is off unless asked for. Links and
  attachments open in browser tabs there instead of through Tauri.
  `dev/incus/mailtest.sh demo USER` delivers realistic demo mail. Run a
  development maild with `FROSTMAIL_SECRETS=file`, or its test passwords
  move into the desktop's keyring (docs/design/accounts.md, Secrets).
- `make ui-check` runs Biome, the TypeScript check and Vitest (happy-dom,
  Testing Library) in the nsl machine. Task cards that touch `app/` are
  verified with it.
- App tests (`app/e2e`, `make ui-e2e`) drive the built app in WebKitGTK
  through `WebKitWebDriver` directly: the session's
  `webkitgtk:browserOptions` launch `build/frostmail-app --automation` with
  `TAURI_WEBVIEW_AUTOMATION=true`, which is all `tauri-driver` does on
  Linux. They run headless under Xvfb, against a maild whose data directory
  `tools/uifixture` built.

## Operational notes

- Nothing persists in the app except pane widths, sidebar visibility and
  conversation mode (local storage). Clearing it resets the layout.
- If maild is not running, the window shows the connection error and keeps
  retrying; `maild` started later is picked up without a restart.
