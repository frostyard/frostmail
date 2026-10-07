# Plan 0004: M2 UI read path

M2 replaces the M0 spike page with a Mail.app-style reader over maild:
sidebar, conversation list, reader with sanitized HTML, toolbar, context
menus and the keyboard map, for the single IMAP account M1 syncs. It follows
[M1](0003-m1-headless-read-path.md) and comes before compose (M3) in the
[roadmap](0002-roadmap.md). Work is split per
[ADR-0008](../adr/0008-local-executor-workflow.md): the planner owns
contracts, the data layer, containers, the reader frame and rendering; cards
own presentational components, pure helpers and simple handlers.

## Decisions taken for M2

- **UI architecture** per [ADR-0009](../adr/0009-ui-architecture.md):
  views through a `ViewModel`, zustand stores, Tailwind over spec tokens,
  Inter and Lucide, client-side window chrome, Vitest with a
  `MockTransport`, app tests in WebKitGTK via `tauri-driver`.
- **API, protocol 1, additive:** `message.summaries`, `message.render`,
  `message.part`, `thread.messages`, `ViewQuery.role` and `.threads`,
  `MessageSummary.threadCount` ([rpc-api](../specs/rpc-api.md)).
- **Rendering in maild** per [design/rendering.md](../design/rendering.md):
  allowlist sanitizer, `mailpart://` rewriting, trackers never loaded, remote
  images only through maild's guarded fetcher.
- **Cards without reference solutions** when the contract is simple: the
  planner checks that given tests compile and fail on the stub, then reviews
  the result. References remain for subtle cards (Phase 1 records which).

## Phase 1 — Contracts and foundations (planner)

- [x] ADR-0009, [specs/ui.md](../specs/ui.md),
  [design/app.md](../design/app.md), [design/rendering.md](../design/rendering.md).
- [x] Schema additions, store and engine stubs; cards T-0024, T-0025.
- App foundation: dependencies (Tailwind, zustand, react-virtual, Lucide,
  Inter, Testing Library, happy-dom, Biome), `tokens.css`, `make ui-check`,
  taskrun running `ui-check` for cards that touch `app/`, the spike removed.
- `MockTransport` with fixture data and real view semantics; `ViewModel`,
  `useView`, the stores, `ClientContext`.
- Presentational stubs with fixed props and given tests for the TS cards.
- **Done when:** `make check` and `make ui-check` are green, and
  `VITE_MOCK=1 pnpm dev` shows the three-pane shell with mock data.

## Phase 2 — Executor batch (runs alongside Phase 3)

| Card | Area | What |
| --- | --- | --- |
| T-0024 | `internal/store` | Role and thread views, `ThreadMessageIDs`, `ThreadCount` |
| T-0025 | `internal/engine` | `message.summaries`, `thread.messages`, view role and threads |
| T-0026 | `app/src/lib` | `format`: list and header dates, sizes, names, initials, avatar tone |
| T-0027 | `app/src/lib` | `mailboxTree`: sidebar sections from mailboxes |
| T-0028 | `features/reader` | `PlainText`: quote levels, collapsing, links, signature |
| T-0029 | `features/list` | `MessageRow` |
| T-0030 | `features/sidebar` | `Sidebar` |
| T-0031 | `features/reader` | `MessageHeader`, `AttachmentStrip`, `RemoteBanner` |
| T-0032 | `features/toolbar` | `Toolbar` with flag and move menus |
| T-0033 | `features/menu` | `ContextMenu`: keyboard navigation, submenus |
| T-0034 | `app/src/lib` | `keymap`: key events to commands |
| T-0035 | `features/search` | `SearchField` and `ScopeBar` |
| T-0036 | `tools/uifixture` | A fixture data directory (store and blobs) of N messages |

- **Done when:** every card is merged with `make check` and `make ui-check`
  green.

## Phase 3 — Rendering and the reader (planner)

- `internal/render`: sanitizer with golden tests and fuzzing, URL rewriting,
  tracker detection, the guarded remote fetcher, the parts cache;
  `message.render` and `message.part` in the engine.
- `MessageFrame` (sandboxed `srcdoc`, sizing, click interception), the
  conversation container, `open_link` and `open_part` commands, the Vite
  `/mailpart/` middleware.
- Window chrome: undecorated window, drag region, controls, capabilities.
- **Done when:** `make app-dev` reads the Dovecot test account: HTML mail
  with inline images renders, remote images load on request, links open in
  the browser, attachments open.

## Phase 4 — Integration and app tests (planner, with cards where they fit)

- Containers connect the Phase 2 components; keyboard map and context menus
  dispatch real commands; selection follows deltas; sync activity in the
  sidebar and toolbar.
- `app/e2e`: a minimal WebDriver client, `tauri-driver` and
  `WebKitWebDriver` in the nsl machine, maild on a `tools/uifixture` data
  directory; the hostile-HTML suite (a canary HTTP server must see zero
  requests and the page must record zero CSP violations reaching the
  network); a scroll test over 100,000 rows.
- **Done when:** the app tests pass in the nsl machine.

## Phase 5 — Exit evidence

- **Done when**, each shown by a test or a recorded run:
  1. The planner reads the Dovecot test account with the built app for a
     session: conversations, HTML and plain mail, flags, moves, deletes and
     search, with no console errors.
  2. Scrolling a 100,000-row Inbox keeps the 95th-percentile frame under
     25 ms in the nsl machine (software rendering), with no blank rows after
     scrolling stops; a native run on the host is checked by eye.
  3. The hostile-HTML suite (at least 40 messages: scripts, event handlers,
     forms, meta refresh, CSS `url()` and `@import`, `srcset`, SVG, `object`,
     remote fonts, link prefetch, `base`, data and `javascript:` URLs) makes
     zero network requests, both sanitized through maild and raw in the
     frame.
  4. `make ui-check` and `make check` are green; every M2 card is merged.

## Later / ideas

- Classic layout (list above reader) and list density options.
- Per-sender "always load remote content" (the `remote_allow` table).
- Find in message (Ctrl+F inside the reader) and printing.

## Open questions

- **Undecorated window resizing on Wayland (GNOME, KDE):** if Tauri's
  resize handles for undecorated windows fall short, keep server-side
  decorations with the unified toolbar beneath. Decide in Phase 3.

## References

- Implements: [specs/ui.md](../specs/ui.md), [design/app.md](../design/app.md),
  [design/rendering.md](../design/rendering.md)
- Rationale: [ADR-0009](../adr/0009-ui-architecture.md),
  [ADR-0005](../adr/0005-html-mail-rendering.md)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
