# 0009 — Build the app as a thin client over live views

- **Status:** Accepted
- **Date:** 2026-10-07

## Context

M2 turns the M0 spike into a Mail.app-style reader. maild already owns every
list as a view: `view.open` returns a count, `view.range` returns rows by
index, and `view.delta` pushes inserts and removals
([ADR-0002](0002-daemon-and-thin-clients.md)). The app runs in WebKitGTK
under Tauri v2 and must look like neither GNOME nor KDE. A local executor
model writes most components from task cards
([ADR-0008](0008-local-executor-workflow.md)), so the structure must make
small, independently testable pieces with exact contracts. Lists reach
100,000 rows.

## Decision

- **React 19 and strict TypeScript, no router.** One window, one `Client`
  over the Tauri transport.
- **Views are the only way to list messages.** A framework-free `ViewModel`
  per open list holds the count and a sparse window of rows, applies
  `view.delta` ops, fetches visible ranges with `view.range`, and refreshes
  rows named by `message.changed` with `message.summaries`. Components read it
  through `useSyncExternalStore`. There is no request cache: data is pushed,
  so TanStack Query would only duplicate the view.
- **Small global stores in zustand:** mailboxes (`mailbox.list`, refreshed on
  `mailbox.changed`), sync status (`sync.progress`), and UI state (selected
  source, selection, focus, pane widths).
- **Virtualized lists** with `@tanstack/react-virtual` and fixed row heights
  from [specs/ui.md](../specs/ui.md).
- **Neutral look:** Tailwind v4 over CSS-variable tokens
  (`app/src/styles/tokens.css`, light and dark), the bundled Inter Variable
  font, Lucide icons, and client-side window chrome: the toolbar is the title
  bar, with our own window controls. No Adwaita, Breeze, GTK or Qt styling.
- **Presentational components take props and call callbacks.** Containers
  connect them to stores and the client. Cards build presentational
  components; the planner owns the data layer, containers and the reader
  frame.
- **Tests at three levels:** Vitest with happy-dom and Testing Library
  against `MockTransport`, an in-memory maild with real view semantics, for
  units and components; Biome for formatting and lint; and app tests in the
  built app under WebKitGTK, driven by `tauri-driver` and Debian's
  `WebKitWebDriver`, against a maild serving a fixture store.
- **Dev loops:** `make app-dev` runs maild and `tauri dev` in the nsl
  machine. `VITE_MOCK=1 pnpm dev` runs the UI against `MockTransport` in any
  browser. Because WebKitGTK will not load the local `mailpart:` scheme into
  the dev server's `http://127.0.0.1:5173` page, a Vite middleware serves
  `/mailpart/` from the parts cache in dev, and the reader rewrites the prefix
  there.

## Consequences

- Scrolling cost is bounded by visible rows plus prefetch, whatever the
  mailbox size; maild does the ordering.
- `MockTransport` must keep up with the API. Its view logic shares the delta
  algorithm's tests, and app tests run against the real maild.
- Client-side chrome makes window controls and edge resizing our problem on
  Wayland and X11. The app tests check that the window can be moved and
  resized.
- App tests run inside the nsl machine with software rendering, so frame-time
  budgets there are looser than on the GPU; native runs confirm smoothness by
  hand.
- Every new runtime dependency is pinned in `pnpm-lock.yaml` and added by the
  planner only.

## Alternatives considered

- **TanStack Query for lists:** a pull cache over a push protocol; views
  already provide ordering, windows and invalidation.
- **Redux or a single global store:** more ceremony than three small stores;
  executor cards would touch shared reducers.
- **Playwright WebKit for app tests:** not the WebKitGTK build users run, and
  its browser downloads are not checksum-pinned (frostyard/core ADR-0023).
- **The desktop's font and title bar:** the app would look like whichever
  desktop it runs on, which is what Frostmail avoids.

## References

- Implements: [specs/ui.md](../specs/ui.md), [design/app.md](../design/app.md)
- Related: [ADR-0005](0005-html-mail-rendering.md) (reader isolation)
