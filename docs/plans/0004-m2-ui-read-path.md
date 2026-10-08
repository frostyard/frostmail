# Plan 0004: M2 UI read path

M2 replaces the M0 spike page with a Mail.app-style reader over maild:
sidebar, conversation list, reader with sanitized HTML, toolbar, context
menus and the keyboard map, for the single IMAP account M1 syncs. It follows
[M1](0003-m1-headless-read-path.md) and comes before compose (M3) in the
[roadmap](0002-roadmap.md). Work is split per
[ADR-0008](../adr/0008-local-executor-workflow.md): the planner owns
contracts, the data layer, containers, the reader frame and rendering; cards
own presentational components, pure helpers and simple handlers.
Status: **done** (2026-10-07) except two checks that need a desktop session:
the native, GPU-rendered scroll check by eye and undecorated-window resizing on
Wayland (see Phase 5 and Open questions).

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
- [x] App foundation: dependencies (Tailwind, zustand, react-virtual, Lucide,
  Inter, Testing Library, happy-dom, Biome), `tokens.css`, `make ui-check`,
  taskrun running `ui-check` for cards that touch `app/` and feeding
  verification failures back to the executor, the spike removed.
- [x] `MockTransport` with fixture data and real view semantics; `ViewModel`,
  `useView`, the stores, `Session`; containers for every pane.
- [x] Presentational stubs with fixed props and given tests for the TS cards.
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
  green. Done.

## Phase 3 — Rendering and the reader (planner)

- [x] `internal/render`: sanitizer with a 50-message hostile corpus, a
  structural safety checker and fuzzing (which found two real bugs: a
  panic on unterminated `url(` strings, and removed CSS gluing its
  neighbors into a new `@import`), URL rewriting, tracker detection, the
  guarded remote fetcher, the parts cache; `message.render` and
  `message.part` in the engine.
- [x] `MessageFrame` (sandboxed `srcdoc`, sizing, click interception), the
  conversation container, `open_link` and `open_part` commands, the Vite
  `/mailpart/` middleware.
- [x] Window chrome: undecorated window, drag region, controls, capabilities.
- [x] `maild -devgw`: the API over a token-protected loopback WebSocket, so
  the UI runs in a browser against a real maild (`VITE_MAILD_WS`);
  `dev/incus/demo` messages and `mailtest.sh demo USER`.
- **Done when:** `make app-dev` reads the Dovecot test account: HTML mail
  with inline images renders, remote images load on request, links open in
  the browser, attachments open.

## Phase 4 — Integration and app tests (planner, with cards where they fit)

- [x] Containers connect the Phase 2 components; keyboard map and context
  menus dispatch real commands; selection follows deltas; sync activity in
  the sidebar and toolbar.
- [x] `app/e2e` (`make ui-e2e`): a minimal WebDriver client driving the
  built app through `WebKitWebDriver` directly (no `tauri-driver`),
  headless under Xvfb, maild on a `tools/uifixture` data directory; the
  hostile-HTML suite (a canary HTTP server must see zero requests, raw in
  the frame and through maild with remote content loaded); a scroll test
  over 100,000 rows; a smoke test.
- **Done when:** the app tests pass in the nsl machine. Done.

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

Evidence, recorded 2026-10-07:

| # | How | Result |
| --- | --- | --- |
| 1 | A reading session on the Dovecot test account (test3, `dev/incus/demo` mail) with maild synced from the server and the UI in the browser pane through `maild -devgw`; the built app in WebKitGTK through `make ui-e2e` (smoke test, fixture data) | HTML mail with an inline image, a table layout and a stripped form and script; remote content loaded on request with the tracker still blocked; a three-message conversation with quote levels, collapsing and the signature; flags set from the context menu by mouse and keyboard, stored on the server as `$MailFlagBit` colors; mark unread with live counts; search; delete moving the selection; attachments opened from the parts cache. No console errors. The session found and fixed three bugs (below). |
| 2 | `make ui-e2e`: scroll test, 100,000-row fixture, nsl machine, Xvfb, software rendering | 299 frames, p95 19.0 ms, max 21.0 ms, 120,000 px scrolled, 0 blank rows after stopping. The native GPU run by eye is pending (needs a desktop session). |
| 3 | `make ui-e2e`: hostile suite, 50 messages, canary server on loopback | Zero requests raw in the sandboxed frame under the app CSP; zero requests through maild with "Load Remote Content" clicked wherever offered (maild's fetcher refused the loopback canary). The test checks that all 50 messages were opened. Sanitizer fuzzing: 236 million inputs in 15 minutes, clean after two fixes. |
| 4 | `make check`, `make ui-check` (188 tests), `go test -race ./...`, `make engine-it` | All green; cards T-0024 to T-0036 merged. |

**Bugs found by using it.** In maild: `mimex` trimmed the `-- ` signature
separator to `--`; local flag changes emitted `message.changed` but not
`mailbox.changed`, so unread counts went stale. In the app: opening a
conversation marked only one of its messages read. Fuzzing found two more
in the sanitizer (see Phase 3).

**Executor record.** 13 cards. 11 passed through taskrun on the first run.
T-0033 failed after three attempts because its contract contradicted ARIA
and Biome (`aria-checked` on `menuitem`, a `div` separator); the planner
corrected the card and finished it. T-0035 passed its tests but failed
Biome, which taskrun did not feed back and which opencode's allowlist did
not let it run; both are fixed. T-0024 stopped and asked when an older
contract test conflicted with its card, instead of editing a protected
file. Three defects slipped past given tests, all from card gaps: folder
rows without `mailboxId`, quote colors built as runtime class names, and a
toolbar double-click that duplicated Tauri's. Lessons for cards: lint the
markup a contract prescribes before handing it over, state the framework's
own behavior (Tauri drag regions, Tailwind's class scanning), and assert
every relationship the contract names.

## Later / ideas

- HTML bodies use the system font: the frame cannot load the app's bundled
  Inter. Inject an `@font-face` for it into the frame page.
- Say when "Load Remote Content" fails (the banner currently stays as it was).
- Submenu hover tolerance (a "safe triangle"), so diagonal moves toward a
  submenu do not open a sibling's.
- Classic layout (list above reader) and list density options.
- Per-sender "always load remote content" (the `remote_allow` table).
- Find in message (Ctrl+F inside the reader) and printing.

## Open questions

- **Undecorated window resizing on Wayland (GNOME, KDE):** if Tauri's
  resize handles for undecorated windows fall short, keep server-side
  decorations with the unified toolbar beneath. Not yet checked: needs a
  native run on a desktop (`make app-run`, or `build/frostmail-app` with a
  maild running). Decide before M3 starts.

## References

- Implements: [specs/ui.md](../specs/ui.md), [design/app.md](../design/app.md),
  [design/rendering.md](../design/rendering.md)
- Rationale: [ADR-0009](../adr/0009-ui-architecture.md),
  [ADR-0005](../adr/0005-html-mail-rendering.md)
- Workflow: [design/agent-workflow.md](../design/agent-workflow.md)
