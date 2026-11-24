# Plan 0016: A shared look for the NSGo apps

- **Status:** Phase 0 is done (2026-11-24): the owner chose D1 to D4
  ([ADR-0062](../adr/0062-give-the-nsgo-client-the-frostyard-look-from-the-shared-kit.md)),
  the kit is in frostui and tagged, and the three apps build on it. Phase
  0.5 is done (2026-11-24). Phase 1 is done (2026-11-24; Brian accepted
  closing it on a desktop picture of 1 of the 3 apps). Phase 2 is done
  (2026-11-24): the kit holds the parts, the control map and the rules,
  at `workspaceui/v0.3.0`. Phase 3 is next.

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

Since Phase 2 the kit's spec holds the standard (frostui's
`docs/specs/workspaceui.md`: the themes, the parts, the control map, and
"The standard for an app", which is the rules below). The rules, the
control map and the kit's table in this section are the draft they were
made from, kept for the phases' references to them; where the two differ,
the kit's spec holds. The window below is not in the kit's spec: the owner
set Phase 2 to colors, controls and the inside of a part (2026-11-24), and
where the parts go in a window, and how one moves between them, is decided
in each app's phase, on its pictures.

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

## Phase 0 — Decisions and the kit's home (S, done 2026-11-24)

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
- **Done (2026-11-24).** ADR-0062 here and frostui's ADR-0014 are
  accepted. frostyard/frostui#70 merged, and Brian tagged
  `workspaceui/v0.1.0` on its merge commit. Each app requires that version,
  with only its import path, `go.mod`, `go.sum` and what follows them
  changed, and each change merged with every check of its pull request
  passing: frostyard/frostmail#387 (12 checks), frostyard/frostnotes#275
  (11) and frostyard/pocketgo#167 (10), which also removed PocketGo's copy
  of the module.

