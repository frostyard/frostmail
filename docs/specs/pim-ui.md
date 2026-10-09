# Spec: people, calendar and tasks in the main window (M4.5)

The look and behavior of the main window's modules (ADR-0020): the module
bar, the People module, the contact card that mail shows for an address,
and, as their phases start, the Calendar and Tasks modules, the To-Do bar,
the invitation card and the reminder window. It binds the presentational
components in `app/src/features/{people,calendar,tasks}/` and
`features/sidebar/ModuleBar.tsx`, the containers in `app/src/app/` that
wire them to maild, and the app tests. Tokens, type, measurements,
selection colors and the conventions for buttons are those of the reader
UI ([ui.md](ui.md)); Mail's own panes are unchanged.

## Modules

- The window has four modules, in this order: **Mail**, **Calendar**,
  **People**, **Tasks**. One is shown at a time; the window opens on Mail.
  The current module is window state (`useUI().module`), not persisted.
- Each module keeps its own sidebar, list and detail panes in the main
  window's three columns, with the same widths and splitters as Mail, and
  its own selection; switching back to a module finds it as it was left.
- **Keys:** Ctrl+1 shows Mail and selects All Inboxes (as before); Ctrl+2
  Calendar, Ctrl+3 People, Ctrl+4 Tasks. They work anywhere, text fields
  included, like Ctrl+1 today. A module that is not built yet ignores its
  key.

### Module bar

```
├──────────────┤
│ ✉  ▦  👥  ☑  │  44
└──────────────┘
```

- At the bottom of every module's sidebar: 44 high, a 1px `--separator`
  top border, `--bg-sidebar`. A `toolbar` named "Modules" with one
  `button` per built module, left to right in module order, starting at
  12px with 4px gaps.
- A button is 36 × 28, rounded 6, an 18px Lucide icon: Mail `Mail`,
  Calendar `Calendar`, People `Users`, Tasks `ListChecks`. Its accessible
  name is the module's name; its tooltip is the name and its key ("People
  (Ctrl+3)"). The current module's button has `aria-pressed="true"` and
  its icon in `--accent`; the others are `--text-secondary` with the
  toolbar buttons' hover.
- When the sidebar is hidden (Ctrl+Alt+S) the bar hides with it; the keys
  still switch.

## People module

```
┌──────────────┬───────────────────────┬──────────────────────────────────┐
│ ▢            │ All Contacts     ✉    │                  ⌕ Search Contacts – □ ✕ │
├──────────────┼───────────────────────┼──────────────────────────────────┤
│ People       │ A                     │ (AL) Ada Lovelace                │
│  All Contacts│ (AL) Ada Lovelace     │      Countess · Engines          │
│ Gmail        │      Engines          │      [✉ Message]                 │
│  Contacts    │ (AT) Alan Turing      │ CONTACTS — GMAIL                 │
│ iCloud       │      alan@…           │      home  ada@example.com       │
│  Card 🔒     │ G                     │    mobile  +44 20 7946 0000      │
│              │ (GH) Grace Hopper     │ RECENT MAIL                      │
│ ✉ ▦ 👥 ☑     │                       │  Engines                  Oct 1  │
└──────────────┴───────────────────────┴──────────────────────────────────┘
```

### People sidebar (`PeopleSidebar`)

- Background, padding, sections, rows, icons and selection exactly as
  Mail's sidebar (ui.md, Sidebar), as a `tree` named "Address Books".
- **Sections:** "People" holds one row, **All Contacts** (icon `Users`,
  key `all`). Then one section per account that has address books, titled
  with the account name, holding a row per address book (icon `BookUser`,
  key `book:<id>`), in collection order. A read-only book shows a 12px
  `Lock` icon after its name (`aria-label="Read-only"`). Sections collapse
  as Mail's do.
- Selecting a row selects the list's source: every person, or the people
  with a contact in that book (`people.list` with `collectionId`).
