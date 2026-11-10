# Plan 0015: The desktop app in MyGo

Port Frostmail's desktop app from Tauri and React to a native Go app drawn
with MyGo and frostui ([ADR-0054](../adr/0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md)),
on Linux and Windows. The React app stays as lodge's web app
([plan 0011](0011-m8-mail-in-lodge.md)). The orchestrating session's brief
of 2026-11-10 sets the rules: two UIs, thin clients, the suite's look, and
no ChairLift pattern where it does not fit.

- **One PR, a draft until parity.** The native app is built beside the
  Tauri shell on one branch, phase by phase. The branch is pushed and the
  PR opened as a draft once Phase 2 passes the gates; it stays a draft,
  with its gap list in its body, until the parity gate (Phase 10) passes,
  and only then deletes `app/src-tauri`. A human merges it.
- **frostui is a dependency, built elsewhere.** The orchestrating session
  builds the pieces Frostmail and Frostnotes share in
  `github.com/frostyard/frostui` ([below](#patterns-for-frostui)) and
  names a pseudo-version (a pushed commit) when they land; `go.mod` pins
  it. Commits carry no `go.work` and no `replace`; a local `go.work` on
  the frostui checkout is for experiments only. This repository never
  edits frostui.
- **Order** (the orchestrator's notes of 2026-11-10): first what needs no
  shared piece: Phase 2's maild side, the pure packages `conn`, `live`
  and `commands` of Phase 1, and Phase 0's spikes; the windows follow
  frostui's pseudo-versions.
- **Gates for every phase:** `make check` (with the view tests, which need
  Pango and cairo), a Windows cross-build of `cmd/frostmail`, and
  `make ui-test` for the web build while the React app changes. A phase
  that touches the reader also runs the hostile and link suites on
  WebKitGTK and WebView2.
- **Parity means today's desktop app,** as the inventory below records it:
  what it lacks today (a tray, an unread badge, a `mailto:` handler, `.eml`
  files, dragging attachments out, search tokens) stays out of this plan.

Status (2026-11-10): Phase 0 is in review (this plan, ADR-0054 and
[ADR-0055](../adr/0055-maild-streams-its-activity-as-typed-entries.md),
Proposed). Phase 0's Linux and Flatpak spikes are run and recorded; its
Windows spike waits for the test VM. Phase 1's window runs on Linux
against a real maild, over `internal/desktop/frame`, a stand-in for
frostui's coming frame. Phase 2's maild side and Phase 3 (sidebar, list, search) are built. The console of maild's activity (Phase 2) was
added on 2026-11-10 at the owner's request, relayed by the orchestrating
session ("it'd be fun to use that console view from chairlift to show
background activity in frostmail").

## The window

```
┌──────────────┬────────────────────┬────────────────────────────────────────────────────┐
│ ◧ ↻          │ Inbox  12 unread ✎ │ ⌫ ▣ ↩ ↩↩ ↪ ⚑▾ ✉ ⇥▾ » ☰ ⌕ Search or run a command ⌃K ▤ ⋮ – □ ✕ │ top bar 52
├──────────────┼────────────────────┼────────────────────────────────────────────────────┤
│ Favorites    │ ⌕ Search Inbox     │ (A) Sender                                   date  │
│  All Inboxes │ All Unread Flagged…│     Subject                                        │
│  Flagged     │ ● Sender     9:41  │ ────────────────────────────────────────────────── │
│ Work         │   Subject       3  │ web view (an untrusted page)                       │
│  Inbox    12 │   preview…         │                                                    │
│ ✉  ▦  👥  ☑  │ …                  │                                                    │
├──────────────┴────────────────────┴────────────────────────────────────────────────────┤
│ Activity  Info ✓ Warn ✓ Error ✓ Debug   ⌕ filter            (Ctrl J; closed by default) │ console
│ 9:41:02  Work   Inbox synced: 3 new, 12 changed (0.4 s)                                  │
│ 9:41:05  Work   sent 1 message (smtp, 1.2 s)                                             │
│ 9:41:30  Home   CardDAV pass: 2 changed                                                  │
├────────────────────────────────────────────────────────────────────────────────────────┤
│ ● Local · Work: Inbox synced 9:41 · Home: offline · Outbox 1    2 selected · 1,204 messages │ status bar 24
└────────────────────────────────────────────────────────────────────────────────────────┘
```

- **Hidden title bar** (`TitleBarHidden`), as ChairLift's: the top bar is
  the drag region, a double click maximizes, and the system's window
  controls sit in `c.TitleBar()`'s room (GTK's on Linux; MyGo's Windows 11
  caption buttons on Windows, which replace lodge-ui's `WindowControls`).
  The bar is 52 high, frostui's `TopBarHeight`.
- **The top bar is Mail.app's unified toolbar**, as today
  ([specs/ui.md](../specs/ui.md#layout)), in three segments that follow
  the panes' widths: the sidebar's (Show Sidebar, Get Mail), the list's
  (the source's title and subtitle, New Message) and the reader's (Delete,
  Archive, Reply, Reply All, Forward, Flag ▾, Mark, Move ▾ and », in the
  code's order; then the To-Do Bar toggle, the palette field, the
  console's button, the main menu and the window controls). Calendar, People and Tasks put their own
  actions in the same segments (New Event, Today, ‹ ›, the title, Day /
  Week / Month; New Contact; New Task, Show Completed).
- **The palette field** ("Search or run a command") is frostui's. Ctrl K
  opens the palette, as in ChairLift; Ctrl+Shift+P, today's key
  ([specs/ui.md](../specs/ui.md#command-palette)), stays as a second key.
  Ctrl+K is free in the main window today; the compose window keeps it
  for Link and opens the palette with Ctrl+Shift+P
  ([rule 1](#rules-shared-with-frostnotes)).
- **The mail search field** moves from the toolbar to the top of the list
  pane, over what it filters (as the narrow layout already places it), and
  the filter row, the sort menu, the scope bar and the coverage line stay
  under it. Ctrl+Alt+F, Ctrl+F with no message shown, and the palette's
  Search focus it.
- **The main menu** (frostui's `Frame.Menu`, ⋮) replaces the Settings
  button: Settings… (Ctrl+,), Server…, Keyboard Shortcuts (frostui's `Info`
  with `ShortcutsBody`), About Frostmail.
- **The status bar** (24 high, `RoleStatus`), new for Frostmail: at its
  start the connection (Local, or lodge's host; "Reconnecting to maild…"
  in the warning tone, with the reason in its tooltip), each account's
  sync (a spinner and its progress, Offline, Read-only, or an error that
  opens Settings › Accounts), with the account's newest activity as its
  text (a click opens the console), and the Outbox when it holds mail (it
  selects the Outbox). At its end the list's counts (selected, messages,
  unread), or the visible range in Calendar. The sidebar keeps its
  per-account spinner and alert icon.
- **The console** is frostui's `Console` and `ConsoleView`, as in
  ChairLift, under the panes in a vertical split: maild's background
  activity ([ADR-0055](../adr/0055-maild-streams-its-activity-as-typed-entries.md)),
  one line per mailbox pass, IDLE wake, fetch, push of flag changes, send,
  Send Later, Remind Me, rule run, CalDAV, CardDAV, Graph or Google Tasks
  pass, token refresh and failure, each with its time, level and source
  (the account's name, or the service). frostui's level toggles and text
  filter apply. It is closed by default and opens with Ctrl J (free
  today; Ctrl+Shift+J is Junk), the top bar's button and the palette's Show Activity. The app
  adds lines of its own about its connection to maild.
- **The module bar** stays at the foot of each sidebar
  ([specs/pim-ui.md](../specs/pim-ui.md#module-bar)); it is Frostmail's
  rail, and frostui's page rail is not used.
- **Toasts** are frostui's (Undo, Undo Send with its seconds, the refused
  changes' notices), **confirmations** are frostui's `Dialogs`, and the
  **sheets** are MyGo `Modal`s with lodge-ui `Sheet`'s look and Escape.
- **Other windows** (compose, event, settings, reminders) have the same
  hidden title bar and a top bar with their own actions, and no status
  bar.

### Rules shared with Frostnotes

The orchestrating session's consistency notes of 2026-11-10; both apps
follow them:

1. **Palette keys.** Ctrl K opens the palette in every main window, except
   inside a text editor (compose here, Frostnotes' editor), where Ctrl K
   is Insert Link. Ctrl+Shift+P opens the palette everywhere, editors
   included, so the compose window gains it.
2. **Status bar order.** The connection item first and always shown (a
   dot with "Local" or lodge's host; the warning tone, with the reason in
   its tooltip, while reconnecting); then sync and background state; then
   the app's items; the current view's counts at the end.
3. **Top bar.** The palette field is the same widget everywhere ("Search
   or run a command" with ⌃K); the main menu (⋮) is the last item before
   the window controls; the sidebar toggle is at the far left. Frostmail
   keeps its segmented, Mail.app-style toolbar between them.
4. **Console.** Frostmail's is the reference; a console in another app
   reuses ADR-0055's typed entries.

### Palette commands

One command table drives the keys, the menus and the palette
([specs/ui.md](../specs/ui.md#keyboard-map); `PALETTE` and `BINDINGS` in
`app/src/lib/keymap.ts` today), so each command has one ID, title, group,
keys and enabled rule, and the palette lists the current module's:

- **Message:** New Message, Reply, Reply All, Forward, Forward as
  Attachment, Redirect…, Archive, Delete, Mark as Spam / Not Spam, Mark as
  Read / Unread, Flag / Unflag, Flag Color ▸, Move to…, Copy to…, Mute /
  Unmute, Remind Me…, Apply Rules, Load Remote Content, Raw Source…, All
  Headers, Save As…, Print…, Find in Message.
- **Go to:** each favorite, mailbox and smart mailbox by name, Follow Up,
  Outbox, Send Later, Mail (All Inboxes), Calendar, People, Tasks.
- **View:** Show Sidebar, To-Do Bar, Conversations, Filter ▸, Sort ▸,
  Today, Day / Week / Month, Previous / Next Period, Show Completed.
- **Organize:** New Smart Mailbox…, New Mailbox…, New Rule…, New Event,
  New Contact, New Task.
- **App:** Get Mail, Search, Show Activity, Settings (each pane),
  Server…, Keyboard Shortcuts, About Frostmail.

## Inventory and mapping

What the desktop app does on 2026-11-10 (`c3ce9989`), and what each piece
becomes. "frostui" means the kit's API as of its extraction (`feat/extract`);
"uikit" means `internal/desktop/uikit` ([below](#patterns-for-frostui)).

### Windows

| Today | Native | Notes |
| --- | --- | --- |
| `main`, 1200 × 780, min 800 × 500, undecorated, shown after its first render | `mygo.NewWindow`, `TitleBarHidden`, `StateKey`; shown without a flash (MyGo's `docs/windows.md`) | the single-instance lock and `--open-message <id>` |
| `compose-<draftId>`, 720 × 560, min 480 × 360; focused if open | a window per draft, kept in a map | in-process, so no init-script globals |
| `event-<id>[-<occurrence>]`, `event-new-<ms>`, 480 × 600 | a window per event | the "Save changes?" prompt becomes a `Dialogs.Ask` |
| `settings`, 760 × 560, one; closes with `main`; works with no maild | one window; panes as a `Sidebar` | the `settings-pane` event becomes a method call |
| `reminders`, 380 × 300, always on top | a window with `AlwaysOnTop` | raised on `calendar.reminders` |
| Tauri events and `BroadcastChannel` between windows (`open-message`, `open-occurrence`, `settings-pane`, PIM restore) | calls inside one process | windows share one connection and the stores |
| `Connecting`, "Reconnecting to maild… (reason)", `LeftMaild` | the status bar's item and an empty state; windows of a left maild close | |

### The main window

| Today | Native | Notes |
| --- | --- | --- |
| Three panes with lodge-ui `Splitter` (220 [160–320], 360 [280–560], reader min 360; arrows move 16) | frostui's `Panes` over `ui.Split` | widths kept with MyGo's window state, not local storage |
| Persisted UI (`frostmail.ui`): sidebar, widths, conversations, sort, ascending, contact photos, To-Do Bar | a small JSON file in the app's config | |
| Sidebar `role=tree`: Favorites (All Inboxes, unified roles, VIPs, Flagged per color, favorites), Smart Mailboxes, accounts' trees with counts and sync state, Follow Up, Outbox, Send Later, the module bar | frostui's `SourceList` over `ui.Outline` | drop targets (move; Alt or Ctrl copies); PR #222's tree roles |
| Filter row (All, Unread, Flagged, Attachments, More ▾), sort menu, scope bar, coverage line | `Segmented`, `MenuButton`, `Banner`-like rows | overflow into More as today |
| Message list: virtual, 84 px rows, multi-select, unread dot, flag, VIP star, muted, paperclip, reminder, thread count, two preview lines, contact photo, hover actions, drag, Not Yet Saved drafts, empty states | `ui.List` with `ListState.Selection`, uikit `SummaryRow`, `Drag` | hover actions hidden from assistive technology (PR #222) |
| Reader: conversation, newest first, 20 then "Show N Earlier", collapsed rows, header, six banners, body, All Headers, attachment chips | native header and banners, a web view per expanded HTML body, native text for text bodies | [hard part 1](#hard-parts) |
| To-Do Bar (280, Mail only): mini month, Upcoming, tasks and flagged mail | `ui.Calendar` (browse), lists | |
| Calendar: Day, Week, Month; mini month and calendars' checkboxes; time grid, all-day strip, now line, month grid, event pane (320); drag in 15-minute steps to move, resize and draw; month and all-day drag; double click to create | drawn by the app with `Box`, `Draw` and pointer drags | [hard part 3](#hard-parts) |
| People: address books, a lettered list with search, the person pane (contact, Recent Mail, Upcoming, Message), inline editor, photo well cropping to a 512 px JPEG | `Outline`, `List` with section headers, `Form`, `image/jpeg` | |
| Tasks: Today, All Tasks, Flagged Mail, lists; New Task field, subtasks, check circles, Show Completed; the task pane | `List`, `DateInput`, `Form` | PR #222's roles (`aria-checked` on the row) |

### Sheets, dialogs, popovers and toasts

| Today | Native |
| --- | --- |
| lodge-ui `Sheet`s: smart mailbox and rule (shared `ConditionEditor`, 22 fields; `ActionEditor`, 7 actions), mailbox (new, rename, move), time (Remind Me, Send Later, Change Time), Redirect, Raw Source, attachment reminder, unsubscribe confirm, repeat, scope (recurring edits), notify invitees | `ui.Modal` with a `Form`; Raw Source with frostui's `CodeDialog` look |
| The command palette (`CommandPalette.tsx`) | frostui `Palette` |
| Native `ask` confirmations (delete draft, mailbox, account, rule, snippet, address, contact; erase deleted, erase junk) | frostui `Dialogs.Ask` with `Destructive` |
| Native open and save dialogs; the Flatpak's save portal (`portal.rs`) | `mygo.Dialog.Open`, `Save` (off the UI thread); the portal in the Flatpak, [Phase 0](#phase-0--decisions-and-spikes-planner-m) checks which GTK's chooser uses |
| Contact popover (Message, Add to Contacts, VIPs, Block, Recent Mail, Upcoming); recipient suggestions | `ui.Popover`; uikit `TokenSuggest` |
| Find bar, compose link bar, compose error banner, scope bar | `ui.FindBar`, a `Popover` with `SetLink`, frostui `Banner` |
| Undo toasts (Sending "…", Undo, seconds left), refused-change notices (8 s) | frostui's `ActionToast` and toasts |

### Menus and keys

| Today | Native |
| --- | --- |
| lodge-ui `Menu`s: the list row's (reply family, Archive, Delete, Spam, Move ▸, Copy ▸, Flag Color ▸, Read, Mute, Remind Me ▸, Apply Rules), mailbox, favorite and smart mailbox rows, the reader's More Actions, the toolbar's Flag ▸ Move ▸ », Filter More, Sort, Send Later ▾, Snippets ▾, Snooze ▾, calendar rows | `ContextMenu` and `MenuButton` from the command table |
| The web view's own menu blocked except in text fields | `Page.OnContextMenu` keeps Copy and Select All only |
| `BINDINGS` (about 60 keys; Meta never bound; text fields pass only Escape, Tab, Ctrl+1–4, Ctrl+, and Ctrl with Alt or Shift) | `c.OnWindowShortcut` and `Element.Shortcut` from the command table, with the same text-field rule |
| Tab and Shift+Tab move between panes | a focus group per pane |
| Calendar keys (Ctrl+Alt+1–3, Ctrl+T, Ctrl+← →, arrows), compose keys (Ctrl+Enter, Ctrl+Shift+A, Ctrl+Shift+B, Ctrl+Backspace, Ctrl+W, Ctrl+K), recipient keys (`,` `;` Enter Tab, Backspace) | the same; `richtext`'s own keys checked against compose's |

### Compose

| Today | Native | Notes |
| --- | --- | --- |
| TipTap: paragraphs, bold, italic, underline, strike, bulleted and numbered lists, quote, links (`http`, `https`, `mailto`, autolink), images from `mailpart:` and `data:` only; no headings, code or rules | `richtext.Editor` with headings, code and tasks turned off, `OnLink` limited to the same schemes | `richtext` keeps all of TipTap's schema; [hard part 2](#hard-parts) |
| HTML only; maild builds the text part, the quote and the signature | `ParseHTML` in, `HTML` out to `draft.update` | maild unchanged |
| Saved 500 ms after an edit, flushed on close; a fresh draft closed untouched is deleted | the same, with a timer | |
| Recipients: tokens, `address.suggest` (8), Bcc toggle, From with several identities | `TokenField`, uikit `TokenSuggest`, `Select` | |
| Attachments: picker (paths, or bytes over `draft.attachData` in lodge mode and the Flatpak), drop anywhere, Remove, 25 MB warning | `Dialog.Open`, `OnFileDrop` | |
| Snippets ▾, Send Later ▾, attachment reminder, Delete Draft, Send then Undo in the main window | the same | |
| Spell check (subject and body; WebKitGTK's, set by `spell.rs`) | none in MyGo | [Phase 11](#phase-11--spell-check-m-after-parity), after parity |

### The reader

| Today | Native |
| --- | --- |
| `<iframe sandbox="allow-same-origin allow-popups" srcdoc>` around maild's sanitized HTML, under the app's CSP (no `connect-src`; images `mailpart:` and `data:`) | an untrusted `ui.WebView` page loaded with `LoadHTML`, the CSP as the template's first element, content rules, navigation and window-open handlers |
| Images as `data:` URLs (ADR-0014); remote content re-renders with `remote: true` | unchanged |
| Links: new-window requests to `open_link` (`http`, `https`, `mailto`), never a window (ADR-0036) | `SetWindowOpenHandler` and `OnWillNavigate` to `mygo.Shell.OpenExternal`; nil, so no window |
| The frame sized to its content (`ResizeObserver`) | `ScrollToContainer` and the height read with `Eval` after load and on resize |
| Text-only bodies as React (`PlainText`: quote bars, "See More", dimmed signature, links) | native text with the same rules |
| Find: the CSS Custom Highlight API across the app and each frame (ADR-0052) | `FindInPage` on each page plus a native search of text bodies, one count |
| Reply with the selection (ADR-0053) | `Eval` of the page's selection; `TextSelection` of a text body |
| Print: `window.print()` with print styles that leave the reader | a print page (headers and body) in an untrusted page, then `Page.Print` |
| Attachments: `open_part` (parts cache, or the content node in lodge mode), refusing executable extensions | the same rules in Go; `mygo.Shell.OpenPath` |
| Save As (`.eml` only): `message.save`, or `save_content` in lodge mode | the same |

### Settings, first run and modes

| Today | Native |
| --- | --- |
| General (notification scope, undo delay, flag names, blocked mail and senders), Accounts (identity, sign-in, servers, options, keep mail, erase old Trash and Junk, Contacts, Calendars, Tasks), Signatures, Snippets, Rules, Sign-In (Google client), Server | the same panes as `Form`s; frostui `SwitchRow` where a pane has switches |
| OAuth: `account.authorize` opens the browser; a pasted address finishes (ADR-0043) | `mygo.Shell.OpenExternal`, the same paste field |
| First run (`FirstRun.tsx`): Add Account…, Connect to lodge…, the Flatpak move note | a welcome view (frostui `Empty`, `Banner`) |
| Modes (`mode.rs`, `remote.rs`): `app.json`, the ordered change of mode, `server_facts`, `host_mail_found` | `internal/config` already reads `app.json`; the ordered steps ported to Go |

### The shell's Rust side (`app/src-tauri`, 3,838 lines, 68 tests)

| Today | Native |
| --- | --- |
| `bridge.rs`: a line bridge per window to the socket or pipe (maild started on Windows when no pipe), or `wss://LODGE/app/mail/api` with Origin; `bridge.socket_lost` | `api.Dial`, or the kit's JSON-RPC client over `coder/websocket`; one connection per process |
| `protocol.rs`: `mailpart://` from the parts cache | not needed: the reader uses `data:` images, and compose loads parts itself |
| `remote.rs`: `app.json`, lodge's address rules, `contentOrigin` from `/lodge/apps`, downloads (120 s, no redirects, 128 MiB, safe names) | `internal/desktop/conn` with `net/http` |
| `mode.rs`, `paths.rs` | `internal/config` (paths exist) and the ported steps |
| `portal.rs` (FileChooser SaveFile over D-Bus) | GTK's portal chooser, else `godbus` as maild uses |
| `announce.rs` (`notify_mail` in lodge mode: D-Bus with Open, or `frostmail-notify.exe`) | `mygo.NewNotification` on Linux; `frostmail-notify.exe` on Windows |
| `spell.rs` (WebKitGTK spelling languages) | see compose |
| `problems.rs` (`frostyard-problems`, component Shell) | `internal/problemrec` (std's `problems` in Go) |
| Plugins: single instance, opener, dialog; `FROSTMAIL_WEBKIT_SAFE`, `FROSTMAIL_OPEN_LOG`, `FROSTMAIL_DEVTOOLS_PORT` | MyGo's lock, shell and dialogs; `App.SetDMABUFRenderer`; the open log kept for tests |

### Notifications, deep links and files

| Today | Native |
| --- | --- |
| Local mode: maild notifies over D-Bus; a click runs `frostmail --open-message <id>` (`open.sh` in the Flatpak) | unchanged; the native app takes `--open-message` |
| Lodge mode: `message.announced` → `notify_mail` | `mygo.NewNotification` with an `ID` per message; Windows through the notifier |
| Windows: `frostmail-notify.exe` (Rust, WinRT), `frostmail-toast:` | kept (MyGo shows balloons without actions) |
| No tray, no badge (P-113 Later), no sounds, no `mailto:` handler, no `.eml` association, no drag out | none added here |

### Packaging, CI and tests

| Today | Native |
| --- | --- |
| Flatpak module `frostmail`: `pnpm tauri build`, node24 and rust-stable, `cargo-sources.json`, `node-sources.json`, `cargo test` | a Go build from `go-sources.json`; the extensions and two sources files go; `go test` of the pure packages |
| `stamp-version.sh` stamps `tauri.conf.json` | stamps an `-X` version in the manifest's Go build |
| Windows: Tauri's per-user NSIS, `hooks.nsh` (AppUserModelID, `frostmail-toast:`, maild's autostart, the uninstaller's data choice), sidecars | MyGo's per-user NSIS with `installerInclude` macros; maild, mailctl and the notifier as helper executables; taking over a Tauri install |
| CI `app` (WebKitGTK, Rust) and `windows-app` (Rust, NSIS, WebView2 e2e) | Go builds; tester tests with Pango, cairo and fonts; the automation build's e2e under Xvfb and on Windows |
| `app/e2e` (WebKitWebDriver; smoke, compose, firstrun, hostile, links, lodge, mode, narrow, remote, scroll, window, screenshots) | the hostile, links, remote, compose, mode and smoke checks rewritten for the native app; lodge and narrow stay with the web build |
| Vitest: 287 files, about 1,470 tests | the React app keeps those of the web build; the desktop's are Go |
| `make screenshots`: 14 PNGs in `docs/images` (README, metainfo) | the native app's frames, with the reader's page captured by `WebView.CapturePage` |
| `internal/workflowcheck` (`governedPaths`, `TestRiskPaths`, `TestDependabotLeavesInternalGitCratesAlone`), `scripts/flatpak_stamp_test.go`, `.github/risk-paths.json`, `.github/dependabot.yml` | changed with the files they name |

## Hard parts

1. **HTML mail in a web view.** MyGo has no switch for JavaScript and no
   CSP setting, so the page's safety is the app's: an untrusted page, a
   CSP meta as the template's first element, content rules (emulated on
   WebView2, where WebSockets and popups pass them; the CSP still stops
   both), and handlers that cancel every navigation and window. Each
   expanded message is its own web view: a conversation of 20 needs
   measuring (memory, the WebKitGTK web process). Linux pages draw on the
   CPU unless `SetDMABUFRenderer(Auto)`. Find and Print span several pages
   and native text. The tester cannot see inside a page, so the reader's
   checks run in the built app.
2. **Compose.** `richtext` holds what TipTap's schema holds, except a
   quotation inside a quotation, which `ParseHTML` merges into 1 level
   ([Phase 0's records](#phase-0s-records-2026-11-10)). A small converter
   keeps the depth in the paragraph's `Indent` and writes nested
   `<blockquote type="cite">` back. `mailpart:` images in a draft must be
   loaded for display and written back as their URLs, at their real
   size; pasted remote images must be dropped, as `SafeImage` does; and
   MyGo has no spell check.
3. **The calendar.** `ui.Calendar` is a month picker. The time grid
   (overlaps, the all-day strip, the now line), the month grid and their
   drags (move, resize, draw, in 15-minute steps) are the app's own
   drawing and pointer code, with keyboard equivalents and names for
   assistive technology (PR #222's full-date names).
4. **Accessibility.** MyGo gives AT-SPI and UI Automation trees, but the
   names and roles PR #222 fixes for axe (tree items holding their
   buttons, options carrying completion, the address buttons' names, the
   calendar's dates) must be rebuilt in the views and tested with
   `AccessByRole`. On Windows a web view's page sits in a window of its
   own for Narrator. Reduced motion is MyGo's (`SetPreferences`).
5. **First of its kind.** ChairLift ships neither a Flatpak nor a Windows
   build; Frostmail is the suite's first MyGo app in both, and frostui's
   icons read freedesktop themes that Windows lacks.

## No MyGo equivalent

- Spell check (none anywhere in MyGo).
- Day, week and month event views (only the month picker).
- Windows toasts with actions (balloons only); the notifier stays.
- A page's selection, height and CSP (through `Eval` and the template).
- Testing inside a web view's page with `ui.NewTester` (`WebViewAt` only).
- A shell without the page rail and with full-height content (frostui's
  `Shell` scrolls its content in a padded column capped at 1,320).
- lodge-ui's React parts (`Sheet`, `Menu`, `Splitter`, `WindowControls`)
  have MyGo counterparts, not equivalents: their look is rebuilt.

## Phase 0 — Decisions and spikes (planner, M)

- ADR-0054 and ADR-0055 accepted; the old ADRs' statuses updated in the
  same change.
- **Reader spike:** an untrusted `WebView` showing the hostile corpus
  (`app/e2e` and `internal/render`'s fixtures) through the reader's
  template: a CSP meta first (`default-src 'none'; img-src data:;
  style-src 'unsafe-inline'`), content rules that block every URL but
  `data:` and `about:`, `OnWillNavigate` cancelling, and a window-open
  handler that opens `http`, `https` and `mailto` through the system and
  makes no window. Run it against a canary server on WebKitGTK (the host
  and `org.gnome.Platform//51`) and on WebView2 (`frostmail-win11`), once
  with maild's sanitizer and once without it. Measure 20 web views in one
  conversation.
- **Eval spike:** `Eval` reads the page's height and selection, and
  `FindInPage` finds, under that CSP, on both engines.
- **Compose spike:** 30 reply and forward drafts (the showcase's and
  `internal/render`'s fixtures) through `richtext.ParseHTML` and `HTML`,
  compared with what TipTap keeps of the same drafts.
- **Flatpak spike:** a MyGo window with a web view in the GNOME 51
  runtime: GTK 3, libepoxy and WebKitGTK 4.1 present, the Wayland app ID,
  the hidden title bar on GNOME and KDE, the file chooser through the
  portal, notifications.
- **Windows spike:** `cmd/frostmail` cross-built with `CGO_ENABLED=0`, run
  on `frostmail-win11`: the title bar, the named pipe, a WebView2 reader,
  frostui's icons.
- **Specs follow the code** where the inventory found them apart:
  `tokens.css` (gone), the toolbar's action order, the keyboard table's
  missing keys (Ctrl+2–4, the calendar's, Menu and Shift+F10) and its
  "anywhere" for Ctrl+F and Ctrl+P, Tab on a collapsed message, design/app.md's
  stores and persisted state, and `open_part`'s refused extensions.
- **Done when:** the spikes' records are in this plan with their observed
  counts (requests, scripts run, losses per draft, memory of 20 views).

### Phase 0's records (2026-11-10)

Throwaway programs, not committed. MyGo v0.8.3.

**Reader, on Linux: run.** The 50 files of `internal/render/testdata/hostile`
without maild's sanitizer, plus 1 script probe, each loaded with `LoadHTML`
in an untrusted web view, against a canary server on loopback. Handlers
cancelled every navigation and made no window. Run on the host (WebKitGTK
2.54.0) and as a command inside the installed Frostmail Flatpak's sandbox
(GNOME 51, `libwebkit2gtk-4.1.so.0.24.4`); the two runs' counts were the
same:

| Defenses | Canary requests | Probe script | Navigations cancelled |
| --- | --- | --- | --- |
| None | 33, from 27 files | ran | 3 |
| CSP meta only | 0 | did not run | 1 |
| Content rules only | 0 | ran | 3 |
| CSP meta and content rules | 0 | did not run | 1 |

No window was opened and no load timed out in any variant. Under the CSP,
`Eval` read the height (1023) and a selection (`alpha needle beta`), and
`FindInPage` found 2 of 2 matches.

- Run in the bare `org.gnome.Platform//51`, with no app, the spike aborted:
  GTK's load of an icon reported that glycin's loader, started through
  `flatpak-spawn --sandbox`, exited early. It did not abort inside the
  Frostmail Flatpak's sandbox.
- MyGo's default session wrote `cookies.sqlite`, `hsts-storage.sqlite`,
  storage and compiled content rules under directories named after the
  program. Message pages use `mygo.NewEphemeralSession()`.
- `CompileContentRules` panics before the app is ready and blocks on the
  main thread: it runs from a goroutine, after `WhenReady`.

**Twenty web views, on the host.** Each held a message of about 40 KB,
5 s after loading; PSS of the process and its children:

| Web views | Processes | PSS |
| --- | --- | --- |
| 1 | 3 | 243 MB |
| 5 | 7 | 395 MB |
| 20 | 22 | 905 MB |

**Compose.** The 6 HTML mails in the repository's fixtures
(`dev/incus/seed/04`, `dev/incus/demo/01` and `06`, the showcase's INBOX
`03`, `04` and `06`), unsanitized, each as a reply's quote, through
`richtext.ParseHTML` and `Text.HTML`. That is fewer than the 30 this
phase asked for; the repository holds no more. Before and after:

| Kept | Before | After |
| --- | --- | --- |
| Words | 297 | 297 |
| Links | 6 | 6 |
| Images | 6 | 6 |
| Quotations | 6 | 6 |
| Headings | 4 | 4 |
| Tables | 6 | 0 |
| Elements with a color | 18 | 0 |

A hand-made draft showed 3 more things: `type="cite"` is dropped (the
TipTap editor's blockquote has no such attribute either); a quotation
inside a quotation merges into 1 level; and a `mailpart:` image with no
loaded picture is written with `width="1" height="1"`. What Phase 5 does
about each is in its work items.

**Windows: not run.** `frostmail-win11` does not start: its `virtio` disk
names `/home/bjk/Downloads/virtio-win-0.1.302.iso`, which no longer
exists. The WebView2 reader run and the Windows spike wait for that.

## Phase 1 — The skeleton and the data layer (L)

- `cmd/frostmail` and `internal/desktop/` (layout [below](#code-layout)):
  the app ID before the display (`frostui.SetAppID`), the single-instance
  lock with `--open-message <id>`, production build flags
  (`-tags mygo_noinspector`, `mygo.production=1`), and problem records
  through `internal/problemrec` (component `shell`; `bridge.socket_lost`
  keeps its name).
- **The connection** (`internal/desktop/conn`): `api.Dial` on the local
  socket or pipe (`internal/config`'s paths; on Windows, starting
  `maild.exe` beside the app when no pipe answers), or lodge's
  `wss://LODGE/app/mail/api` with its Origin and a 30 s ping
  ([design/desktop.md](../design/desktop.md#on-a-maild-behind-lodge));
  reconnecting every second; one connection shared by the windows.
- **Live data** (`internal/desktop/live`, no MyGo import): the view model
  of [design/app.md](../design/app.md#data) in Go (sparse rows, pages of
  100, deltas, generations, `message.changed` refreshes), the mail store
  (accounts, mailboxes, sync, outbox, VIPs, blocked, settings, smart
  mailboxes, rules), the UI state, and the event fan-out; updates reach
  the windows through frostui's coalescing waker.
- **The shell:** frostui's frame without the rail (top bar segments,
  palette, status bar, console split, full-height content, dialogs,
  toasts), its `ConnectionStatus` and "connecting" screen, its
  `CommandTable`, its coalescing waker, and its embedded icons on
  Windows, from the pseudo-version the orchestrating session names.
- Tests: the view model against `rpctest` (a real maild), view tests with
  `ui.NewTester`, and a test that `uikit` imports only the standard
  library, MyGo, purego and frostui.
- **Done when:** the native window, against a maild serving
  `tools/uifixture`'s data, shows the mailbox tree and a 100,000-row Inbox
  that scrolls with every visible row filled, and comes back by itself
  after maild restarts; the Windows build starts and connects on
  `frostmail-win11`.
- **Status (2026-11-10): built, with its Linux check run; Windows waits.**
  `conn`, `live`, `commands`, `mainwin`, `frame`, `uikit.SummaryRow` and
  `cmd/frostmail`. The check, driven with `mygo-automation` on this
  desktop against a maild with `FROSTMAIL_SYNC=off`: the window showed
  `100000 messages`; after 8 wheel events of 50,000 DIPs, the 14 rows
  built around row 4,762 all held their text; killing maild showed
  `● Reconnecting to maild…`, and restarting it showed `● Local` and rows
  again, within the 4 s the check waited. `cmd/frostmail` cross-builds for
  Windows; running it there waits for `frostmail-win11`
  ([Phase 0's records](#phase-0s-records-2026-11-10)).
- **Not in this phase after all:** `--open-message` needs the reader, so
  it moves to Phase 4; problem records and the production build flags
  move to Phase 9, with the build they belong to.

## Phase 2 — maild's activity stream and the console (M)

- [ADR-0055](../adr/0055-maild-streams-its-activity-as-typed-entries.md):
  `schema/rpc/activity.yaml` (`ActivityEntry` with no string field, its
  enums, `activity.recent`, `activity.subscribe`, `activity.unsubscribe`,
  the transient `activity.added`), `make gen`.
- `internal/activity`: the ring of 2,000 entries, the per-account limit
  and its `dropped` entry, the subscribers' buffers of 256, the typed
  constructors and `Classify`. The rpcserver serves the methods to
  `local` and `session` callers.
- Emit points: mailsync (connect, list, each mailbox pass, IDLE, fetch
  batches, pushes of offline ops, draft copies, sends and their failures,
  Send Later, rules, Remind Me, announcements, erasing), pimsync (each
  CalDAV, CardDAV, Graph and Google Tasks pass, refused changes) and
  oauth (refreshes, sign-in needed).
- The three tests of ADR-0055: no string field, one package builds
  entries, and the canary run (with `imapxtest`, `smtpsink`, `davtest` and
  the recorded Graph and Google sessions) whose entries hold no canary.
- `mailctl activity [-f]`: the newest entries, then (with `-f`) the live
  ones, as lines written by `activity.Line` from each kind, with the
  account's and the mailbox's names from `account.list` and
  `mailbox.list`. The desktop writes its lines with the same function.
- **Done when (maild):** against the Dovecot container (`make
  engine-it`'s), entries are recorded for a mailbox pass, an IDLE wake, a
  flag push, a send, a refused send and a rule run, each naming its
  account, and `mailctl activity` prints them; an `agent` caller's
  `activity.recent` is refused; and the canary test passes.
- **Status of the maild side: built (2026-11-10).** `internal/activity`
  (the hub, the constructors, `Classify`, `Line`), the server's methods,
  the emit points, `mailctl activity`, and the tests:
  `TestAnEntryHasNoStringField`, `TestOnlyThisPackageMakesEntries` (which
  failed on a planted `api.ActivityEntry{}` literal in `internal/engine`,
  then passed with it removed), `TestErrorTextNeverReachesAnEntry`,
  `TestActivityHoldsNoCanary` (26 entries of 9 kinds, none holding a
  canary or a secret), `TestAgentsGetNoActivity`, and the Dovecot checks
  `TestActivityAgainstDovecot` and `TestActivityOfARefusedSendAgainstDovecot`.
  Their run of 2026-11-10 against `frostmail-mailtest` recorded, for
  test5: `mailbox 6 synced: 5 new`, `mailbox 6: the server reported
  changes (IDLE)`, `sent a message (0.1 s)`, `sent 2 offline changes to
  the server`, `rules acted on 1 message`, and with an unknown login
  `send failed: auth (535)`. A first run also showed a sign-in error
  recorded by a sync loop that a password change was stopping; the loop
  now records only while its context is live, and 3 runs after the
  change showed none.
- Two choices made while building: the limit is per account and kind,
  and warnings and errors are never limited (a burst of mailbox passes
  had hidden a failed send in the first canary run); a PIM pass counts
  the collections it changed, not the items.
- The desktop, once frostui's frame lands: frostui's `Console` fed from
  `activity.recent` at connect and `activity.added` after, Ctrl J, the
  button, the palette command, the status bar's newest entry.
- **Done when (console):** the native app's console shows the same lines
  as `mailctl activity`, filtered by level and by account.

## Phase 3 — Mail: sidebar, list and search (L)

- The sidebar and its menus; the list, its rows, menus, drags and empty
  states; the filter row, sort, search with its scope bar and coverage
  line; Follow Up, the Outbox and Send Later; Undo of moves (ADR-0028,
  ADR-0032).
- The command table, the keys, the menus and the palette.
- **Done when:** every [parity](../specs/parity.md) row of Mailboxes and
  the sidebar, of the message list and of search has a Go test that fails
  when it breaks, beside its React test.
- **Status (2026-11-10): built, except as below.** `internal/desktop/mail`
  holds the rules; `mainwin` the views. Parity's new Desktop test column
  names each row's test, run against a maild syncing the in-memory IMAP
  server.
- **Not in this phase after all:** New, Rename and Move Mailbox and Save
  as Smart Mailbox are sheets, so they go to Phase 6 with the others
  (P-107, P-602); the To-Do Bar needs Calendar and Tasks, so it goes to
  Phase 7; the reply family in the menus goes to Phase 5; opening a
  Follow Up row and moving between panes with Tab go to Phase 4, with
  the reader.
- **Gap:** contact photos (P-209) are built, in the Sort menu's Contact
  Photos, and have no Go test yet.

## Phase 4 — The reader (L)

- The header, the six banners, the conversation (collapsed rows, Show N
  Earlier), the page and its handlers from Phase 0, text bodies, All
  Headers, attachments (open, with the refused extensions), Save As,
  Raw Source, Print, find, reply with the selection, the contact popover.
- **Done when:** the hostile suite makes zero requests and runs no script
  in the built app on WebKitGTK and WebView2, a link opens only in the
  system's handler and never makes a window, and the reader's parity rows
  have tests (Go, or the built app's for the page).

## Phase 5 — Compose (L)

- The draft's HTML through a converter around `richtext`: a quotation's
  depth kept in `Indent` and written back as nested
  `<blockquote type="cite">`, headings read as body paragraphs (TipTap's
  schema has none), and `mailpart:` images loaded so that they keep their
  real size.
- The compose window per draft: identities, recipients with suggestions,
  Bcc, the subject, `richtext` with the format bar and link bar, images
  of `mailpart:` parts, attachments (pick, drop; bytes in lodge mode and
  the Flatpak), snippets, the attachment reminder, Send, Send Later, Undo
  Send's toast in the main window, Delete Draft, saving (spell check is
  Phase 11).
- **Done when:** a reply, a reply with a selection, a forward and a new
  message with an attachment and an inline image are sent to `smtpsink`
  from the native app and match the React app's MIME structure; the
  compose parity rows have Go tests.

## Phase 6 — Organize (M)

- The smart mailbox and rule sheets over the condition and action
  editors ([specs/organize-ui.md](../specs/organize-ui.md)), VIPs, flags
  and flag names, mute and block, Remind Me, Send Later, Redirect, the
  mailbox sheet, erasing old Trash and Junk, the refused notices.
- **Done when:** the organize parity rows have Go tests.

## Phase 7 — Calendar, People and Tasks (L)

- Calendar: Day, Week and Month, the mini month and calendars, the event
  pane, the drags, the keys, the event window with its repeat, scope and
  notify sheets ([ADR-0031](../adr/0031-edit-a-recurring-event-in-three-scopes.md),
  ADR-0034), invitations, the reminders window and Snooze.
- People: address books, the list, the person pane, the editor, photos.
- Tasks: the sources, the list with subtasks, the task pane.
- **Done when:** [specs/pim-ui.md](../specs/pim-ui.md)'s behaviors have
  Go tests, and the week view of a 400-event fixture month draws a frame
  in under 16 ms on the CI runner.

## Phase 8 — Settings, accounts and the desktop (M)

- The settings window and its seven panes, adding an account and OAuth
  sign-in through the system browser
  ([ADR-0043](../adr/0043-finish-a-sign-in-with-a-pasted-address.md)),
  first run and the move note, the change of mode, Save As through the
  portal in the Flatpak, notifications in lodge mode.
- **Done when:** a fresh profile goes through first run to a synced
  account in the native app, a change of mode from local to lodge and
  back keeps one maild, and a notification click opens its message.

## Phase 9 — Packaging and CI (M)

- **Flatpak:** the `frostmail` module builds `cmd/frostmail` from
  `go-sources.json`; `cargo-sources.json`, `node-sources.json` and the
  node24 and rust-stable extensions leave the manifest; the launcher, the
  Background helper, `open.sh` and the smoke test stay; `stamp-version.sh`
  stamps the Go build.
- **Windows:** MyGo's per-user NSIS installer, with `installerInclude`
  macros for `hooks.nsh`'s steps and for taking over an install the Tauri
  installer made (its folder and uninstaller; `Data` and `Cache` kept),
  and maild, mailctl and `frostmail-notify.exe` as helper executables.
- **CI:** the `app` and `windows-app` jobs build and test the Go app; the
  React job keeps `ui-check`, `web-build` and the lodge e2e; the desktop's
  end-to-end checks drive the automation build (`-tags automation`,
  `mygo-automation`) under Xvfb and on Windows; the axe audit (PR #222)
  moves to the web build.
- `make screenshots`, `make app-build`, `make app-dev` and `make app-test`
  build the Go app; the nsl machine is needed only for the web build.
- `internal/workflowcheck`, `.github/risk-paths.json` (the installer
  hooks at L1), `.github/dependabot.yml` (no cargo entry for
  `app/src-tauri`), `scripts/flatpak_stamp_test.go` and AGENTS.md's
  internal-crate line change with the files they name.
- **Done when:** CI builds the Flatpak and the Windows installer from the
  Go app, and both install and run on the test machines.

## Phase 10 — The parity gate and removing Tauri (M)

- Every parity row and every [specs/pim-ui.md](../specs/pim-ui.md) and
  [specs/settings-ui.md](../specs/settings-ui.md) behavior has a desktop
  test, or a gap Brian accepted.
- Manual passes: GNOME and KDE (Wayland and X11), Windows 11 on
  `frostmail-win11`, Orca on Linux and Narrator on Windows.
- Delete `app/src-tauri`, the Tauri transport and the desktop-only code of
  `app/src`; rewrite [design/app.md](../design/app.md),
  [design/desktop.md](../design/desktop.md),
  [design/testing.md](../design/testing.md) and AGENTS.md's map.
- **Done when:** the PR is ready for review with no gap list, and a
  release candidate's Flatpak and Windows installer pass plan 0013's
  clean-install gate.

## Phase 11 — Spell check (M, after parity)

Decided 2026-11-10 (the orchestrator's notes): spell check comes after
the parity gate and does not block it. MyGo has none.

- A checker over the system's Hunspell dictionaries (the Flatpak
  runtime's, and a bundled English one on Windows), its marks drawn with
  `richtext`'s `WavyUnderline`, suggestions in the editor's context menu,
  in compose's subject and body.
- **Done when:** a misspelled word in compose's body and subject is
  marked in the native app on Linux and Windows, and a suggestion from
  the context menu replaces it.

Windows notifications keep `frostmail-notify.exe` (decided the same day).

## Code layout

| Path | What | Imports MyGo |
| --- | --- | --- |
| `internal/activity` (maild) | the activity ring, limits, constructors and `Classify` (ADR-0055) | no |
| `cmd/frostmail` | main: flags, the single instance, the windows | yes |
| `internal/desktop/conn` | the maild connection: local, lodge, reconnect, ping, downloads | no |
| `internal/desktop/live` | view models, stores, events, the waker's input | no |
| `internal/desktop/commands` | the command table and keymap | no |
| `internal/desktop/mail` | sources and queries, the mailbox tree, the actions' rules, undo, date formats | no |
| `internal/desktop/compose` | the compose window | yes |
| `internal/desktop/calendar`, `people`, `tasks` | the modules and their windows | yes |
| `internal/desktop/settings` | the settings window, first run, modes | yes |
| `internal/desktop/platform` | notifications, the portal, Windows' pieces | some |
| `internal/desktop/uikit` | SafeHTMLView, SummaryRow, TokenSuggest | yes (only std, MyGo, purego, frostui) |
| `internal/desktop/frame` | a stand-in for frostui's frame and waker, deleted when they land | yes |
| `internal/desktop/mainwin` | the main window: frame, panes, sidebar, list, search, commands | yes |

Pure packages hold the decisions and are tested with `go test` alone; view
packages are tested with `ui.NewTester` against `rpctest`, and write PNGs
when `FROSTMAIL_SNAPSHOT_DIR` is set, as ChairLift's do.

## ChairLift's patterns

| Kept | Dropped |
| --- | --- |
| Hidden title bar, top bar as drag region, `SetAppID`, single instance | The page rail (the sidebar and module bar are Frostmail's) |
| Palette field and Ctrl K, `Command` and `Filter` | The job queue (maild owns sync, the outbox and rules) |
| Status bar and `StatusItem` | The console's command and dry-run levels (no commands run here) |
| The console (`Console`, `ConsoleView`, Ctrl J), for maild's activity | |
| `Dialogs`, `Info`, `ShortcutsBody`, `AboutBody`, `CodeDialog`'s look | Pipeline, gauges, charts, figures, tiles (dashboard parts) |
| Toasts, `Banner`, `Empty`, `Loading`, `Problem` | `PageHeader` and the padded, width-capped page scroll |
| Theme (`Adapt`, tones, spacing, `Chrome`, `Panel`), text styles, icons | `LaunchDesktopApp`, the wizard's hero |
| `Pill`, `DotPill` (flags, labels), `KeyValues`, `SwitchRow` (settings) | |
| `frostuitest.Loop`, snapshot PNGs, the automation build | |

## Patterns for frostui

**Built in frostui** by the orchestrating session, for Frostmail and
Frostnotes alike; Frostmail does not build them:

- the frame without the rail (full-height content, top-bar segments, the
  status bar, the console split), first sketched here as PaneShell;
- `Panes` (minimum and maximum widths, keyboard separators, kept widths,
  collapsible sides);
- `CommandTable` (the keymap, and palette sources asked per query);
- `ActionToast` (an action and a countdown: Undo, Undo Send);
- `ConnectionStatus`, with the "connecting to the daemon" screen;
- `SourceList` and its tree, on MyGo's outline;
- a settings window's layout;
- embedded icons for Windows;
- ChairLift's coalescing waker.

**In `internal/desktop/uikit` for now,** importing only the standard
library, MyGo, purego and frostui, so they can move by changing a path:

1. **SafeHTMLView:** an untrusted web view for HTML the app did not write:
   the CSP template, content rules that allow only `data:`, links to the
   system's handlers, no windows, its height, selection and find through
   `Eval` and `FindInPage`. frostread needs it as much as mail does.
2. **SummaryRow:** a list row of title, date, subtitle, two lines of
   preview and badges, at a fixed height.
3. **TokenSuggest:** `TokenField` with an autocomplete popover (recipients,
   attendees, tags).

## Open questions

- **Conversation pages:** each web view measured about 35 MB of PSS and
  1 process ([Phase 0's records](#phase-0s-records-2026-11-10)). Proposed:
  a web view only for an expanded HTML message, destroyed when it
  collapses. Decided in Phase 4, against how many messages a real
  conversation expands.

## References

- Implements: [ADR-0054](../adr/0054-draw-the-desktop-app-natively-with-mygo-and-frostui.md),
  [ADR-0055](../adr/0055-maild-streams-its-activity-as-typed-entries.md)
- Specs it keeps: [specs/ui.md](../specs/ui.md),
  [specs/compose-ui.md](../specs/compose-ui.md),
  [specs/pim-ui.md](../specs/pim-ui.md),
  [specs/settings-ui.md](../specs/settings-ui.md),
  [specs/organize-ui.md](../specs/organize-ui.md),
  [specs/search.md](../specs/search.md), [specs/parity.md](../specs/parity.md)
- Designs it changes: [design/app.md](../design/app.md),
  [design/desktop.md](../design/desktop.md),
  [design/rendering.md](../design/rendering.md),
  [design/testing.md](../design/testing.md)
- Related plans: [plan 0011](0011-m8-mail-in-lodge.md) (the web build),
  [plan 0013](0013-m6-the-standalone-desktop-flatpak.md) (the Flatpak and
  its gates)
- MyGo v0.8.3's docs (in the module cache): `docs/ui/web-view.md`,
  `docs/frontend.md` (untrusted pages), `docs/windows.md` (pages, find,
  printing, blocking content), `docs/ui/rich-text-editor.md`,
  `docs/ui/testing.md`, `docs/native.md` (notifications),
  `docs/distribution.md` (the Windows installer)
- ChairLift's ADR-0017 and `docs/design/frostui.md`
  (frostyard/chairlift#282)