## Phase 0.5 — Live tests in FrostMail and Frostnotes (M, done 2026-11-24)

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
- **FrostMail's part (2026-11-24).** `make nsgo-live` runs the suite under
  Xvfb in a GTK 4 window: 17 tests pass and 2 skip on the host.
  `cmd/frostmail-nsgo/comparison/linux` holds 14 pictures of today's look,
  from 5 picture tours (`make nsgo-pictures`), and CI's NSGo client job
  runs the target and uploads its pictures
  ([design/testing.md](../design/testing.md#the-nsgo-clients-live-suite)).
  What the real window showed that the headless run does not:
  - 2 of the 14 workflows fail there and pass headless: the quick actions
    of a message row never show under the pointer, and a row dragged onto a
    mailbox does not move (frostyard/frostmail#391, #392). They skip on a
    target. The cause of the first
    is not found (the testing design has what was tried); the second was
    not looked into. Phase 3 rebuilds these rows: its 3b is not done while
    either skips.
  - The tests assumed their working directory; on a target it is another.
  - Unsetting `WAYLAND_DISPLAY` does not keep GTK on Xvfb: on a Wayland
    desktop the windows of the first runs opened on that desktop, and the
    first kept pictures were taken there. Frostnotes' part found it. The
    Makefile now sets `GDK_BACKEND=x11`, the pictures were taken again on
    Xvfb's X11, and the 2 failures above were seen there too.
  - The fixture's dates follow the day, so the pictures of the list and
    the Calendar change from day to day.
  Not built: bounds checks, which wait for the standard's measures
  (Phases 2 and 3).
- **Frostnotes' part (2026-11-24),** frostyard/frostnotes#277: its suite
  passes headless and in a GTK window on Xvfb's X11 (12 tests, none
  skipped), and it keeps 19 pictures of that day's look. 3 of its 4
  workflows failed in the window at first, each on a difference of NSGo
  v0.3.0's test host (the app's clock stands still during a wait; a line
  break typed into a multiline field is dropped; a paste runs headless
  only), and its tests now allow for them with no change to the app.
- **Done (2026-11-24).** Both suites pass in a real Linux window on the
  host and in CI: FrostMail's NSGo client job (#390, #394) and Frostnotes'
  Verify job (frostyard/frostnotes#277) ran `make nsgo-live` and passed on
  their pull requests. Each repository holds the "before" pictures of its
  views: FrostMail's 14 in the commit of #394, Frostnotes' 19 in #277.

## Phase 1 — Theme, type and the title row (M, done 2026-11-24)

- `theme`: both palettes from frostui's spec, the named roles, Inter
  (bundled for the native builds, served with the app for the web one),
  the type and space tokens, the contrast test.
- `Apply` in the three apps, with the appearance setting.
- `Shell` stops drawing the brand where the system's title bar shows it.
- **Done when:** each app's live suite passes, and its pictures, made
  again in each theme, show Frostyard's colors and Inter with one title
  row; the contrast test passes; PocketGo's pane-width tests pass.
- **The kit's part (2026-11-24),** frostyard/frostui#72, tagged
  `workspaceui/v0.2.0`: `Theme`, `Apply`, `Frame`, `ColorsOf` and
  `AppearanceChoice`, a contrast test of 69 pairs in each theme at 4.5:1
  or over, the brand left to the system's title bar on Linux, and the
  compact shell's ground, which Frostnotes' dark narrow picture showed
  missing. Not carried over from the MyGo kit: the high-contrast values.
- **FrostMail's part (2026-11-24).** The client calls `Apply`, sits in a
  `Frame`, and keeps the General pane's choice of System, Light or Dark in
  the device's storage (a desktop only: Lodge sets the web build's
  appearance). Its 14 pictures are made again in Frostyard light, and 6
  are added in Frostyard; they show the themes' colors in the shell, the
  list, a menu, a dialog and the Calendar, the text in the bundled Inter
  where Xvfb's own face is another, and the narrow dark window on its
  ground. One title row is not shown by them: a capture holds the window's
  content, not the system's title bar; the navigation's head no longer
  names the app. Not checked: the web build under Lodge and Windows.
  Still as before, for Phases 3 and 4: every layout, and each row of the
  control map.
- **Frostnotes' part (2026-11-24),** frostyard/frostnotes#278: `Apply`,
  `Frame`, and the choice of appearance as a row of its Manage vaults
  dialog, kept on a desktop. 19 pictures made again and 3 added (17 light,
  5 dark). Its dark narrow picture, unreadable before, is readable. With
  the host's own Inter hidden the pictures were byte-identical, and with
  the kit's registration also removed the text was Noto Sans.
- **PocketGo's part (2026-11-24),** frostyard/pocketgo#168: `Apply`, and
  `Frame` around its zoom view; its Theme setting is the kit's segmented
  control, with its storage unchanged. Its 13 Linux and 13 web pictures
  are made again (7 dark, 6 light each), and it gains `make nsgo-live` and
  `make nsgo-pictures`. Its pane-width tests pass unchanged; 7 tests each
  lose the press that opened the old select.
- **Where the phase stands (2026-11-24).** Of its "Done when": each app's
  live suite passes on its pull request (#395, frostyard/frostnotes#278,
  frostyard/pocketgo#168), the pictures of each are made again in both
  themes and show Frostyard's colors and Inter, the kit's contrast test
  passes, and PocketGo's pane-width tests pass. **Not shown: one title
  row.** A capture holds the window's content, not the system's title bar,
  so no picture shows the bar and the toolbar together; what the pictures
  show is that the navigation's head no longer names the app on Linux.
  The phase stays open until the owner looks at a client's window on a
  desktop, or accepts closing without it.
- **The owner's picture (2026-11-24).** Brian's screenshot of PocketGo's
  window on his desktop, in Frostyard, shows the system's title bar with
  the app's name and, under it, the navigation's head with its toggle and
  no name: the name once. It shows PocketGo only. FrostMail's and
  Frostnotes' windows were not looked at on a desktop; the same `Shell`
  of the kit draws their heads.
- **Closed (2026-11-24).** Brian accepted closing the phase on that
  picture. The missing piece of "Done when": a desktop picture with one
  title row for FrostMail and for Frostnotes.
- **Seen in the pictures, for Phase 2:** a dialog's and a palette's panel
  let the view under them show through and take the ground's color, not
  the panel's; in a wide window the content's ground is 1 step of 255 off
  the theme's; PocketGo's settings dialog draws its sidebar in a neutral
  gray. Every button is still the filled one and a chosen row a neutral
  gray, as the control map and rule 3 say they are today.

## Phase 2 — The kit's parts and the gallery (L, done 2026-11-24)

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
- **Done (2026-11-24),** in frostui, 4 rounds (frostyard/frostui#74, #75,
  #76), tagged `workspaceui/v0.3.0`:
  - The gallery, a module of its own: a draft of the standard window on
    fixture mail and 7 pages of controls, with tours that keep 30 Linux
    pictures, each view in both themes. The tours pass in a headless
    Chromium too.
  - The owner looked at each round's Linux pictures, ran the app on his
    desktop, and accepted the look as drawn: the soft fill of a chosen
    row, the toolbar on NSGo's inset strip, NSGo's tinted danger button,
    an opaque Frostyard panel under a dialog, a palette, a popover and a
    toast, initials on a neutral avatar, the summary row of 3 lines,
    modules as tabs of an icon alone, a search field with its icon and key
    hint, and NSGo's corners and sizes.
  - The accepted parts are the kit's, each with a layout test: `NavLabel`,
    `SummaryRow`, `SoftRows`, `Banner`, `EmptyState`, `DialogPanel`,
    `ConnectionItem`, `SearchField`.
  - The kit's spec holds the control map, with "NSGo has none" beside each
    part that wraps no `ui` control (`Banner`, `EmptyState`,
    `SearchField`), and the rules for an app.
  - Not looked at by the owner: the web pictures (the agent looked at 1 of
    30), and the search field's clear button, added after he accepted the
    field.
  - What the theme cannot reach, for Phase 6: a menu's translucent ground,
    the neutral side of NSGo's settings view, and the list's light scroll
    bar in the dark theme on the web.
- **The phase's bounds, set by the owner (2026-11-24):** Phase 2 decided
  colors, controls and the inside of a part. Draft windows for a calendar,
  people, tasks, compose and settings were left out of it for that reason:
  Phases 3 to 5 decide each window's layout and flow, on that app's
  pictures.

## Phase 3 — FrostMail's main window (L, done 2026-11-24)

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
- **3a (2026-11-24), built; the owner has not looked at its pictures.**
  `cmd/frostmail-nsgo/shell.go` holds the shell:
  - **The navigation.** The modules are a segment of 4 icons at its head
    (`ui.Tabs`). A row is the kit's `NavLabel`: the icon of the row's role,
    from the sidebar model's icon names, its name, and its count at the
    row's end, muted for Flagged and its colors, which count every
    message. A section with no row is not drawn. At the foot, the kit's
    `ConnectionItem` (the target's name, in the warning tone while not
    connected) opens Activity, and a gear opens Settings; neither is a row.
  - **The toolbar.** The title with the source's unread count under it,
    Compose as the one filled button, a `ui.Toolbar` of icon buttons in 3
    groups (Archive, Delete, Junk; Reply, Reply all, Forward; Flag, Move,
    More), and the palette's field, which mod+K opens. With no message
    open the buttons are dimmed and keep their places. The reader's row of
    text buttons is gone. The toolbar fits the pane under it, whose width
    the navigation and the To-Do Bar take from: Compose and the palette
    become icons, and under 720 the actions are one menu.
  - **Placed here, and the plan did not say where.** Local Drafts and
    Outbox are rows of a section, Outgoing, at the navigation's end, and
    open the lists they opened as toolbar buttons. Undo is an item of
    More; a toast with Undo (rule 5) waits for 3b. Flag toggles the flag,
    and its colors are a submenu of More. In Calendar, People and Tasks
    the toolbar has the title, Compose as an icon and the palette, and the
    navigation still lists mail's rows (Phase 4).
  - **Checks.** `make nsgo-test` passes. `make nsgo-live` on Xvfb's X11:
    22 passed, 2 skipped (the 2 of Phase 0.5). All 20 kept pictures
    changed. 2 new workflows: the toolbar's buttons cannot be pressed with
    no message open and can with one, and Flag becomes Unflag and back;
    the modules are tabs, a row's count is the title's unread count, and
    exactly 1 control is named Activity and 1 Settings.
    `dev/lodge/nsgo-smoke.py` was not run.
  - **Seen on the way.** While the toolbar was built again on each change
    of the open message, the picture taken after a press of More showed no
    menu, though the test had found the menu's items. With nodes that stay
    and read the message as they draw, the picture shows the menu. The
    cause was taken to be the message's change to read just after it
    opens; that was not checked apart. In the Xvfb window, with a message open, the window's picture is
    blank from about a second after the To-Do Bar opens; the same steps on
    main (65637f05) give the same blank picture, so the tour takes the
    To-Do Bar beside the list alone. Not tried on a desktop.
- **3b (2026-11-24), built; the owner has not looked at its pictures.**
  `cmd/frostmail-nsgo/list.go` holds the list pane:
  - **Rows.** NSGo's `ui.List` with the app's multiple `ui.Selection` and
    the kit's `SoftRows`; a row is the kit's `SummaryRow` (the unread dot,
    the avatar with the sender's photo or initials, the sender and the
    date, the subject, the preview by the setting). Its marks are icons:
    the VIP's star, the flag, the paperclip, the reminder's clock, and a
    conversation's count as a badge. A row not yet sent is `ui.Skeleton`.
  - **Selection.** A press chooses a row and opens it; Ctrl and Shift
    choose several, and the "Select messages" checkbox is gone. With
    several chosen the reader names their count, and the toolbar's
    Delete, Flag and Move act on them all, with Mark as read, Mark as
    unread, Clear flags and Apply mail rules in More. Archive and Junk
    wait for one message, as the old menu of a selection had neither.
  - **Over the rows.** The kit's `SearchField`, which names the source and
    whose button ends a search; a segment of All, Unread and Flagged; a
    menu of the other filters; and a Sort menu, which also holds Show
    conversations, Contact photos, Search all mailboxes and Save search as
    a smart mailbox. Their nodes stay while rows arrive.
  - **States.** The list's `EmptyState` says that nothing is there, that
    nothing matches, or that the messages did not load, and shows
    skeleton rows while it waits. The reader's says that no message, or
    how many, are chosen (the rest of the reader is 3c).
  - **Rule 5.** A move, an archive or a deletion answers with a toast
    whose action is Undo. A change of flags does not; Undo stays in More.
  - **The 2 skips of Phase 0.5 are gone** (frostyard/frostmail#391,
    frostyard/frostmail#392). The row's wrapper under its context menu
    had a flex basis of 0 in a column, so it was 0 points high (346 by 0
    in the headless tree) and its content stood outside it, which is
    taken to be why the view that listened never heard the pointer. The
    quick actions now show from the list row's own hover
    state, and the wrapper has its content's height. A drag did drop in
    the GTK window: the mailbox took the message, and the same release
    also pressed the mailbox's row, so the list changed to that mailbox,
    where the test found the message it expected to be gone. The press
    that comes in the drop's own turn of the loop is now passed over.
  - **Lost or left** (the owner accepted the list on its pictures,
    2026-11-24, and asked for these as issues). The list no longer
    follows the text size setting: `SummaryRow` has fixed sizes
    (frostyard/frostmail#400). A flag's 7 colors draw in the kit's 5
    tones, so Yellow draws as Orange and Purple as the accent: the kit
    has no flag colors (frostyard/frostmail#401). Both need a change of
    the kit. Archive and Junk for several chosen messages are
    frostyard/frostmail#402, and a test of the undo of a move is
    frostyard/frostmail#403.
  - **Checks.** `make nsgo-test` passes. `make nsgo-live` on Xvfb's X11:
    27 passed, 0 skipped. 23 kept pictures: the 20 changed, and 3 are
    new (a row under the pointer, several rows chosen, a search with no
    match). 3 new workflows: several rows chosen and flagged from the
    toolbar; a search, a search with no match, its clear button and the
    Flagged tab; and an archive's toast with its Undo action. The undo
    of a move is not run: the fixture's mail does not sync, and maild
    answers that the messages are still moving. `dev/lodge/nsgo-smoke.py`
    was not run. No row was dragged or pointed at by a person on a
    desktop.
- **3c (2026-11-24), built; the owner has not looked at its pictures.**
  `cmd/frostmail-nsgo/reader_pane.go` holds the reader pane:
  - **The header block.** The sender's avatar (the photo or initials),
    the sender's name and address, the recipients on one line, and the
    date at the row's end; then the subject as the pane's largest text.
  - **Banners.** Each is the kit's `Banner` in its tone, with at most one
    action: a failed sender check (danger), a reminder (accent, Remove
    reminder), remote images not shown (Load remote images), a calendar
    invitation (accent, Open invitation), a mailing list (Unsubscribe),
    and a conversation (View conversation). They replace 6 text buttons
    and lines.
  - **Files.** A caption that counts them, and a row for each: its name,
    its size, and Open and Save as icon buttons.
  - **The page.** Inside a hairline with rounded corners, with the pane's
    room around it, in both themes. The page keeps the sender's colors.
    The ground behind a page that has not painted is the pane's, not
    white as `uikit.PageFrame` made it: the kit has no page color.
  - **A conversation.** Its messages stay a stack in a dialog, each with
    a header (avatar, sender, date) that opens and closes it, and the
    page in the same frame. They are not stacked in the reader pane: a
    page's height is not known without script, which the page may not
    run, so each page there has a fixed height of 270.
  - **Also.** Every dialog, the toasts and the palette are on the kit's
    opaque panel (rule 4's ground; the dialogs' forms are Phase 4). In a
    window under 800 the way back to the list is a quiet button, not a
    filled one.
  - **Checks.** `make nsgo-test` passes. `make nsgo-live` on Xvfb's X11:
    29 passed, 0 skipped. 26 kept pictures: 16 changed and 3 are new (an
    invitation, a message with a file, a message of a conversation). 1 new workflow, of the header block, the
    banners of an invitation and a conversation, a file's row, and no
    banner on a message with nothing to say. `dev/lodge/nsgo-smoke.py`
    was not run with this change: the machine had no geckodriver or
    Firefox for it. It was run afterwards, below.
- **Lodge's browser (2026-11-24).** `dev/lodge/nsgo-smoke.py` passed 2
  times of 2 on 3c's commit (ff750202), in a headless Firefox on the
  owner's desktop: `PASS: inbox, isolated reader, settings, Back, refresh
  and editable draft permalink`. The client was staged with `make
  nsgo-lodge-app IN_NSL='sh -c'`, as CI stages it. The tools were Lodge's
  `lodge-e2e`, `lodgectl` and shell as that checkout had them built (the
  binary names itself `ui/v0.7.0-dirty`, commit 6217d07), geckodriver
  0.37.1 and Firefox 153.8.0esr, each fetched from Mozilla and checked
  against its published SHA-256. The script opens a row, Settings and
  Compose: it does not press the toolbar, a banner or a row's actions.
- **Done (2026-11-24).** The 3 changes are built, each with its changed
  pictures; `make nsgo-test`, the Linux target (29 passed, 0 skipped) and
  the smoke test pass. Brian accepted the main window (2026-11-24) from
  the live suite's pictures as each change sent them, in both themes.
  Missing from "Done when", and accepted by him as it is: the pictures
  were not set beside [main.png](../images/main.png) in one view.

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
- **In 5 changes** (2026-11-24), each with its changed pictures, as
  Phase 3 went:
  - **4a, buttons and confirmations.** Rules 3 and 4's confirmation in
    every surface at once: one filled button for a region, destroying
    actions in the danger variant, confirmations on `ui.AlertDialog`.
  - **4b, Settings** on `ui.Settings`, with its panes' rows on
    `ui.FormSection` and `ui.FormRow`, and the panels that are lists of
    buttons (an account's, a mailbox's) as lists and menus.
  - **4c, Compose:** its toolbar, recipients on `ui.TagInput`, the
    formatting bar, the schedule on `ui.DatePicker` and `ui.TimeField`.
  - **4d, Calendar, People, Tasks, the To-Do Bar and Activity,** each
    with its own navigation in place of mail's rows.
  - **4e, the narrow layout and Lodge's web build** in a browser, the
    style test of rule 2, and the README's pictures.
- **4a (2026-11-24), built; the owner has not looked at its pictures.**
  - **Buttons.** `button` draws quiet (NSGo's secondary variant), and
    `primary` and `danger` name a region's main action and a destroying
    one (`cmd/frostmail-nsgo/accounts.go`). Before, `button` drew every
    button filled: the Calendar's bar had 5 filled buttons and has 1, New
    event. 14 buttons are named primary and 10 danger, and 13 others
    that were filled by default are quiet.
  - **Confirmations.** `confirm` and `destroy` are `ui.AlertDialog` on the
    kit's panel: Cancel first and quiet, the action filled, or in the
    danger variant for the 13 confirmations that remove something. An
    error inside a dialog or a form is the kit's danger `Banner`.
  - **Left for 4b to 4d.** The panels that are lists of buttons keep
    their buttons, now quiet. A dialog's footer keeps the order its
    caller gave it; those seen put Cancel first.
  - **Seen on the way.** The mail tour's search step failed 1 time
    headless, and passed in each of 5 later runs. The search field was
    built again with every change of the store, which is taken to be
    why: it is now built again only when its source's name changes.
  - **Checks.** `make nsgo-test` passes. `make nsgo-live` on Xvfb's X11:
    31 passed, 0 skipped. 21 of the 26 kept pictures changed. 2 new
    workflows: the 3 helpers' variants, and Delete's alert with Cancel
    before Delete, which a cancel leaves undone.
- **4b (2026-11-24), built; the owner has not looked at its pictures.**
  `cmd/frostmail-nsgo/settings_view.go` holds Settings:
  - **The view.** NSGo's `ui.Settings`: 7 categories at its side
    (Accounts, General, Identities, Snippets, Filters, Mailboxes, Sign-in,
    and Server on a desktop) in place of a row of tabs, groups of rows,
    and a search through every row. An account, an identity, a snippet, a
    rule, a smart mailbox and a mailbox are rows of NSGo's own, so the
    search finds them by name as it finds a preference.
  - **General.** A row for each preference, with what it does under its
    name and its control at the row's end; the tray's choice is a switch,
    and the options read in words (Inbox, Mark as blocked) where they
    showed the stored values.
  - **Rows' actions.** A group's main action is at its heading (Add
    account, New rule). A row's are icon buttons at its end, named for
    what they act on (Edit, Delete, Duplicate, Move up and down), in
    place of a card of 5 text buttons, or of a dialog that held 2.
  - **Panels.** An account's panel and a mailbox's are a list of rows to
    press (`ui.FormSection` and `ui.FormRow`), the destroying one in the
    danger color, in place of a stack of buttons. So are the lists that
    choose an account and a parent mailbox.
  - **Built again when what it lists changes,** not with every change of
    the store, so a sync leaves an open menu and the scroll in place.
  - **Left.** Mail's navigation still stands beside Settings' own list
    of categories (4d). The Server pane's content is as it was. The forms
    the rows open are 4a's form dialog.
  - **Checks.** `make nsgo-test` passes. `make nsgo-live` on Xvfb's X11:
    33 passed, 0 skipped. 31 kept pictures: 15 changed and 5 are new (3
    categories, an account's panel and a mailbox's). 1 new workflow: the
    categories, a search for a preference and for an account, and an
    account's panel, whose Remove asks in an alert that a cancel leaves.
    The settings' 3 older workflows pass with 2 handles changed: General
    is a row of the view's list, not a tab, and a saved snippet is a
    row's text. Not looked at: the view in a window under 800.

## Phase 5 — Frostnotes and PocketGo (M each, beside Phases 3 and 4)

- First in Frostnotes, at the owner's word (2026-11-24): NSGo's tree in
  place of the second sidebar that lists its notes and folders.
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
  smoke tests. Not tried yet. What FrostMail's part found: its tests are
  Linux-only and each starts a maild with `os/exec`, which a browser
  cannot, so the suite as written cannot run with `-target web`; a web run
  needs the daemon started outside the test.
- **Pictures as a gate:** whether a picture that differs fails a run, or
  the pictures are evidence a person reads and only the suite's checks
  fail. Fonts and drawing differ between hosts. By Phase 0.5. What
  frostyard/pocketgo#167 recorded, from one host on 2026-11-24: a headless
  run captures no picture; 4 of the 13 committed web pictures differed
  from what `main` drew there, and the committed Linux pictures were 39
  px taller than that host's Xvfb captures; 1 picture of 13 differed
  between runs of the same tree (an animated icon); and 1 Linux-target run
  captured a blank picture and failed a test that passed on 2 reruns.
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
