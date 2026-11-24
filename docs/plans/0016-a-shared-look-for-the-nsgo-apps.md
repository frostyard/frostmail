# Plan 0016: A shared look for the NSGo apps

- **Status:** Phase 0 in progress (2026-11-24). The owner chose D1 to D4 on
  2026-11-24
  ([ADR-0062](../adr/0062-give-the-nsgo-client-the-frostyard-look-from-the-shared-kit.md)).
  No part of the look is built.

This plan brings FrostMail's NSGo client
([ADR-0061](../adr/0061-share-the-mail-client-with-nsgo.md)) to the polish
the product was specified with (Mail.app's), and gives FrostMail, Frostnotes
and PocketGo one look from one shared library. The port of 2026-11-23 moved
the behaviour and lost the look that the MyGo client had
([plan 0015](0015-mygo-desktop.md#the-frostyard-look),
[README picture](../images/main.png)). The work is in three places: the
shared library (`workspaceui`, which moves from `frostyard/pocketgo` to
`frostyard/frostui` in Phase 0), this repository, and the two sibling apps. The MyGo client stays the default
(`FROSTMAIL_UI=mygo`) until Phase 4 closes; changing the default is not
part of this plan.

## What is wrong today

Read from the owner's screenshot of 2026-11-24 and the code at `0115d20a`.

| What shows | Why (code) |
| --- | --- |
| Two title rows, and "FrostMail" twice | `workspaceui.Shell` restores the system title bar on Linux and still draws `Brand` in the sidebar's head (`shell.go`). NSGo v0.3.0's GTK backend can move an undecorated window (`gdk_toplevel_begin_move`) but has no resize call and draws no window controls. |
| Stock grey and `#007AFF` | No app calls `cx.SetTheme`. Frostyard's palette and Inter (frostui's `docs/specs/frostyard-theme.md`) are not applied in any of the three apps. |
| A toolbar of text buttons | `main.go`'s `Actions`: Commands, Compose, Drafts, Outbox in three variants. The reader's actions are a second row of text buttons in the reader (`actions.go`, `triage`). |
| Filter and List options as primary blue buttons, a "Select messages" checkbox, "Save search" | `list.go`, `listOptions`: `ui.Button` with no variant is the primary one. |
| Rows with marks typed into the sender's name | `main.go`, `messageList`: `"●  "`, `"★ "`, `" · ⚑"`, `" · ◷"`, `" · 2 messages"` are joined into one string. The row has no date, no hover, and a photo only when one exists, so the text column moves. |
| Counts typed into the mailbox's name, one icon for every mailbox | `main.go`, `sections`: `label += "  ·  %d"`, and `Icon: lucide.Inbox` for all. |
| Six module rows over the mailboxes | Mail, Calendar, People, Tasks, Settings and Activity are a sidebar section. |
| "Load remote images" as a full-width primary button; the page with no frame | `main.go`, `messageReader`. |
| "Loading…" and "Select a message" as bare text | No empty or loading state. |

Frostnotes and PocketGo have the same gaps, and differ from each other:
Frostnotes uses the primary variant for every button (Cancel included),
PocketGo the secondary one; PocketGo confirms with the system's alert,
Frostnotes with a dialog; Frostnotes shows every notice as a warning;
paddings, window sizes and tree widths differ by a few points each.

## What NSGo v0.3.0 gives, and what it does not

- **Themable:** every palette role, per light and dark, and custom named
  roles (`core.Theme`, `Palette.SetColor`), read back with `ui.ColorsOf` and
  `cx.Palette()`. Fonts load with `nsgo.Fonts` or `cx.RegisterFont`.
- **Fixed in `ui`:** radii (`RadiusControl` 8, `RadiusCard` 12,
  `RadiusSmall` 4), heights (`ButtonHeight` 28, `HeaderHeight` 48,
  `RowHeight` 36) and the type scale (`TypeCaption` 12 … `TypeHeading` 20).
  `Theme.Radius` reaches Tailwind classes only.
- **No `Style` field:** `ui.IconButton`, `ui.Badge`, `ui.Sidebar`,
  `ui.SidebarItem`. They change by variant, by `SidebarItem.Content`, or by
  a view of our own.
- **Not there:** a search field, a divider, row hover in `ui.List`, sticky
  headers together with selection, swipe actions, a list that knows it has
  focus (`Ref.HoldsFocus` can be read), window controls and edge resize
  under a hidden title bar on Linux, materials and backdrop blur on Linux.
- **Proof it is enough:** GoVault (MIT, NSGo v0.3.0) reaches the bar with
  views of its own over `base.List`, about twenty small parts (tile, keycap,
  pill, card, field row, search field, sidebar row) and a table of named
  colors. Onegit reaches a lesser one on the stock shells with no per-OS
  code. This plan takes Onegit's way first, NSGo's own shells and controls,
  and GoVault's measures and colors; a part of our own, as GoVault's, only
  where NSGo has no control ([rule 11](#rules-for-every-app)). Ideas and
  measures only, written again here.

## The standard

One kit, three apps. An app supplies destinations, content and commands;
the kit decides how they look.

### The window

```
┌───────────────────────────────────────────────────────────────────────────────────┐
│ FrostMail                                                               –  □  ✕  │ system title bar: Linux only
├───────────────┬───────────────────────┬───────────────────────────────────────────┤
│ ◧   ✉ ▦ 👥 ☑  │ All Inboxes        ✎  │ ▣ ⌫ ⚠ │ ↩ ↩↩ ↪ │ ⚑▾ ⇥▾ ⋯    ⌕ Search or run… ⌃K  ▤ │ toolbar, 48
├───────────────┼───────────────────────┼───────────────────────────────────────────┤
│ FAVORITES     │ ⌕ Search All Inboxes  │ (DK)  David King                     7:34 │
│ ▣ All Inboxes │ All Unread Flagged  ⇅ │       To: Ann Lee                         │
│         7,655 │ ● (DK) David King 7:34│ Updates to Discord's Policies             │
│ ✎ All Drafts  │   Subject           ⚑ │ ┌ ⓘ 1 remote image is not shown.  [Load] ┐│
│ ➤ All Sent 14 │   preview, muted…     │ ┌─────────────────────────────────────────┐│
│ ICLOUD        │   (TF) Tijuana Flats  │ │ the page, white, inside a hairline      ││
│ ▣ Inbox 2,113 │   Subject        2 📎 │ │                                         ││
│ …             │   preview…            │ │                                         ││
├───────────────┤ …                     │ └─────────────────────────────────────────┘│
│ ● lodge…   ⚙ │                       │                                           │
└───────────────┴───────────────────────┴───────────────────────────────────────────┘
```

- **Toolbar.** One row, NSGo's `HeaderHeight`. Its list segment takes the
  list pane's width from the same signal and holds the source's title, its
  unread count as a muted subtitle, and Compose, the window's only filled
  button. Its reader segment holds icon buttons with tooltips, in groups
  with a separator: Archive, Delete, Junk; Reply, Reply All, Forward; Flag,
  Move, More. At its end: the palette field with its key hint, and the
  To-Do Bar toggle. A segment with no subject (no message open) dims its
  buttons; it does not move.
- **Sidebar.** At its head the sidebar toggle and FrostMail's module bar
  (Mail, Calendar, People, Tasks as an icon segment). Section captions are
  small, semibold and muted. A row is 28 to 30 high: a 16-point icon by the
  mailbox's role (inbox, drafts, sent, junk, trash, archive, flag, smart,
  folder), the name, and the count at the row's end in tabular figures,
  muted (the unread count in the text's color). At its foot: the connection
  item (a dot and "Local" or lodge's host, the warning tone while
  reconnecting; a press opens Activity) and the Settings gear.
- **List.** The search field sits over what it filters, with a clear
  button; under it a filter segment (All, Unread, Flagged, More) and a sort
  menu as icon buttons. Selecting several rows is Ctrl and Shift on the
  rows, not a mode: the "Select messages" checkbox goes. A row has a fixed
  gutter for the unread dot, a 32-point avatar (the photo, else a monogram
  on a tint from the address, so the column never moves), the sender in
  semibold with the date at the row's end, the subject, and zero to three
  muted preview lines by the setting. Marks at the row's end are icons:
  the flag in its color, the paperclip, the reminder clock, the thread's
  count as a small pill, the VIP star. Rows have a hover fill, a rounded
  selected fill and the hover actions they have today. A row that is
  loading is a skeleton, not text.
- **Reader.** A header block (avatar, sender, recipients, date, then the
  subject as the pane's largest text), banners, then the page. A banner is
  one component: a tone's fill and line, an icon, a sentence, at most one
  button at its end. Remote images, the sender check, a reminder, an
  invitation and a mailing list all use it. The page stays white inside a
  hairline with room around it (as `uikit.PageFrame` did). With no message:
  an empty state (an icon, one line, the keys that help).
- **Narrow windows and Lodge.** The same parts; the compact layout that
  `workspaceui.Shell` has today stays, with the toolbar's overflow in one
  menu.

### Rules for every app

1. **Theme.** The kit's `Apply` at the root: Frostyard on a dark desktop,
   Frostyard light otherwise, Inter, and a System / Light / Dark setting in
   every app's settings. The colors are frostui's spec, as NSGo roles and
   named roles (`chrome`, `panel`, `selected`, `label`, the tones' fills
   and lines).
2. **No literal looks in app code.** No hex color, `FontSize`, `CornerRadius`
   or `Padding` number outside the kit: an app takes a kit part or a kit
   token. A test in each app holds this (as plan 0015 did for MyGo).
3. **Buttons.** One filled button for each window region. Toolbar actions
   are icon buttons with tooltips. In a dialog, Cancel is secondary and
   before the confirming button; a destroying action is the danger variant.
4. **Dialogs.** One form dialog (title, fields, a busy state, an error line,
   Cancel and confirm) and one confirmation, drawn by NSGo in all three
   apps. Settings is NSGo's `ui.Settings` with the kit's rows.
5. **Notices.** A notice takes its real tone. An action that can be undone
   answers with a toast that carries Undo.
6. **States.** Every list and pane has an empty, a loading and an error
   state from the kit.
7. **Keys.** mod+K the palette (mod+Shift+P inside an editor), mod+, the
   settings, mod+N the app's new item, mod+F the search of what is shown.
8. **Window.** The size, the place and the panes' widths are kept, clamped
   so the main pane never falls under its least width.
9. **Names.** Labels and test ids are the tests' handles (61 selectors in
   `workspace_test.go`): a restyle keeps them, or changes the test in the
   same change.

10. **Live tests.** An app's UI tests are one suite on NSGo's `nsgotest`,
    written once against fixture data, that runs three ways: headless in
    the app's verify gate, in a real Linux window (`go tool nsgo test
    -target linux`) and in a browser (`-target web`). The suite names a
    picture at each main view (`app.Screenshot`), and the repository keeps
    each target's pictures. A change of the look changes its pictures in
    the same pull request, and the review reads the look from them.
    PocketGo's suite is the model ([Phase 0.5](#phase-05--live-tests-in-frostmail-and-frostnotes-m)).

11. **The right control.** A need takes NSGo's control for that need, used
    as NSGo's docs and gallery show it. In order: a `ui` control; that
    control's own variant or field; a kit part built on the `base`
    behaviour, only where `ui` has none, with the gap and its upstream
    issue named in the kit's spec. No view stands in for a control (a
    pressable view drawn as a button, a text field for a date), and no
    control does another's job (a checkbox as a mode or an action, a menu
    for a choice of three, a dialog for a confirmation). The
    [control map](#the-control-map) is the reference, and the live suite
    finds a control by its role where it can (`role:tab`, `role:switch`),
    so a wrong control fails a test.

### The control map

The starting map, from NSGo v0.3.0's `ui` package and a count of what each
client constructs today (2026-11-24). Phase 2 checks each row against
NSGo's gallery and docs before a part is built, and the map moves to the
kit's spec.

| Need | NSGo's control | Today |
| --- | --- | --- |
| The window and its navigation | `ui.SidebarShell`, through the kit's `Shell`; `ui.Sidebar` with the count in `SidebarItem.Content` | The count is typed into the row's name |
| A tree of folders or notes | `ui.Tree` | Frostnotes puts a second `ui.Sidebar` in the tree's place; PocketGo uses `ui.Tree` |
| A choice among a few views (modules, All / Unread / Flagged, Day / Week / Month) | `ui.Tabs`, segmented, or `ui.ToggleGroup` | Modules are sidebar rows; the filter is a menu on a filled button |
| A toolbar | `ui.Toolbar`, `ui.ToolbarGroup`, `ui.ToolbarSeparator`, `ui.IconButton` (its label is its tooltip), `ui.ToggleButton` for a panel's toggle | No client constructs a `ui.Toolbar`; FrostMail has 58 `ui.Button` and 4 `ui.IconButton` |
| The palette and its field | `ui.CommandDialog`; the field is `ui.SidebarButton` in its field variant, with its shortcut | The dialog is used; the trigger is a text button |
| A list and its selection | `ui.List` with a multiple `ui.Selection`, and the list's own keys | A "Select messages" checkbox turns on a checkbox in each row |
| A row's menu | `ui.ContextMenu`; `ui.Menu` with `ui.MenuSeparator`, icons, shortcuts and the danger variant | Menus are flat lists of text |
| A person's picture | `ui.Avatar` | `core.Image` with a corner radius |
| A count or a state | `ui.Badge` | Text; PocketGo uses `ui.Badge` |
| Loading | `ui.Skeleton`, `ui.Spinner`, a button's `Busy` | The text "Loading…" |
| A confirmation | `ui.AlertDialog`, the destroying action in the danger variant | FrostMail and Frostnotes: `ui.Dialog` with two buttons; PocketGo: the system's alert |
| A form in a dialog | `ui.Dialog` with `ui.FormSection`, `ui.FormRow` and `ui.Field` | Inputs laid out by hand |
| Settings | `ui.Settings` or `ui.SettingsDialog` | FrostMail: `ui.Tabs` over hand-laid panes; PocketGo: `ui.SettingsDialog` |
| An on/off setting | `ui.Switch`; `ui.Checkbox` for an item of a list and a task's done mark | Checkboxes for both; in the To-Do Bar a checkbox clears a flag, which is an action |
| A date and a time | `ui.DatePicker`, `ui.TimeField` | Text typed as `2006-01-02 15:04` for an event, Send Later and a reminder |
| Recipients | `ui.TagInput`, with `ui.SuggestList` for the matches | One text field with its own list of matches |
| A message's text | NSGo's rich editor | As it should be |
| Detail beside the content | `ui.Popover`, `ui.HoverCard` (a sender's card), `ui.Sheet` | Dialogs |
| A disclosure | `ui.Collapsible`, `ui.Accordion` | The kit's `Section` builds its own on `base.Button` |
| Paging | `ui.Pagination` | PocketGo's own Previous and Next |
| A notice of something done | `ui.Toaster` | As it should be |

`ui` has no control for these, so the kit holds them: the empty state, the
banner, the divider, the keycap, the icon and key hint around a search
`ui.Input`, and the layout of a summary row (the row's parts are `ui`
controls).

### The kit

| Part | Holds |
| --- | --- |
| `theme` | The two palettes, the named roles, the type and space tokens, `Apply`, Inter, the contrast test from frostui's table |
| `shell` | Today's `Shell`, `Inspector`, `FileExplorer`, `Conversation`; plus the toolbar's segments (each a `ui.Toolbar`), the connection item, the title bar's policy for each system, kept window state |
| `nav` | The content of a `ui.Sidebar` row (icon, name, count as a `ui.Badge`), today's `BranchLabel` and `ChildLabel` |
| `rows` | The summary row's layout over `ui.Avatar`, `ui.Badge` and `ui.Skeleton`; the unread dot; the hover and selected fills for `ui.List` |
| `controls` | Only what `ui` lacks: the icon and key hint around a search `ui.Input`, the keycap, the banner, the divider |
| `surfaces` | `Page`, `Card`, `Section` over `ui.Collapsible`, the field row, the empty state; settings rows are `ui.Settings`' |
| `dialogs` | A form dialog over `ui.Dialog` and NSGo's form rows, `Notice`; a confirmation is `ui.AlertDialog` |
| `gallery` | An app that shows every part in both themes, built for Linux and the web: what the owner signs off, and what the screenshot tests compare |

## Phase 0 — Decisions and the kit's home (S)

- The owner answered D1 to D4 on 2026-11-24, with the recommended choice
  for each:
  [ADR-0062](../adr/0062-give-the-nsgo-client-the-frostyard-look-from-the-shared-kit.md).
- frostui holds the kit's own ADR (its ADR-0014), a spec of what the module
  draws today and the kit's plan. The standard above stays here as the
  draft, and becomes the kit's spec as Phases 1 and 2 build it: frostui's
  specs change only with the code that holds them.
- The kit moves: `github.com/frostyard/frostui/workspaceui`, a module of its
  own in frostui with its gates in `make ci`, holding `workspaceui`'s 6
  files as they are at `desktop/workspaceui/v0.1.0-rc.1`. frostui merges by
  squash, so the files' history stays in pocketgo and the commit names
  where they came from.
- The maintainer tags the module (`workspaceui/v0.1.0`). Then each of the
  three apps takes the new import path in a change of its own, with no
  change in what it draws, and PocketGo removes its copy.
- **Done when:** the ADRs are accepted, and FrostMail, Frostnotes and
  PocketGo build and pass their gates on one tagged version of the kit.

## Phase 0.5 — Live tests in FrostMail and Frostnotes (M)

Before any look changes, so that every later phase shows its change as
pictures.

- **The model, PocketGo** (`desktop/internal/nsgoclient` at `995d86d`): its
  tests drive the client by label, text and test id against a fixture
  service (`pocketdtest`, with scenes such as the busy team), and check
  bounds (the panes' widths, where the board's lanes and the file tree
  sit). `TestMain` is `nsgotest.Main`, so the same tests run headless in
  `make verify` and, from its README's two commands, in a Linux window and
  in Chromium. Each main view is named with `app.Screenshot`, and
  `comparison/linux` and `comparison/web` each hold 13 pictures of the same
  views. Its README's preview command serves a scene to a native and a web
  client at once. No workflow or Makefile target runs the two real-target
  commands.
- **FrostMail today:** `cmd/frostmail-nsgo/workspace_test.go` has 14
  workflows on a real maild with `tools/uifixture`'s showcase, headless
  only (`make nsgo-test`): no `TestMain`, no picture, no real window.
  Lodge's browser path is `dev/lodge/nsgo-smoke.py`, apart from the suite.
  Frostnotes is the same: 2 test files on a real notesd, headless, and its
  Firefox tests (`internal/webevidence`) apart.
- **To build, in each of the two apps:**
  - `nsgotest.Main`, and the suite passing on the Linux target, under Xvfb
    where there is no display.
  - A picture at each main view on the fixture. FrostMail: the three panes
    with a message open, a conversation, the narrow layout, compose,
    Settings, Calendar, People, Tasks, the To-Do Bar and a dialog.
    Frostnotes: the editor with a note open, the note list, quick open,
    the vault manager and a dialog.
  - Bounds checks where the standard gives a measure (a pane's width, a
    row's text inside the row), as PocketGo's.
  - A `make` target for the Linux target, the pictures kept in the
    repository beside the suite, and a CI job that runs the target under
    Xvfb and uploads its pictures.
  - [design/testing.md](../design/testing.md) says how to run each way and
    how to make the pictures again.
- **Done when:** in FrostMail and in Frostnotes the suite passes in a real
  Linux window on the host and in CI, and each repository holds the
  pictures of today's look for the views above: the "before" set.

## Phase 1 — Theme, type and the title row (M)

- `theme`: both palettes from frostui's spec, the named roles, Inter
  (bundled for the native builds, served with the app for the web one),
  the type and space tokens, the contrast test.
- `Apply` in the three apps, with the appearance setting.
- `Shell` stops drawing the brand where the system's title bar shows it.
- **Done when:** each app's live suite passes, and its pictures, made
  again in each theme, show Frostyard's colors and Inter with one title
  row; the contrast test passes; PocketGo's pane-width tests pass.

## Phase 2 — The kit's parts and the gallery (L)

- The control map checked row by row against NSGo's gallery and docs
  (rule 11): each kit part names the `ui` control it wraps, or the gap that
  leaves none to wrap.
- The parts of the table above, each with a layout test (a row keeps its
  text inside it, a count sits at the row's end, a tooltip shows on hover)
  in the manner of GoVault's and PocketGo's `nsgotest` checks.
- The gallery, with a live suite of its own (rule 10): a picture of every
  part in both themes, from the Linux and the web target.
- **Done when:** the gallery shows every part in both themes on Linux and
  in a browser, the kit's spec holds the control map with a gap named for
  each part that wraps no `ui` control, and the owner accepts the
  pictures. This is the look's sign-off: Phases 3 to 5 only place parts.

## Phase 3 — FrostMail's main window (L)

Three changes, each with `make nsgo-test` green, and each replacing the
stand-ins of its panes with the control map's controls (rule 11):

- **3a, shell.** The toolbar's segments and icon buttons, the module bar,
  the sidebar's rows with role icons and counts, the connection item and
  the gear. Settings and Activity leave the sidebar's list.
- **3b, list.** The summary row, the search field, the filter segment and
  sort menu, selection by Ctrl and Shift (`base.List` with a multiple
  `Selection`), hover, skeletons, the empty states, the bulk actions in the
  toolbar while several rows are chosen.
- **3c, reader.** The header block, the banners, the page's frame, the
  conversation's stacked messages, the empty state.
- **Done when:** the live suite's picture of the main window, in both
  themes, is set beside [main.png](../images/main.png) and the owner
  accepts it; each of the three changes carried its changed pictures; and
  `make nsgo-test`, the Linux target and `dev/lodge/nsgo-smoke.py` pass.

## Phase 4 — FrostMail's other surfaces (L)

- Compose (its toolbar, recipients on `ui.TagInput`, the formatting bar),
  dates and times on `ui.DatePicker` and `ui.TimeField`, Settings
  on `ui.Settings`, Calendar, People, Tasks, the To-Do Bar, Activity, the
  palette, and every dialog on the form dialog and the confirmation.
- The narrow layout and Lodge's web build, checked in a browser.
- The style test of rule 2 turned on for `cmd/frostmail-nsgo`.
- **Done when:** the README's pictures are the live suite's, from the NSGo
  client; the style test passes with no exception listed; and no row of
  the control map's "Today" column is still true of FrostMail.

## Phase 5 — Frostnotes and PocketGo (M each, beside Phases 3 and 4)

- Each app replaces its own helpers with the kit's parts (PocketGo:
  `inboxHeader`, `agentActivity`, `signIn`, `confirm`, the pager;
  Frostnotes: the editor's toolbar, `notice`, `vaultForm`, the five
  dialogs), and takes rules 3 to 8.
- Frostnotes' second toolbar row moves to the shell's toolbar.
- **Done when:** each app's style test and live suite pass, its own gates
  pass, no row of the control map's "Today" column is still true of it,
  and the three suites' pictures of the main window, side by side,
  show one toolbar, one sidebar row, one selected fill and one dialog.

## Phase 6 — Upstream (S, alongside)

Each is a gap this plan works around. The owner files them, or approves the
text an agent drafts:

- Edge resize and window controls for an undecorated GTK window (it would
  let the toolbar be the title bar on Linux, as on Windows).
- Themable radii and type scale in `ui`.
- A `Style` on `IconButton`, `Badge` and `SidebarItem`; a count on
  `SidebarItem`.
- Row hover, and sticky headers with selection, in `ui.List`.
- **Done when:** each has an issue whose link is in the kit's spec, and the
  spec names the workaround to remove when it lands.

## Later / ideas

- Swipe actions on a touch screen; dragging a message out to the desktop.
- Mica behind the sidebar on Windows.
- A density setting (the row's height and the preview's lines together).

## Open questions

- **FrostMail's and Frostnotes' web target:** the browser client reaches
  its daemon only through Lodge, where PocketGo's reaches a fixture service
  over HTTP. Either the suite runs with `-target web` behind a test
  gateway to the fixture daemon, or the web pictures stay with the Lodge
  smoke tests. Decided by a trial in Phase 0.5.
- **Pictures as a gate:** whether a picture that differs fails a run, or
  the pictures are evidence a person reads and only the suite's checks
  fail. Fonts and drawing differ between hosts. By Phase 0.5.
- **Lodge's web build:** whether it takes the kit's palettes or Lodge's own
  theme tokens when the two differ. By Phase 1.

## References

- Implements: [specs/ui.md](../specs/ui.md) (the window's behaviour, which
  this plan does not change), [design/app.md](../design/app.md)
- [ADR-0061](../adr/0061-share-the-mail-client-with-nsgo.md),
  [ADR-0062](../adr/0062-give-the-nsgo-client-the-frostyard-look-from-the-shared-kit.md);
  [plan 0015](0015-mygo-desktop.md), The window and The Frostyard look
- frostui: `docs/specs/frostyard-theme.md`, ADR-0011, ADR-0013
- pocketgo: ADR-0068, `desktop/workspaceui` at `desktop/workspaceui/v0.1.0-rc.1`
- Looks studied: [GoVault](https://github.com/txchen/govault),
  [Onegit](https://github.com/oneo-me/onegit), the
  [NSGo gallery](https://nsgo.dev/gallery/)
