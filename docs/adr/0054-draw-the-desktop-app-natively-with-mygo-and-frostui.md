# 0054 — Draw the desktop app natively with MyGo and frostui

- **Status:** Proposed
- **Date:** 2026-11-10

## Context

[ADR-0002](0002-daemon-and-thin-clients.md) made the desktop app a Tauri v2
window with a React UI, because "the two hardest UI surfaces in a mail
client are web problems": HTML mail needs a browser engine, and composing
needs a mature rich-text editor. [ADR-0009](0009-ui-architecture.md) set
the React app's structure. Since then:

- **The React app has two hosts.** Tauri on the desktop (Linux and, since
  [ADR-0038](0038-windows-as-a-second-platform.md), Windows), and lodge's
  web build for phones and agents
  ([ADR-0041](0041-maild-runs-as-a-lodge-app.md)). The desktop adds a Rust
  shell (11 files, about 3,800 lines: the bridge, the `mailpart` protocol,
  modes, portals, spell check, problem records), a Rust notifier on
  Windows, and WebKitGTK and WebView2 workarounds in the TypeScript.
- **The suite moves its desktop apps to MyGo.** On 2026-11-10 the owner
  chose [MyGo](https://github.com/egoist/mygo) and frostyard's kit,
  frostui, for frostyard's desktop apps. ChairLift was ported first
  (frostyard/chairlift#282, its ADR-0017), and frostui is being moved out
  of it into `github.com/frostyard/frostui`. Frostnotes is being ported the
  same way. The owner's rules for this port:
  - **Two UIs.** Frostmail's desktop app becomes native MyGo and frostui,
    replacing Tauri. The React UI stays only as lodge's web app.
  - **Thin clients.** Logic stays in maild; both UIs are clients of the
    same JSON-RPC API.
  - **Shared look.** The window follows ChairLift's: its hidden title bar
    and own top bar, the command palette (Ctrl K), the status bar, toasts,
    dialogs and the frostui theme. Frostmail's own needs come first
    (Mail.app's three panes, compose windows, calendar, people and tasks),
    and no ChairLift pattern is forced where it does not fit.
- **What MyGo offers a mail client** (v0.8.3, checked in its docs):
  - It is plain Go with no CGO; it draws its widgets on GTK 3 and OpenGL
    (Linux) loaded with purego, and builds for Linux and Windows from one
    machine.
  - `ui.WebView` shows the system's web view (WebKitGTK, WebView2) inside
    native UI, with native popovers, menus and dialogs drawn over it. A
    page can be **untrusted** (`PageOptions.Untrusted`): it calls no Go,
    opens no window, downloads nothing and navigates nowhere unless a
    handler of the app allows it. **Content rules** block what a page
    loads (WebKit's content blockers; on WebView2, MyGo fails each blocked
    request). The keys a page leaves unhandled reach the native UI, so J,
    K and Ctrl K keep working while the message has the keyboard; its
    docs use a mail reader as the example. Pages have `FindInPage`,
    `Print`, `PrintToPDF` and `Eval`.
  - `richtext` edits attributed text natively, with lists, quotations,
    links, inline images, undo, paste from HTML and RTF, and export to
    HTML. It has no spell check.
  - `ui.List` and `ui.Table` build only the rows in view (a million-row
    list in about 15 µs a frame), `ui.Calendar` is a month date picker,
    and the kit has token fields, autocomplete, split panes, trees,
    context menus, the tray, notifications, deep links, file
    associations, a single-instance lock, drag and drop of files in and
    out, clipboard formats, and the Secret Service.
  - Its widgets are in the accessibility tree (AT-SPI on Linux, UI
    Automation on Windows), and so is a web view's page.
  - `ui.NewTester` runs a view headless in a unit test: clicks, keys,
    menus, what assistive technology sees, and an image of the frame.
- **The Go client exists.** `api.Client` (generated from `schema/rpc` by
  `tools/rpcgen`) is what `mailctl` uses; a Go app needs no transport of
  its own for a local maild.
- **What stays fixed.** The reader's three layers of safety
  ([ADR-0005](0005-html-mail-rendering.md)): maild's sanitizer, an
  isolated page that runs no script, and no network for the page. Images
  reach it as `data:` URLs ([ADR-0014](0014-images-embedded-in-the-reader-frame.md)).
  Links open in the system's browser and never make a window
  ([ADR-0036](0036-reader-links-open-as-new-window-requests.md)). The
  Flatpak carries its own maild ([ADR-0046](0046-the-desktop-flatpak-carries-its-own-maild.md)),
  and Windows is a supported platform (ADR-0038).

## Decision

- **The desktop app is a Go program drawn with MyGo and frostui.**
  `cmd/frostmail` and its packages under `internal/desktop/` replace
  `app/src-tauri` and the desktop path of `app/src`. The React app stays,
  built only for lodge (`make web-build`): its Tauri transport, its
  desktop-only code and its WebKitGTK end-to-end suite go.
- **It is a thin client of maild through `api.Client`.** A local maild is
  reached over its socket or named pipe, as `mailctl` does; lodge's remote
  mode over lodge's connection, with the same API. The windows are one
  process and share one connection and its stores, where each Tauri
  window had its own. Live views keep
  ADR-0009's model (`view.open`, `view.range`, `view.delta`, a sparse
  window of rows), ported to a Go view model that imports no MyGo and is
  tested with plain `go test`.
- **The reader keeps ADR-0005's three layers.** An HTML body shows in a
  `ui.WebView` whose page is untrusted, loaded with `LoadHTML` from the
  same template as today. Its first element is a CSP that allows no
  script and loads only `data:` images and inline styles; content rules
  block every load but `data:`; `OnWillNavigate` cancels every
  navigation; and the window-open handler opens `http`, `https` and
  `mailto` links in the system's handlers and makes no window. The app
  reads a page's height, its selection (ADR-0053) and its find matches
  (ADR-0052) from Go, with `Eval` and `FindInPage`; the page gets no
  bridge. A text-only body is native text. The hostile-mail suite proves
  zero script and zero network on WebKitGTK and on WebView2 before the
  reader ships.
- **Compose is native.** The body is a `richtext` editor limited to what
  the TipTap editor allows today (paragraphs, bold, italic, underline,
  strikethrough, lists, quotations, `http`, `https` and `mailto` links,
  and images of `mailpart:` and `data:` URLs). A draft is still maild's
  HTML ([ADR-0010](0010-compose-and-send.md)), read with `ParseHTML` and
  saved with `HTML`. The window's chrome (identities, address tokens with
  suggestions, subject, attachments, Send) is native.
- **The window follows frostui's shell.** A hidden title bar with
  frostui's top bar, the command palette, the status bar, toasts and
  dialogs, and frostui's theme in light and dark. Mail.app's three panes
  (mailboxes, list, reader) and the module bar (Mail, Calendar, People,
  Tasks; [ADR-0020](0020-one-window-for-mail-calendar-people-and-tasks.md))
  replace ChairLift's page rail. ChairLift's console shows maild's
  background activity ([ADR-0055](0055-maild-streams-its-activity-as-typed-entries.md));
  its job queue is not used, since maild owns the work.
- **Generic patterns wait in one package.** UI patterns that other
  frostyard apps could use go in `internal/desktop/uikit`, which imports
  only the standard library, MyGo, purego and frostui (a test enforces
  it), so they can move into frostui by changing an import path. The
  first is a shell without the page rail: frostui's `Shell` puts its
  content in a padded scroll capped at 1,320 wide, which three
  full-height panes cannot use. Frostmail never edits frostui from this
  repository.
- **Accessibility is tested in Go.** Views name every control, and
  `ui.NewTester` tests check roles, names and states with `AccessByRole`
  and `Access`, as the axe audit of [#222](https://github.com/frostyard/frostmail/pull/222)
  does for the web build, and carry its fixes (the sidebar's tree items,
  the task rows' state, the address buttons' and calendar cells' names).
  Orca and Narrator passes stay manual.
- **Packaging keeps its contracts.** The Flatpak keeps its ID, its maild
  and mailctl, its launcher and its modes (ADR-0046); its app module
  becomes a Go build from `go-sources.json`, so it needs no Rust or Node.
  On Windows, MyGo's per-user NSIS installer, with installer hooks, carries
  maild, mailctl and the notifier, and registers what Tauri's did.
- **Windows keeps its notifier.** MyGo shows notifications on Windows as
  tray balloons without actions, so `frostmail-notify.exe` (ADR-0038)
  keeps posting toasts, with Snooze and Dismiss, for maild and for the app
  in lodge mode. On Linux the app's own notifications (lodge mode) use
  MyGo's, through the desktop's notification service.
- **Tauri stays until parity.** The native app is built beside the Tauri
  shell, and the PR that removes `app/src-tauri` is the one whose parity
  list ([specs/parity.md](../specs/parity.md)) has no desktop gap.

## Consequences

- One look across frostyard's desktop apps, and a desktop app written in
  the language the rest of Frostmail is written in. The desktop no longer
  needs Rust (Tauri) or Node at build time; the Flatpak's sources shrink to
  Go's.
- **Two UIs to keep in step.** Every UI feature is specified once (the
  specs stay the contract) and built twice: in Go for the desktop, in
  React for lodge. specs/parity.md gains a column per UI.
- The reader still runs in the system's web view, so its safety still has
  to be proved on WebKitGTK and WebView2 and kept proved as they update.
  The CSP moves from the app's configuration into the page's template.
- `richtext` keeps all of TipTap's schema, so a reply loses no more of a
  quoted message than it does today: both flatten its tables, colors and
  layout. Spell check, which WebKitGTK gave the TipTap editor, has to be
  built or found; MyGo has none.
- Find in a conversation and Print span several pages and native text,
  where the React app searched and printed one document.
- The calendar's day, week and month views are drawn by the app; MyGo's
  `Calendar` is only the month picker.
- MyGo is young, with one main author. The app pins it in `go.mod` and
  keeps frostui between its views and MyGo where it can. It needs Go
  1.27.3, which `go.mod`'s `go` line moves to.
- Frostmail is the suite's first MyGo app in a Flatpak and on Windows:
  ChairLift ships a system package for Linux only. Plan 0015's Phase 0
  checks both before the port depends on them.
- Unit tests of views need Pango and cairo (no display); CI installs them.
- The WebKitGTK end-to-end suite (`app/e2e`) and #222's axe audit move
  to the web build; the desktop's end-to-end checks drive the built app
  through MyGo's automation build, since `ui.NewTester` cannot see inside
  a web view's page.

## Alternatives considered

- **Keep Tauri and React on the desktop:** one UI for both hosts, but the
  desktop keeps a Rust shell and a look apart from the suite's, which is
  what the owner chose to leave.
- **A MyGo window hosting the React UI in a web view:** MyGo supports a
  web frontend with bound Go methods, so this keeps one UI, but it is
  Tauri's shape in another shell: no native widgets, no shared frostui
  look, and Node stays in the desktop build.
- **Compose in a web view with the current TipTap editor:** keeps
  WebKit's spell check, but keeps a JavaScript bundle, its toolchain and
  a trusted page with a bridge in the desktop app, for one window, and
  keeps no more of a quoted message than `richtext` does.

## References

- Shapes: [design/app.md](../design/app.md),
  [design/desktop.md](../design/desktop.md),
  [design/rendering.md](../design/rendering.md),
  [design/testing.md](../design/testing.md),
  [specs/ui.md](../specs/ui.md), [specs/parity.md](../specs/parity.md),
  [plans/0015](../plans/0015-mygo-desktop.md)
- Builds on: [ADR-0002](0002-daemon-and-thin-clients.md),
  [ADR-0005](0005-html-mail-rendering.md),
  [ADR-0010](0010-compose-and-send.md),
  [ADR-0014](0014-images-embedded-in-the-reader-frame.md),
  [ADR-0020](0020-one-window-for-mail-calendar-people-and-tasks.md),
  [ADR-0038](0038-windows-as-a-second-platform.md),
  [ADR-0041](0041-maild-runs-as-a-lodge-app.md),
  [ADR-0046](0046-the-desktop-flatpak-carries-its-own-maild.md)
- Supersedes in part, on acceptance (each old ADR's status says which
  part): ADR-0002's Tauri bullet, [ADR-0009](0009-ui-architecture.md) for
  the desktop app, ADR-0005's iframe and the CSP's place, the mechanism of
  [ADR-0036](0036-reader-links-open-as-new-window-requests.md) on the
  desktop (its rule holds), ADR-0038's Tauri installer, and
  [ADR-0045](0045-the-desktop-app-builds-as-a-flatpak-in-ci.md)'s Tauri
  module
- Elsewhere: ChairLift's ADR-0017 and `docs/design/frostui.md`
  (frostyard/chairlift#282)