- The module bar follows the tree.

### People list (`PeopleList`)

- A `listbox` named "Contacts", virtualized, in `--bg-window`.
- **Index headers:** before the first person of each letter, a 24px
  header (`role="presentation"`), sticky at the top while its letter
  scrolls, `--bg-sidebar`, 12px left padding, 11/14 600
  `--text-secondary`, showing the person's `index` (A–Z, or # for
  anything else).
- **Rows** (`role="option"`, `aria-selected`) are 44 high: 12px left
  padding, a 28px avatar circle, 10px gap, then two lines: the name 13/18
  600 truncated (the email when the name is empty); below it the
  organization, else the email, 12/16 `--text-secondary` truncated, left
  out when it would repeat the first line. A 1px `--separator` at the bottom inset 50
  from the left.
- **Avatar:** initials (as the reader's, from the name, else the email)
  13/600 white on `--avatar-N` by `avatarTone(email)`; the list shows no
  photos.
- **Selection, focus and keys** as the message list: click selects; ↑/↓,
  Home/End move it; the selection is `--accent` with `--accent-contrast`
  text while the list has focus, `--selection-inactive` otherwise. There is
  no multiple selection.
- **Empty:** "No Contacts" centered, 15/400 `--text-secondary`; while
  searching, "No Results".

### Person pane (`PersonPane`)

- Background `--bg-window`, 24px padding, scrolling. **Empty:** "No
  Contact Selected" centered, 15/400 `--text-secondary`.
- **Header:** a 64px avatar (the person's photo, `object-fit: cover`, when
  there is one; else initials as in the list at 20/600); 16px gap; the name
  20/26 600 (an `h2`); below it the first contact's title and the person's
  organization joined by " · " (13/18 `--text-secondary`, omitted when
  both are empty). Under them a **Message** button (`Mail` icon and label,
  28 high, `--accent` fill, `--accent-contrast` text, rounded 6) that
  starts a new message to the person's first email; disabled when the
  person has none.
- **Contacts:** one block per contact, in the person's order, 20px apart:
  a `section` labeled by its heading, an `h3` with the address book's name
  and the account's name joined by " — ", plus " · Read-only" for a
  read-only contact, shown uppercase (CSS `uppercase`), 11/14 600
  `--text-secondary`. Then a two-column grid (`dl`): labels right-aligned
  in a 96px column, 12/18 `--text-secondary`; values 13/18
  `--text-primary`, 12px column gap, 6px row gap. Rows, each omitted when
  empty, in this order:
  - each email: label as given, else "email"; the value is a link-styled
    `button` (`--accent`) that starts a new message to it;
  - each phone: label as given, else "phone"; plain selectable text;
  - each address: label as given, else "address"; street, then
    "locality region postcode" (single spaces, empty parts skipped), then
    country, each non-empty line its own `div`;
  - each URL: label as given, else "homepage"; an `http` or `https` URL
    is a link that opens in the system browser (the click is the app's,
    never the webview's); any other URL is plain text;
  - birthday: label "birthday"; `YYYY-MM-DD` as the locale's long date
    ("December 10, 1815"), `--MM-DD` as month and day ("December 9");
  - nickname: label "nickname";
  - note: label "note"; `white-space: pre-wrap`.
- **Recent Mail:** when the contact card of the person's first email has
  recent messages, a `section` labeled "Recent Mail" with that heading (as
  the blocks' headings) and a row per message, 32 high: the subject 13/18 truncated ("(No Subject)" in
  `--text-tertiary` when empty), and at the right the list's date format
  (ui.md, Message list: Dates) 12/16 `--text-secondary`. A row is a
  `button`; clicking it shows the message in Mail (Mail module, the
  message's mailbox, the message selected).
- **Upcoming:** with the Calendar module, the card's upcoming events the
  same way ("UPCOMING"); hidden while empty.

### People toolbar

The main toolbar keeps its three segments and its right end (search field,
Settings, window controls). In People:

- over the sidebar: the sidebar toggle only;
- over the list: the title ("All Contacts", or the book's name) and the
  subtitle ("1 contact", "N contacts", or "N results" while searching);
- over the person pane: **New Message** (`SquarePen`, tooltip "New Message
  (Ctrl+N)") addressed to the selected person's first email, or blank
  without a selection; none of Mail's message actions.
- The search field's placeholder is "Search Contacts"; it filters the list
  (`people.list` with `query`) 250 ms after typing stops; Escape clears it.
  Mail's scope bar does not appear.

### Behavior

- **Data:** the list comes from `people.list` (with the book and the
  query); the pane from `people.get` for the selected person, `people.card`
  of their first email for Recent Mail, and `people.photo` when
  `hasPhoto`. A `people.changed` event refetches the list and the selected
  person within 100 ms; `account.changed` refetches the address books.
- **Selection** follows the person's ID across refetches; when the person
  is gone, nothing is selected.
- **Focus:** Tab and Shift+Tab move between sidebar, list and pane as in
  Mail. Ctrl+F (and Ctrl+Alt+F) focus the search field. Ctrl+N starts a
  new message (to the selected person's first email when the list has
  focus).

## Contact card

A popover that mail shows for an address, the way Outlook's contact card
does.

- **Where:** the reader's sender name and each name in its To and Cc lines
  is a button (named by the address); clicking it opens the card anchored
  below it. Escape, a click outside, or opening another closes it.
- **Box:** 320 wide, `--bg-window`, 1px `--separator` border, rounded 10,
  a soft shadow, 16px padding, `role="dialog"` named by the card's name or
  address.
- **Header:** a 48px avatar (the person's photo, else initials on the
  address's tone), the name 15/20 600 (the card's name, else the address),
  the address 12/16 `--text-secondary`, and the person's organization when
  there is one.
- **Actions** (a row of buttons, 28 high): **Message** (new message to the
  address); **Add to Contacts** when `canAdd` (calls `people.add` and shows
  "Added" in place of the button when it succeeds, or the error under the
  row); **Open in People** when there is a person (switches to People with
  them selected).
- **Recent Mail** and **Upcoming** as in the person pane, at most 5 rows
  each; a recent message opens in Mail and closes the card.
- The card asks `people.card` each time it opens; while the answer is
  pending it shows the name and address it was opened with.

## Calendar module (Phase 3)

Day, week and month views over `calendar.range`, a small month and the
calendars of each account in its sidebar, the event pane, and the To-Do
bar's agenda. Specified with the Phase 3 cards.

## Tasks module and the To-Do bar (Phase 4)

Lists with subtasks, due dates and completion, flagged mail as a list of
its own (`view.open` with `flagged`), and the To-Do bar beside Mail's
reader: a small month, the next events and the tasks due soon. Specified
with the Phase 3 and 4 cards.

## Invitation card and reminder window (Phases 3 and 4)

The reader's invitation card (`calendar.invitation`, answering with
`calendar.respond`) and the small reminder window (`calendar.reminders`,
Snooze and Dismiss). Specified with their cards.

## Rules

- Components in `features/` are presentational: data and callbacks by
  props, no imports from `app/src/data/` or `app/src/rpc/` except types.
- Tokens only; no literal colors. Photos are data: URLs from
  `people.photo`; nothing is fetched from the network.
- Every control is reachable by keyboard and has an accessible name.

## References

- Rationale: [ADR-0020](../adr/0020-one-window-for-mail-calendar-people-and-tasks.md),
  [ADR-0017](../adr/0017-sync-contacts-calendars-and-tasks.md)
- Context: [design/pim.md](../design/pim.md), [ui.md](ui.md)
- Plan: [plans/0007-m4.5-people-calendar-tasks.md](../plans/0007-m4.5-people-calendar-tasks.md)
