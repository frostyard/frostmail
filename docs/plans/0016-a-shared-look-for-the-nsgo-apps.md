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
  code. This plan takes GoVault's parts and Onegit's shell: ideas and
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

### The kit

| Part | Holds |
| --- | --- |
| `theme` | The two palettes, the named roles, the type and space tokens, `Apply`, Inter, the contrast test from frostui's table |
| `shell` | Today's `Shell`, `Inspector`, `FileExplorer`, `Conversation`; plus toolbar segments, the module bar, the connection item, the title bar's policy for each system, kept window state |
| `nav` | The sidebar row (icon, name, count, badge), today's `BranchLabel` and `ChildLabel`, the section caption |
| `rows` | The summary row, the avatar and its monogram, the unread dot, the count pill, the hover and selected fills, the skeleton row, a section header |
| `controls` | The search field with its key hint, the toolbar button and separator, the keycap, the pill, the banner, the divider |
| `surfaces` | `Page`, `Card`, `Section`, the field row, the option row, the empty state |
| `dialogs` | The form dialog, the confirmation, `Notice` |
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

## Phase 1 — Theme, type and the title row (M)

- `theme`: both palettes from frostui's spec, the named roles, Inter
  (bundled for the native builds, served with the app for the web one),
  the type and space tokens, the contrast test.
- `Apply` in the three apps, with the appearance setting.
- `Shell` stops drawing the brand where the system's title bar shows it.
- **Done when:** a picture of each app's main window in each theme, on
  fixture data, shows Frostyard's colors and Inter with one title row; the
  contrast test passes; PocketGo's comparison pictures are made again and
  its pane-width tests pass.

## Phase 2 — The kit's parts and the gallery (L)

- The parts of the table above, each with a layout test (a row keeps its
  text inside it, a count sits at the row's end, a tooltip shows on hover)
  in the manner of GoVault's and PocketGo's `nsgotest` checks.
- The gallery, with screenshot comparisons on Linux and the web.
- **Done when:** the gallery shows every part in both themes on Linux and
  in a browser, and the owner accepts its pictures. This is the look's
  sign-off: Phases 3 to 5 only place parts.

## Phase 3 — FrostMail's main window (L)

Three changes, each with `make nsgo-test` green:

- **3a, shell.** The toolbar's segments and icon buttons, the module bar,
  the sidebar's rows with role icons and counts, the connection item and
  the gear. Settings and Activity leave the sidebar's list.
- **3b, list.** The summary row, the search field, the filter segment and
  sort menu, selection by Ctrl and Shift (`base.List` with a multiple
  `Selection`), hover, skeletons, the empty states, the bulk actions in the
  toolbar while several rows are chosen.
- **3c, reader.** The header block, the banners, the page's frame, the
  conversation's stacked messages, the empty state.
- **Done when:** the NSGo client on `tools/uifixture`'s showcase, at the
  README picture's size, is set beside [main.png](../images/main.png) in
  both themes and the owner accepts it; `make nsgo-test` and
  `dev/lodge/nsgo-smoke.py` pass.

## Phase 4 — FrostMail's other surfaces (L)

- Compose (its toolbar, the address fields, the formatting bar), Settings
  on `ui.Settings`, Calendar, People, Tasks, the To-Do Bar, Activity, the
  palette, and every dialog on the form dialog and the confirmation.
- The narrow layout and Lodge's web build, checked in a browser.
- The style test of rule 2 turned on for `cmd/frostmail-nsgo`.
- **Done when:** the README's pictures are taken again from the NSGo
  client, and the style test passes with no exception listed.

## Phase 5 — Frostnotes and PocketGo (M each, beside Phases 3 and 4)

- Each app replaces its own helpers with the kit's parts (PocketGo:
  `inboxHeader`, `agentActivity`, `signIn`, `confirm`, the pager;
  Frostnotes: the editor's toolbar, `notice`, `vaultForm`, the five
  dialogs), and takes rules 3 to 8.
- Frostnotes' second toolbar row moves to the shell's toolbar.
- **Done when:** each app's style test passes, its own gates pass, and a
  picture of the three main windows side by side shows one toolbar, one
  sidebar row, one selected fill and one dialog.

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
